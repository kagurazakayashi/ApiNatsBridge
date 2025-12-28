// Package main 提供 HTTP 伺服器的即時日誌輸出與統計資訊記錄。
//
// 此檔案定義 httpLogger 函式，作為 HTTP 伺服器模組的日誌回呼，
// 在每次請求處理時附加輸出目前的服務執行統計資訊。
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
		logHTTPStat(lLog.LogHttpStat(),
			stats.TotalRequests,
			stats.CurrentConns,
			stats.TotalBytesSent,
			stats.TotalBytesRecv,
			stats.Uptime,
			stats.BlockedIPs,
		)
	}
}
