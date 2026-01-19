//go:generate go-winres make

// Package main 是 ApiNatsBridge 的進入點，呼叫 src 套件的 Run 啟動服務。
package main

import "github.com/kagurazakayashi/ApiNatsBridge/src"

func main() {
	src.Run()
}
