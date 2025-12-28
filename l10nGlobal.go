//go:generate .\go-gen-l10n.exe -dir .\l10n -pkg l10n -lang zh_Hant

// Package main 提供多國語言（l10n）初始化與全域語言實例管理。
//
// 此檔案定義三個全域語言實例變數（lLog、lHTTP、lCLI），
// 並提供 InitL10n 函式，讓設定載入階段可以根據 bridge.language 設定
// 動態切換日誌、HTTP 錯誤回應與 CLI 訊息使用的語言。
package main

import (
	"github.com/kagurazakayashi/ApiNatsBridge/l10n"
)

var (
	// lLog 是用於產生日誌相關格式化文字的語言實例。
	// 預設使用繁體中文（zh_Hant），可透過設定檔中的 bridge.language.log 動態切換。
	lLog l10n.AppLocalizations = l10n.GetLocalizations("zh_Hant")

	// lHTTP 是用於產生 HTTP 錯誤回應文字的語言實例。
	// 預設使用英文（en），可透過設定檔中的 bridge.language.http 動態切換。
	lHTTP l10n.AppLocalizations = l10n.GetLocalizations("en")

	// lCLI 是用於產生命令列說明文字的語言實例。
	// 預設使用繁體中文（zh_Hant），可透過設定檔中的 bridge.language.cli 動態切換。
	lCLI l10n.AppLocalizations = l10n.GetLocalizations("zh_Hant")
)

// InitL10n 根據橋接層語言設定初始化全域語言實例。
//
// 若 cfg 為 nil，則保留預設語言設定並直接返回。
// 此函式應在日誌與 HTTP 處理元件初始化之前呼叫，
// 以確保所有後續文字輸出皆使用正確的語言。
//
// 參數：
//   - cfg：橋接層語言設定，包含 log、http 與 cli 三個獨立語言欄位。
func InitL10n(cfg *BridgeLanguageConfig) {
	if cfg == nil {
		return
	}
	if cfg.Log != "" {
		lLog = l10n.GetLocalizations(cfg.Log)
	}
	if cfg.HTTP != "" {
		lHTTP = l10n.GetLocalizations(cfg.HTTP)
	}
	if cfg.CLI != "" {
		lCLI = l10n.GetLocalizations(cfg.CLI)
	}
}
