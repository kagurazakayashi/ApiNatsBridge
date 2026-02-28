// Package src 提供 HTTP 伺服器的即時日誌輸出。
//
// 此檔案定義 HTTPLogger 函式，作為 HTTP 伺服器模組的日誌回呼。
package src

// HTTPLogger 負責處理 HTTP 伺服器的即時日誌輸出。
//
// 參數：
//   - line: 單筆 HTTP 日誌文字內容。
func HTTPLogger(line string) {
	if len(line) == 0 {
		return
	}
	LogHTTP("%s", line)
}
