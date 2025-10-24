package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kagurazakayashi/libNyaruko_Go/nyaapiserver"
)

// main 為程式進入點，負責完成以下工作：
// 1. 建立並初始化 HTTP API 伺服器設定。
// 2. 啟動伺服器並監聽啟動期間可能發生的錯誤。
// 3. 監聽系統終止訊號，於收到訊號後執行具逾時控制的優雅關閉流程。
func main() {
	// 建立伺服器預設設定，並依需求覆寫必要參數。
	var conf *nyaapiserver.HttpAPIServerConfig = nyaapiserver.DefaultConfig()
	conf.Host = "127.0.0.1"
	conf.Port = 9080
	conf.LimitRequests = 5
	conf.Logger = httpLogger

	// 綁定 HTTP 請求處理器，供伺服器於收到請求時呼叫。
	var handler func(req *nyaapiserver.HTTPRequest) *nyaapiserver.HTTPResponse = httpHandler

	// 依據設定與處理器建立伺服器實例。
	var srv *nyaapiserver.Server = nyaapiserver.NewServer(conf, handler)

	// 於獨立 Goroutine 中啟動伺服器，避免阻塞主流程，
	// 並於啟動失敗時輸出錯誤資訊。
	go func() {
		if err := srv.Start("libNyaruko_Go TestRunServer", "1.0"); err != nil {
			fmt.Printf("[main] [錯誤] 伺服器啟動失敗：%v\n", err)
		}
	}()

	// 建立系統訊號通道，監聽中斷與終止訊號，以便觸發優雅關閉。
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// 阻塞等待結束訊號。
	<-quit

	// 建立具 5 秒逾時的 Context，避免關閉程序無限等待。
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 執行伺服器停止流程，若停止失敗則輸出錯誤資訊。
	if err := srv.Stop(ctx); err != nil {
		fmt.Printf("[main] [錯誤] 伺服器停止失敗：%v\n", err)
	}
}
