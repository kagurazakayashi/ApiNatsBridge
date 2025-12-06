package main

import (
	"bytes"
	"encoding/json"
	"fmt"
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
}

// NewBridgeHandler 根據路由設定建立一個新的 BridgeHandler。
func NewBridgeHandler(natsClient *nyanats.NyaNATS, routes []RouteConfig, cdnHeaders []string) *BridgeHandler {
	routeMap := make(map[string]string)
	timeoutMap := make(map[string]time.Duration)
	methodMap := make(map[string]map[string]struct{})
	contentTypeMap := make(map[string]string)
	routeSchemas := make(map[string]*jsonschema.Schema)
	routeSchemaMaps := make(map[string]map[string]interface{})

	compiler := jsonschema.NewCompiler()

	for _, r := range routes {
		fmt.Printf("[httpHandler] 載入路由: %s -> %s (timeout=%v, methods=%v, content_type=%q)\n", r.Path, r.NatsSubject, r.TimeoutDuration(), r.AllowedMethods(), r.ContentType)
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
		schemaMap := buildSchema(&r)
		if schemaMap != nil {
			routeSchemaMaps[r.Path] = schemaMap
			schemaJSON, jsonErr := json.Marshal(schemaMap)
			if jsonErr != nil {
				fmt.Printf("[httpHandler] 路由 %s 的 schema_body 序列化失敗: %v\n", r.Path, jsonErr)
				continue
			}
			url := "config://routes" + r.Path
			if err := compiler.AddResource(url, bytes.NewReader(schemaJSON)); err != nil {
				fmt.Printf("[httpHandler] 路由 %s 的 schema_body 資源添加失敗: %v\n", r.Path, err)
				continue
			}
			schema, err := compiler.Compile(url)
			if err != nil {
				fmt.Printf("[httpHandler] 路由 %s 的 schema_body 無效: %v\n", r.Path, err)
				continue
			}
			routeSchemas[r.Path] = schema
			fmt.Printf("[httpHandler] 路由 %s 已載入 JSON Schema 校驗\n", r.Path)
		}
	}
	fmt.Printf("[httpHandler] 共載入 %d 條路由, %d 個 CDN 標頭\n", len(routeMap), len(cdnHeaders))
	return &BridgeHandler{
		natsClient:        natsClient,
		routes:            routeMap,
		routeTimeouts:     timeoutMap,
		routeMethods:      methodMap,
		routeContentTypes: contentTypeMap,
		routeSchemas:      routeSchemas,
		routeSchemaMaps:   routeSchemaMaps,
		cdnHeaders:        cdnHeaders,
	}
}

// Handle 為 HTTP 請求的統一入口，依序檢查設定檔路由、硬編碼路由後回傳回應。
func (h *BridgeHandler) Handle(req *nyaapiserver.HTTPRequest) *nyaapiserver.HTTPResponse {
	fmt.Printf("\n[httpHandler] HTTP 請求：%s %s | 來源：%s\n", req.Method, req.Path, req.RemoteAddr)

	if len(req.Params) > 0 {
		fmt.Printf("[httpHandler] HTTP 參數：%v\n", req.Params)
	}

	if len(req.Cookies) > 0 {
		fmt.Printf("[httpHandler] HTTP Cookie：%v\n", req.Cookies)
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
		if ct, hasCT := h.routeContentTypes[req.Path]; hasCT && strings.HasPrefix(ct, "application/x-www-form-urlencoded") && len(req.Body) > 0 {
			formValues, urlErr := url.ParseQuery(string(req.Body))
			if urlErr != nil {
				return nyaapiserver.JSONResponse(400, map[string]interface{}{
					"error":  "Invalid form body",
					"detail": urlErr.Error(),
				})
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
					fmt.Printf("[httpHandler] Schema 校驗失敗 for %s: %v\n", req.Path, err)
					return nyaapiserver.JSONResponse(400, map[string]interface{}{
						"error":  "Schema validation failed",
						"detail": err.Error(),
					})
				}
			}
			jsonBody, jsonErr := json.Marshal(formMap)
			if jsonErr != nil {
				return nyaapiserver.JSONResponse(500, map[string]string{
					"error": "Internal Server Error: failed to marshal form data",
				})
			}
			req.Body = jsonBody
			return h.forwardToNats(req, natsSubject)
		}
		if schema, hasSchema := h.routeSchemas[req.Path]; hasSchema && len(req.Body) > 0 {
			var bodyJSON interface{}
			if err := json.Unmarshal(req.Body, &bodyJSON); err != nil {
				return nyaapiserver.JSONResponse(400, map[string]interface{}{
					"error":  "Invalid JSON body",
					"detail": err.Error(),
				})
			}
			if err := schema.Validate(bodyJSON); err != nil {
				fmt.Printf("[httpHandler] Schema 校驗失敗 for %s: %v\n", req.Path, err)
				return nyaapiserver.JSONResponse(400, map[string]interface{}{
					"error":  "Schema validation failed",
					"detail": err.Error(),
				})
			}
		}
		return h.forwardToNats(req, natsSubject)
	}

	switch req.Path {
	case "/ping":
		return apiping(req)
	default:
		return &nyaapiserver.HTTPResponse{StatusCode: 404, Body: []byte("Not Found")}
	}
}

// forwardToNats 將 HTTP 請求序列化後，通過 NATS Request 轉發至對應微服務並等待回應。
func (h *BridgeHandler) forwardToNats(req *nyaapiserver.HTTPRequest, natsSubject string) *nyaapiserver.HTTPResponse {
	bridgeReq := BridgeRequest{
		Method:     req.Method,
		Path:       req.Path,
		Headers:    req.Headers,
		Cookies:    req.Cookies,
		RemoteAddr: req.RemoteAddr,
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
	// 自動判斷實際用戶端 IP
	// 優先序：CDN 標頭 > X-Real-IP > X-Forwarded-For 第一段 > RemoteAddr
	for _, header := range h.cdnHeaders {
		if ip, ok := bridgeReq.Headers[header]; ok && ip != "" {
			bridgeReq.IP = ip
			break
		}
		lower := strings.ToLower(header)
		if ip, ok := bridgeReq.Headers[lower]; ok && ip != "" {
			bridgeReq.IP = ip
			break
		}
	}
	if bridgeReq.IP == "" {
		if ip := bridgeReq.Headers["X-Real-Ip"]; ip != "" {
			bridgeReq.IP = ip
		} else if ip := bridgeReq.Headers["x-real-ip"]; ip != "" {
			bridgeReq.IP = ip
		} else if xff := bridgeReq.Headers["X-Forwarded-For"]; xff != "" {
			bridgeReq.IP = strings.Split(xff, ",")[0]
			bridgeReq.IP = strings.TrimSpace(bridgeReq.IP)
		} else if xff := bridgeReq.Headers["x-forwarded-for"]; xff != "" {
			bridgeReq.IP = strings.Split(xff, ",")[0]
			bridgeReq.IP = strings.TrimSpace(bridgeReq.IP)
		}
	}
	if bridgeReq.IP == "" {
		bridgeReq.IP = bridgeReq.RemoteAddr
	}

	reqJSON, err := json.Marshal(bridgeReq)
	if err != nil {
		fmt.Printf("[httpHandler] BridgeRequest 序列化失敗: %v\n", err)
		return nyaapiserver.JSONResponse(500, map[string]string{
			"error": "Internal Server Error: failed to marshal request",
		})
	}

	timeout := h.routeTimeouts[req.Path]
	fmt.Printf("[httpHandler] 轉發請求到 NATS: subject=%s, timeout=%v\n", natsSubject, timeout)
	respStr, err := h.natsClient.Request(natsSubject, string(reqJSON), timeout)
	if err != nil {
		fmt.Printf("[httpHandler] NATS 請求失敗: %v\n", err)
		return nyaapiserver.JSONResponse(502, map[string]string{
			"error": fmt.Sprintf("Bad Gateway: NATS request failed: %v", err),
		})
	}

	var bridgeResp BridgeResponse
	if err := json.Unmarshal([]byte(respStr), &bridgeResp); err != nil {
		fmt.Printf("[httpHandler] BridgeResponse 解析失敗: %v, 原始回應: %s\n", err, respStr)
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
