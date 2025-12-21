package main

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

	// HTTPStat 表示 HTTP 伺服器執行統計日誌檔案路徑。
	HTTPStat string `json:"httpstat,omitempty" yaml:"httpstat,omitempty"`

	// Module 表示通用模組日誌檔案路徑，例如 /ping 等模組輸出。
	Module string `json:"module,omitempty" yaml:"module,omitempty"`
}

// BridgeLogConfig 定義橋接層日誌相關設定。
//
// 此設定可控制是否輸出到終端機、是否啟用 debug、
// 是否覆寫既有日誌檔，以及是否使用彩色輸出。
type BridgeLogConfig struct {
	// Stdout 表示是否輸出到控制台 stdout 或 stderr；設為 false 時僅寫入檔案。
	Stdout bool `json:"stdout" yaml:"stdout"`

	// Debug 表示是否啟用除錯等級日誌；設為 false 時僅輸出 Info 及以上等級。
	Debug bool `json:"debug" yaml:"debug"`

	// Overwrite 表示是否以覆蓋模式寫入日誌檔案；設為 true 時會在啟動時清空既有日誌。
	Overwrite bool `json:"overwrite,omitempty" yaml:"overwrite,omitempty"`

	// Color 表示是否使用彩色控制台輸出；nil 或 true 代表啟用彩色，false 代表純文字輸出。
	Color *bool `json:"color,omitempty" yaml:"color,omitempty"`

	// Files 定義各模組的獨立日誌檔案路徑。
	Files BridgeLogFilesConfig `json:"files,omitempty" yaml:"files,omitempty"`
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
// CDN 真實 IP 標頭、請求與回應限制、錯誤細節白名單與 Cookie UUID 設定。
type BridgeConfig struct {
	// Language 定義各模組使用的語言設定；未提供時使用程式預設值。
	Language *BridgeLanguageConfig `json:"language,omitempty" yaml:"language,omitempty"`

	// Log 定義橋接層日誌相關設定；未提供時使用程式預設值。
	Log *BridgeLogConfig `json:"log,omitempty" yaml:"log,omitempty"`

	// Timezone 定義日誌或時間處理使用的時區，支援 IANA 時區名稱，例如 Asia/Taipei，或小時偏移，例如 8、-5。
	Timezone string `json:"timezone,omitempty" yaml:"timezone,omitempty"`

	// CdnHeader 定義 CDN 或反向代理傳遞真實用戶端 IP 的標頭清單。
	CdnHeader []string `json:"cdnheader" yaml:"cdnheader"`

	// Limits 定義全域請求欄位長度限制規則；可被單一路由設定覆蓋。
	Limits *LimitRule `json:"limits,omitempty" yaml:"limits,omitempty"`

	// ResponseLimits 定義全域回應欄位長度限制規則；可被單一路由設定覆蓋。
	ResponseLimits *LimitRule `json:"response_limits,omitempty" yaml:"response_limits,omitempty"`

	// ErrorDetailIPs 定義允許接收錯誤詳細資訊的 IP 白名單，通常用於開發或除錯環境。
	ErrorDetailIPs []string `json:"error_detail_ips,omitempty" yaml:"error_detail_ips,omitempty"`

	// CookieUUIDKey 定義自動寫入用戶端 Cookie 的 UUID 鍵名；留空時不啟用。
	CookieUUIDKey string `json:"cookie_uuid_key,omitempty" yaml:"cookie_uuid_key,omitempty"`
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
//   - -v：啟用詳細模式，記錄完整請求資料
//
// 回傳值依序為：
//   - 是否成功載入設定
//   - HTTP API 伺服器設定
//   - NATS 用戶端設定
//   - 橋接層設定
//   - 路由轉發規則清單
func LoadConfig() (bool, *nyaapiserver.HttpAPIServerConfig, *nyanats.NatsConfig, BridgeConfig, []RouteConfig) {
	var configPath string                                                // 保存命令列指定或自動推導出的 YAML 設定檔路徑。
	flag.StringVar(&configPath, "c", "", lCLI.CliFlagConfig())   // 註冊 -c 參數，用於指定設定檔路徑。
	flag.BoolVar(&verbose, "v", false, lCLI.CliFlagVerbose()) // 註冊 -v 參數，用於啟用詳細請求資料日誌。
	flag.Parse()                                                         // 解析命令列參數，將結果寫入已註冊的變數。

	configPath, appConfig, appConfigErr := loadConfigFile(configPath) // 讀取並解析 YAML 設定檔。
	logMain(lLog.LogConfigFile(), configPath)                          // 記錄實際使用的設定檔路徑。
	if appConfigErr != nil {                                          // 若設定檔讀取或解析失敗，記錄錯誤並回傳失敗狀態。
		logError("MAIN", "%v", appConfigErr)        // 將設定載入錯誤寫入主流程錯誤日誌。
		return false, nil, nil, BridgeConfig{}, nil // 回傳失敗狀態與空設定，避免後續使用無效資料。
	}

	var httpAPIServerConfig *nyaapiserver.HttpAPIServerConfig = &appConfig.HttpAPIServerConfig // 取得 HTTP API 伺服器設定指標。
	var natsConfig *nyanats.NatsConfig = &appConfig.NatsConfig                                 // 取得 NATS 用戶端設定指標。

	return true, httpAPIServerConfig, natsConfig, appConfig.Bridge, appConfig.Routes // 回傳成功狀態與完整設定。
}
