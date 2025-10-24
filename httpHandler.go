package main

import (
	"fmt"

	"github.com/kagurazakayashi/libNyaruko_Go/nyaapiserver"
)

// httpHandler 負責處理所有進入的 HTTP 請求，並依據請求路徑分派至對應的處理函式。
//
// 處理流程說明：
//  1. 輸出請求的基本資訊，包含 HTTP 方法、路徑與來源位址，便於追蹤請求來源。
//  2. 當請求附帶參數或 Cookie 時，輸出其內容以利除錯與問題排查。
//  3. 依據請求路徑進行路由分派，目前僅支援 /ping。
//  4. 若路徑未命中任何已註冊端點，則回傳 404 Not Found。
//
// 參數：
//   - req: 由伺服器框架封裝的 HTTP 請求物件，包含方法、路徑、參數、Cookie 與來源位址等資訊。
//
// 回傳值：
//   - *nyaapiserver.HTTPResponse: 對應請求的 HTTP 回應物件。
func httpHandler(req *nyaapiserver.HTTPRequest) *nyaapiserver.HTTPResponse {
	// 輸出請求的核心資訊，方便於開發與除錯階段追蹤請求流向。
	fmt.Printf("\n[httpHandler] HTTP 請求：%s %s | 來源：%s\n", req.Method, req.Path, req.RemoteAddr)

	// 當請求參數存在時，輸出完整參數內容以利分析請求行為。
	if len(req.Params) > 0 {
		fmt.Printf("[httpHandler] HTTP 參數：%v\n", req.Params)
	}

	// 當請求攜帶 Cookie 時，輸出 Cookie 內容以利驗證會話或狀態資訊。
	if len(req.Cookies) > 0 {
		fmt.Printf("[httpHandler] HTTP Cookie：%v\n", req.Cookies)
	}

	// 依據請求路徑分派至對應的 API 處理函式。
	switch req.Path {
	case "/ping":
		return apiping(req)
	default:
		// 當請求路徑不存在時，回傳 404 狀態碼與標準錯誤訊息。
		return &nyaapiserver.HTTPResponse{StatusCode: 404, Body: []byte("Not Found")}
	}
}
