// Package src 提供設定檔解析與 CLI 參數處理相關的型別定義與函式。
//
// 此檔案定義 ApiNatsBridge 使用的全部設定結構，包括 HTTP 伺服器、
// NATS 用戶端、橋接層、路由規則與欄位長度限制，並提供設定檔載入與逾時計算等輔助方法。
package src

import (
	"flag"          // 匯入命令列參數解析套件，用於讀取 -c、-v 等啟動參數。
	"os"            // 匯入作業系統介面套件，用於取得執行檔路徑與讀取檔案。
	"path/filepath" // 匯入檔案路徑處理套件，用於解析執行檔檔名與副檔名。
	"strings"       // 匯入字串處理套件，用於移除執行檔副檔名。
	"time"          // 匯入時間處理套件，用於將秒數轉換為 time.Duration。

	"github.com/kagurazakayashi/libNyaruko_Go/nyaapiserver" // 匯入 HTTP API 伺服器設定與相關型別。
	"github.com/kagurazakayashi/libNyaruko_Go/nyanats"      // 匯入 NATS 用戶端設定與相關型別。
	"gopkg.in/yaml.v3"                                      // 匯入 YAML 解析套件，用於讀取設定檔內容。
)

// defaultTimeoutSeconds 定義路由未指定逾時時間時使用的預設秒數。
//
// 此常數會在 RouteConfig.Timeout 小於等於 0 時作為 fallback 使用，
// 避免路由未設定 timeout 時造成無限制等待。
const defaultTimeoutSeconds = 30

// defaultTokenTimeout 定義令牌驗證 NATS 請求的預設逾時秒數。
const defaultTokenTimeout = 5

// defaultTokenNatsSubject 定義令牌驗證使用的預設 NATS 主題。
const defaultTokenNatsSubject = "auth.token.verify"

// defaultTokenHeaderName 定義令牌所在的預設 HTTP 標頭名稱。
const defaultTokenHeaderName = "Authorization"

// defaultTokenMinLength 定義令牌的最小長度預設值。
const defaultTokenMinLength = 10

// defaultTokenMaxLength 定義令牌的最大長度預設值。
const defaultTokenMaxLength = 4096

// defaultTokenTagSeparator 定義 tag 與回應內容之間的分隔符預設值。
const defaultTokenTagSeparator = "|"

// defaultTokenSuccessValue 定義令牌驗證成功時的回傳值預設值。
const defaultTokenSuccessValue = "0"

// defaultTokenCacheMaxEntries 定義令牌驗證快取的最大筆數預設值。
//
// 當快取筆數達到此上限時，會清空整個快取後再重新填入。
const defaultTokenCacheMaxEntries = 1000

// defaultTokenMaxConcurrent 定義令牌驗證的預設最大並行數。
//
// 超過此數量時，新的驗證請求會立即收到 HTTP 503 回應。
const defaultTokenMaxConcurrent = 256

// TokenConfig 定義令牌驗證相關設定。
//
// 當此設定存在時，ApiNatsBridge 會在處理每個 HTTP 請求時，
// 從指定的 HTTP 標頭中提取令牌，並透過 NATS Request 模式
// 發送至令牌驗證主題進行驗證。
// 路徑白名單內的請求將略過令牌檢查。
type TokenConfig struct {
	// NatsSubject 定義令牌驗證的 NATS 主題名稱（預設：auth.token.verify）
	NatsSubject string `json:"nats_subject,omitempty" yaml:"nats_subject,omitempty"`

	// PathWhitelist 定義不需要檢查令牌的路徑白名單
	PathWhitelist []string `json:"path_whitelist,omitempty" yaml:"path_whitelist,omitempty"`

	// HeaderName 定義令牌所在的 HTTP 標頭名稱（預設：Authorization）
	HeaderName string `json:"header_name,omitempty" yaml:"header_name,omitempty"`

	// MinLength 定義令牌的最小長度（預設：10）
	MinLength int `json:"min_length,omitempty" yaml:"min_length,omitempty"`

	// MaxLength 定義令牌的最大長度（預設：4096）
	MaxLength int `json:"max_length,omitempty" yaml:"max_length,omitempty"`

	// TagSeparator 定義 tag 部分的分隔符（預設：|）
	//
	// 此字元用於分隔請求 tag 與回傳內容。
	// 發送到 NATS 的格式為 "tag|2|令牌"，
	// 回覆格式為 "tag|0" 或 "tag|{JSON}"。
	TagSeparator string `json:"tag_separator,omitempty" yaml:"tag_separator,omitempty"`

	// SuccessValue 定義令牌驗證成功時的回傳值（預設：0）
	//
	// 與 tag 結合後，預期的回覆格式為 "tag|0"。
	// 若回覆值不等於此值，則視為令牌驗證失敗。
	SuccessValue string `json:"success_value,omitempty" yaml:"success_value,omitempty"`

	// Timeout 定義等待令牌驗證回覆的超時秒數（預設：5）
	Timeout int `json:"timeout,omitempty" yaml:"timeout,omitempty"`

	// CacheMaxEntries 定義令牌驗證快取的最大筆數（預設：1000）
	//
	// 快取用於減少對 NATS 驗證服務的重複請求。
	// 快取的 key 為令牌完整字串，value 為驗證結果。
	// 當快取筆數達到此上限時，會清空快取並重新開始填入。
	// 設為 0 或負數時使用預設值。
	CacheMaxEntries int `json:"cache_max_entries,omitempty" yaml:"cache_max_entries,omitempty"`

	// MaxConcurrent 定義令牌驗證的最大並行請求數（預設：256）
	//
	// 當同時進行中的驗證請求達到此數量時，新的驗證請求會立即收到 HTTP 503 回應。
	// 接近上限（≥90%）時會在日誌中輸出警告。
	// 設為 0 或負數時使用預設值。
	MaxConcurrent int `json:"max_concurrent,omitempty" yaml:"max_concurrent,omitempty"`

	// PasetoSecretKey 定義用於本地解密 PASETO 令牌的對稱金鑰（選填）
	//
	// 支援兩種格式：
	//   A. 單一十六進位字串（向後相容）：
	//        paseto_secret_key: "404142..."
	//
	//   B. 金鑰輪替字典（推薦）：
	//        paseto_secret_key:
	//          1748736000: "404142..."
	//          1779840000: "606162..."
	//
	// 若未設定，ApiNatsBridge 完全依賴 UserValidator 的驗證回覆來取得令牌 claims。
	// 若已設定，可用於本地解密令牌以提取自訂 claims（如 uuid），
	// 減少對 UserValidator 驗證回覆格式的依賴。
	PasetoSecretKey interface{} `json:"paseto_secret_key,omitempty" yaml:"paseto_secret_key,omitempty"`
}

// EffectiveNatsSubject 回傳實際使用的 NATS 主題名稱。
//
// 若未設定則使用預設值 defaultTokenNatsSubject。
func (t *TokenConfig) EffectiveNatsSubject() string {
	if t.NatsSubject == "" {
		return defaultTokenNatsSubject
	}
	return t.NatsSubject
}

// EffectiveHeaderName 回傳實際使用的 HTTP 標頭名稱。
//
// 若未設定則使用預設值 defaultTokenHeaderName。
func (t *TokenConfig) EffectiveHeaderName() string {
	if t.HeaderName == "" {
		return defaultTokenHeaderName
	}
	return t.HeaderName
}

// EffectiveMinLength 回傳實際使用的令牌最小長度。
//
// 若未設定則使用預設值 defaultTokenMinLength。
func (t *TokenConfig) EffectiveMinLength() int {
	if t.MinLength <= 0 {
		return defaultTokenMinLength
	}
	return t.MinLength
}

// EffectiveMaxLength 回傳實際使用的令牌最大長度。
//
// 若未設定則使用預設值 defaultTokenMaxLength。
func (t *TokenConfig) EffectiveMaxLength() int {
	if t.MaxLength <= 0 {
		return defaultTokenMaxLength
	}
	return t.MaxLength
}

// EffectiveTagSeparator 回傳實際使用的 tag 分隔符。
//
// 若未設定則使用預設值 defaultTokenTagSeparator。
func (t *TokenConfig) EffectiveTagSeparator() string {
	if t.TagSeparator == "" {
		return defaultTokenTagSeparator
	}
	return t.TagSeparator
}

// EffectiveSuccessValue 回傳實際使用的成功回傳值。
//
// 若未設定則使用預設值 defaultTokenSuccessValue。
func (t *TokenConfig) EffectiveSuccessValue() string {
	if t.SuccessValue == "" {
		return defaultTokenSuccessValue
	}
	return t.SuccessValue
}

// EffectiveTimeout 回傳實際使用的令牌驗證逾時秒數。
//
// 若未設定則使用預設值 defaultTokenTimeout。
func (t *TokenConfig) EffectiveTimeout() int {
	if t.Timeout <= 0 {
		return defaultTokenTimeout
	}
	return t.Timeout
}

// EffectiveCacheMaxEntries 回傳實際使用的令牌驗證快取最大筆數。
//
// 若未設定則使用預設值 defaultTokenCacheMaxEntries。
func (t *TokenConfig) EffectiveCacheMaxEntries() int {
	if t.CacheMaxEntries <= 0 {
		return defaultTokenCacheMaxEntries
	}
	return t.CacheMaxEntries
}

// EffectiveMaxConcurrent 回傳實際使用的令牌驗證最大並行數。
//
// 若未設定則使用預設值 defaultTokenMaxConcurrent。
func (t *TokenConfig) EffectiveMaxConcurrent() int {
	if t.MaxConcurrent <= 0 {
		return defaultTokenMaxConcurrent
	}
	return t.MaxConcurrent
}

// ScalarLimitRule 定義純量欄位的長度限制，例如 path 或 body。
//
// 純量欄位通常代表單一字串或單一資料區塊，
// 例如 HTTP request path 或 request/response body。
type ScalarLimitRule struct {
	// MaxLength 表示最大長度，單位為位元組；0 表示不限制。
	MaxLength int `json:"max_length,omitempty" yaml:"max_length,omitempty"`
}

// MapLimitRule 定義鍵值對集合的長度限制，例如 headers、cookies 或 params。
//
// 此規則適用於 map-like 結構，除了限制整體筆數外，
// 也能分別限制 key 與 value 的最大長度。
type MapLimitRule struct {
	// MaxCount 表示最大筆數；0 表示不限制。
	MaxCount int `json:"max_count,omitempty" yaml:"max_count,omitempty"`

	// MaxKeyLen 表示每個鍵的最大長度，單位為位元組；0 表示不限制。
	MaxKeyLen int `json:"max_key_length,omitempty" yaml:"max_key_length,omitempty"`

	// MaxValueLen 表示每個值的最大長度，單位為位元組；0 表示不限制。
	MaxValueLen int `json:"max_value_length,omitempty" yaml:"max_value_length,omitempty"`
}

// LimitRule 定義 HTTP 請求或回應中各欄位的長度限制規則。
//
// 此結構可用於全域層級，也可用於單一路由層級。
// 路由層級的設定通常會覆蓋或補充全域設定。
type LimitRule struct {
	// Path 定義請求路徑的長度限制。
	Path ScalarLimitRule `json:"path,omitempty" yaml:"path,omitempty"`

	// Headers 定義 HTTP 標頭集合的數量與鍵值長度限制。
	Headers MapLimitRule `json:"headers,omitempty" yaml:"headers,omitempty"`

	// Cookies 定義 Cookie 集合的數量與鍵值長度限制。
	Cookies MapLimitRule `json:"cookies,omitempty" yaml:"cookies,omitempty"`

	// Params 定義查詢參數集合的數量與鍵值長度限制。
	Params MapLimitRule `json:"params,omitempty" yaml:"params,omitempty"`

	// Body 定義請求或回應本文的長度限制。
	Body ScalarLimitRule `json:"body,omitempty" yaml:"body,omitempty"`
}

// BridgeLogFilesConfig 定義各模組日誌的獨立檔案路徑。
//
// 每個欄位對應一類日誌輸出，允許依模組拆分檔案，
// 方便後續追蹤主流程、HTTP、NATS 與模組行為。
type BridgeLogFilesConfig struct {
	// Main 表示主流程日誌檔案路徑。
	Main string `json:"main,omitempty" yaml:"main,omitempty"`

	// Bridge 表示橋接路由日誌檔案路徑。
	Bridge string `json:"bridge,omitempty" yaml:"bridge,omitempty"`

	// HTTP 表示 HTTP 請求日誌檔案路徑。
	HTTP string `json:"http,omitempty" yaml:"http,omitempty"`

	// NATS 表示 NATS 用戶端日誌檔案路徑。
	NATS string `json:"nats,omitempty" yaml:"nats,omitempty"`

	// Status 表示狀態統計日誌檔案路徑。
	Status string `json:"status,omitempty" yaml:"status,omitempty"`

	// Module 表示通用模組日誌檔案路徑，例如 /ping 等模組輸出。
	Module string `json:"module,omitempty" yaml:"module,omitempty"`
}

// BridgeLogConfig 定義橋接層日誌相關設定。
//
// 此設定可控制是否輸出到終端機、是否啟用 debug 模式、
// 是否覆寫既有日誌檔，以及是否使用彩色輸出。
type BridgeLogConfig struct {
	// Stdout 表示是否輸出到控制台 stdout 或 stderr；設為 false 時僅寫入檔案。
	Stdout bool `json:"stdout" yaml:"stdout"`

	// Debug 表示要啟用的除錯模式清單；可選值：HTTP、NATS、LIMIT。
	// 設定後即啟用除錯等級日誌，並依模式輸出對應的詳細通訊內容。
	// 留空陣列則僅輸出 Info 及以上等級。
	Debug []string `json:"debug" yaml:"debug"`

	// Overwrite 表示是否以覆蓋模式寫入日誌檔案；設為 true 時會在啟動時清空既有日誌。
	Overwrite bool `json:"overwrite,omitempty" yaml:"overwrite,omitempty"`

	// Color 表示是否使用彩色控制台輸出；nil 或 true 代表啟用彩色，false 代表純文字輸出。
	Color *bool `json:"color,omitempty" yaml:"color,omitempty"`

	// Files 定義各模組的獨立日誌檔案路徑。
	Files BridgeLogFilesConfig `json:"files,omitempty" yaml:"files,omitempty"`

	// StatusIntervalSeconds 定義 STATUS 狀態日誌的輸出間隔秒數；0 或未設定時預設 60 秒。
	StatusIntervalSeconds int `json:"status_interval_seconds,omitempty" yaml:"status_interval_seconds,omitempty"`
}

// BridgeLanguageConfig 定義各模組使用的語言設定。
//
// 支援的語言代碼包含：en、zh、zh_Hant、ja。
// 未設定時使用預設值：Log="zh_Hant"、HTTP="en"、CLI="zh_Hant"。
type BridgeLanguageConfig struct {
	// Log 定義日誌輸出使用的語言。
	Log string `json:"log,omitempty" yaml:"log,omitempty"`

	// HTTP 定義 HTTP 回應錯誤訊息使用的語言。
	HTTP string `json:"http,omitempty" yaml:"http,omitempty"`

	// CLI 定義命令列相關訊息使用的語言。
	CLI string `json:"cli,omitempty" yaml:"cli,omitempty"`
}

// BridgeConfig 定義 HTTP API 與 NATS 之間的橋接層設定。
//
// 此結構集中保存橋接服務的全域行為，包含日誌、時區、
// CDN 真實 IP 標頭、請求與回應限制與錯誤細節白名單。
type BridgeConfig struct {
	// Language 定義各模組使用的語言設定；未提供時使用程式預設值。
	Language *BridgeLanguageConfig `json:"language,omitempty" yaml:"language,omitempty"`

	// Log 定義橋接層日誌相關設定；未提供時使用程式預設值。
	Log *BridgeLogConfig `json:"log,omitempty" yaml:"log,omitempty"`

	// Timezone 定義日誌或時間處理使用的時區，支援 IANA 時區名稱，例如 Asia/Taipei，或小時偏移，例如 8、-5。
	Timezone string `json:"timezone,omitempty" yaml:"timezone,omitempty"`

	// TimeFormat 定義日誌時間日期的顯示格式。
	//
	// 預設值為 "YYYY-MM-DD HH:mm:ss"（Go 格式字串為 "2006-01-02 15:04:05"）。
	// 若設為空字串 ""，控制台輸出將不顯示時間日期，但日誌檔案仍會依照預設格式記錄時間日期。
	// 可被單一路由設定中的同名欄位覆蓋。
	TimeFormat *string `json:"time_format,omitempty" yaml:"time_format,omitempty"`

	// CdnHeader 定義 CDN 或反向代理傳遞真實用戶端 IP 的標頭清單。
	CdnHeader []string `json:"cdnheader" yaml:"cdnheader"`

	// Limits 定義全域請求欄位長度限制規則；可被單一路由設定覆蓋。
	Limits *LimitRule `json:"limits,omitempty" yaml:"limits,omitempty"`

	// ResponseLimits 定義全域回應欄位長度限制規則；可被單一路由設定覆蓋。
	ResponseLimits *LimitRule `json:"response_limits,omitempty" yaml:"response_limits,omitempty"`

	// ErrorDetailIPs 定義允許接收錯誤詳細資訊的 IP 白名單，通常用於開發或除錯環境。
	ErrorDetailIPs []string `json:"error_detail_ips,omitempty" yaml:"error_detail_ips,omitempty"`

	// HTTPCodeKey 定義微服務回傳 JSON 中用來表示 HTTP 狀態碼的全域預設鍵名；可被單一路由設定覆蓋。
	HTTPCodeKey string `json:"http_code_key,omitempty" yaml:"http_code_key,omitempty"`

	// ErrorCodeKey 定義微服務回傳 JSON 中用來表示錯誤碼的全域預設鍵名；可被單一路由設定覆蓋。
	ErrorCodeKey string `json:"error_code_key,omitempty" yaml:"error_code_key,omitempty"`

	// ResponseSchemaBody 定義回應本文的 JSON Schema 驗證全域預設規則；可被單一路由設定覆蓋。
	ResponseSchemaBody map[string]interface{} `json:"response_schema_body,omitempty" yaml:"response_schema_body,omitempty"`

	// ResponseErrorSchemaBody 定義回應本文發生錯誤時的 JSON Schema 驗證全域預設規則；可被單一路由設定覆蓋。
	ResponseErrorSchemaBody map[string]interface{} `json:"response_error_schema_body,omitempty" yaml:"response_error_schema_body,omitempty"`

	// ErrorInfoShow 定義微服務錯誤資訊的顯示模式全域預設值；可被單一路由設定覆蓋。
	//
	// 有效值：
	//   0 = 不在日誌檔案中記錄（僅保留基本的 [HTTP] 日誌）。
	//   1 = 在日誌檔案中記錄並輸出。
	//   2 = 在日誌檔案中記錄並輸出，並且將內容回傳給白名單 IP 使用者。
	//   3 = 在日誌檔案中記錄並輸出，並且將內容回傳給所有使用者。
	//   4 = 不記錄日誌檔案和輸出（僅保留基本的 [HTTP] 日誌），將內容回傳給白名單 IP 使用者。
	//   5 = 不記錄日誌檔案和輸出（僅保留基本的 [HTTP] 日誌），將內容回傳給所有使用者。
	ErrorInfoShow *int `json:"error_info_show,omitempty" yaml:"error_info_show,omitempty"`

	// Token 定義令牌驗證相關設定；nil 表示不啟用令牌驗證。
	Token *TokenConfig `json:"token,omitempty" yaml:"token,omitempty"`

	// MaxConcurrent 定義全域轉發 NATS 的最大並行請求數（預設：0 = 不限制）
	//
	// 當同時進行中的 NATS 轉發請求達到此數量時，新請求會收到 HTTP 503。
	// 接近上限（≥90%）時會在日誌中輸出警告。
	MaxConcurrent int `json:"max_concurrent,omitempty" yaml:"max_concurrent,omitempty"`

	// HTTPMaxConcurrent 定義 HTTP 層的最大並行請求處理數（預設：0 = 不限制）
	//
	// 此限制作用於整個 HTTP 請求處理流程，包含令牌驗證與 NATS 轉發。
	// 接近上限（≥90%）時會在日誌中輸出警告。
	HTTPMaxConcurrent int `json:"http_max_concurrent,omitempty" yaml:"http_max_concurrent,omitempty"`

	// ResponseHeaders 定義橋接層全域自訂回應標頭。
	//
	// 這些標頭會附加到所有 HTTP 回應中，可用於設定 CORS 標頭、
	// 安全性標頭或其他自訂回應標頭。
	// 路由層級的 response_headers 會覆蓋全域同名標頭。
	ResponseHeaders map[string]string `json:"response_headers,omitempty" yaml:"response_headers,omitempty"`
}

// RouteConfig 定義單一路由的 HTTP 到 NATS 轉發規則。
//
// 每筆 RouteConfig 描述一條 HTTP 路由如何被驗證、限制，
// 並轉發到指定的 NATS Subject。
type RouteConfig struct {
	// Path 定義要匹配的 HTTP 請求路徑。
	Path string `json:"path" yaml:"path"`

	// NatsSubject 定義此路由轉發至 NATS 時使用的 Subject。
	NatsSubject string `json:"nats_subject" yaml:"nats_subject"`

	// Timeout 定義等待 NATS 回應的逾時秒數；未設定或小於等於 0 時使用預設逾時。
	Timeout int `json:"timeout,omitempty" yaml:"timeout,omitempty"`

	// Methods 定義允許的 HTTP 方法清單；未提供時由路由處理邏輯決定是否限制。
	Methods []string `json:"methods,omitempty" yaml:"methods,omitempty"`

	// ContentType 定義要求的 Content-Type 前綴，例如 application/json。
	ContentType string `json:"content_type,omitempty" yaml:"content_type,omitempty"`

	// SchemaBody 定義請求本文的 JSON Schema 驗證規則。
	SchemaBody map[string]interface{} `json:"schema_body,omitempty" yaml:"schema_body,omitempty"`

	// Limits 定義此路由的請求欄位長度限制規則；會與 bridge.limits 合併並覆蓋對應欄位。
	Limits *LimitRule `json:"limits,omitempty" yaml:"limits,omitempty"`

	// ResponseSchemaBody 定義回應本文的 JSON Schema 驗證規則。
	ResponseSchemaBody map[string]interface{} `json:"response_schema_body,omitempty" yaml:"response_schema_body,omitempty"`

	// ResponseErrorSchemaBody 定義回應本文發生錯誤時的 JSON Schema 驗證規則。
	//
	// 當回應狀態碼 >=400 時，若此欄位有設定，則優先使用此 Schema 驗證回應本文；
	// 若未設定則沿用 ResponseSchemaBody 的規則。
	ResponseErrorSchemaBody map[string]interface{} `json:"response_error_schema_body,omitempty" yaml:"response_error_schema_body,omitempty"`

	// ResponseLimits 定義此路由的回應欄位長度限制規則；會與 bridge.response_limits 合併並覆蓋對應欄位。
	ResponseLimits *LimitRule `json:"response_limits,omitempty" yaml:"response_limits,omitempty"`

	// ReturnFields 定義轉發到 NATS 時要包含的欄位清單。
	//
	// 可選值包含：
	//   - method
	//   - path
	//   - headers
	//   - cookies
	//   - remote_addr
	//   - ip
	//   - params
	//   - body
	//
	// 留空或未提供時，預設返回所有欄位。
	ReturnFields []string `json:"return_fields,omitempty" yaml:"return_fields,omitempty"`

	// HTTPCodeKey 定義微服務回傳 JSON 中用來表示 HTTP 狀態碼的鍵名。
	//
	// 橋接層會檢查微服務回應 JSON 中是否存在此鍵，以判斷是否為 BridgeResponse 格式。
	// 若未指定（空字串），預設狀態碼為 200。
	// 若指定此鍵，回應中的該鍵值必須為 100-599 之間的整數，且回傳給用戶端時會移除此鍵。
	HTTPCodeKey string `json:"http_code_key,omitempty" yaml:"http_code_key,omitempty"`

	// ErrorCodeKey 定義微服務回傳 JSON 中用來表示錯誤碼的鍵名。
	//
	// 當橋接層在微服務回應 JSON 中檢測到存在此鍵時，
	// 會觸發使用 response_error_schema_body 進行回應驗證。
	// 預設值為空（不啟用），值必須為 int32 範圍內的數值。
	ErrorCodeKey string `json:"error_code_key,omitempty" yaml:"error_code_key,omitempty"`

	// ErrorInfoShow 定義微服務錯誤資訊的顯示模式；未設定時使用 bridge 層全域預設值。
	//
	// 有效值：
	//   0 = 不在日誌檔案中記錄（僅保留基本的 [HTTP] 日誌）。
	//   1 = 在日誌檔案中記錄並輸出。
	//   2 = 在日誌檔案中記錄並輸出，並且將內容回傳給白名單 IP 使用者。
	//   3 = 在日誌檔案中記錄並輸出，並且將內容回傳給所有使用者。
	//   4 = 不記錄日誌檔案和輸出（僅保留基本的 [HTTP] 日誌），將內容回傳給白名單 IP 使用者。
	//   5 = 不記錄日誌檔案和輸出（僅保留基本的 [HTTP] 日誌），將內容回傳給所有使用者。
	ErrorInfoShow *int `json:"error_info_show,omitempty" yaml:"error_info_show,omitempty"`

	// TimeFormat 定義此路由的日誌時間日期顯示格式。
	//
	// 若未設定（nil），則使用 bridge 層 TimeFormat 設定。
	// 若 bridge 層亦未設定，則使用程式預設值 "YYYY-MM-DD HH:mm:ss"。
	// 若設為空字串 ""，控制台輸出將不顯示時間日期，但日誌檔案仍會依照預設格式記錄時間日期。
	TimeFormat *string `json:"time_format,omitempty" yaml:"time_format,omitempty"`

	// MaxConcurrent 定義此路由的最大並行 NATS 轉發請求數（預設：0 = 不限制）。
	//
	// 此限制獨立於 bridge.max_concurrent 全域轉發限制，
	// 用於保護特定後端微服務不被過量請求淹沒。
	// 超限時回傳 HTTP 503。
	MaxConcurrent int `json:"max_concurrent,omitempty" yaml:"max_concurrent,omitempty"`

	// ResponseHeaders 定義此路由的自訂回應標頭。
	//
	// 這些標頭會附加到此路由的所有 HTTP 回應中。
	// 若與 bridge.response_headers 中的同名標頭衝突，路由層級設定優先。
	ResponseHeaders map[string]string `json:"response_headers,omitempty" yaml:"response_headers,omitempty"`

	// TokenFields 定義要從令牌驗證回覆中提取並轉發給下游微服務的欄位清單（向後相容）。
	//
	// 新版設定建議改用 return_fields 統一管理：
	// 將令牌欄位名稱直接加入 return_fields 即可，不屬於 BridgeRequest
	// 已知欄位（method、path、headers、cookies、remote_addr、ip、params、body）
	// 的名稱會自動從令牌 claims 中解析。
	//
	// 若仍使用此欄位，提取後的欄位會以頂層欄位形式注入轉發資料。
	//
	// 範例（新版 return_fields 方式）：
	//   return_fields: ["method", "body", "sub", "username"]
	//   轉發的資料將包含：
	//   { "method": "...", "body": "...", "sub": "admin", "username": "admin" }
	//
	// 支援的欄位取決於 UserValidator 的層級 2 回覆內容，
	// 包含標準 claims（username、app、sub、iss、iat、nbf、exp、jti）
	// 以及透過 custom_claims 寫入的自訂 claims（如 uuid）。
	// 留空或未設定則不注入令牌欄位。
	TokenFields []string `json:"token_fields,omitempty" yaml:"token_fields,omitempty"`
}

// TimeoutDuration 回傳此路由等待 NATS 回應的逾時時間。
//
// 當 Timeout 小於等於 0 時，會使用 defaultTimeoutSeconds 作為預設逾時秒數。
func (r *RouteConfig) TimeoutDuration() time.Duration {
	if r.Timeout <= 0 { // 若未設定 timeout 或設定為非正數，改用全域預設秒數。
		return defaultTimeoutSeconds * time.Second // 將預設秒數轉換為 time.Duration 後回傳。
	}
	return time.Duration(r.Timeout) * time.Second // 將路由設定的秒數轉換為 time.Duration 後回傳。
}

// AllowedMethods 回傳此路由允許的 HTTP 方法清單。
//
// 此方法目前直接回傳 Methods 欄位，讓路由處理邏輯可統一透過方法取得允許清單。
func (r *RouteConfig) AllowedMethods() []string {
	return r.Methods // 回傳此路由設定中允許的 HTTP methods。
}

// ApiNatsBridgeConfig 定義 ApiNatsBridge 完整的執行設定。
//
// 此結構對應 YAML 設定檔的最外層內容，
// 將 HTTP API 伺服器、NATS、橋接層與路由規則集中管理。
type ApiNatsBridgeConfig struct {
	// HttpAPIServerConfig 定義 HTTP API 伺服器設定。
	HttpAPIServerConfig nyaapiserver.HttpAPIServerConfig `json:"httpapiserver_config" yaml:"httpapiserver_config"`

	// NatsConfig 定義 NATS 用戶端連線與行為設定。
	NatsConfig nyanats.NatsConfig `json:"nats_config" yaml:"nats_config"`

	// Bridge 定義 HTTP 與 NATS 之間的橋接層設定。
	Bridge BridgeConfig `json:"bridge" yaml:"bridge"`

	// Routes 定義 HTTP 路由到 NATS Subject 的轉發規則清單。
	Routes []RouteConfig `json:"routes" yaml:"routes"`
}

// loadConfigFile 讀取並解析 YAML 設定檔。
//
// 若 configPath 為空字串，會根據目前執行檔名稱推導預設設定檔名稱。
// 例如執行檔為 api-nats-bridge，則預設讀取 api-nats-bridge.yaml。
//
// 回傳值依序為：
//   - 實際使用的設定檔路徑
//   - 解析後的 ApiNatsBridgeConfig
//   - 讀取或解析時發生的錯誤
func loadConfigFile(configPath string) (string, ApiNatsBridgeConfig, error) {
	if configPath == "" { // 若呼叫端未指定設定檔路徑，則依執行檔名稱自動推導。
		exePath, err := os.Executable() // 嘗試取得目前執行檔的完整路徑。
		if err != nil {                 // 若取得執行檔路徑失敗，改用 os.Args[0] 作為備援。
			exePath = os.Args[0] // 使用啟動命令中的執行檔名稱或路徑。
		}
		exeBase := filepath.Base(exePath)                                         // 取出執行檔檔名，不包含目錄路徑。
		configPath = strings.TrimSuffix(exeBase, filepath.Ext(exeBase)) + ".yaml" // 移除原副檔名並加上 .yaml。
	}

	var fileConf ApiNatsBridgeConfig                 // 建立設定結構，用於承接 YAML 解析結果。
	yamlData, errReadFile := os.ReadFile(configPath) // 讀取指定 YAML 設定檔的原始內容。
	if errReadFile == nil {                          // 若檔案讀取成功，繼續解析 YAML。
		errUnmarshal := yaml.Unmarshal(yamlData, &fileConf) // 將 YAML 內容反序列化到設定結構。
		if errUnmarshal == nil {                            // 若 YAML 解析成功，回傳設定檔路徑與解析結果。
			return configPath, fileConf, nil
		} else {
			return configPath, fileConf, errUnmarshal // 若 YAML 解析失敗，回傳目前設定結構與解析錯誤。
		}
	} else {
		return configPath, fileConf, errReadFile // 若檔案讀取失敗，回傳設定檔路徑與讀取錯誤。
	}
}

// LoadConfig 從命令列參數指定的 YAML 設定檔載入應用程式設定。
//
// 支援的命令列參數：
//   - -c：指定 YAML 設定檔路徑
//   - -o：指定統一日誌檔案路徑
//
// 回傳值依序為：
//   - 是否成功載入設定
//   - HTTP API 伺服器設定
//   - NATS 用戶端設定
//   - 橋接層設定
//   - 路由轉發規則清單
func LoadConfig() (bool, *nyaapiserver.HttpAPIServerConfig, *nyanats.NatsConfig, BridgeConfig, []RouteConfig) {
	var configPath string                                                    // 保存命令列指定或自動推導出的 YAML 設定檔路徑。
	flag.StringVar(&configPath, "c", "", LCLI.CliFlagConfig())       // 註冊 -c 參數，用於指定設定檔路徑。
	flag.StringVar(&logFilePath, "o", "", LCLI.CliFlagOutput()) // 註冊 -o 參數，用於指定統一日誌檔案路徑。
	flag.Parse()                                                             // 解析命令列參數，將結果寫入已註冊的變數。

	configPath, appConfig, appConfigErr := loadConfigFile(configPath) // 讀取並解析 YAML 設定檔。
	LogMain(LLog.LogConfigFile(), configPath)                          // 記錄實際使用的設定檔路徑。
	if appConfigErr != nil {                                          // 若設定檔讀取或解析失敗，記錄錯誤並回傳失敗狀態。
		LogError("MAIN", "%v", appConfigErr)        // 將設定載入錯誤寫入主流程錯誤日誌。
		return false, nil, nil, BridgeConfig{}, nil // 回傳失敗狀態與空設定，避免後續使用無效資料。
	}

	var httpAPIServerConfig *nyaapiserver.HttpAPIServerConfig = &appConfig.HttpAPIServerConfig // 取得 HTTP API 伺服器設定指標。
	var natsConfig *nyanats.NatsConfig = &appConfig.NatsConfig                                 // 取得 NATS 用戶端設定指標。

	return true, httpAPIServerConfig, natsConfig, appConfig.Bridge, appConfig.Routes // 回傳成功狀態與完整設定。
}
