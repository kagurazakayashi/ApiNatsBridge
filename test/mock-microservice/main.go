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

// mockLogLevel 定義 mock service 輸出日誌時使用的最低日誌等級。
const mockLogLevel = nyalog.Debug

var (
	// logFile 保存可選的檔案日誌輸出目標；未指定時僅輸出到主控台。
	logFile *os.File

	// noOutput 控制是否抑制主控台日誌輸出。
	noOutput bool

	// logMu 保護 logFile 的併發寫入，避免多個 goroutine 同時寫檔造成內容交錯。
	logMu sync.Mutex
)

// writeLog 依指定等級與顏色輸出 mock service 日誌，並在設定 logFile 時同步寫入檔案。
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

// mockNatsWriter 將 NATS 用戶端的標準 log 輸出轉接到 mock service 的日誌系統。
type mockNatsWriter struct{}

// Write 實作 io.Writer 介面，將 NATS 用戶端輸出的單行文字寫入 mock service 日誌。
func (w *mockNatsWriter) Write(p []byte) (n int, err error) {
	line := strings.TrimRight(string(p), "\n\r")
	if line == "" {
		return len(p), nil
	}
	writeLog(nyalog.Info, nyalog.Green, line)
	return len(p), nil
}

// logMock 輸出一般 mock service 執行狀態日誌。
func logMock(format string, a ...interface{}) {
	writeLog(nyalog.Info, nyalog.Green, fmt.Sprintf(format, a...))
}

// logMockRequest 輸出請求與回應處理流程相關日誌。
func logMockRequest(format string, a ...interface{}) {
	writeLog(nyalog.Info, nyalog.Yellow, fmt.Sprintf(format, a...))
}

// logMockError 輸出 mock service 執行過程中的錯誤日誌。
func logMockError(format string, a ...interface{}) {
	writeLog(nyalog.Error, nyalog.Red, fmt.Sprintf(format, a...))
}

// routeConfig 定義單一路由與 NATS Subject 的對應關係。
type routeConfig struct {
	// Path 表示橋接層接收到的 HTTP 請求路徑。
	Path string `json:"path" yaml:"path"`

	// NatsSubject 表示此 HTTP 路徑對應的 NATS Subject。
	NatsSubject string `json:"nats_subject" yaml:"nats_subject"`
}

// mockServiceConfig 定義 mock service 啟動時所需的完整設定。
type mockServiceConfig struct {
	// NatsConfig 保存 NATS 伺服器連線與用戶端相關設定。
	NatsConfig nyanats.NatsConfig `json:"nats_config" yaml:"nats_config"`

	// Routes 保存 HTTP 路徑與 NATS Subject 的路由對應清單。
	Routes []routeConfig `json:"routes" yaml:"routes"`
}

// bridgeRequest 定義由橋接層傳入 mock service 的請求資料格式。
type bridgeRequest struct {
	// Method 表示 HTTP 請求方法，例如 GET、POST 等。
	Method string `json:"method"`

	// Path 表示 HTTP 請求路徑。
	Path string `json:"path"`

	// Headers 保存 HTTP 請求標頭集合。
	Headers map[string]string `json:"headers"`

	// Cookies 保存 HTTP Cookie 鍵值對集合。
	Cookies map[string]string `json:"cookies"`

	// RemoteAddr 表示直接連線的用戶端位址，通常取自 socket。
	RemoteAddr string `json:"remote_addr"`

	// IP 表示自動判斷出的實際用戶端 IP，優先序通常為 X-Real-IP、X-Forwarded-For 第一段、RemoteAddr。
	IP string `json:"ip"`

	// Params 保存請求參數集合，包含 URL 查詢參數與 POST 表單資料。
	Params map[string]string `json:"params"`

	// Body 保存 HTTP 請求本文內容。
	Body string `json:"body"`
}

// bridgeResponse 定義 mock service 回傳給橋接層的回應資料格式。
type bridgeResponse struct {
	// StatusCode 表示 HTTP 回應狀態碼。
	StatusCode int `json:"status_code"`

	// Headers 保存 HTTP 回應標頭集合。
	Headers map[string]string `json:"headers"`

	// Body 保存 HTTP 回應本文內容。
	Body string `json:"body"`
}

// loadConfig 載入指定路徑的 YAML 設定檔；若未指定路徑，則依執行檔名稱推導預設設定檔名稱。
func loadConfig(configPath string) (*mockServiceConfig, string, error) {
	// 未透過參數指定設定檔時，使用「執行檔名稱.yaml」作為預設設定檔名稱。
	if configPath == "" {
		exePath, err := os.Executable()
		if err != nil {
			exePath = os.Args[0]
		}
		exeBase := filepath.Base(exePath)
		configPath = strings.TrimSuffix(exeBase, filepath.Ext(exeBase)) + ".yaml"
	}

	var cfg mockServiceConfig

	// 讀取 YAML 設定檔內容。
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, configPath, err
	}

	// 將 YAML 內容反序列化為 mockServiceConfig。
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

	// 註冊命令列參數。
	flag.StringVar(&configPath, "c", "", "yaml config file")
	flag.StringVar(&logPath, "log", "", "log file path")
	flag.BoolVar(&noOutput, "noout", false, "suppress console output")
	flag.Parse()

	// 若指定日誌檔路徑，則開啟或建立檔案並以附加模式寫入。
	if logPath != "" {
		f, err := os.OpenFile(logPath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "無法開啟日誌檔案: %v\n", err)
			os.Exit(1)
		}
		logFile = f
		defer logFile.Close()
	}

	// 載入 mock service 設定檔。
	cfg, resolvedPath, err := loadConfig(configPath)
	if err != nil {
		logMockError("載入設定檔失敗: %v (path: %s)", err, resolvedPath)
		return
	}

	logMock("設定檔: %s", resolvedPath)
	logMock("NATS 伺服器: %s:%d", cfg.NatsConfig.NatsServerHost, cfg.NatsConfig.NatsServerPort)
	logMock("共載入 %d 條路由", len(cfg.Routes))

	// 建立 NATS 用戶端並確認連線狀態。
	natsClient := nyanats.NewC(cfg.NatsConfig, natsLogger())
	if err := natsClient.Error(); err != nil {
		logMockError("NATS 連線失敗: %v", err)
		return
	}

	// 依設定檔中的每條路由建立對應的 NATS 訂閱。
	for _, route := range cfg.Routes {
		subject := route.NatsSubject
		httpPath := route.Path

		err := natsClient.Subscribe(subject, func(m string) string {
			logMockRequest("===== 收到請求 =====")
			logMockRequest("NATS Subject : %s", subject)
			logMockRequest("HTTP Path    : %s", httpPath)
			logMockRequest("原始訊息      : %s", m)

			var req bridgeRequest

			// 將橋接層傳入的 JSON 訊息解析為 bridgeRequest。
			if err := json.Unmarshal([]byte(m), &req); err != nil {
				logMockError("解析失敗      : %v", err)
				return `{"status_code":400,"body":"Invalid request JSON"}`
			}

			// 輸出基本請求資訊，方便觀察橋接層傳入內容。
			logMockRequest("HTTP Method  : %s", req.Method)
			logMockRequest("HTTP Path    : %s", req.Path)
			logMockRequest("Remote Addr  : %s", req.RemoteAddr)
			logMockRequest("IP           : %s", req.IP)

			// 將 Headers map 格式化為可讀的單行文字。
			headerLines := make([]string, 0, len(req.Headers))
			for k, v := range req.Headers {
				headerLines = append(headerLines, fmt.Sprintf("%s: %s", k, v))
			}
			if len(headerLines) > 0 {
				logMockRequest("Headers      : %s", strings.Join(headerLines, ", "))
			}

			// 將 Cookies map 格式化為可讀的單行文字。
			cookieLines := make([]string, 0, len(req.Cookies))
			for k, v := range req.Cookies {
				cookieLines = append(cookieLines, fmt.Sprintf("%s: %s", k, v))
			}
			if len(cookieLines) > 0 {
				logMockRequest("Cookies      : %s", strings.Join(cookieLines, ", "))
			}

			// 將 Params map 格式化為可讀的單行文字。
			paramLines := make([]string, 0, len(req.Params))
			for k, v := range req.Params {
				paramLines = append(paramLines, fmt.Sprintf("%s = %s", k, v))
			}
			if len(paramLines) > 0 {
				logMockRequest("Params       : %s", strings.Join(paramLines, ", "))
			}
			logMockRequest("Body         : %s", req.Body)

			// 建立 mock HTTP 回應資料，並將原始 method 與 path 回填到 echo 欄位。
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

			// 將 bridgeResponse 序列化為 JSON 字串後回傳給橋接層。
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

	// 監聽系統中斷與終止訊號，用於觸發優雅關閉流程。
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	// 收到退出訊號後，取消所有訂閱並關閉 NATS 用戶端連線。
	logMock("正在關閉...")
	if err := natsClient.UnsubscribeAll(); err != nil {
		logMockError("UnsubscribeAll 錯誤: %v", err)
	}
	natsClient.Close()
	logMock("已關閉")
}
