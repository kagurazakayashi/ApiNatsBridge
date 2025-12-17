package main

import (
	"strconv"
	"time"

	"github.com/kagurazakayashi/libNyaruko_Go/nyaapiserver"
)

// ApiResponsePing 定義 ping API 的 JSON 回應結構。
type ApiResponsePing struct {
	// 伺服器與客戶端時間戳差值（毫秒）
	Pong int64 `json:"pong"`
}

// apiping 處理 ping 請求，並依據是否帶有 X-Timestamp-Ms 標頭決定回應內容。
//
// 處理流程說明：
//   - 若請求標頭中包含 X-Timestamp-Ms，則嘗試將其解析為毫秒時間戳。
//   - 解析成功時，回傳 JSON 格式資料，Pong 為伺服器當前時間與客戶端時間戳的差值。
//   - 若標頭不存在或解析失敗，則回傳純文字 "pong"。
//
// 備註：
//   - 本函式不修改既有業務邏輯，僅補強註解與除錯輸出內容。
//   - 除錯輸出統一加上 [apiping] 前綴，以利追蹤來源。
func apiping(req *nyaapiserver.HTTPRequest) *nyaapiserver.HTTPResponse {
	// 讀取客戶端傳入的毫秒級時間戳標頭，第二個回傳值表示該標頭是否存在。
	clientTimestamp, clientTimestamp1 := req.Headers["X-Timestamp-Ms"]
	if clientTimestamp1 {
		// 將字串格式的毫秒時間戳轉為 int64，供後續延遲計算使用。
		clientTimestampMs, err := strconv.ParseInt(clientTimestamp, 10, 64)
		if err == nil {
			// 取得伺服器目前的 Unix 毫秒時間戳。
			var nowTimestampMs int64 = time.Now().UnixMilli()

			// 建立 JSON 回應，Pong 為伺服器時間與客戶端時間的差值。
			var response *nyaapiserver.HTTPResponse = nyaapiserver.JSONResponse(200, ApiResponsePing{
				Pong: nowTimestampMs - clientTimestampMs,
			})

			// 輸出回應標頭，便於除錯確認 JSON 回應是否正確附帶必要標頭。
			logPing("回應標頭：%v", response.Headers)
			return response
		}

		// 當時間戳格式不正確時輸出除錯資訊，協助定位客戶端傳值問題。
		logPing("X-Timestamp-Ms 解析失敗：%s", clientTimestamp)
	}

	// 若未提供有效的時間戳標頭，則回傳基本文字 pong。
	var response *nyaapiserver.HTTPResponse = &nyaapiserver.HTTPResponse{
		StatusCode: 200,
		Body:       []byte("pong"),
		// Headers:    map[string]string{"X-Custom-Header": "Go-Server"},
	}

	// 輸出退回純文字回應的原因，便於除錯判斷請求是否缺少必要標頭。
	logPing("使用純文字 pong 回應，原因：未提供或無法解析 X-Timestamp-Ms")
	return response
}
