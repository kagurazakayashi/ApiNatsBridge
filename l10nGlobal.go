//go:generate .\go-gen-l10n.exe -dir .\l10n -pkg l10n -lang zh_Hant

package main

import (
	"github.com/kagurazakayashi/ApiNatsBridge/l10n"
)

var lLog l10n.AppLocalizations = l10n.GetLocalizations("zh_Hant")
var lHTTP l10n.AppLocalizations = l10n.GetLocalizations("en")
var lCLI l10n.AppLocalizations = l10n.GetLocalizations("zh_Hant")

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
