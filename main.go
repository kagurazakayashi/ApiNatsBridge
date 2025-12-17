//go:generate go-winres make

package main

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

var verbose bool

func main() {

	isOK, httpAPIServerConfig, natsConfig, bridgeConfig, routes := LoadConfig()
	if !isOK {
		return
	}

	logMain("載入路由數量: %d", len(routes))
	routeStrs := make([]string, len(routes))
	for i, r := range routes {
		routeStrs[i] = fmt.Sprintf("%s -> %s", r.Path, r.NatsSubject)
	}
	logMain("路由: %s", strings.Join(routeStrs, ", "))
	cdnStrs := make([]string, len(bridgeConfig.CdnHeader))
	for i, cdn := range bridgeConfig.CdnHeader {
		cdnStrs[i] = cdn
	}
	if len(cdnStrs) > 0 {
		logMain("CDN 標頭: %s", strings.Join(cdnStrs, ", "))
	}

	var natsClient *nyanats.NyaNATS = nyanats.NewC(*natsConfig, natsLogger())
	if err := natsClient.Error(); err != nil {
		logError("NATS", "%v", err)
		return
	}

	handler := NewBridgeHandler(natsClient, routes, bridgeConfig.CdnHeader, bridgeConfig.Limits, bridgeConfig.ResponseLimits, bridgeConfig.ErrorDetailIPs, bridgeConfig.CookieUUIDKey)
	var httpAPIServer *nyaapiserver.Server = nyaapiserver.NewServer(httpAPIServerConfig, handler.Handle, httpLogger)

	go func() {
		if err := httpAPIServer.Start("ApiNatsBridge", "1.0"); err != nil {
			logError("HTTP", "%v", err)
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
		logError("MAIN", "Stop Server: %v", err)
	}

	// 停止 NATS 連線
	if err := natsClient.UnsubscribeAll(); err != nil {
		logError("MAIN", "UnsubscribeAll: %v", err)
	}
	natsClient.Close()
}
