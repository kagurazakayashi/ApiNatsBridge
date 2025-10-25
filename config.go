package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/kagurazakayashi/libNyaruko_Go/nyaapiserver"
	"gopkg.in/yaml.v3"
)

type ApiNatsBridgeConfig struct {
	HttpAPIServerConfig nyaapiserver.HttpAPIServerConfig `json:"httpapiserver_config" yaml:"httpapiserver_config"`
}

func LoadConfigFile(configPath string) (string, ApiNatsBridgeConfig, error) {
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
