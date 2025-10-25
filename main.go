package main

import (
	"context"
	"flag"
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
	var configPath string
	flag.StringVar(&configPath, "c", "", "yaml/json config file")
	flag.Parse()
	configPath, appConfig, appConfigErr := LoadConfigFile(configPath)
	fmt.Printf("[main] Config File: %s\n", configPath)
	if appConfigErr != nil {
		fmt.Printf("[main][ERROR] %v\n", appConfigErr)
		return
	}

	// 建立伺服器預設設定，並依需求覆寫必要參數。
	var httpAPIServerConfig *nyaapiserver.HttpAPIServerConfig = &appConfig.HttpAPIServerConfig

	// 依據設定與處理器建立伺服器實例。
	var httpAPIServer *nyaapiserver.Server = nyaapiserver.NewServer(httpAPIServerConfig, httpHandler, httpLogger)

	// 於獨立 Goroutine 中啟動伺服器，避免阻塞主流程，
	// 並於啟動失敗時輸出錯誤資訊。
	go func() {
		if err := httpAPIServer.Start("ApiNatsBridge", "1.0"); err != nil {
			fmt.Printf("[main][ERROR] %v\n", err)
			return
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
	if err := httpAPIServer.Stop(ctx); err != nil {
		fmt.Printf("[main] [ERROR] Stop Server: %v\n", err)
	}
}
