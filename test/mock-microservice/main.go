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
	"sync"
	"syscall"
	"time"

	"github.com/kagurazakayashi/libNyaruko_Go/nyalog"
	"github.com/kagurazakayashi/libNyaruko_Go/nyanats"
	"gopkg.in/yaml.v3"
)

const mockLogLevel = nyalog.Debug

var (
	logFile  *os.File
	noOutput bool
	logMu    sync.Mutex
)

func writeLog(level nyalog.LogLevel, color nyalog.ConsoleColor, msg string) {
	if !noOutput {
		nyalog.LogCC(mockLogLevel, level, color, "[mock_service]", msg)
	}
	if logFile != nil {
		logMu.Lock()
		fmt.Fprintf(logFile, "[%s] [%s] [mock_service] %s\n", level.String(), time.Now().Format("2006-01-02 15:04:05"), msg)
		logMu.Unlock()
	}
}

type mockNatsWriter struct{}

func (w *mockNatsWriter) Write(p []byte) (n int, err error) {
	line := strings.TrimRight(string(p), "\n\r")
	if line == "" {
		return len(p), nil
	}
	writeLog(nyalog.Info, nyalog.Green, line)
	return len(p), nil
}

func logMock(format string, a ...interface{}) {
	writeLog(nyalog.Info, nyalog.Green, fmt.Sprintf(format, a...))
}

func logMockRequest(format string, a ...interface{}) {
	writeLog(nyalog.Info, nyalog.Yellow, fmt.Sprintf(format, a...))
}

func logMockError(format string, a ...interface{}) {
	writeLog(nyalog.Error, nyalog.Red, fmt.Sprintf(format, a...))
}

// routeConfig 定義單一路由與 NATS Subject 的對應關係。
type routeConfig struct {
	// HTTP 請求路徑
	Path string `json:"path" yaml:"path"`
	// 對應的 NATS Subject
	NatsSubject string `json:"nats_subject" yaml:"nats_subject"`
}

// mockServiceConfig 定義 mock service 啟動時所需的完整設定。
type mockServiceConfig struct {
	// NATS 用戶端設定
	NatsConfig nyanats.NatsConfig `json:"nats_config" yaml:"nats_config"`
	// 路由清單
	Routes []routeConfig `json:"routes" yaml:"routes"`
}

// bridgeRequest 定義由橋接層傳入 mock service 的請求資料格式。
type bridgeRequest struct {
	// HTTP 請求方法（GET、POST 等）
	Method string `json:"method"`
	// HTTP 請求路徑
	Path string `json:"path"`
	// HTTP 請求標頭集合
	Headers map[string]string `json:"headers"`
	// HTTP Cookie 鍵值對集合
	Cookies map[string]string `json:"cookies"`
	// 直接連線的用戶端 IP（取自 socket）
	RemoteAddr string `json:"remote_addr"`
	// 自動判斷的實際用戶端 IP（優先序：X-Real-IP > X-Forwarded-For 第一段 > RemoteAddr）
	IP string `json:"ip"`
	// 請求參數集合（URL 查詢參數與 POST 表單資料）
	Params map[string]string `json:"params"`
	// HTTP 請求本文內容
	Body string `json:"body"`
}

// bridgeResponse 定義 mock service 回傳給橋接層的回應資料格式。
type bridgeResponse struct {
	// HTTP 狀態碼
	StatusCode int `json:"status_code"`
	// HTTP 回應標頭集合
	Headers map[string]string `json:"headers"`
	// HTTP 回應本文內容
	Body string `json:"body"`
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

// natsLogger 建立供 NATS 用戶端使用的日誌輸出器。
func natsLogger() *log.Logger {
	return log.New(&mockNatsWriter{}, "", 0)
}

// main 啟動 mock service，完成設定載入、NATS 連線、路由訂閱與優雅關閉流程。
func main() {
	var configPath string
	var logPath string
	flag.StringVar(&configPath, "c", "", "yaml config file")
	flag.StringVar(&logPath, "log", "", "log file path")
	flag.BoolVar(&noOutput, "noout", false, "suppress console output")
	flag.Parse()

	if logPath != "" {
		f, err := os.OpenFile(logPath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "無法開啟日誌檔案: %v\n", err)
			os.Exit(1)
		}
		logFile = f
		defer logFile.Close()
	}

	cfg, resolvedPath, err := loadConfig(configPath)
	if err != nil {
		logMockError("載入設定檔失敗: %v (path: %s)", err, resolvedPath)
		return
	}

	logMock("設定檔: %s", resolvedPath)
	logMock("NATS 伺服器: %s:%d", cfg.NatsConfig.NatsServerHost, cfg.NatsConfig.NatsServerPort)
	logMock("共載入 %d 條路由", len(cfg.Routes))

	natsClient := nyanats.NewC(cfg.NatsConfig, natsLogger())
	if err := natsClient.Error(); err != nil {
		logMockError("NATS 連線失敗: %v", err)
		return
	}

	for _, route := range cfg.Routes {
		subject := route.NatsSubject
		httpPath := route.Path

		err := natsClient.Subscribe(subject, func(m string) string {
			logMockRequest("===== 收到請求 =====")
			logMockRequest("NATS Subject : %s", subject)
			logMockRequest("HTTP Path    : %s", httpPath)
			logMockRequest("原始訊息      : %s", m)

			var req bridgeRequest
			if err := json.Unmarshal([]byte(m), &req); err != nil {
				logMockError("解析失敗      : %v", err)
				return `{"status_code":400,"body":"Invalid request JSON"}`
			}

			logMockRequest("HTTP Method  : %s", req.Method)
			logMockRequest("HTTP Path    : %s", req.Path)
			logMockRequest("Remote Addr  : %s", req.RemoteAddr)
			logMockRequest("IP           : %s", req.IP)
			headerLines := make([]string, 0, len(req.Headers))
			for k, v := range req.Headers {
				headerLines = append(headerLines, fmt.Sprintf("%s: %s", k, v))
			}
			if len(headerLines) > 0 {
				logMockRequest("Headers      : %s", strings.Join(headerLines, ", "))
			}
			cookieLines := make([]string, 0, len(req.Cookies))
			for k, v := range req.Cookies {
				cookieLines = append(cookieLines, fmt.Sprintf("%s: %s", k, v))
			}
			if len(cookieLines) > 0 {
				logMockRequest("Cookies      : %s", strings.Join(cookieLines, ", "))
			}
			paramLines := make([]string, 0, len(req.Params))
			for k, v := range req.Params {
				paramLines = append(paramLines, fmt.Sprintf("%s = %s", k, v))
			}
			if len(paramLines) > 0 {
				logMockRequest("Params       : %s", strings.Join(paramLines, ", "))
			}
			logMockRequest("Body         : %s", req.Body)

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

			logMockRequest("----- 發送回覆 -----")
			logMockRequest("回應 JSON    : %s", respBody)
			logMockRequest("狀態碼       : %d", resp.StatusCode)
			logMockRequest("=====")

			return respBody
		})

		if err != nil {
			logMockError("訂閱失敗 %s: %v", subject, err)
			return
		}
		logMock("已訂閱: %s  ->  %s", httpPath, subject)
	}

	logMock("所有訂閱已完成，等待請求... (Ctrl+C 退出)")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logMock("正在關閉...")
	if err := natsClient.UnsubscribeAll(); err != nil {
		logMockError("UnsubscribeAll 錯誤: %v", err)
	}
	natsClient.Close()
	logMock("已關閉")
}
