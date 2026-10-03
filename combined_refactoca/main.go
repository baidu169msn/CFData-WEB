package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
)

var boolFlagNames = []string{"cli", "progress", "nocolor", "compactipv4", "github", "skipgeo", "v6bracket"}

func rewriteBoolFlagArgs() {
	if len(os.Args) <= 2 {
		return
	}
	boolSet := map[string]struct{}{}
	for _, n := range boolFlagNames {
		boolSet[n] = struct{}{}
	}
	rewritten := append([]string(nil), os.Args[:1]...)
	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		if name, ok := matchBoolFlag(arg, boolSet); ok && i+1 < len(os.Args) {
			next := strings.ToLower(os.Args[i+1])
			if next == "true" || next == "false" || next == "1" || next == "0" || next == "t" || next == "f" {
				rewritten = append(rewritten, "-"+name+"="+next)
				i++
				continue
			}
		}
		rewritten = append(rewritten, arg)
	}
	os.Args = rewritten
}

func matchBoolFlag(arg string, boolSet map[string]struct{}) (string, bool) {
	if !strings.HasPrefix(arg, "-") {
		return "", false
	}
	name := strings.TrimLeft(arg, "-")
	if strings.Contains(name, "=") {
		return "", false
	}
	if _, ok := boolSet[name]; ok {
		return name, true
	}
	return "", false
}

func hasNoColorArg() bool {
	for _, arg := range os.Args[1:] {
		name := strings.TrimLeft(arg, "-")
		if name == "nocolor" || strings.HasPrefix(name, "nocolor=") {
			if name == "nocolor" || strings.EqualFold(strings.TrimPrefix(name, "nocolor="), "true") {
				return true
			}
		}
	}
	return false
}

func main() {
	rewriteBoolFlagArgs()
	if !enableTerminalANSI() || os.Getenv("NO_COLOR") != "" || hasNoColorArg() {
		disableANSIColors()
	}
	cliCfg := registerCLIFlags()

	flag.StringVar(&speedTestURL, "url", autoSpeedURLValue, "测速下载地址（同 -offurl）；auto 表示自动选择内置测速源")
	flag.BoolVar(&skipGeoCheck, "skipgeo", false, "跳过地区/代理环境验证（定时任务必须开启）")
	flag.StringVar(&customDNSServer, "dns", defaultDNSServers, "自定义 DNS 服务器，例如 223.5.5.5、8.8.8.8:53 或逗号分隔多个；默认系统 DNS 优先、失败回退到该内置 DNS，显式提供时强制使用指定 DNS")
	flag.Var(debugFlagValue{}, "debug", "开启调试输出等级：error、all；也兼容 true/false，-debug 默认为 error")
	flag.Parse()
	if debugMode && flag.NArg() > 0 {
		for _, arg := range flag.Args() {
			if normalizeDebugLevel(arg) == "all" {
				debugLevel = "all"
			}
		}
	}
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "dns" {
			customDNSForced = true
		}
	})

	if err := prepareCLIConfig(cliCfg); err != nil {
		if errors.Is(err, errCLIConfigCreated) {
			return
		}
		recordProgramDebugError("cli_prepare", err.Error())
		fmt.Printf("CLI 执行失败: %v\n", err)
		os.Exit(1)
	}
	configureHTTPClients()
	initLocations()
	if err := runCLI(cliCfg); err != nil {
		recordProgramDebugError("cli_run", err.Error())
		fmt.Printf("CLI 执行失败: %v\n", err)
		os.Exit(1)
	}
}
