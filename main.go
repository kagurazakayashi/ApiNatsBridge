package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kagurazakayashi/libNyaruko_Go/nyaapiserver"
	"github.com/kagurazakayashi/libNyaruko_Go/nyanats"
)

func main() {

	isOK, httpAPIServerConfig, natsConfig, routes := LoadConfig()
	if !isOK {
		return
	}

	fmt.Printf("[main] 載入路由數量: %d\n", len(routes))
	for _, r := range routes {
		fmt.Printf("[main] 路由: %s -> %s\n", r.Path, r.NatsSubject)
	}

	var natsClient *nyanats.NyaNATS = nyanats.NewC(*natsConfig, natsLogger())
	if err := natsClient.Error(); err != nil {
		fmt.Printf("[ERROR][NSTA] %v\n", err)
		return
	}

	handler := NewBridgeHandler(natsClient, routes)
	var httpAPIServer *nyaapiserver.Server = nyaapiserver.NewServer(httpAPIServerConfig, handler.Handle, httpLogger)

	go func() {
		if err := httpAPIServer.Start("ApiNatsBridge", "1.0"); err != nil {
			fmt.Printf("[ERROR][HTTP] %v\n", err)
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
		fmt.Printf("[MAIN] [ERROR] Stop Server: %v\n", err)
	}

	// 停止 NATS 連線
	if err := natsClient.UnsubscribeAll(); err != nil {
		fmt.Printf("[MAIN] [ERROR] UnsubscribeAll: %v\n", err)
	}
	natsClient.Close()
}
