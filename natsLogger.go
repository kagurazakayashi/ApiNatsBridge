package main

import (
	"log"
	"os"
)

// natsLogger 建立一個格式化的 *log.Logger，用於 NATS 用戶端的日誌輸出。
// 輸出格式與 httpLogger 風格一致，使用 [NATS] 前綴並包含時間戳記。
func natsLogger() *log.Logger {
	return log.New(os.Stdout, "[NATS]  ", log.LstdFlags)
}
