package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kagurazakayashi/libNyaruko_Go/nyaapiserver"
	"github.com/kagurazakayashi/libNyaruko_Go/nyanats"
	"gopkg.in/yaml.v3"
)

const defaultTimeoutSeconds = 30

// ScalarLimitRule 定義純量欄位的長度限制（如 path、body）。
type ScalarLimitRule struct {
	// 最大長度（位元組），0 表示不限制
	MaxLength int `json:"max_length,omitempty" yaml:"max_length,omitempty"`
}

// MapLimitRule 定義鍵值對集合的長度限制（如 headers、cookies、params）。
type MapLimitRule struct {
	// 最大筆數，0 表示不限制
	MaxCount int `json:"max_count,omitempty" yaml:"max_count,omitempty"`
	// 每個鍵的最大長度（位元組），0 表示不限制
	MaxKeyLen int `json:"max_key_length,omitempty" yaml:"max_key_length,omitempty"`
	// 每個值的最大長度（位元組），0 表示不限制
	MaxValueLen int `json:"max_value_length,omitempty" yaml:"max_value_length,omitempty"`
}

// LimitRule 定義請求各欄位的長度限制。
type LimitRule struct {
	Path    ScalarLimitRule `json:"path,omitempty" yaml:"path,omitempty"`
	Headers MapLimitRule    `json:"headers,omitempty" yaml:"headers,omitempty"`
	Cookies MapLimitRule    `json:"cookies,omitempty" yaml:"cookies,omitempty"`
	Params  MapLimitRule    `json:"params,omitempty" yaml:"params,omitempty"`
	Body    ScalarLimitRule `json:"body,omitempty" yaml:"body,omitempty"`
}

// BridgeConfig 定義橋接層設定。
type BridgeConfig struct {
	// CDN 服務商傳遞真實 IP 的標頭清單
	CdnHeader []string `json:"cdnheader" yaml:"cdnheader"`
	// 請求欄位長度限制規則
	Limits *LimitRule `json:"limits,omitempty" yaml:"limits,omitempty"`
}

// RouteConfig 定義單一路由的轉發規則。
type RouteConfig struct {
	// HTTP 請求路徑
	Path string `json:"path" yaml:"path"`
	// 對應的 NATS Subject
	NatsSubject string `json:"nats_subject" yaml:"nats_subject"`
	// NATS 等待回應的逾時秒數
	Timeout int `json:"timeout,omitempty" yaml:"timeout,omitempty"`
	// 允許的 HTTP 方法清單
	Methods []string `json:"methods,omitempty" yaml:"methods,omitempty"`
	// 要求的 Content-Type 前綴
	ContentType string `json:"content_type,omitempty" yaml:"content_type,omitempty"`
	// JSON Schema 校驗規則定義
	SchemaBody map[string]interface{} `json:"schema_body,omitempty" yaml:"schema_body,omitempty"`
	// 請求欄位長度限制規則（每項皆可選，會合併覆蓋 bridge.limits 對應欄位）
	Limits *LimitRule `json:"limits,omitempty" yaml:"limits,omitempty"`
	// 轉發到 NATS 時要包含的欄位清單，可選值：method, path, headers, cookies, remote_addr, ip, params, body
	// 留空陣列或不提供則返回所有欄位
	ReturnFields []string `json:"return_fields,omitempty" yaml:"return_fields,omitempty"`
}

func (r *RouteConfig) TimeoutDuration() time.Duration {
	if r.Timeout <= 0 {
		return defaultTimeoutSeconds * time.Second
	}
	return time.Duration(r.Timeout) * time.Second
}

func (r *RouteConfig) AllowedMethods() []string {
	return r.Methods
}

// ApiNatsBridgeConfig 定義 ApiNatsBridge 完整的執行設定。
type ApiNatsBridgeConfig struct {
	// HTTP API 伺服器設定
	HttpAPIServerConfig nyaapiserver.HttpAPIServerConfig `json:"httpapiserver_config" yaml:"httpapiserver_config"`
	// NATS 用戶端設定
	NatsConfig nyanats.NatsConfig `json:"nats_config" yaml:"nats_config"`
	// 橋接層設定
	Bridge BridgeConfig `json:"bridge" yaml:"bridge"`
	// 路由轉發規則清單
	Routes []RouteConfig `json:"routes" yaml:"routes"`
}

func loadConfigFile(configPath string) (string, ApiNatsBridgeConfig, error) {
	if configPath == "" {
		exePath, err := os.Executable()
		if err != nil {
			exePath = os.Args[0]
		}
		exeBase := filepath.Base(exePath)
		configPath = strings.TrimSuffix(exeBase, filepath.Ext(exeBase)) + ".yaml"
	}
	var fileConf ApiNatsBridgeConfig
	yamlData, errReadFile := os.ReadFile(configPath)
	if errReadFile == nil {
		errUnmarshal := yaml.Unmarshal(yamlData, &fileConf)
		if errUnmarshal == nil {
			return configPath, fileConf, nil
		} else {
			return configPath, fileConf, errUnmarshal
		}
	} else {
		return configPath, fileConf, errReadFile
	}
}

func LoadConfig() (bool, *nyaapiserver.HttpAPIServerConfig, *nyanats.NatsConfig, BridgeConfig, []RouteConfig) {
	var configPath string
	flag.StringVar(&configPath, "c", "", "yaml/json config file")
	flag.Parse()
	configPath, appConfig, appConfigErr := loadConfigFile(configPath)
	fmt.Printf("[main] Config File: %s\n", configPath)
	if appConfigErr != nil {
		fmt.Printf("[main][ERROR] %v\n", appConfigErr)
		return false, nil, nil, BridgeConfig{}, nil
	}
	var httpAPIServerConfig *nyaapiserver.HttpAPIServerConfig = &appConfig.HttpAPIServerConfig
	var natsConfig *nyanats.NatsConfig = &appConfig.NatsConfig
	return true, httpAPIServerConfig, natsConfig, appConfig.Bridge, appConfig.Routes
}
