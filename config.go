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

func LoadConfig() (bool, *nyaapiserver.HttpAPIServerConfig, *nyanats.NatsConfig, []RouteConfig) {
	var configPath string
	flag.StringVar(&configPath, "c", "", "yaml/json config file")
	flag.Parse()
	configPath, appConfig, appConfigErr := loadConfigFile(configPath)
	fmt.Printf("[main] Config File: %s\n", configPath)
	if appConfigErr != nil {
		fmt.Printf("[main][ERROR] %v\n", appConfigErr)
		return false, nil, nil, nil
	}
	var httpAPIServerConfig *nyaapiserver.HttpAPIServerConfig = &appConfig.HttpAPIServerConfig
	var natsConfig *nyanats.NatsConfig = &appConfig.NatsConfig
	return true, httpAPIServerConfig, natsConfig, appConfig.Routes
}
