package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/kagurazakayashi/libNyaruko_Go/nyaapiserver"
	"github.com/kagurazakayashi/libNyaruko_Go/nyanats"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

// BridgeRequest 是通過 NATS 轉發給微服務的請求結構。
type BridgeRequest struct {
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers"`
	Params  map[string]string `json:"params"`
	Body    string            `json:"body"`
}

// BridgeResponse 是微服務通過 NATS 返回的回應結構。
type BridgeResponse struct {
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers"`
	Body       string            `json:"body"`
}

// BridgeHandler 負責 HTTP 請求的路由分派與 NATS 轉發。
type BridgeHandler struct {
	natsClient        *nyanats.NyaNATS
	routes            map[string]string              // path -> nats_subject
	routeTimeouts     map[string]time.Duration       // path -> timeout
	routeMethods      map[string]map[string]struct{} // path -> set of allowed methods
	routeContentTypes map[string]string              // path -> required Content-Type
	routeSchemas      map[string]*jsonschema.Schema  // path -> JSON Schema
}

// NewBridgeHandler 根據路由設定建立一個新的 BridgeHandler。
func NewBridgeHandler(natsClient *nyanats.NyaNATS, routes []RouteConfig) *BridgeHandler {
	routeMap := make(map[string]string)
	timeoutMap := make(map[string]time.Duration)
	methodMap := make(map[string]map[string]struct{})
	contentTypeMap := make(map[string]string)
	routeSchemas := make(map[string]*jsonschema.Schema)

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
		if r.SchemaBody != nil {
			schemaJSON, jsonErr := json.Marshal(r.SchemaBody)
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
	fmt.Printf("[httpHandler] 共載入 %d 條路由\n", len(routeMap))
	return &BridgeHandler{
		natsClient:        natsClient,
		routes:            routeMap,
		routeTimeouts:     timeoutMap,
		routeMethods:      methodMap,
		routeContentTypes: contentTypeMap,
		routeSchemas:      routeSchemas,
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
		Method:  req.Method,
		Path:    req.Path,
		Headers: req.Headers,
		Params:  req.Params,
		Body:    string(req.Body),
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
