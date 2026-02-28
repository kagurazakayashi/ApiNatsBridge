// Package src 提供 NATS 用戶端的日誌輸出橋接。
//
// 此檔案定義 natsLogWriter，將 NATS 底層日誌串接至專案統一日誌系統，
// 並產生可供 NATS 用戶端初始化時使用的標準 log.Logger 實例。
package src

import (
	"log"
	"strings"

	"github.com/kagurazakayashi/libNyaruko_Go/nyalog"
)

// natsLogWriter 將 NATS 相關日誌導向專案內部的標準輸出與檔案日誌處理流程。
type natsLogWriter struct{}

// Write 實作 io.Writer 介面，用於接收 NATS logger 輸出的日誌內容。
//
// 當 NATS debug 模式未啟用時，會過濾掉包含訊息酬載（->、<- 行）
// 與加密設定資訊（S# 行）的日誌，僅保留連線、訂閱與錯誤等狀態資訊。
func (w *natsLogWriter) Write(p []byte) (n int, err error) {
	// 移除行尾換行符號，避免輸出時產生多餘空行。
	line := strings.TrimRight(string(p), "\n\r")
	if line == "" {
		return len(p), nil
	}

	// 若非 NATS 偵錯模式，過濾包含敏感資訊的 NATS 日誌行。
	// -> 為發送訊息酬載，<- 為接收訊息酬載，S# 為加密金鑰設定資訊。
	if !HasDebugNats() {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "->") || strings.HasPrefix(trimmed, "<-") || strings.HasPrefix(trimmed, "S#") {
			return len(p), nil
		}
	}

	// 若未載入日誌設定，或設定允許輸出到標準輸出，則將 NATS 日誌寫入 stdout。
	if logConfig == nil || logConfig.Stdout {
		stdoutLog(logLevel, nyalog.Info, nyalog.Green, "[NATS]", line)
	}

	// 若已設定 NATS 日誌檔案路徑，則同步寫入對應的檔案日誌。
	if logConfig != nil && logConfig.Files.NATS != "" {
		writeToFile(logConfig.Files.NATS, nyalog.Green, "[NATS]", line)
	}

	// 若已設定統一日誌檔案路徑，則同步寫入統一日誌檔案。
	writeToFile(logFilePath, nyalog.Green, "[NATS]", line)

	return len(p), nil
}

// NatsLogger 建立供 NATS 使用的標準 log.Logger，並將輸出交由 natsLogWriter 處理。
func NatsLogger() *log.Logger {
	return log.New(&natsLogWriter{}, "", 0)
}
