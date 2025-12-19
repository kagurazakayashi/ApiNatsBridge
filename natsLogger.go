package main

import (
	"log"
	"strings"

	"github.com/kagurazakayashi/libNyaruko_Go/nyalog"
)

// natsLogWriter 將 NATS 相關日誌導向專案內部的標準輸出與檔案日誌處理流程。
type natsLogWriter struct{}

// Write 實作 io.Writer 介面，用於接收 NATS logger 輸出的日誌內容。
func (w *natsLogWriter) Write(p []byte) (n int, err error) {
	// 移除行尾換行符號，避免輸出時產生多餘空行。
	line := strings.TrimRight(string(p), "\n\r")
	if line == "" {
		return len(p), nil
	}

	// 若未載入日誌設定，或設定允許輸出到標準輸出，則將 NATS 日誌寫入 stdout。
	if logConfig == nil || logConfig.Stdout {
		stdoutLog(logLevel, nyalog.Info, nyalog.Green, "[NATS]", line)
	}

	// 若已設定 NATS 日誌檔案路徑，則同步寫入對應的檔案日誌。
	if logConfig != nil && logConfig.Files.NATS != "" {
		writeToFile(logConfig.Files.NATS, nyalog.Green, "[NATS]", line)
	}

	return len(p), nil
}

// natsLogger 建立供 NATS 使用的標準 log.Logger，並將輸出交由 natsLogWriter 處理。
func natsLogger() *log.Logger {
	return log.New(&natsLogWriter{}, "", 0)
}
