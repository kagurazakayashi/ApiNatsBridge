package main

import (
	"fmt"

	"github.com/kagurazakayashi/libNyaruko_Go/nyalog"
)

const logLevel = nyalog.Debug

func logMain(format string, a ...interface{}) {
	nyalog.LogCC(logLevel, nyalog.Info, nyalog.Cyan, "[MAIN]", fmt.Sprintf(format, a...))
}

func logError(module string, format string, a ...interface{}) {
	nyalog.LogCC(logLevel, nyalog.Error, nyalog.Red, fmt.Sprintf("[ERROR][%s]", module), fmt.Sprintf(format, a...))
}

func logBridge(format string, a ...interface{}) {
	nyalog.LogCC(logLevel, nyalog.Info, nyalog.Yellow, "[BRIDGE]", fmt.Sprintf(format, a...))
}

func logHTTP(format string, a ...interface{}) {
	nyalog.LogCC(logLevel, nyalog.Info, nyalog.Blue, "[HTTP]", fmt.Sprintf(format, a...))
}

func logHTTPStat(format string, a ...interface{}) {
	nyalog.LogCC(logLevel, nyalog.OK, nyalog.Purple, "[HTTPSTAT]", fmt.Sprintf(format, a...))
}

func logPing(format string, a ...interface{}) {
	nyalog.LogCC(logLevel, nyalog.Info, nyalog.Cyan, "[apiping]", fmt.Sprintf(format, a...))
}
