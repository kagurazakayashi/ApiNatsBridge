package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kagurazakayashi/libNyaruko_Go/nyalog"
)

// logLevel 定義目前應用程式使用的全域日誌等級。
// 預設值為 Debug；若設定檔未啟用 Debug 模式，初始化時會降為 Info。
// 此變數會影響 stdoutLog 輸出時的最低可見日誌等級。
var logLevel = nyalog.Debug

// logConfig 保存橋接服務的全域日誌設定。
// 若為 nil，代表尚未載入外部設定，日誌系統會採用預設輸出行為。
var logConfig *BridgeLogConfig

// logTimezone 保存日誌時間戳使用的時區字串。
// 可接受 IANA 時區名稱，例如 "Asia/Taipei"，或 -12 到 12 的整數 UTC 偏移。
// 實際使用時會透過 resolveTimeLocation 轉換為 *time.Location。
var logTimezone string

// fileMu 用於保護多個 goroutine 同時寫入日誌檔案時的同步安全。
// 所有檔案寫入操作都應透過此 mutex 序列化，避免多行日誌內容交錯。
var fileMu sync.Mutex

// InitLogConfig 初始化日誌設定。
//
// 此函式會設定全域日誌設定、時區、輸出等級，並依照設定決定是否清空既有日誌檔案。
// 若 cfg 為 nil，則保留預設輸出行為並直接返回。
// 若 cfg.Debug 為 false，會將全域日誌等級切換為 Info。
// 若 cfg.Overwrite 為 true，會在啟動時清空設定中指定的日誌檔案。
//
// 參數：
//   - cfg：橋接服務日誌設定。
//   - timezone：日誌時間戳使用的時區。
func InitLogConfig(cfg *BridgeLogConfig, timezone string) {
	logConfig = cfg
	logTimezone = timezone
	if cfg == nil {
		return
	}
	if !cfg.Debug {
		logLevel = nyalog.Info
	}
	loc := resolveTimeLocation(timezone)
	nyalog.SetTimeZone(loc)
	if cfg.Overwrite {
		truncateLogFiles(cfg)
	}
}

// truncateLogFiles 清空設定中指定的所有日誌檔案。
//
// 此函式會依序處理 main、bridge、HTTP、NATS、HTTP 統計與 module 日誌檔案。
// 若檔案路徑為空字串，則略過該項目。
// 若目標目錄不存在，會自動建立。
// 寫入 nil 內容會將既有檔案截斷為空檔案。
//
// 參數：
//   - cfg：包含各類日誌檔案路徑的橋接服務日誌設定。
func truncateLogFiles(cfg *BridgeLogConfig) {
	filePaths := []string{
		cfg.Files.Main,
		cfg.Files.Bridge,
		cfg.Files.HTTP,
		cfg.Files.NATS,
		cfg.Files.HTTPStat,
		cfg.Files.Module,
	}
	for _, p := range filePaths {
		if p == "" {
			continue
		}
		dir := filepath.Dir(p)
		os.MkdirAll(dir, 0755)
		os.WriteFile(p, nil, 0644)
	}
}

// resolveTimeLocation 解析時區字串並回傳對應的 time.Location。
//
// 支援以下格式：
//   - 空字串：回傳 UTC。
//   - IANA 時區名稱，例如 "Asia/Taipei"。
//   - -12 到 12 的整數，代表 UTC 小時偏移。
//
// 若解析失敗，會回退為 UTC，以確保日誌時間戳仍可正常產生。
//
// 參數：
//   - tz：時區字串。
//
// 回傳：
//   - *time.Location：解析後的時區位置。
func resolveTimeLocation(tz string) *time.Location {
	if tz == "" {
		return time.UTC
	}
	if loc, err := time.LoadLocation(tz); err == nil {
		return loc
	}
	if n, err := strconv.Atoi(tz); err == nil && n >= -12 && n <= 12 {
		return time.FixedZone("", n*3600)
	}
	return time.UTC
}

// stdoutLog 將日誌輸出到標準錯誤輸出。
//
// 若設定允許彩色輸出，會使用 nyalog 的彩色輸出函式。
// 若停用彩色輸出，則手動組合時間戳、等級與訊息內容後輸出。
// 當 logConfig 為 nil、Color 為 nil，或 Color 指向 true 時，皆視為啟用彩色輸出。
//
// 參數：
//   - level：目前允許輸出的最低日誌等級。
//   - nowLevel：本次日誌訊息的等級。
//   - color：彩色輸出時使用的主控台顏色。
//   - obj：欲輸出的任意訊息內容。
func stdoutLog(level nyalog.LogLevel, nowLevel nyalog.LogLevel, color nyalog.ConsoleColor, obj ...interface{}) {
	useColor := logConfig == nil || logConfig.Color == nil || *logConfig.Color
	if useColor {
		nyalog.LogCC(level, nowLevel, color, obj...)
	} else {
		loc := resolveTimeLocation(logTimezone)
		ts := time.Now().In(loc).Format("2006-01-02 15:04:05")
		levelChar := nowLevel.String()
		prefix := fmt.Sprintf("[%s %s]", levelChar, ts)
		parts := []string{prefix}
		for _, o := range obj {
			parts = append(parts, fmt.Sprint(o))
		}
		fmt.Fprintln(os.Stderr, strings.Join(parts, " "))
	}
}

// writeToFile 將單行日誌寫入指定檔案。
//
// 若 filePath 為空，函式會直接返回。
// 寫入前會確保目標目錄存在，並使用全域 mutex 避免並行寫入造成內容交錯。
// 檔案會以讀寫、建立與追加模式開啟；若開啟失敗，會直接略過本次檔案寫入。
// 寫入格式為：
//
//	YYYY-MM-DD HH:MM:SS [PREFIX] message
//
// 參數：
//   - filePath：日誌檔案路徑。
//   - color：保留參數，目前檔案寫入未使用顏色。
//   - prefix：日誌前綴，例如 "[MAIN]"、"[HTTP]"。
//   - msg：日誌訊息內容。
func writeToFile(filePath string, color nyalog.ConsoleColor, prefix string, msg string) {
	if filePath == "" {
		return
	}
	fileMu.Lock()
	defer fileMu.Unlock()

	dir := filepath.Dir(filePath)
	os.MkdirAll(dir, 0755)

	f, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	loc := resolveTimeLocation(logTimezone)
	ts := time.Now().In(loc).Format("2006-01-02 15:04:05")
	line := fmt.Sprintf("%s %s %s\n", ts, prefix, msg)
	f.WriteString(line)
}

// logMain 輸出主流程模組的資訊日誌。
//
// 此函式適合記錄應用程式主流程、啟動、初始化與一般執行狀態。
// 日誌會依照設定輸出到標準錯誤輸出，並可同步寫入 main 日誌檔案。
// 若 logConfig 為 nil 或 logConfig.Stdout 為 true，會輸出到標準錯誤輸出。
//
// 參數：
//   - format：fmt.Sprintf 相容的格式字串。
//   - a：格式化參數。
func logMain(format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	if logConfig == nil || logConfig.Stdout {
		stdoutLog(logLevel, nyalog.Info, nyalog.Cyan, "[MAIN]", msg)
	}
	if logConfig != nil {
		writeToFile(logConfig.Files.Main, nyalog.Cyan, "[MAIN]", msg)
	}
}

// logError 輸出指定模組的錯誤日誌。
//
// 此函式會依 module 名稱選擇對應的日誌檔案。
// 若 module 不在已知清單中，會回退寫入 main 日誌檔案。
// 錯誤日誌的前綴格式為 [MODULE][ERROR]，便於快速辨識錯誤來源。
//
// 支援的 module 包含：
//   - MAIN
//   - NATS
//   - HTTP
//   - HTTPSTAT
//   - BRIDGE
//   - MODULE
//
// 參數：
//   - module：模組名稱。
//   - format：fmt.Sprintf 相容的格式字串。
//   - a：格式化參數。
func logError(module string, format string, a ...interface{}) {
	prefix := fmt.Sprintf("[%s][ERROR]", module)
	msg := fmt.Sprintf(format, a...)
	if logConfig == nil || logConfig.Stdout {
		stdoutLog(logLevel, nyalog.Error, nyalog.Red, prefix, msg)
	}
	if logConfig != nil {
		var filePath string
		switch module {
		case "MAIN":
			filePath = logConfig.Files.Main
		case "NATS":
			filePath = logConfig.Files.NATS
		case "HTTP":
			filePath = logConfig.Files.HTTP
		case "HTTPSTAT":
			filePath = logConfig.Files.HTTPStat
		case "BRIDGE":
			filePath = logConfig.Files.Bridge
		case "MODULE":
			filePath = logConfig.Files.Module
		default:
			filePath = logConfig.Files.Main
		}
		writeToFile(filePath, nyalog.Red, prefix, msg)
	}
}

// logBridge 輸出橋接流程相關的資訊日誌。
//
// 此函式適合記錄跨服務、跨協定或資料轉送流程中的狀態訊息。
// 日誌會依照設定輸出到標準錯誤輸出，並可同步寫入 bridge 日誌檔案。
//
// 參數：
//   - format：fmt.Sprintf 相容的格式字串。
//   - a：格式化參數。
func logBridge(format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	if logConfig == nil || logConfig.Stdout {
		stdoutLog(logLevel, nyalog.Info, nyalog.Yellow, "[BRIDGE]", msg)
	}
	if logConfig != nil {
		writeToFile(logConfig.Files.Bridge, nyalog.Yellow, "[BRIDGE]", msg)
	}
}

// logHTTP 輸出 HTTP 模組相關的資訊日誌。
//
// 此函式適合記錄 HTTP 服務啟動、請求處理、路由或一般 HTTP 模組狀態。
// 日誌會依照設定輸出到標準錯誤輸出，並可同步寫入 HTTP 日誌檔案。
//
// 參數：
//   - format：fmt.Sprintf 相容的格式字串。
//   - a：格式化參數。
func logHTTP(format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	if logConfig == nil || logConfig.Stdout {
		stdoutLog(logLevel, nyalog.Info, nyalog.Blue, "[HTTP]", msg)
	}
	if logConfig != nil {
		writeToFile(logConfig.Files.HTTP, nyalog.Blue, "[HTTP]", msg)
	}
}

// logHTTPStat 輸出 HTTP 統計相關的日誌。
//
// 此類日誌通常用於記錄 HTTP 請求統計、狀態或觀測資訊。
// 日誌會依照設定輸出到標準錯誤輸出，並可同步寫入 HTTPStat 日誌檔案。
// 此函式使用 nyalog.OK 等級，適合呈現統計或觀測結果類型的正常事件。
//
// 參數：
//   - format：fmt.Sprintf 相容的格式字串。
//   - a：格式化參數。
func logHTTPStat(format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	if logConfig == nil || logConfig.Stdout {
		stdoutLog(logLevel, nyalog.OK, nyalog.Purple, "[HTTPSTAT]", msg)
	}
	if logConfig != nil {
		writeToFile(logConfig.Files.HTTPStat, nyalog.Purple, "[HTTPSTAT]", msg)
	}
}

// logModule 輸出一般模組相關的資訊日誌。
//
// 適合用於非 MAIN、BRIDGE、HTTP、NATS 等專屬分類的模組訊息。
// 日誌會依照設定輸出到標準錯誤輸出，並可同步寫入 module 日誌檔案。
// 若後續模組尚未建立獨立日誌分類，可先統一透過此函式輸出。
//
// 參數：
//   - format：fmt.Sprintf 相容的格式字串。
//   - a：格式化參數。
func logModule(format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	if logConfig == nil || logConfig.Stdout {
		stdoutLog(logLevel, nyalog.Info, nyalog.Cyan, "[MODULE]", msg)
	}
	if logConfig != nil {
		writeToFile(logConfig.Files.Module, nyalog.Cyan, "[MODULE]", msg)
	}
}
