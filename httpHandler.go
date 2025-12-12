package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/kagurazakayashi/libNyaruko_Go/nyaapiserver"
	"github.com/kagurazakayashi/libNyaruko_Go/nyanats"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

// BridgeRequest 是通過 NATS 轉發給微服務的請求結構。
type BridgeRequest struct {
	// HTTP 請求方法（GET、POST 等）
	Method string `json:"method"`
	// HTTP 請求路徑
	Path string `json:"path"`
	// HTTP 請求標頭集合
	Headers map[string]string `json:"headers"`
	// HTTP Cookie 鍵值對集合
	Cookies map[string]string `json:"cookies"`
	// 直接連線的用戶端 IP（取自 socket）
	RemoteAddr string `json:"remote_addr"`
	// 自動判斷的實際用戶端 IP（優先序：X-Real-IP > X-Forwarded-For 第一段 > RemoteAddr）
	IP string `json:"ip"`
	// 請求參數集合（URL 查詢參數與 POST 表單資料）
	Params map[string]string `json:"params"`
	// HTTP 請求本文內容
	Body string `json:"body"`
}

// BridgeResponse 是微服務通過 NATS 返回的回應結構。
type BridgeResponse struct {
	// HTTP 狀態碼
	StatusCode int `json:"status_code"`
	// HTTP 回應標頭集合
	Headers map[string]string `json:"headers"`
	// HTTP 回應本文內容
	Body string `json:"body"`
}

// buildSchema 從 schema_body 中提取控制鍵（root_type、strict），其餘作為 JSON Schema 傳遞。
func buildSchema(r *RouteConfig) map[string]interface{} {
	if r.SchemaBody == nil {
		return nil
	}
	m := make(map[string]interface{}, len(r.SchemaBody))
	for k, v := range r.SchemaBody {
		m[k] = v
	}
	if rootType, ok := m["root_type"]; ok {
		delete(m, "root_type")
		if s, ok := rootType.(string); ok && s != "" {
			m["type"] = s
		}
	}
	if strict, ok := m["strict"]; ok {
		delete(m, "strict")
		if b, ok := strict.(bool); ok && b {
			m["additionalProperties"] = false
		}
	}
	return m
}
func coerceFormValues(formMap map[string]interface{}, schemaMap map[string]interface{}) {
	props, ok := schemaMap["properties"]
	if !ok {
		return
	}
	propMap, ok := props.(map[string]interface{})
	if !ok {
		return
	}
	for key, val := range formMap {
		propDef, hasProp := propMap[key]
		if !hasProp {
			continue
		}
		propDefMap, ok := propDef.(map[string]interface{})
		if !ok {
			continue
		}
		propType, hasType := propDefMap["type"]
		if !hasType {
			continue
		}
		typeStr, ok := propType.(string)
		if !ok {
			continue
		}
		strVal, ok := val.(string)
		if !ok {
			continue
		}
		switch typeStr {
		case "integer":
			if n, err := strconv.ParseInt(strVal, 10, 64); err == nil {
				formMap[key] = n
			}
		case "number":
			if n, err := strconv.ParseFloat(strVal, 64); err == nil {
				formMap[key] = n
			}
		case "boolean":
			switch strings.ToLower(strVal) {
			case "true", "1":
				formMap[key] = true
			case "false", "0", "":
				formMap[key] = false
			}
		}
	}
}

type BridgeHandler struct {
	// NATS 用戶端實例
	natsClient *nyanats.NyaNATS
	// 路徑到 NATS Subject 的對應表
	routes map[string]string
	// 各路徑的逾時設定
	routeTimeouts map[string]time.Duration
	// 各路徑允許的 HTTP 方法集合
	routeMethods map[string]map[string]struct{}
	// 各路徑要求的 Content-Type
	routeContentTypes map[string]string
	// 各路徑的 JSON Schema 編譯結果
	routeSchemas map[string]*jsonschema.Schema
	// 各路徑的原始 Schema 對應表（供表單類型轉換使用）
	routeSchemaMaps map[string]map[string]interface{}
	// CDN 服務商對應的 IP 標頭清單
	cdnHeaders []string
	// 全域請求欄位長度限制規則
	limits *LimitRule
	// 各路徑的請求欄位長度限制規則
	routeLimits map[string]*LimitRule
	// 各路徑的返回欄位集合（空集合表示返回全部）
	routeReturnFields map[string]map[string]struct{}
	// 允許接收錯誤詳細資訊的 IP 集合
	errorDetailIPs map[string]struct{}
}

// NewBridgeHandler 根據路由設定建立一個新的 BridgeHandler。
func NewBridgeHandler(natsClient *nyanats.NyaNATS, routes []RouteConfig, cdnHeaders []string, limits *LimitRule, errorDetailIPs []string) *BridgeHandler {
	routeMap := make(map[string]string)
	timeoutMap := make(map[string]time.Duration)
	methodMap := make(map[string]map[string]struct{})
	contentTypeMap := make(map[string]string)
	routeSchemas := make(map[string]*jsonschema.Schema)
	routeSchemaMaps := make(map[string]map[string]interface{})
	routeLimitsMap := make(map[string]*LimitRule)
	returnFieldsMap := make(map[string]map[string]struct{})

	errorIPSet := make(map[string]struct{}, len(errorDetailIPs))
	for _, ip := range errorDetailIPs {
		errorIPSet[ip] = struct{}{}
	}

	compiler := jsonschema.NewCompiler()

	for _, r := range routes {
		fmt.Printf("[BRIDGE] 載入路由: %s -> %s (timeout=%v, methods=%v, content_type=%q, return_fields=%v)\n", r.Path, r.NatsSubject, r.TimeoutDuration(), r.AllowedMethods(), r.ContentType, r.ReturnFields)
		routeMap[r.Path] = r.NatsSubject
		timeoutMap[r.Path] = r.TimeoutDuration()
		allowed := make(map[string]struct{})
		for _, m := range r.Methods {
			allowed[strings.ToUpper(m)] = struct{}{}
		}
		methodMap[r.Path] = allowed
		if r.ContentType != "" {
			contentTypeMap[r.Path] = r.ContentType
		}
		if r.Limits != nil {
			routeLimitsMap[r.Path] = r.Limits
			fmt.Printf("[BRIDGE] 路由 %s 已載入自訂長度限制\n", r.Path)
		}
		if len(r.ReturnFields) > 0 {
			fieldSet := make(map[string]struct{}, len(r.ReturnFields))
			for _, f := range r.ReturnFields {
				fieldSet[f] = struct{}{}
			}
			returnFieldsMap[r.Path] = fieldSet
		}
		schemaMap := buildSchema(&r)
		if schemaMap != nil {
			routeSchemaMaps[r.Path] = schemaMap
			schemaJSON, jsonErr := json.Marshal(schemaMap)
			if jsonErr != nil {
				fmt.Printf("[BRIDGE] 路由 %s 的 schema_body 序列化失敗: %v\n", r.Path, jsonErr)
				continue
			}
			url := "config://routes" + r.Path
			if err := compiler.AddResource(url, bytes.NewReader(schemaJSON)); err != nil {
				fmt.Printf("[BRIDGE] 路由 %s 的 schema_body 資源添加失敗: %v\n", r.Path, err)
				continue
			}
			schema, err := compiler.Compile(url)
			if err != nil {
				fmt.Printf("[BRIDGE] 路由 %s 的 schema_body 無效: %v\n", r.Path, err)
				continue
			}
			routeSchemas[r.Path] = schema
			fmt.Printf("[BRIDGE] 路由 %s 已載入 JSON Schema 校驗\n", r.Path)
		}
	}
	fmt.Printf("[BRIDGE] 共載入 %d 條路由, %d 個 CDN 標頭\n", len(routeMap), len(cdnHeaders))
	return &BridgeHandler{
		natsClient:         natsClient,
		routes:             routeMap,
		routeTimeouts:      timeoutMap,
		routeMethods:       methodMap,
		routeContentTypes:  contentTypeMap,
		routeSchemas:       routeSchemas,
		routeSchemaMaps:    routeSchemaMaps,
		cdnHeaders:         cdnHeaders,
		limits:             limits,
		routeLimits:        routeLimitsMap,
		routeReturnFields:  returnFieldsMap,
		errorDetailIPs:     errorIPSet,
	}
}

// Handle 為 HTTP 請求的統一入口，依序檢查設定檔路由、硬編碼路由後回傳回應。
func (h *BridgeHandler) Handle(req *nyaapiserver.HTTPRequest) *nyaapiserver.HTTPResponse {
	fmt.Printf("\n[BRIDGE] HTTP 請求：%s %s | 來源：%s\n", req.Method, req.Path, req.RemoteAddr)

	if len(req.Params) > 0 {
		if verbose {
			fmt.Printf("[BRIDGE] HTTP 參數：%v\n", req.Params)
		} else {
			fmt.Printf("[BRIDGE] HTTP 參數：%d 項\n", len(req.Params))
		}
	}

	if len(req.Cookies) > 0 {
		if verbose {
			fmt.Printf("[BRIDGE] HTTP Cookie：%v\n", req.Cookies)
		} else {
			fmt.Printf("[BRIDGE] HTTP Cookie：%d 項\n", len(req.Cookies))
		}
	}

	clientIP := h.resolveClientIP(req.Headers, req.RemoteAddr)
	if clientIP == "" {
		fmt.Printf("[BRIDGE] 無法解析用戶端 IP\n")
		return &nyaapiserver.HTTPResponse{StatusCode: 400, Body: []byte("Bad Request: unable to resolve client IP")}
	}

	if natsSubject, ok := h.routes[req.Path]; ok {
		if allowed, hasMethods := h.routeMethods[req.Path]; hasMethods && len(allowed) > 0 {
			if _, ok := allowed[req.Method]; !ok {
				return &nyaapiserver.HTTPResponse{StatusCode: 405, Body: []byte("Method Not Allowed")}
			}
		}
		if ct, hasCT := h.routeContentTypes[req.Path]; hasCT {
			if len(req.Body) > 0 {
				reqContentType := req.Headers["Content-Type"]
				if reqContentType == "" {
					reqContentType = req.Headers["content-type"]
				}
				if !strings.HasPrefix(reqContentType, ct) {
					return &nyaapiserver.HTTPResponse{StatusCode: 415, Body: []byte("Unsupported Media Type")}
				}
			}
		}

		effectiveLimits := h.limits
		if rl, has := h.routeLimits[req.Path]; has {
			effectiveLimits = mergeLimitRule(h.limits, rl)
		}
		if effectiveLimits != nil {
			if resp := h.validateLimits(req, effectiveLimits); resp != nil {
				return resp
			}
		}

		if ct, hasCT := h.routeContentTypes[req.Path]; hasCT && strings.HasPrefix(ct, "application/x-www-form-urlencoded") && len(req.Body) > 0 {
			formValues, urlErr := url.ParseQuery(string(req.Body))
			if urlErr != nil {
				return h.errResp(400, "Invalid form body", urlErr.Error(), clientIP)
			}
			formMap := make(map[string]interface{}, len(formValues))
			for k, v := range formValues {
				if len(v) == 1 {
					formMap[k] = v[0]
				} else {
					formMap[k] = v
				}
			}
			if schemaMap, ok := h.routeSchemaMaps[req.Path]; ok {
				coerceFormValues(formMap, schemaMap)
			}
			if schema, hasSchema := h.routeSchemas[req.Path]; hasSchema {
				if err := schema.Validate(formMap); err != nil {
					if verbose {
						fmt.Printf("[BRIDGE] Schema 校驗失敗 for %s: %v\n", req.Path, err)
					} else {
						fmt.Printf("[BRIDGE] Schema 校驗失敗 for %s\n", req.Path)
					}
					return h.errResp(400, "Schema validation failed", err.Error(), clientIP)
				}
			}
			jsonBody, jsonErr := json.Marshal(formMap)
			if jsonErr != nil {
				return nyaapiserver.JSONResponse(500, map[string]string{
					"error": "Internal Server Error: failed to marshal form data",
				})
			}
			req.Body = jsonBody
			return h.forwardToNats(req, natsSubject, clientIP)
		}
		if schema, hasSchema := h.routeSchemas[req.Path]; hasSchema && len(req.Body) > 0 {
			var bodyJSON interface{}
			if err := json.Unmarshal(req.Body, &bodyJSON); err != nil {
				return h.errResp(400, "Invalid JSON body", err.Error(), clientIP)
			}
			if err := schema.Validate(bodyJSON); err != nil {
				if verbose {
					fmt.Printf("[BRIDGE] Schema 校驗失敗 for %s: %v\n", req.Path, err)
				} else {
					fmt.Printf("[BRIDGE] Schema 校驗失敗 for %s\n", req.Path)
				}
				return h.errResp(400, "Schema validation failed", err.Error(), clientIP)
			}
		}
		return h.forwardToNats(req, natsSubject, clientIP)
	}

	switch req.Path {
	case "/ping":
		return apiping(req)
	default:
		return &nyaapiserver.HTTPResponse{StatusCode: 404, Body: []byte("Not Found")}
	}
}

// mergeLimitRule 合併全域與路由層級的限制規則，路由層級非零值會覆蓋全域對應欄位。
func mergeLimitRule(base, overlay *LimitRule) *LimitRule {
	if overlay == nil {
		return base
	}
	if base == nil {
		return overlay
	}
	merged := *base
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
	return &merged
}

// validateLimits 根據 LimitRule 檢查請求各欄位長度，不符合則回傳 400 錯誤。
func (h *BridgeHandler) validateLimits(req *nyaapiserver.HTTPRequest, r *LimitRule) *nyaapiserver.HTTPResponse {

	if r.Path.MaxLength > 0 && len(req.Path) > r.Path.MaxLength {
		return nyaapiserver.JSONResponse(400, map[string]interface{}{
			"error": "Path too long",
			"field": "path",
			"limit": r.Path.MaxLength,
		})
	}

	if r.Body.MaxLength > 0 && len(req.Body) > r.Body.MaxLength {
		return nyaapiserver.JSONResponse(400, map[string]interface{}{
			"error": "Body too long",
			"field": "body",
			"limit": r.Body.MaxLength,
		})
	}

	if resp := h.validateMapLimit(req.Headers, r.Headers, "headers"); resp != nil {
		return resp
	}
	if resp := h.validateMapLimit(req.Cookies, r.Cookies, "cookies"); resp != nil {
		return resp
	}
	if resp := h.validateMapLimit(req.Params, r.Params, "params"); resp != nil {
		return resp
	}

	return nil
}

func (h *BridgeHandler) validateMapLimit(m map[string]string, rule MapLimitRule, field string) *nyaapiserver.HTTPResponse {
	if rule.MaxCount <= 0 && rule.MaxKeyLen <= 0 && rule.MaxValueLen <= 0 {
		return nil
	}
	if rule.MaxCount > 0 && len(m) > rule.MaxCount {
		return nyaapiserver.JSONResponse(400, map[string]interface{}{
			"error":  "Too many entries",
			"field":  field,
			"limit":  rule.MaxCount,
			"actual": len(m),
		})
	}
	for k, v := range m {
		if rule.MaxKeyLen > 0 && len(k) > rule.MaxKeyLen {
			return nyaapiserver.JSONResponse(400, map[string]interface{}{
				"error":  "Key too long",
				"field":  field,
				"key":    k,
				"limit":  rule.MaxKeyLen,
				"actual": len(k),
			})
		}
		if rule.MaxValueLen > 0 && len(v) > rule.MaxValueLen {
			return nyaapiserver.JSONResponse(400, map[string]interface{}{
				"error":  "Value too long",
				"field":  field,
				"key":    k,
				"limit":  rule.MaxValueLen,
				"actual": len(v),
			})
		}
	}
	return nil
}

// isValidIP 檢查字串是否為有效的 IPv4 或 IPv6 位址。
func isValidIP(s string) bool {
	return net.ParseIP(strings.TrimSpace(s)) != nil
}

// resolveClientIP 從請求標頭中判斷實際用戶端 IP，所有來源均須通過 IP 格式驗證。
// 優先序：CDN 標頭 > X-Real-IP > X-Forwarded-For 第一段合法 IP > RemoteAddr
// 若無人通過 IP 格式驗證，則回傳空字串。
func (h *BridgeHandler) resolveClientIP(headers map[string]string, remoteAddr string) string {
	for _, header := range h.cdnHeaders {
		if ip, ok := headers[header]; ok && ip != "" && isValidIP(ip) {
			return ip
		}
		lower := strings.ToLower(header)
		if ip, ok := headers[lower]; ok && ip != "" && isValidIP(ip) {
			return ip
		}
	}
	if ip := headers["X-Real-Ip"]; ip != "" && isValidIP(ip) {
		return ip
	}
	if ip := headers["x-real-ip"]; ip != "" && isValidIP(ip) {
		return ip
	}
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
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
		remoteAddr = host
	}
	if isValidIP(remoteAddr) {
		return remoteAddr
	}
	return ""
}

// isErrorDetailIP 檢查 IP 是否在錯誤詳細資訊白名單中。
func (h *BridgeHandler) isErrorDetailIP(ip string) bool {
	if ip == "" {
		return false
	}
	_, ok := h.errorDetailIPs[ip]
	return ok
}

// errResp 建立錯誤回應，白名單 IP 可以看到詳細原因，其他 IP 只會看到通用訊息。
// detail 會同時輸出到控制台日誌。
func (h *BridgeHandler) errResp(statusCode int, msg string, detail string, clientIP string) *nyaapiserver.HTTPResponse {
	if detail != "" {
		if verbose {
			fmt.Printf("[BRIDGE] %s: %s\n", msg, detail)
		} else {
			fmt.Printf("[BRIDGE] %s\n", msg)
		}
	}
	if h.isErrorDetailIP(clientIP) && detail != "" {
		return nyaapiserver.JSONResponse(statusCode, map[string]interface{}{
			"error": msg, "detail": detail,
		})
	}
	return nyaapiserver.JSONResponse(statusCode, map[string]string{
		"error": msg,
	})
}

// forwardToNats 將 HTTP 請求序列化後，通過 NATS Request 轉發至對應微服務並等待回應。
// 若路由設定了 return_fields，則只包含指定欄位；未設定或空陣列則包含全部欄位。
func (h *BridgeHandler) forwardToNats(req *nyaapiserver.HTTPRequest, natsSubject string, clientIP string) *nyaapiserver.HTTPResponse {
	fields, hasFields := h.routeReturnFields[req.Path]

	var reqJSON []byte
	var err error

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
		if bridgeReq.Headers == nil {
			bridgeReq.Headers = make(map[string]string)
		}
		if bridgeReq.Cookies == nil {
			bridgeReq.Cookies = make(map[string]string)
		}
		if bridgeReq.Params == nil {
			bridgeReq.Params = make(map[string]string)
		}
		reqJSON, err = json.Marshal(bridgeReq)
	} else if len(fields) == 1 {
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
		data := make(map[string]interface{})
		include := func(name string) bool {
			_, ok := fields[name]
			return ok
		}
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
		reqJSON, err = json.Marshal(data)
	}

	if err != nil {
		return h.errResp(500, "Internal Server Error: failed to marshal request", err.Error(), clientIP)
	}

	timeout := h.routeTimeouts[req.Path]
	fmt.Printf("[BRIDGE] 轉發請求到 NATS: subject=%s, timeout=%v\n", natsSubject, timeout)
	respStr, err := h.natsClient.Request(natsSubject, string(reqJSON), timeout)
	if err != nil {
		return h.errResp(502, "Bad Gateway: NATS request failed", err.Error(), clientIP)
	}

	var bridgeResp BridgeResponse
	if err := json.Unmarshal([]byte(respStr), &bridgeResp); err != nil {
		if verbose {
			fmt.Printf("[BRIDGE] BridgeResponse 解析失敗: %v, 原始回應: %s\n", err, respStr)
		} else {
			fmt.Printf("[BRIDGE] BridgeResponse 解析失敗: %v\n", err)
		}
		return &nyaapiserver.HTTPResponse{
			StatusCode: 200,
			Body:       []byte(respStr),
		}
	}

	if bridgeResp.StatusCode == 0 {
		bridgeResp.StatusCode = 200
	}

	resp := &nyaapiserver.HTTPResponse{
		StatusCode: bridgeResp.StatusCode,
		Body:       []byte(bridgeResp.Body),
		Headers:    bridgeResp.Headers,
	}
	if resp.Headers == nil {
		resp.Headers = make(map[string]string)
	}
	return resp
}
