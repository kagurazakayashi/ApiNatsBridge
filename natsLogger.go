package main

import (
	"log"
	"strings"

	"github.com/kagurazakayashi/libNyaruko_Go/nyalog"
)

type natsLogWriter struct{}

func (w *natsLogWriter) Write(p []byte) (n int, err error) {
	line := strings.TrimRight(string(p), "\n\r")
	if line == "" {
		return len(p), nil
	}
	nyalog.LogCC(logLevel, nyalog.Info, nyalog.Green, "[NATS]", line)
	return len(p), nil
}

// natsLogger 建立一個格式化的 *log.Logger，用於 NATS 用戶端的日誌輸出。
func natsLogger() *log.Logger {
	return log.New(&natsLogWriter{}, "", 0)
}
