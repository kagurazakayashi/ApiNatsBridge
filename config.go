package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kagurazakayashi/libNyaruko_Go/nyaapiserver"
	"github.com/kagurazakayashi/libNyaruko_Go/nyanats"
	"gopkg.in/yaml.v3"
)

type RouteConfig struct {
	Path        string `json:"path" yaml:"path"`
	NatsSubject string `json:"nats_subject" yaml:"nats_subject"`
}

type ApiNatsBridgeConfig struct {
	HttpAPIServerConfig nyaapiserver.HttpAPIServerConfig `json:"httpapiserver_config" yaml:"httpapiserver_config"`
	NatsConfig          nyanats.NatsConfig               `json:"nats_config" yaml:"nats_config"`
	Routes              []RouteConfig                    `json:"routes" yaml:"routes"`
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
