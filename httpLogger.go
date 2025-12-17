package main

import (
	"github.com/kagurazakayashi/libNyaruko_Go/nyaapiserver"
)

// httpLogger 負責處理 HTTP 伺服器的即時日誌輸出，並在符合條件時附帶輸出目前服務統計資訊。
//
// 參數：
//   - line: 單筆 HTTP 日誌文字內容。
func httpLogger(line string) {
	if len(line) == 0 {
		return
	}
	logHTTP("%s", line)
	if line[0] != '#' {
		stats := nyaapiserver.GetStats()
		logHTTPStat("請求數=%d，目前連線=%d，累計傳送=%d，累計接收=%d，運作時間=%s，封鎖IP=%v",
			stats.TotalRequests,
			stats.CurrentConns,
			stats.TotalBytesSent,
			stats.TotalBytesRecv,
			stats.Uptime,
			stats.BlockedIPs,
		)
	}
}
