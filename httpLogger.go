package main

import (
	"fmt"

	"github.com/kagurazakayashi/libNyaruko_Go/nyaapiserver"
)

// httpLogger 負責處理 HTTP 伺服器的即時日誌輸出，並在符合條件時附帶輸出目前服務統計資訊。
//
// 行為說明：
//  1. 當輸入日誌內容為空字串時，直接略過，避免產生無意義輸出。
//  2. 先輸出原始 HTTP 日誌內容，便於追蹤請求流程。
//  3. 若日誌內容不是以 '#' 開頭，則視為一般有效事件，進一步輸出目前伺服器統計狀態。
//  4. 若日誌內容以 '#' 開頭，通常代表註解性或控制類型訊息，此時不額外輸出狀態資訊，避免洗版。
//
// 參數：
//   - line: 單筆 HTTP 日誌文字內容。
func httpLogger(line string) {
	// 空字串不進行任何處理，避免輸出無效日誌。
	if len(line) == 0 {
		return
	}

	// 輸出原始 HTTP 日誌內容，保留即時觀測能力。
	fmt.Printf("[HTTP]  %s\n", line)

	// 僅針對一般事件輸出統計資訊；
	// 以 '#' 開頭的訊息通常為系統內部標記，不額外附帶狀態快照。
	if line[0] != '#' {
		stats := nyaapiserver.GetStats()

		// 輸出目前伺服器執行狀態，方便快速檢視請求量、連線數、傳輸量、運作時間與封鎖 IP 清單。
		fmt.Printf(
			"[HTTPSTAT] 請求數=%d，目前連線=%d，累計傳送=%d，累計接收=%d，運作時間=%s，封鎖IP=%v\n",
			stats.TotalRequests,
			stats.CurrentConns,
			stats.TotalBytesSent,
			stats.TotalBytesRecv,
			stats.Uptime,
			stats.BlockedIPs,
		)
	}
}
