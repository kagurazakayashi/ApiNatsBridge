package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/kagurazakayashi/libNyaruko_Go/nyanats"
	"gopkg.in/yaml.v3"
)

// routeConfig 定義單一路由與 NATS Subject 的對應關係。
type routeConfig struct {
	Path        string `json:"path" yaml:"path"`
	NatsSubject string `json:"nats_subject" yaml:"nats_subject"`
}

// mockServiceConfig 定義 mock service 啟動時所需的完整設定。
type mockServiceConfig struct {
	NatsConfig nyanats.NatsConfig `json:"nats_config" yaml:"nats_config"`
	Routes     []routeConfig      `json:"routes" yaml:"routes"`
}

// bridgeRequest 定義由橋接層傳入 mock service 的請求資料格式。
type bridgeRequest struct {
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers"`
	Params  map[string]string `json:"params"`
	Body    string            `json:"body"`
}

// bridgeResponse 定義 mock service 回傳給橋接層的回應資料格式。
type bridgeResponse struct {
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers"`
	Body       string            `json:"body"`
}

// loadConfig 載入指定路徑的 YAML 設定檔；若未指定路徑，則依執行檔名稱推導預設設定檔名稱。
func loadConfig(configPath string) (*mockServiceConfig, string, error) {
	if configPath == "" {
		exePath, err := os.Executable()
		if err != nil {
			exePath = os.Args[0]
		}
		exeBase := filepath.Base(exePath)
		configPath = strings.TrimSuffix(exeBase, filepath.Ext(exeBase)) + ".yaml"
	}

	var cfg mockServiceConfig
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, configPath, err
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, configPath, err
	}

	return &cfg, configPath, nil
}

// natsLogger 建立供 NATS 用戶端使用的標準日誌輸出器。
func natsLogger() *log.Logger {
	return log.New(os.Stdout, "[mock_service] ", log.LstdFlags)
}

// main 啟動 mock service，完成設定載入、NATS 連線、路由訂閱與優雅關閉流程。
func main() {
	log.SetOutput(os.Stdout)
	log.SetPrefix("[mock_service] ")
	log.SetFlags(0)

	var configPath string
	flag.StringVar(&configPath, "c", "", "yaml config file")
	flag.Parse()

	cfg, resolvedPath, err := loadConfig(configPath)
	if err != nil {
		log.Printf("載入設定檔失敗: %v (path: %s)", err, resolvedPath)
		return
	}

	log.Println()
	log.Printf("設定檔: %s", resolvedPath)
	log.Printf("NATS 伺服器: %s:%d", cfg.NatsConfig.NatsServerHost, cfg.NatsConfig.NatsServerPort)
	log.Printf("共載入 %d 條路由", len(cfg.Routes))
	log.Println()

	natsClient := nyanats.NewC(cfg.NatsConfig, natsLogger())
	if err := natsClient.Error(); err != nil {
		log.Printf("NATS 連線失敗: %v", err)
		return
	}

	for _, route := range cfg.Routes {
		subject := route.NatsSubject
		httpPath := route.Path

		err := natsClient.Subscribe(subject, func(m string) string {
			log.Println()
			log.Println("===== 收到請求 =====")
			log.Printf("NATS Subject : %s", subject)
			log.Printf("HTTP Path    : %s", httpPath)
			log.Printf("原始訊息      : %s", m)

			var req bridgeRequest
			if err := json.Unmarshal([]byte(m), &req); err != nil {
				log.Printf("解析失敗      : %v", err)
				return `{"status_code":400,"body":"Invalid request JSON"}`
			}

			log.Printf("HTTP Method  : %s", req.Method)
			log.Printf("HTTP Path    : %s", req.Path)
			log.Println("Headers      :")
			for k, v := range req.Headers {
				log.Printf("  %s: %s", k, v)
			}
			log.Println("Params       :")
			for k, v := range req.Params {
				log.Printf("  %s = %s", k, v)
			}
			log.Printf("Body         : %s", req.Body)

			resp := bridgeResponse{
				StatusCode: 200,
				Headers: map[string]string{
					"Content-Type": "application/json; charset=utf-8",
				},
				Body: fmt.Sprintf(
					`{"message":"hello from mock_service","echo":{"method":"%s","path":"%s"}}`,
					req.Method, req.Path,
				),
			}
			respJSON, _ := json.Marshal(resp)
			respBody := string(respJSON)

			log.Println("----- 發送回覆 -----")
			log.Printf("回應 JSON    : %s", respBody)
			log.Printf("狀態碼       : %d", resp.StatusCode)
			log.Println("=====")
			log.Println()

			return respBody
		})

		if err != nil {
			log.Printf("訂閱失敗 %s: %v", subject, err)
			return
		}
		log.Printf("已訂閱: %s  ->  %s", httpPath, subject)
	}

	log.Println()
	log.Println("所有訂閱已完成，等待請求... (Ctrl+C 退出)")
	log.Println()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println()
	log.Println("正在關閉...")
	if err := natsClient.UnsubscribeAll(); err != nil {
		log.Printf("UnsubscribeAll 錯誤: %v", err)
	}
	natsClient.Close()
	log.Println("已關閉")
}
