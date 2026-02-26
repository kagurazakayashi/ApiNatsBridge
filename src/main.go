// Package src 提供 ApiNatsBridge 的核心執行邏輯。
//
// 此檔案定義 Run 函式，作為程式的實際進入點，負責載入設定、初始化日誌、
// 建立 NATS 用戶端與 HTTP API 伺服器，並在收到系統中斷或終止訊號時執行優雅關閉流程。
package src

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/kagurazakayashi/libNyaruko_Go/nyaapiserver"
	"github.com/kagurazakayashi/libNyaruko_Go/nyanats"
)

// Run 是程式的實際進入點，負責載入設定、初始化日誌、建立 NATS 用戶端與 HTTP API 伺服器，
// 並在收到系統中斷或終止訊號時執行優雅關閉流程。
func Run() {

	// 載入 HTTP API 伺服器、NATS、橋接器與路由設定；若設定無效則直接結束程式。
	isOK, httpAPIServerConfig, natsConfig, bridgeConfig, routes := LoadConfig()
	if !isOK {
		return
	}

	// 依橋接器設定初始化多國語言。
	InitL10n(bridgeConfig.Language)

	// 依橋接器設定初始化日誌與時區。
	InitLogConfig(bridgeConfig.Log, bridgeConfig.Timezone)

	// 輸出已載入的路由數量與路由對應關係，方便啟動時確認設定狀態。
	LogMain(LLog.LogLoadRouteCount(), len(routes))
	routeStrs := make([]string, len(routes))
	for i, r := range routes {
		routeStrs[i] = fmt.Sprintf("%s -> %s", r.Path, r.NatsSubject)
	}
	LogMain(LLog.LogRoutes(), strings.Join(routeStrs, ", "))

	// 整理並輸出 CDN 標頭設定；若未設定 CDN 標頭則不輸出。
	if len(bridgeConfig.CdnHeader) > 0 {
		LogMain(LLog.LogCdnHeaders(), strings.Join(bridgeConfig.CdnHeader, ", "))
	}

	// 建立 NATS 用戶端，並在初始化失敗時輸出錯誤後結束程式。
	var natsClient *nyanats.NyaNATS = nyanats.NewC(*natsConfig, NatsLogger())
	if err := natsClient.Error(); err != nil {
		LogError("NATS", "%v", err)
		return
	}

	// 建立橋接處理器，負責將 HTTP 請求依路由設定轉送至對應的 NATS subject。
	handler := NewBridgeHandler(natsClient, routes, bridgeConfig.CdnHeader, bridgeConfig.Limits, bridgeConfig.ResponseLimits, bridgeConfig.ErrorDetailIPs, bridgeConfig.HTTPCodeKey, bridgeConfig.ErrorCodeKey, bridgeConfig.ResponseSchemaBody, bridgeConfig.ResponseErrorSchemaBody, bridgeConfig.ErrorInfoShow)

	// 建立 HTTP API 伺服器，並掛載橋接處理器作為請求入口。
	var httpAPIServer *nyaapiserver.Server = nyaapiserver.NewServer(httpAPIServerConfig, handler.Handle, HTTPLogger)

	// 以 goroutine 啟動 HTTP API 伺服器，避免阻塞後續的系統訊號監聽流程。
	go func() {
		if err := httpAPIServer.Start("ApiNatsBridge", "1.0"); err != nil {
			LogError("HTTP", "%v", err)
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
		LogError("MAIN", LLog.LogStopServer(), err)
	}

	// 解除所有 NATS 訂閱，避免關閉前仍保留未釋放的訂閱資源。
	if err := natsClient.UnsubscribeAll(); err != nil {
		LogError("MAIN", LLog.LogUnsubscribeAll(), err)
	}

	// 關閉 NATS 連線，完成程式結束前的資源釋放。
	natsClient.Close()
}
