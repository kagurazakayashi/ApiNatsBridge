package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kagurazakayashi/libNyaruko_Go/nyaapiserver"
	"github.com/kagurazakayashi/libNyaruko_Go/nyanats"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

// BridgeRequest 是透過 NATS 轉發給後端微服務的 HTTP 橋接請求結構。
//
// 此結構會把原始 HTTP 請求中的方法、路徑、標頭、Cookie、來源位址、
// 請求參數與本文內容整理成 JSON，再由 NATS Request 模式送往指定 Subject。
// 後端微服務可依此結構還原請求上下文，並進一步完成業務邏輯處理。
type BridgeRequest struct {
	// Method 是原始 HTTP 請求方法，例如 GET、POST、PUT、DELETE。
	Method string `json:"method"`

	// Path 是原始 HTTP 請求路徑。
	//
	// 此欄位通常會與設定檔中的路由 path 對應，用於識別請求來源路由。
	Path string `json:"path"`

	// Headers 是原始 HTTP 請求標頭集合。
	//
	// 傳遞至微服務時會以 map 形式保存，方便後端讀取授權、內容型別、
	// 代理來源或其他自訂標頭資訊。
	Headers map[string]string `json:"headers"`

	// Cookies 是原始 HTTP Cookie 鍵值對集合。
	//
	// 若橋接層啟用了自動 UUID Cookie，此集合也可能包含系統自動補入的識別值。
	Cookies map[string]string `json:"cookies"`

	// RemoteAddr 是直接連線的用戶端位址，通常取自底層 socket。
	//
	// 在存在反向代理或 CDN 的情境下，此值可能是代理節點位址，而非終端用戶端 IP。
	RemoteAddr string `json:"remote_addr"`

	// IP 是系統推定的實際用戶端 IP。
	//
	// 解析優先序通常為：
	// CDN 指定標頭 > X-Real-IP > X-Forwarded-For 第一個合法 IP > RemoteAddr。
	IP string `json:"ip"`

	// Params 是請求參數集合，包含 URL 查詢參數與 POST 表單資料。
	//
	// 若同一參數有多個值，會依上游 HTTPRequest 的解析結果保存。
	Params map[string]string `json:"params"`

	// Body 是 HTTP 請求本文內容。
	//
	// 對 JSON 請求通常保留原始 JSON 字串；對表單請求則可能已被轉換為 JSON 字串。
	Body string `json:"body"`
}

// BridgeResponse 是後端微服務透過 NATS 回傳給橋接層的 HTTP 回應結構。
//
// 若微服務回傳內容可被解析為此結構，橋接層會依照 StatusCode、Headers 與 Body
// 組裝成 HTTP 回應；若無法解析，則會把原始回應內容直接作為一般本文返回。
type BridgeResponse struct {
	// StatusCode 是 HTTP 回應狀態碼；若為 0，系統會預設為 200。
	StatusCode int `json:"status_code"`

	// Headers 是 HTTP 回應標頭集合。
	//
	// 此欄位會直接套用至最終 HTTP 回應；若為 nil，橋接層會補成空 map。
	Headers map[string]string `json:"headers"`

	// Body 是 HTTP 回應本文內容。
	//
	// 橋接層會將此字串轉換為 []byte 後寫入 HTTPResponse。
	Body string `json:"body"`
}

// buildSchemaFromMap 會從 schema_body 對應表中建立可供 jsonschema 編譯的 JSON Schema。
//
// 此函式會先複製原始 map，避免直接修改設定來源，並處理以下控制鍵：
//   - root_type：轉換為 JSON Schema 的 type 欄位。
//   - strict：若為 true，則設定 additionalProperties=false。
//
// 其餘欄位會保留為標準 JSON Schema 欄位。
// 若輸入為 nil，會直接回傳 nil，代表該路由不啟用此 Schema。
func buildSchemaFromMap(schemaBody map[string]interface{}) map[string]interface{} {
	// 沒有設定 schema_body 時，直接回傳 nil，表示不啟用 Schema 驗證。
	if schemaBody == nil {
		return nil
	}

	// 複製輸入 map，避免後續轉換 root_type 或 strict 時改到原始設定資料。
	m := make(map[string]interface{}, len(schemaBody))
	for k, v := range schemaBody {
		m[k] = v
	}

	// 將自訂控制鍵 root_type 轉換為標準 JSON Schema 的 type 欄位。
	if rootType, ok := m["root_type"]; ok {
		delete(m, "root_type")
		if s, ok := rootType.(string); ok && s != "" {
			m["type"] = s
		}
	}

	// 將自訂控制鍵 strict 轉換為 additionalProperties=false。
	if strict, ok := m["strict"]; ok {
		delete(m, "strict")
		if b, ok := strict.(bool); ok && b {
			m["additionalProperties"] = false
		}
	}

	// 回傳已轉換為 JSON Schema 相容格式的 map。
	return m
}

// buildSchema 會根據 RouteConfig 的 SchemaBody 建立 JSON Schema 對應表。
//
// 此函式主要作為 RouteConfig 與 buildSchemaFromMap 之間的薄封裝，
// 讓路由載入流程可以用一致方式處理請求本文驗證規則。
func buildSchema(r *RouteConfig) map[string]interface{} {
	// 將路由設定中的 SchemaBody 統一交由 buildSchemaFromMap 轉換。
	return buildSchemaFromMap(r.SchemaBody)
}

// coerceFormValues 會依照 JSON Schema 的 properties 定義轉換表單欄位型別。
//
// application/x-www-form-urlencoded 表單資料預設皆為字串，
// 因此在進行 JSON Schema 驗證前，需先根據 schema 中的 type 定義，
// 將可轉換的欄位轉為 integer、number 或 boolean。
// 無法轉換或未宣告型別的欄位會保持原值。
func coerceFormValues(formMap map[string]interface{}, schemaMap map[string]interface{}) {
	// 只有 schema 中存在 properties 時，才有欄位型別定義可供轉換。
	props, ok := schemaMap["properties"]
	if !ok {
		return
	}

	// properties 必須是 map[string]interface{}，否則無法逐欄位查詢型別。
	propMap, ok := props.(map[string]interface{})
	if !ok {
		return
	}

	// 逐一檢查表單欄位，並嘗試依 schema 宣告型別進行轉換。
	for key, val := range formMap {
		propDef, hasProp := propMap[key]
		if !hasProp {
			continue
		}

		// 取得單一欄位的 schema 定義。
		propDefMap, ok := propDef.(map[string]interface{})
		if !ok {
			continue
		}

		// 沒有 type 定義時，保留原字串值。
		propType, hasType := propDefMap["type"]
		if !hasType {
			continue
		}

		// 目前僅處理字串型態的 type 宣告。
		typeStr, ok := propType.(string)
		if !ok {
			continue
		}

		// 表單值若不是字串，代表可能已經由其他流程轉換，這裡不再處理。
		strVal, ok := val.(string)
		if !ok {
			continue
		}

		// 依 schema type 嘗試轉為對應 Go 型別。
		switch typeStr {
		case "integer":
			// 將可解析的整數字串轉為 int64。
			if n, err := strconv.ParseInt(strVal, 10, 64); err == nil {
				formMap[key] = n
			}
		case "number":
			// 將可解析的數值字串轉為 float64。
			if n, err := strconv.ParseFloat(strVal, 64); err == nil {
				formMap[key] = n
			}
		case "boolean":
			// 支援常見布林字串與數值表示法。
			switch strings.ToLower(strVal) {
			case "true", "1":
				formMap[key] = true
			case "false", "0", "":
				formMap[key] = false
			}
		}
	}
}

// BridgeHandler 是 HTTP 與 NATS 微服務之間的橋接處理器。
//
// 此處理器負責：
//   - 根據請求路徑找到對應的 NATS Subject。
//   - 驗證 HTTP 方法、Content-Type、請求長度限制與 JSON Schema。
//   - 解析實際用戶端 IP。
//   - 將 HTTP 請求轉換為 NATS Request。
//   - 驗證微服務回應並轉換為 HTTPResponse。
//   - 視設定自動產生並寫入 UUID Cookie。
//
// 此結構會在服務啟動時依照設定檔初始化，請求處理階段則盡量只讀取已建立的對應表。
type BridgeHandler struct {
	// natsClient 是用於發送 NATS Request 的 NATS 用戶端實例。
	natsClient *nyanats.NyaNATS

	// routes 是 HTTP 路徑到 NATS Subject 的對應表。
	routes map[string]string

	// routeTimeouts 是各路徑的 NATS Request 逾時設定。
	routeTimeouts map[string]time.Duration

	// routeMethods 是各路徑允許的 HTTP 方法集合。
	//
	// map 的 key 為大寫 HTTP 方法名稱；value 使用空 struct 以節省記憶體。
	routeMethods map[string]map[string]struct{}

	// routeContentTypes 是各路徑要求的 Content-Type。
	//
	// 若某路徑未設定 Content-Type，則不會對請求本文的 Content-Type 進行限制。
	routeContentTypes map[string]string

	// routeSchemas 是各路徑的請求 JSON Schema 編譯結果。
	//
	// 僅已成功編譯的 Schema 會被放入此對應表。
	routeSchemas map[string]*jsonschema.Schema

	// routeSchemaMaps 是各路徑的原始請求 Schema 對應表，主要供表單型別轉換使用。
	routeSchemaMaps map[string]map[string]interface{}

	// cdnHeaders 是 CDN 或反向代理服務商傳遞真實 IP 的標頭清單。
	cdnHeaders []string

	// limits 是全域請求欄位長度限制規則。
	//
	// 若路由層級有自訂限制，會在請求處理時與此規則合併。
	limits *LimitRule

	// routeLimits 是各路徑的請求欄位長度限制規則。
	routeLimits map[string]*LimitRule

	// routeReturnFields 是各路徑轉發至 NATS 時允許包含的欄位集合。
	//
	// 若未設定或集合為空，表示轉發完整 BridgeRequest。
	routeReturnFields map[string]map[string]struct{}

	// routeResponseSchemas 是各路徑的回應 JSON Schema 編譯結果。
	routeResponseSchemas map[string]*jsonschema.Schema

	// routeResponseSchemaMaps 是各路徑的原始回應 Schema 對應表。
	routeResponseSchemaMaps map[string]map[string]interface{}

	// responseLimits 是全域回應欄位長度限制規則。
	responseLimits *LimitRule

	// routeResponseLimits 是各路徑的回應欄位長度限制規則。
	routeResponseLimits map[string]*LimitRule

	// errorDetailIPs 是允許接收錯誤詳細資訊的 IP 白名單集合。
	//
	// map 的 key 為 IP 字串；value 使用空 struct 表示存在即可。
	errorDetailIPs map[string]struct{}

	// cookieUUIDKey 是自動為用戶端 Cookie 寫入 UUID 的鍵名。
	//
	// 若為空字串，表示不啟用自動 UUID Cookie。
	cookieUUIDKey string
}

// NewBridgeHandler 會根據路由設定建立並初始化新的 BridgeHandler。
//
// 初始化過程會建立路徑、NATS Subject、逾時、HTTP 方法、Content-Type、
// 請求與回應 Schema、欄位長度限制、回傳欄位白名單及錯誤詳細資訊白名單等對應表。
// 若 Schema 編譯失敗，該路由的對應 Schema 驗證會被略過，但路由本身仍可繼續載入。
func NewBridgeHandler(natsClient *nyanats.NyaNATS, routes []RouteConfig, cdnHeaders []string, limits *LimitRule, responseLimits *LimitRule, errorDetailIPs []string, cookieUUIDKey string) *BridgeHandler {
	// 初始化路由、逾時、方法、Content-Type 與各種驗證規則的索引表。
	routeMap := make(map[string]string)
	timeoutMap := make(map[string]time.Duration)
	methodMap := make(map[string]map[string]struct{})
	contentTypeMap := make(map[string]string)
	routeSchemas := make(map[string]*jsonschema.Schema)
	routeSchemaMaps := make(map[string]map[string]interface{})
	routeLimitsMap := make(map[string]*LimitRule)
	returnFieldsMap := make(map[string]map[string]struct{})
	routeResponseSchemas := make(map[string]*jsonschema.Schema)
	routeResponseSchemaMaps := make(map[string]map[string]interface{})
	routeResponseLimitsMap := make(map[string]*LimitRule)

	// 將錯誤詳細資訊白名單轉為 set，方便請求處理時 O(1) 查詢。
	errorIPSet := make(map[string]struct{}, len(errorDetailIPs))
	for _, ip := range errorDetailIPs {
		errorIPSet[ip] = struct{}{}
	}

	// 建立 JSON Schema 編譯器，後續請求與回應 Schema 都共用此編譯器。
	compiler := jsonschema.NewCompiler()

	// 逐一路由載入設定，並建立執行期需要的查詢表。
	for _, r := range routes {
		logBridge(lLog.LogLoadRoute(), r.Path, r.NatsSubject, r.TimeoutDuration(), r.AllowedMethods(), r.ContentType, r.ReturnFields)

		// 建立 HTTP path 到 NATS Subject 的對應。
		routeMap[r.Path] = r.NatsSubject

		// 保存此路由的 NATS Request 逾時時間。
		timeoutMap[r.Path] = r.TimeoutDuration()

		// 將允許的 HTTP 方法統一轉為大寫後建立 set。
		allowed := make(map[string]struct{})
		for _, m := range r.Methods {
			allowed[strings.ToUpper(m)] = struct{}{}
		}
		methodMap[r.Path] = allowed

		// 若設定了 Content-Type，後續會用於請求本文類型檢查。
		if r.ContentType != "" {
			contentTypeMap[r.Path] = r.ContentType
		}

		// 保存路由層級的請求長度限制，稍後會與全域限制合併。
		if r.Limits != nil {
			routeLimitsMap[r.Path] = r.Limits
			logBridge(lLog.LogRouteCustomLimits(), r.Path)
		}

		// 將 return_fields 建立為 set，控制轉發給微服務的欄位範圍。
		if len(r.ReturnFields) > 0 {
			fieldSet := make(map[string]struct{}, len(r.ReturnFields))
			for _, f := range r.ReturnFields {
				fieldSet[f] = struct{}{}
			}
			returnFieldsMap[r.Path] = fieldSet
		}

		// 建立並編譯請求本文 JSON Schema。
		schemaMap := buildSchema(&r)
		if schemaMap != nil {
			routeSchemaMaps[r.Path] = schemaMap
			url := "config://routes" + r.Path
			if err := compiler.AddResource(url, schemaMap); err != nil {
				logBridge(lLog.LogSchemaAddFailed(), r.Path, err)
				continue
			}
			schema, err := compiler.Compile(url)
			if err != nil {
				logBridge(lLog.LogSchemaInvalid(), r.Path, err)
				continue
			}
			routeSchemas[r.Path] = schema
			logBridge(lLog.LogSchemaLoaded(), r.Path)
		}

		// 保存路由層級的回應長度限制。
		if r.ResponseLimits != nil {
			routeResponseLimitsMap[r.Path] = r.ResponseLimits
			logBridge(lLog.LogRouteResponseLimits(), r.Path)
		}

		// 建立並編譯微服務回應本文 JSON Schema。
		responseSchemaMap := buildSchemaFromMap(r.ResponseSchemaBody)
		if responseSchemaMap != nil {
			routeResponseSchemaMaps[r.Path] = responseSchemaMap
			url := "config://routes" + r.Path + "/response"
			if err := compiler.AddResource(url, responseSchemaMap); err != nil {
				logBridge(lLog.LogResponseSchemaAddFailed(), r.Path, err)
				continue
			}
			schema, err := compiler.Compile(url)
			if err != nil {
				logBridge(lLog.LogResponseSchemaInvalid(), r.Path, err)
				continue
			}
			routeResponseSchemas[r.Path] = schema
			logBridge(lLog.LogResponseSchemaLoaded(), r.Path)
		}
	}

	// 輸出初始化摘要，方便啟動時確認設定載入狀態。
	logBridge(lLog.LogLoadedSummary(), len(routeMap), len(cdnHeaders))
	if cookieUUIDKey != "" {
		logBridge(lLog.LogUuidCookieEnabled(), cookieUUIDKey)
	}

	// 回傳已完成初始化的 BridgeHandler。
	return &BridgeHandler{
		natsClient:              natsClient,
		routes:                  routeMap,
		routeTimeouts:           timeoutMap,
		routeMethods:            methodMap,
		routeContentTypes:       contentTypeMap,
		routeSchemas:            routeSchemas,
		routeSchemaMaps:         routeSchemaMaps,
		cdnHeaders:              cdnHeaders,
		limits:                  limits,
		routeLimits:             routeLimitsMap,
		routeReturnFields:       returnFieldsMap,
		routeResponseSchemas:    routeResponseSchemas,
		routeResponseSchemaMaps: routeResponseSchemaMaps,
		responseLimits:          responseLimits,
		routeResponseLimits:     routeResponseLimitsMap,
		errorDetailIPs:          errorIPSet,
		cookieUUIDKey:           cookieUUIDKey,
	}
}

// Handle 是 HTTP 請求的統一入口。
//
// 此方法會先依照 cookieUUIDKey 設定檢查是否需要為用戶端產生 UUID Cookie，
// 再呼叫 handleRequest 執行實際請求處理。若產生了新的 UUID，會在回應中加入 Set-Cookie。
// 此方法適合作為外部 HTTP 伺服器呼叫 BridgeHandler 的主要進入點。
func (h *BridgeHandler) Handle(req *nyaapiserver.HTTPRequest) *nyaapiserver.HTTPResponse {
	// newUUID 用於記錄本次請求是否新產生了 Cookie 識別值。
	var newUUID string

	// 若設定了 cookieUUIDKey，代表啟用自動 UUID Cookie 機制。
	if h.cookieUUIDKey != "" {
		// 確保 Cookie map 已初始化，避免後續寫入時 panic。
		if req.Cookies == nil {
			req.Cookies = make(map[string]string)
		}

		// 若用戶端尚未帶入指定 Cookie，則產生新的 UUID 並放入請求上下文。
		if _, exists := req.Cookies[h.cookieUUIDKey]; !exists {
			newUUID = strings.ToUpper(strings.ReplaceAll(uuid.New().String(), "-", ""))
			req.Cookies[h.cookieUUIDKey] = newUUID
			if verbose {
				logBridge(lLog.LogUuidCookieNewVerbose(), h.cookieUUIDKey, newUUID)
			} else {
				logBridge(lLog.LogUuidCookieNew(), h.cookieUUIDKey)
			}
		}
	}

	// 交由內部請求處理流程執行路由、驗證與 NATS 轉發。
	resp := h.handleRequest(req)

	// 若本次有產生新 UUID，將其透過 Set-Cookie 寫回用戶端。
	if newUUID != "" && resp != nil {
		if resp.Headers == nil {
			resp.Headers = make(map[string]string)
		}
		resp.Headers["Set-Cookie"] = h.cookieUUIDKey + "=" + newUUID + "; Path=/; HttpOnly"
	}

	// 回傳最終 HTTPResponse。
	return resp
}

// handleRequest 是 HTTP 請求的實際處理邏輯。
//
// 處理流程包含來源 IP 解析、路由匹配、HTTP 方法檢查、Content-Type 檢查、
// 請求限制檢查、表單或 JSON Schema 驗證，以及轉發至 NATS。
// 若請求不符合任何設定路由，則會檢查內建路由，例如 /ping。
func (h *BridgeHandler) handleRequest(req *nyaapiserver.HTTPRequest) *nyaapiserver.HTTPResponse {
	// 記錄請求基本資訊，作為橋接層操作追蹤入口。
	logBridge(lLog.LogHttpRequest(), req.Method, req.Path, req.RemoteAddr)

	// 根據 verbose 模式決定輸出完整參數或僅輸出項目數。
	if len(req.Params) > 0 {
		if verbose {
			logBridge(lLog.LogHttpParamsVerbose(), req.Params)
		} else {
			logBridge(lLog.LogHttpParams(), len(req.Params))
		}
	}

	// 根據 verbose 模式決定輸出完整 Cookie 或僅輸出項目數。
	if len(req.Cookies) > 0 {
		if verbose {
			logBridge(lLog.LogHttpCookiesVerbose(), req.Cookies)
		} else {
			logBridge(lLog.LogHttpCookies(), len(req.Cookies))
		}
	}

	// 解析實際用戶端 IP，後續會用於轉發、錯誤詳細資訊判斷與日誌。
	clientIP := h.resolveClientIP(req.Headers, req.RemoteAddr)
	if clientIP == "" {
		logBridge(lLog.LogResolveIpFailed())
		return &nyaapiserver.HTTPResponse{StatusCode: 400, Body: []byte(lHTTP.HttpBadRequestResolveIp())}
	}

	// 優先檢查是否命中設定檔中的路由。
	if natsSubject, ok := h.routes[req.Path]; ok {
		// 若路由設定了允許方法，則檢查目前 HTTP Method 是否被允許。
		if allowed, hasMethods := h.routeMethods[req.Path]; hasMethods && len(allowed) > 0 {
			if _, ok := allowed[req.Method]; !ok {
				return &nyaapiserver.HTTPResponse{StatusCode: 405, Body: []byte(lHTTP.HttpMethodNotAllowed())}
			}
		}

		// 若路由設定了 Content-Type，且請求有 body，則檢查 Content-Type 是否符合。
		if ct, hasCT := h.routeContentTypes[req.Path]; hasCT {
			if len(req.Body) > 0 {
				reqContentType := req.Headers["Content-Type"]
				if reqContentType == "" {
					reqContentType = req.Headers["content-type"]
				}
				if !strings.HasPrefix(reqContentType, ct) {
					return &nyaapiserver.HTTPResponse{StatusCode: 415, Body: []byte(lHTTP.HttpUnsupportedMediaType())}
				}
			}
		}

		// 合併全域請求限制與路由層級請求限制。
		effectiveLimits := h.limits
		if rl, has := h.routeLimits[req.Path]; has {
			effectiveLimits = mergeLimitRule(h.limits, rl)
		}

		// 執行請求欄位長度與數量限制檢查。
		if effectiveLimits != nil {
			if resp := h.validateLimits(req, effectiveLimits); resp != nil {
				return resp
			}
		}

		// 表單路由需要先將 form-urlencoded 內容轉成 map，再進行型別轉換與 Schema 驗證。
		if ct, hasCT := h.routeContentTypes[req.Path]; hasCT && strings.HasPrefix(ct, "application/x-www-form-urlencoded") && (len(req.Body) > 0 || len(req.Params) > 0) {
			var formMap map[string]interface{}

			// 優先解析 body 中的表單資料。
			if len(req.Body) > 0 {
				formValues, urlErr := url.ParseQuery(string(req.Body))
				if urlErr != nil {
					return h.errResp(400, lHTTP.HttpInvalidFormBody(), urlErr.Error(), clientIP)
				}
				formMap = make(map[string]interface{}, len(formValues))
				for k, v := range formValues {
					if len(v) == 1 {
						formMap[k] = v[0]
					} else {
						formMap[k] = v
					}
				}
			} else {
				// 若 body 為空，則使用已解析的 Params 作為表單資料來源。
				formMap = make(map[string]interface{}, len(req.Params))
				for k, v := range req.Params {
					formMap[k] = v
				}
			}

			// 若此路由有 schemaMap，先依 properties type 將表單字串轉為對應型別。
			if schemaMap, ok := h.routeSchemaMaps[req.Path]; ok {
				coerceFormValues(formMap, schemaMap)
			}

			// 若此路由有編譯後的 Schema，對表單 map 進行驗證。
			if schema, hasSchema := h.routeSchemas[req.Path]; hasSchema {
				if err := schema.Validate(formMap); err != nil {
					if verbose {
						logBridge(lLog.LogSchemaValidationFailedVerbose(), req.Path, err)
					} else {
						logBridge(lLog.LogSchemaValidationFailed(), req.Path)
					}
					return h.errResp(400, lHTTP.HttpSchemaValidationFailed(), err.Error(), clientIP)
				}
			}

			// 將表單 map 序列化為 JSON，讓下游微服務接收一致格式。
			jsonBody, jsonErr := json.Marshal(formMap)
			if jsonErr != nil {
			return nyaapiserver.JSONResponse(500, map[string]string{
				"error": lHTTP.HttpInternalErrorMarshalForm(),
			})
			}
			req.Body = jsonBody

			// 表單轉換完成後，進入 NATS 轉發流程。
			return h.forwardToNats(req, natsSubject, clientIP)
		}

		// 非表單請求若設定了 Schema，則將 body 視為 JSON 並進行驗證。
		if schema, hasSchema := h.routeSchemas[req.Path]; hasSchema && len(req.Body) > 0 {
		var bodyJSON interface{}
		if err := json.Unmarshal(req.Body, &bodyJSON); err != nil {
			return h.errResp(400, lHTTP.HttpInvalidJsonBody(), err.Error(), clientIP)
		}
		if err := schema.Validate(bodyJSON); err != nil {
			if verbose {
				logBridge(lLog.LogSchemaValidationFailedVerbose(), req.Path, err)
			} else {
				logBridge(lLog.LogSchemaValidationFailed(), req.Path)
			}
			return h.errResp(400, lHTTP.HttpSchemaValidationFailed(), err.Error(), clientIP)
		}
	}

		// 所有檢查通過後，將請求送往對應 NATS Subject。
		return h.forwardToNats(req, natsSubject, clientIP)
	}

	// 未命中設定路由時，檢查內建路由。
	switch req.Path {
	case "/ping":
		return apiping(req)
	default:
		return &nyaapiserver.HTTPResponse{StatusCode: 404, Body: []byte(lHTTP.HttpNotFound())}
	}
}

// mergeLimitRule 會合併全域與路由層級的限制規則。
//
// overlay 為路由層級規則，base 為全域規則。
// 當 overlay 中的數值大於 0 時，會覆蓋 base 對應欄位；
// 未設定的欄位則沿用 base。
// 此函式不會修改原始 base 或 overlay，而是回傳合併後的新規則指標。
func mergeLimitRule(base, overlay *LimitRule) *LimitRule {
	// 沒有路由層級限制時，直接使用全域限制。
	if overlay == nil {
		return base
	}

	// 沒有全域限制時，直接使用路由層級限制。
	if base == nil {
		return overlay
	}

	// 複製 base，避免修改原始全域規則。
	merged := *base

	// 以下欄位只在 overlay 有正數設定時覆蓋 base。
	if overlay.Path.MaxLength > 0 {
		merged.Path.MaxLength = overlay.Path.MaxLength
	}
	if overlay.Body.MaxLength > 0 {
		merged.Body.MaxLength = overlay.Body.MaxLength
	}
	if overlay.Headers.MaxCount > 0 {
		merged.Headers.MaxCount = overlay.Headers.MaxCount
	}
	if overlay.Headers.MaxKeyLen > 0 {
		merged.Headers.MaxKeyLen = overlay.Headers.MaxKeyLen
	}
	if overlay.Headers.MaxValueLen > 0 {
		merged.Headers.MaxValueLen = overlay.Headers.MaxValueLen
	}
	if overlay.Cookies.MaxCount > 0 {
		merged.Cookies.MaxCount = overlay.Cookies.MaxCount
	}
	if overlay.Cookies.MaxKeyLen > 0 {
		merged.Cookies.MaxKeyLen = overlay.Cookies.MaxKeyLen
	}
	if overlay.Cookies.MaxValueLen > 0 {
		merged.Cookies.MaxValueLen = overlay.Cookies.MaxValueLen
	}
	if overlay.Params.MaxCount > 0 {
		merged.Params.MaxCount = overlay.Params.MaxCount
	}
	if overlay.Params.MaxKeyLen > 0 {
		merged.Params.MaxKeyLen = overlay.Params.MaxKeyLen
	}
	if overlay.Params.MaxValueLen > 0 {
		merged.Params.MaxValueLen = overlay.Params.MaxValueLen
	}

	// 回傳合併後的新限制規則。
	return &merged
}

// validateLimits 會根據 LimitRule 檢查 HTTP 請求各欄位長度與數量。
//
// 檢查範圍包含 path、body、headers、cookies 與 params。
// 若任一欄位超出限制，會回傳 400 JSON 錯誤回應；全部通過則回傳 nil。
func (h *BridgeHandler) validateLimits(req *nyaapiserver.HTTPRequest, r *LimitRule) *nyaapiserver.HTTPResponse {
	// 檢查請求路徑長度是否超過限制。
	if r.Path.MaxLength > 0 && len(req.Path) > r.Path.MaxLength {
		return nyaapiserver.JSONResponse(400, map[string]interface{}{
			"error": lHTTP.HttpPathTooLong(),
			"field": "path",
			"limit": r.Path.MaxLength,
		})
	}

	// 檢查請求本文長度是否超過限制。
	if r.Body.MaxLength > 0 && len(req.Body) > r.Body.MaxLength {
		return nyaapiserver.JSONResponse(400, map[string]interface{}{
			"error": lHTTP.HttpBodyTooLong(),
			"field": "body",
			"limit": r.Body.MaxLength,
		})
	}

	// 檢查 headers 的數量、key 長度與 value 長度。
	if resp := h.validateMapLimit(req.Headers, r.Headers, "headers"); resp != nil {
		return resp
	}

	// 檢查 cookies 的數量、key 長度與 value 長度。
	if resp := h.validateMapLimit(req.Cookies, r.Cookies, "cookies"); resp != nil {
		return resp
	}

	// 檢查 params 的數量、key 長度與 value 長度。
	if resp := h.validateMapLimit(req.Params, r.Params, "params"); resp != nil {
		return resp
	}

	// 所有限制檢查皆通過。
	return nil
}

// validateMapLimit 會檢查 map 型欄位的項目數量、鍵長度與值長度。
//
// 此函式用於 headers、cookies 與 params 等 map[string]string 欄位。
// 若未設定任何限制，會直接回傳 nil；若違反限制，會回傳 400 JSON 錯誤回應。
// field 參數會被寫入錯誤回應中，用於指出是哪一類欄位觸發限制。
func (h *BridgeHandler) validateMapLimit(m map[string]string, rule MapLimitRule, field string) *nyaapiserver.HTTPResponse {
	// 若此類 map 欄位沒有任何限制，直接略過。
	if rule.MaxCount <= 0 && rule.MaxKeyLen <= 0 && rule.MaxValueLen <= 0 {
		return nil
	}

	// 檢查 map 項目數是否超過限制。
	if rule.MaxCount > 0 && len(m) > rule.MaxCount {
		return nyaapiserver.JSONResponse(400, map[string]interface{}{
			"error":  lHTTP.HttpTooManyEntries(),
			"field":  field,
			"limit":  rule.MaxCount,
			"actual": len(m),
		})
	}

	// 逐一檢查每個 key 與 value 的長度。
	for k, v := range m {
		if rule.MaxKeyLen > 0 && len(k) > rule.MaxKeyLen {
			return nyaapiserver.JSONResponse(400, map[string]interface{}{
				"error":  lHTTP.HttpKeyTooLong(),
				"field":  field,
				"key":    k,
				"limit":  rule.MaxKeyLen,
				"actual": len(k),
			})
		}
		if rule.MaxValueLen > 0 && len(v) > rule.MaxValueLen {
			return nyaapiserver.JSONResponse(400, map[string]interface{}{
				"error":  lHTTP.HttpValueTooLong(),
				"field":  field,
				"key":    k,
				"limit":  rule.MaxValueLen,
				"actual": len(v),
			})
		}
	}

	// map 欄位限制檢查通過。
	return nil
}

// isValidIP 會檢查字串是否為有效的 IPv4 或 IPv6 位址。
//
// 此函式會先移除字串前後空白，再使用 net.ParseIP 進行格式驗證。
// 回傳 true 表示輸入可被解析為合法 IP；否則回傳 false。
func isValidIP(s string) bool {
	// net.ParseIP 回傳非 nil 代表字串是合法 IPv4 或 IPv6。
	return net.ParseIP(strings.TrimSpace(s)) != nil
}

// resolveClientIP 會從請求標頭與 RemoteAddr 中解析實際用戶端 IP。
//
// 所有候選來源皆必須通過 IP 格式驗證。
// 解析優先序如下：
//   - 設定檔指定的 CDN 標頭。
//   - X-Real-IP。
//   - X-Forwarded-For 中第一個合法 IP。
//   - RemoteAddr。
//
// 若所有來源皆無法解析為合法 IP，則回傳空字串。
func (h *BridgeHandler) resolveClientIP(headers map[string]string, remoteAddr string) string {
	// 優先使用設定檔中指定的 CDN 或反向代理真實 IP 標頭。
	for _, header := range h.cdnHeaders {
		if ip, ok := headers[header]; ok && ip != "" && isValidIP(ip) {
			return ip
		}
		lower := strings.ToLower(header)
		if ip, ok := headers[lower]; ok && ip != "" && isValidIP(ip) {
			return ip
		}
	}

	// 其次檢查 X-Real-IP，分別處理常見大小寫形式。
	if ip := headers["X-Real-Ip"]; ip != "" && isValidIP(ip) {
		return ip
	}
	if ip := headers["x-real-ip"]; ip != "" && isValidIP(ip) {
		return ip
	}

	// 再檢查 X-Forwarded-For，取第一個合法 IP 作為用戶端 IP。
	if xff := headers["X-Forwarded-For"]; xff != "" {
		for _, ip := range strings.Split(xff, ",") {
			ip = strings.TrimSpace(ip)
			if ip != "" && isValidIP(ip) {
				return ip
			}
		}
	}
	if xff := headers["x-forwarded-for"]; xff != "" {
		for _, ip := range strings.Split(xff, ",") {
			ip = strings.TrimSpace(ip)
			if ip != "" && isValidIP(ip) {
				return ip
			}
		}
	}

	// 最後使用 RemoteAddr；若包含 port，先拆出 host。
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
		remoteAddr = host
	}

	// RemoteAddr 若為合法 IP，則作為最後 fallback。
	if isValidIP(remoteAddr) {
		return remoteAddr
	}

	// 所有來源皆無效時回傳空字串。
	return ""
}

// isErrorDetailIP 會檢查指定 IP 是否位於錯誤詳細資訊白名單中。
//
// 白名單內的 IP 在發生錯誤時可取得 detail 欄位；
// 非白名單 IP 僅會收到通用錯誤訊息。
// 空字串永遠不會被視為白名單 IP。
func (h *BridgeHandler) isErrorDetailIP(ip string) bool {
	// 空 IP 不具備識別能力，因此不允許取得詳細錯誤。
	if ip == "" {
		return false
	}

	// 透過 set 查詢此 IP 是否在白名單中。
	_, ok := h.errorDetailIPs[ip]
	return ok
}

// errResp 會建立標準化 JSON 錯誤回應。
//
// detail 會寫入橋接層日誌；若 clientIP 位於錯誤詳細資訊白名單，
// 回應內容也會包含 detail，方便受信任來源除錯。
// 非白名單來源只會收到通用錯誤訊息，避免暴露內部細節。
func (h *BridgeHandler) errResp(statusCode int, msg string, detail string, clientIP string) *nyaapiserver.HTTPResponse {
	// 若有詳細錯誤，依 verbose 模式決定是否完整輸出。
	if detail != "" {
		if verbose {
			logBridge("%s: %s", msg, detail)
		} else {
			logBridge("%s", msg)
		}
	}

	// 僅白名單 IP 可在回應中取得 detail，避免對外洩漏內部實作細節。
	if h.isErrorDetailIP(clientIP) && detail != "" {
		return nyaapiserver.JSONResponse(statusCode, map[string]interface{}{
			"error": msg, "detail": detail,
		})
	}

	// 一般來源只回傳通用錯誤訊息。
	return nyaapiserver.JSONResponse(statusCode, map[string]string{
		"error": msg,
	})
}

// validateResponse 會驗證從 NATS 微服務回傳的 BridgeResponse 是否符合設定規則。
//
// 驗證內容包含：
//   - 回應本文長度限制。
//   - 回應標頭數量、鍵長度與值長度限制。
//   - 回應本文 JSON Schema 驗證。
//
// 若回應不符合規則，會回傳 502 錯誤；驗證通過則回傳 nil。
// 此方法可避免後端微服務回傳過大、格式錯誤或不符合契約的資料直接暴露給用戶端。
func (h *BridgeHandler) validateResponse(resp BridgeResponse, path string, clientIP string) *nyaapiserver.HTTPResponse {
	// 合併全域回應限制與路由層級回應限制。
	effectiveLimits := h.responseLimits
	if rl, has := h.routeResponseLimits[path]; has {
		effectiveLimits = mergeLimitRule(h.responseLimits, rl)
	}

	// 若存在回應限制規則，檢查 body 與 headers。
	if effectiveLimits != nil {
		// 檢查回應本文長度。
		if effectiveLimits.Body.MaxLength > 0 && len(resp.Body) > effectiveLimits.Body.MaxLength {
			return h.errResp(502, lHTTP.HttpResponseBodyTooLong(),
				fmt.Sprintf(lHTTP.HttpDetailBodyLength(), len(resp.Body), effectiveLimits.Body.MaxLength), clientIP)
		}

		// 檢查回應標頭數量與長度限制。
		rule := effectiveLimits.Headers
		if rule.MaxCount > 0 && len(resp.Headers) > rule.MaxCount {
			return h.errResp(502, lHTTP.HttpResponseHeadersTooMany(),
				fmt.Sprintf(lHTTP.HttpDetailCount(), len(resp.Headers), rule.MaxCount), clientIP)
		}
		for k, v := range resp.Headers {
			if rule.MaxKeyLen > 0 && len(k) > rule.MaxKeyLen {
				return h.errResp(502, lHTTP.HttpResponseHeaderKeyTooLong(),
					fmt.Sprintf(lHTTP.HttpDetailKeyLength(), k, len(k), rule.MaxKeyLen), clientIP)
			}
			if rule.MaxValueLen > 0 && len(v) > rule.MaxValueLen {
				return h.errResp(502, lHTTP.HttpResponseHeaderValueTooLong(),
					fmt.Sprintf(lHTTP.HttpDetailValueLength(), k, len(v), rule.MaxValueLen), clientIP)
			}
		}
	}

	// 若路由設定了回應 Schema，則驗證 BridgeResponse.Body 必須是合法且符合契約的 JSON。
	if schema, hasSchema := h.routeResponseSchemas[path]; hasSchema && len(resp.Body) > 0 {
		var bodyJSON interface{}
		if err := json.Unmarshal([]byte(resp.Body), &bodyJSON); err != nil {
			return h.errResp(502, lHTTP.HttpResponseBodyNotJson(), err.Error(), clientIP)
		}
		if err := schema.Validate(bodyJSON); err != nil {
			if verbose {
				logBridge(lLog.LogResponseSchemaValidationFailedVerbose(), path, err)
			} else {
				logBridge(lLog.LogResponseSchemaValidationFailed(), path)
			}
			return h.errResp(502, lHTTP.HttpResponseSchemaFailed(), err.Error(), clientIP)
		}
	}

	// 回應驗證通過。
	return nil
}

// forwardToNats 會將 HTTP 請求序列化後，透過 NATS Request 轉發至對應微服務並等待回應。
//
// 若路由設定了 return_fields，則只會把指定欄位送往微服務；
// 若未設定或集合為空，則會送出完整 BridgeRequest。
// 回應若可解析為 BridgeResponse，會依其內容建立 HTTPResponse；
// 若無法解析，則會將原始 NATS 回應本文以 200 狀態碼直接返回。
func (h *BridgeHandler) forwardToNats(req *nyaapiserver.HTTPRequest, natsSubject string, clientIP string) *nyaapiserver.HTTPResponse {
	// 取得此路由的欄位白名單，用於決定轉發 payload 的形狀。
	fields, hasFields := h.routeReturnFields[req.Path]

	var reqJSON []byte
	var err error

	// 未設定 return_fields 時，送出完整 BridgeRequest。
	if !hasFields {
		bridgeReq := BridgeRequest{
			Method:     req.Method,
			Path:       req.Path,
			Headers:    req.Headers,
			Cookies:    req.Cookies,
			RemoteAddr: req.RemoteAddr,
			IP:         clientIP,
			Params:     req.Params,
			Body:       string(req.Body),
		}

		// 確保 map 欄位不為 nil，讓下游收到穩定 JSON 結構。
		if bridgeReq.Headers == nil {
			bridgeReq.Headers = make(map[string]string)
		}
		if bridgeReq.Cookies == nil {
			bridgeReq.Cookies = make(map[string]string)
		}
		if bridgeReq.Params == nil {
			bridgeReq.Params = make(map[string]string)
		}

		// 將完整橋接請求序列化為 JSON。
		reqJSON, err = json.Marshal(bridgeReq)
	} else if len(fields) == 1 {
		// 若只指定一個欄位，則直接送出該欄位內容，避免額外包一層 JSON 物件。
		for name := range fields {
			switch name {
			case "body":
				reqJSON = req.Body
			case "method":
				reqJSON = []byte(req.Method)
			case "path":
				reqJSON = []byte(req.Path)
			case "remote_addr":
				reqJSON = []byte(req.RemoteAddr)
			case "ip":
				reqJSON = []byte(clientIP)
			case "headers":
				headers := req.Headers
				if headers == nil {
					headers = make(map[string]string)
				}
				reqJSON, err = json.Marshal(headers)
			case "cookies":
				cookies := req.Cookies
				if cookies == nil {
					cookies = make(map[string]string)
				}
				reqJSON, err = json.Marshal(cookies)
			case "params":
				params := req.Params
				if params == nil {
					params = make(map[string]string)
				}
				reqJSON, err = json.Marshal(params)
			}
			break
		}
	} else {
		// 指定多個 return_fields 時，建立只包含指定欄位的 JSON 物件。
		data := make(map[string]interface{})

		// include 用於判斷某欄位是否被允許送出。
		include := func(name string) bool {
			_, ok := fields[name]
			return ok
		}

		// 依欄位白名單逐一填入資料。
		if include("method") {
			data["method"] = req.Method
		}
		if include("path") {
			data["path"] = req.Path
		}
		if include("headers") {
			headers := req.Headers
			if headers == nil {
				headers = make(map[string]string)
			}
			data["headers"] = headers
		}
		if include("cookies") {
			cookies := req.Cookies
			if cookies == nil {
				cookies = make(map[string]string)
			}
			data["cookies"] = cookies
		}
		if include("remote_addr") {
			data["remote_addr"] = req.RemoteAddr
		}
		if include("ip") {
			data["ip"] = clientIP
		}
		if include("params") {
			params := req.Params
			if params == nil {
				params = make(map[string]string)
			}
			data["params"] = params
		}
		if include("body") {
			data["body"] = string(req.Body)
		}

		// 將裁剪後的請求資料序列化為 JSON。
		reqJSON, err = json.Marshal(data)
	}

	// 若序列化失敗，代表橋接層內部資料無法轉成可傳輸格式。
	if err != nil {
		return h.errResp(500, lHTTP.HttpInternalErrorMarshalRequest(), err.Error(), clientIP)
	}

	// 取得此路由的 NATS Request 逾時設定。
	timeout := h.routeTimeouts[req.Path]

	// 發送 NATS Request 並等待微服務回應。
	logBridge(lLog.LogForwardNats(), natsSubject, timeout)
	respStr, err := h.natsClient.Request(natsSubject, string(reqJSON), timeout)
	if err != nil {
		return h.errResp(502, lHTTP.HttpBadGatewayNats(), err.Error(), clientIP)
	}

	// 嘗試將微服務回應解析為標準 BridgeResponse。
	var bridgeResp BridgeResponse
	if err := json.Unmarshal([]byte(respStr), &bridgeResp); err != nil {
		// 若解析失敗，代表微服務回傳的是普通字串或非標準格式，直接以 200 回傳原文。
		if verbose {
			logBridge(lLog.LogBridgeResponseParseFailedVerbose(), err, respStr)
		} else {
			logBridge(lLog.LogBridgeResponseParseFailed(), err)
		}
		return &nyaapiserver.HTTPResponse{
			StatusCode: 200,
			Body:       []byte(respStr),
		}
	}

	// 若微服務未指定狀態碼，預設視為 200。
	if bridgeResp.StatusCode == 0 {
		bridgeResp.StatusCode = 200
	}

	// 在回傳用戶端前，先檢查微服務回應是否符合限制與 Schema。
	if validationErr := h.validateResponse(bridgeResp, req.Path, clientIP); validationErr != nil {
		return validationErr
	}

	// 將 BridgeResponse 轉換為 HTTPResponse。
	resp := &nyaapiserver.HTTPResponse{
		StatusCode: bridgeResp.StatusCode,
		Body:       []byte(bridgeResp.Body),
		Headers:    bridgeResp.Headers,
	}

	// 確保 Headers 不為 nil，方便後續流程追加標頭。
	if resp.Headers == nil {
		resp.Headers = make(map[string]string)
	}

	// 回傳最終 HTTP 回應。
	return resp
}
