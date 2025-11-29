package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/kagurazakayashi/libNyaruko_Go/nyaapiserver"
	"github.com/kagurazakayashi/libNyaruko_Go/nyanats"
)

const natsRequestTimeout = 30 * time.Second

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
	natsClient *nyanats.NyaNATS
	routes     map[string]string // path -> nats_subject
}

// NewBridgeHandler 根據路由設定建立一個新的 BridgeHandler。
func NewBridgeHandler(natsClient *nyanats.NyaNATS, routes []RouteConfig) *BridgeHandler {
	routeMap := make(map[string]string)
	for _, r := range routes {
		fmt.Printf("[httpHandler] 載入路由: %s -> %s\n", r.Path, r.NatsSubject)
		routeMap[r.Path] = r.NatsSubject
	}
	fmt.Printf("[httpHandler] 共載入 %d 條路由\n", len(routeMap))
	return &BridgeHandler{
		natsClient: natsClient,
		routes:     routeMap,
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

	fmt.Printf("[httpHandler] 轉發請求到 NATS: subject=%s\n", natsSubject)
	respStr, err := h.natsClient.Request(natsSubject, string(reqJSON), natsRequestTimeout)
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
