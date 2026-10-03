package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type cliConfig struct {
	configResolved bool
	scanMode       string
	ipType         int
	threads        int
	port           int
	delay          int
	dc             string
	outFile        string
	speedLimit     int
	speedMin       float64
	showProgress   bool
	noColor        bool
	compactIPv4    bool
	export         cliExportConfig
}

type cliExportConfig struct {
	ConfigFile   string `json:"-"`
	Format       string `json:"format"`
	Fields       string `json:"fields"`
	Custom       string `json:"custom"`
	V6Bracket    bool   `json:"v6bracket"`
	V6BracketSet bool   `json:"-"`
	GitHub       bool   `json:"github"`
	GitHubSet    bool   `json:"-"`
	GHRepo       string `json:"ghrepo"`
	GHBranch     string `json:"ghbranch"`
	GHPath       string `json:"ghpath"`
	GHMessage    string `json:"ghmessage"`
	GHToken      string `json:"ghtoken"`
	GHTokenFile  string `json:"ghtokenfile"`
	GHUpload     string `json:"ghupload"`
}

type cliFileConfig struct {
	SkipGeo     bool    `json:"skipgeo"`
	ScanMode    string  `json:"scanmode"`
	IPType      int     `json:"offiptype"`
	Threads     int     `json:"offthreads"`
	Out         string  `json:"offout"`
	Progress    bool    `json:"progress"`
	NoColor     bool    `json:"nocolor"`
	URL         string  `json:"offurl"`
	DNS         string  `json:"dns"`
	Debug       any     `json:"debug"`
	CompactIPv4 bool    `json:"compactipv4"`
	TestPort    int     `json:"offport"`
	Delay       int     `json:"offdelay"`
	DC          string  `json:"offdc"`
	SpeedLimit  int     `json:"offspeedlimit"`
	SpeedMin    float64 `json:"offspeedmin"`
	Format      string  `json:"format"`
	Fields      string  `json:"fields"`
	Custom      string  `json:"custom"`
	V6Bracket   bool    `json:"v6bracket"`
	GitHub      bool    `json:"github"`
	GHRepo      string  `json:"ghrepo"`
	GHBranch    string  `json:"ghbranch"`
	GHPath      string  `json:"ghpath"`
	GHMessage   string  `json:"ghmessage"`
	GHToken     string  `json:"ghtoken"`
	GHTokenFile string  `json:"ghtokenfile"`
	GHUpload    string  `json:"ghupload"`
}

type cliResultRow map[string]string

type cliResultField struct {
	Key   string
	Label string
}

type cliCustomField struct {
	Key   string
	Label string
	Value string
}

var cliResultFields = []cliResultField{
	{Key: "ipport", Label: "ip:port"},
	{Key: "ip", Label: "IP地址"},
	{Key: "port", Label: "端口号"},
	{Key: "tls", Label: "TLS"},
	{Key: "lossRate", Label: "丢包率"},
	{Key: "latency", Label: "网络延迟"},
	{Key: "scanMode", Label: "扫描方式"},
	{Key: "speed", Label: "下载速度"},
	{Key: "outboundIP", Label: "出站IP"},
	{Key: "ipType", Label: "IP类型"},
	{Key: "originalInput", Label: "原始输入"},
	{Key: "dc", Label: "数据中心"},
	{Key: "dcCountry", Label: "落地区域"},
	{Key: "loc", Label: "源IP位置"},
	{Key: "region", Label: "地区"},
	{Key: "city", Label: "城市"},
	{Key: "asnNumber", Label: "ASN号码"},
	{Key: "asnOrg", Label: "ASN组织"},
	{Key: "visitScheme", Label: "访问协议"},
	{Key: "tlsVersion", Label: "TLS版本"},
	{Key: "sni", Label: "SNI"},
	{Key: "httpVersion", Label: "HTTP版本"},
	{Key: "warp", Label: "WARP"},
	{Key: "gateway", Label: "Gateway"},
	{Key: "rbi", Label: "RBI"},
	{Key: "kex", Label: "密钥交换"},
	{Key: "timestamp", Label: "时间戳"},
}

type cliFlagInfo struct {
	name         string
	description  string
	defaultValue string
}

var (
	ansiReset       = "\033[0m"
	ansiBold        = "\033[1m"
	ansiGreen       = "\033[32m"
	ansiBrightGreen = "\033[92m"
	ansiYellow      = "\033[33m"
	ansiRed         = "\033[31m"
	ansiCyan        = "\033[36m"
	ansiMagenta     = "\033[35m"

	cliCommonFlags = []cliFlagInfo{
		{name: "config", description: "配置文件路径，不存在时在二进制目录自动生成模板", defaultValue: "二进制目录/cfdata-config.json"},
		{name: "skipgeo", description: "跳过地区/代理环境验证（定时任务/无人值守必须开启，否则会等待交互输入）", defaultValue: "false"},
		{name: "offout", description: "本地输出文件名（只含测速合格 IP）", defaultValue: "ip.csv"},
		{name: "progress", description: "是否输出进度日志", defaultValue: "true"},
		{name: "nocolor", description: "禁用颜色输出（日志重定向到文件时建议开启）", defaultValue: "false"},
		{name: "dns", description: "自定义 DNS 服务器，例如 1.1.1.1 或 223.5.5.5,8.8.8.8；默认系统 DNS 优先，失败回退内置 DNS；显式设置时强制使用指定 DNS", defaultValue: defaultDNSServers},
		{name: "debug", description: "调试输出等级：error、all；true 等同 error", defaultValue: "false"},
		{name: "compactipv4", description: "精简本地 IPv4 地址库：按 /24 子网测 TCP:80 连通性并覆盖 ips-v4.txt", defaultValue: "false"},
		{name: "format", description: "导出格式：csv 或 txt", defaultValue: "txt"},
		{name: "fields", description: "导出字段：compact、all、ipport 或逗号分隔字段 key；可用 -custom 增加常量字段", defaultValue: "compact"},
		{name: "custom", description: "自定义导出字段，格式 标题:内容，多项用逗号分隔", defaultValue: ""},
		{name: "v6bracket", description: "TXT 导出时对 IPv6 地址加方括号（[IPv6]:端口）", defaultValue: "true"},
		{name: "github", description: "导出后上传到 GitHub（仅上传测速合格的 IP）", defaultValue: "false"},
		{name: "ghrepo", description: "GitHub 仓库，格式 owner/repo", defaultValue: ""},
		{name: "ghbranch", description: "GitHub 分支", defaultValue: "main"},
		{name: "ghpath", description: "GitHub 目标路径；留空时按 -format 自动使用 results/ip.csv 或 results/ip.txt", defaultValue: "<自动>"},
		{name: "ghmessage", description: "GitHub 提交信息", defaultValue: "update cfdata results"},
		{name: "ghtoken", description: "GitHub token（不推荐直接写入配置；强烈建议使用仅限制指定仓库读写权限的 token）", defaultValue: ""},
		{name: "ghtokenfile", description: "GitHub token 文件路径", defaultValue: ""},
		{name: "ghupload", description: "快速上传指定文件到 GitHub，不执行测试；需配合 -github", defaultValue: ""},
	}
	cliOfficialFlags = []cliFlagInfo{
		{name: "scanmode", description: "扫描方式：tcping（默认，TCP 握手延迟）或 httping（HTTP TTFB，延迟比 tcping 高属正常）", defaultValue: "tcping"},
		{name: "offiptype", description: "IP 类型：4 或 6", defaultValue: "4"},
		{name: "offthreads", description: "扫描并发数", defaultValue: "100"},
		{name: "offport", description: "详细测试与测速端口", defaultValue: "443"},
		{name: "offdelay", description: "延迟阈值（毫秒）", defaultValue: "500"},
		{name: "offdc", description: "指定数据中心；不填时自动选择最低延迟数据中心", defaultValue: ""},
		{name: "offurl", description: "测速下载地址；auto 表示自动选择内置测速源", defaultValue: autoSpeedURLValue},
		{name: "offspeedlimit", description: "测速达标结果上限（必须大于 0，只导出/上传达标 IP）", defaultValue: "5"},
		{name: "offspeedmin", description: "测速达标下限，单位 MB/s", defaultValue: "0.1"},
	}
)

var errCLIConfigCreated = errors.New("CLI 配置文件已生成")

func registerCLIFlags() *cliConfig {
	cfg := &cliConfig{}
	flag.Usage = printCLIUsage
	var legacyCLI bool
	flag.BoolVar(&legacyCLI, "cli", true, "兼容旧版参数，无实际作用（精简版始终为 CLI 模式）")
	flag.StringVar(&cfg.scanMode, "scanmode", "tcping", "扫描方式：tcping（默认）或 httping")
	flag.IntVar(&cfg.ipType, "offiptype", 4, "IP 类型：4 或 6")
	flag.IntVar(&cfg.threads, "offthreads", 100, "扫描并发数")
	flag.IntVar(&cfg.port, "offport", 443, "目标测试端口")
	flag.IntVar(&cfg.delay, "offdelay", 500, "延迟阈值（毫秒）")
	flag.StringVar(&cfg.dc, "offdc", "", "指定数据中心，不填则自动选择最低延迟数据中心")
	flag.StringVar(&cfg.outFile, "offout", "ip.csv", "输出文件名")
	flag.IntVar(&cfg.speedLimit, "offspeedlimit", 5, "测速达标结果上限，必须大于 0")
	flag.Float64Var(&cfg.speedMin, "offspeedmin", 0.1, "测速达标下限，单位 MB/s")
	flag.StringVar(&speedTestURL, "offurl", autoSpeedURLValue, "测速下载地址")
	flag.BoolVar(&cfg.showProgress, "progress", true, "输出进度日志")
	flag.BoolVar(&cfg.noColor, "nocolor", false, "禁用 ANSI 颜色输出")
	flag.BoolVar(&cfg.compactIPv4, "compactipv4", false, "精简本地 IPv4 地址库，按 /24 子网探测 TCP:80 连通性后覆盖 ips-v4.txt")
	flag.StringVar(&cfg.export.ConfigFile, "config", "", "配置文件路径")
	flag.StringVar(&cfg.export.Format, "format", "", "导出格式：csv 或 txt")
	flag.StringVar(&cfg.export.Fields, "fields", "", "导出字段：compact、all、ipport 或逗号分隔字段 key")
	flag.StringVar(&cfg.export.Custom, "custom", "", "自定义导出字段，格式 标题:内容，多项用逗号分隔")
	flag.BoolVar(&cfg.export.V6Bracket, "v6bracket", true, "TXT 导出时对 IPv6 地址加方括号（[IPv6]:端口）")
	flag.BoolVar(&cfg.export.GitHub, "github", false, "导出后上传到 GitHub")
	flag.StringVar(&cfg.export.GHRepo, "ghrepo", "", "GitHub 仓库 owner/repo")
	flag.StringVar(&cfg.export.GHBranch, "ghbranch", "", "GitHub 分支")
	flag.StringVar(&cfg.export.GHPath, "ghpath", "", "GitHub 目标路径")
	flag.StringVar(&cfg.export.GHMessage, "ghmessage", "", "GitHub 提交信息")
	flag.StringVar(&cfg.export.GHToken, "ghtoken", "", "GitHub token")
	flag.StringVar(&cfg.export.GHTokenFile, "ghtokenfile", "", "GitHub token 文件")
	flag.StringVar(&cfg.export.GHUpload, "ghupload", "", "快速上传指定文件到 GitHub，不执行测试")
	return cfg
}

func runCLI(cfg *cliConfig) error {
	if !cfg.configResolved {
		if err := prepareCLIConfig(cfg); err != nil {
			return err
		}
	}
	applyCLISpeedDefault()
	printCLIConfig(cfg)

	if !skipGeoCheck {
		ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
		country, ok := detectCloudflareTraceCountry(ctx)
		cancel()
		if !confirmCLIProxyCountry(country, ok) {
			return fmt.Errorf("已取消：当前网络环境标签为 %s", firstNonEmpty(country, "未知"))
		}
	} else {
		fmt.Println("[proxy-check] 已跳过地区/代理环境验证")
	}

	if cfg.compactIPv4 {
		return runCompactIPv4CLI(cfg)
	}
	if strings.TrimSpace(cfg.export.GHUpload) != "" {
		return runCLIQuickGitHubUpload(cfg)
	}
	return runOfficialCLI(cfg)
}

func applyCLISpeedDefault() {
	if !isAutoSpeedURL(speedTestURL) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	_, info, err := resolveStartupSpeedTestURL(ctx, speedTestURL)
	cancel()
	if err != nil {
		recordDebugError("speed_isp_check", err.Error())
		return
	}
	recordDebugByLevel("all", "speed_isp_check", fmt.Sprintf("cli asn=%d org=%s mobile=%v selected=%s", info.ASN, info.ASOrganization, isChinaMobileISP(info), currentAutoSpeedURLDefault()))
}

func prepareCLIConfig(cfg *cliConfig) error {
	if err := resolveCLIExportConfig(cfg); err != nil {
		return err
	}
	if cfg.noColor {
		disableANSIColors()
	}
	cfg.configResolved = true
	return nil
}

func runCLIQuickGitHubUpload(cfg *cliConfig) error {
	if !cfg.export.GitHub {
		return fmt.Errorf("使用 -ghupload 快速上传时需要同时启用 -github")
	}
	path := expandHome(cfg.export.GHUpload)
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if strings.TrimSpace(cfg.export.GHPath) == "" || cfg.export.GHPath == "results/ip."+cfg.export.Format {
		cfg.export.GHPath = "results/" + filepath.Base(path)
	}
	return uploadCLIExportToGitHub(cfg, string(content))
}

func runCompactIPv4CLI(cfg *cliConfig) error {
	session := newCLISession(cfg)
	if err := session.runTaskSync(func(ctx context.Context, session *appSession) {
		runCompactIPv4Task(ctx, session)
	}); err != nil {
		return cliTaskError(err)
	}
	return nil
}

func resolveCLIExportConfig(cfg *cliConfig) error {
	provided := map[string]bool{}
	flag.Visit(func(f *flag.Flag) { provided[f.Name] = true })
	configPath := cfg.export.ConfigFile
	if configPath == "" {
		configPath = os.Getenv("CFDATA_CONFIG")
	}
	if configPath == "" {
		configPath = defaultCLIConfigPath()
	}
	fileCfg, created, err := loadOrCreateCLIConfig(configPath)
	if err != nil {
		return err
	}
	if created {
		fmt.Printf("[config] 已生成配置文件: %s\n", configPath)
		fmt.Println("[config] 优先级: 命令行参数 > 配置文件 > 环境变量 > 默认值")
		fmt.Println("[config] 请退出后按需编辑配置文件，再重新开始测试。")
		return errCLIConfigCreated
	}
	envCfg := cliExportConfig{
		Format:      os.Getenv("CFDATA_FORMAT"),
		Fields:      os.Getenv("CFDATA_FIELDS"),
		Custom:      os.Getenv("CFDATA_CUSTOM"),
		GHRepo:      os.Getenv("CFDATA_GHREPO"),
		GHBranch:    os.Getenv("CFDATA_GHBRANCH"),
		GHPath:      os.Getenv("CFDATA_GHPATH"),
		GHMessage:   os.Getenv("CFDATA_GHMESSAGE"),
		GHToken:     firstNonEmpty(os.Getenv("CFDATA_GHTOKEN"), os.Getenv("GITHUB_TOKEN")),
		GHTokenFile: os.Getenv("CFDATA_GHTOKENFILE"),
		GHUpload:    os.Getenv("CFDATA_GHUPLOAD"),
	}
	if value := strings.TrimSpace(os.Getenv("CFDATA_GITHUB")); value != "" {
		envCfg.GitHub = parseBoolEnv(value)
		envCfg.GitHubSet = true
	}
	if value := strings.TrimSpace(os.Getenv("CFDATA_V6BRACKET")); value != "" {
		envCfg.V6Bracket = parseBoolEnv(value)
		envCfg.V6BracketSet = true
	}
	applyCLIEnvConfig(cfg, provided)
	merged := defaultCLIExportConfig()
	mergeCLIExportConfig(&merged, envCfg, false)
	applyCLIFileConfig(cfg, fileCfg, provided)
	mergeCLIExportConfig(&merged, fileCfg.Export(), false)
	mergeCLIExportConfig(&merged, cfg.export, true, provided)
	merged.ConfigFile = configPath
	merged.Format = strings.ToLower(strings.TrimSpace(merged.Format))
	if merged.Format == "" {
		merged.Format = "txt"
	}
	if merged.Format != "csv" && merged.Format != "txt" {
		return fmt.Errorf("不支持的 -format: %s", merged.Format)
	}
	if strings.TrimSpace(merged.Fields) == "" {
		merged.Fields = "compact"
	}
	if strings.TrimSpace(merged.GHBranch) == "" {
		merged.GHBranch = "main"
	}
	if strings.TrimSpace(merged.GHMessage) == "" {
		merged.GHMessage = "update cfdata results"
	}
	if strings.TrimSpace(merged.GHPath) == "" || (!provided["ghpath"] && fileCfg.GHPath == "" && envCfg.GHPath == "") {
		merged.GHPath = "results/ip." + merged.Format
	}
	if merged.GHToken == "" && strings.TrimSpace(merged.GHTokenFile) != "" {
		data, err := os.ReadFile(expandHome(merged.GHTokenFile))
		if err != nil {
			return fmt.Errorf("读取 token 文件失败: %w", err)
		}
		merged.GHToken = strings.TrimSpace(string(data))
	}
	cfg.export = merged
	return nil
}

func applyCLIEnvConfig(cfg *cliConfig, provided map[string]bool) {
	setString := func(flagName, envName string, target *string) {
		if !provided[flagName] && strings.TrimSpace(os.Getenv(envName)) != "" {
			*target = strings.TrimSpace(os.Getenv(envName))
		}
	}
	setInt := func(flagName, envName string, target *int) {
		if provided[flagName] || strings.TrimSpace(os.Getenv(envName)) == "" {
			return
		}
		if v, err := strconv.Atoi(strings.TrimSpace(os.Getenv(envName))); err == nil {
			*target = v
		}
	}
	setFloat := func(flagName, envName string, target *float64) {
		if provided[flagName] || strings.TrimSpace(os.Getenv(envName)) == "" {
			return
		}
		if v, err := strconv.ParseFloat(strings.TrimSpace(os.Getenv(envName)), 64); err == nil {
			*target = v
		}
	}
	setBool := func(flagName, envName string, target *bool) {
		if !provided[flagName] && strings.TrimSpace(os.Getenv(envName)) != "" {
			*target = parseBoolEnv(os.Getenv(envName))
		}
	}
	setString("scanmode", "CFDATA_SCANMODE", &cfg.scanMode)
	setInt("offiptype", "CFDATA_OFFIPTYPE", &cfg.ipType)
	setInt("offthreads", "CFDATA_OFFTHREADS", &cfg.threads)
	setString("offout", "CFDATA_OFFOUT", &cfg.outFile)
	setBool("progress", "CFDATA_PROGRESS", &cfg.showProgress)
	setBool("nocolor", "CFDATA_NOCOLOR", &cfg.noColor)
	setBool("skipgeo", "CFDATA_SKIPGEO", &skipGeoCheck)
	if !provided["offurl"] && !provided["url"] && strings.TrimSpace(os.Getenv("CFDATA_OFFURL")) != "" {
		speedTestURL = strings.TrimSpace(os.Getenv("CFDATA_OFFURL"))
	}
	if !provided["dns"] && strings.TrimSpace(os.Getenv("CFDATA_DNS")) != "" {
		customDNSServer = strings.TrimSpace(os.Getenv("CFDATA_DNS"))
		customDNSForced = true
	}
	if !provided["debug"] && strings.TrimSpace(os.Getenv("CFDATA_DEBUG")) != "" {
		_ = setDebugFlag(os.Getenv("CFDATA_DEBUG"))
	}
	setBool("compactipv4", "CFDATA_COMPACTIPV4", &cfg.compactIPv4)
	setInt("offport", "CFDATA_OFFPORT", &cfg.port)
	setInt("offdelay", "CFDATA_OFFDELAY", &cfg.delay)
	setString("offdc", "CFDATA_OFFDC", &cfg.dc)
	setInt("offspeedlimit", "CFDATA_OFFSPEEDLIMIT", &cfg.speedLimit)
	setFloat("offspeedmin", "CFDATA_OFFSPEEDMIN", &cfg.speedMin)
}

func defaultCLIExportConfig() cliExportConfig {
	return cliExportConfig{Format: "txt", Fields: "compact", Custom: "", V6Bracket: true, GitHub: false, GHBranch: "main", GHPath: "", GHMessage: "update cfdata results"}
}

func defaultCLIFileConfig() cliFileConfig {
	return cliFileConfig{SkipGeo: false, ScanMode: "tcping", IPType: 4, Threads: 100, Out: "ip.csv", Progress: true, NoColor: false, URL: autoSpeedURLValue, DNS: defaultDNSServers, Debug: false, CompactIPv4: false, TestPort: 443, Delay: 500, DC: "", SpeedLimit: 5, SpeedMin: 0.1, Format: "txt", Fields: "compact", Custom: "", V6Bracket: true, GitHub: false, GHBranch: "main", GHPath: "", GHMessage: "update cfdata results"}
}

func (c cliFileConfig) Export() cliExportConfig {
	return cliExportConfig{Format: c.Format, Fields: c.Fields, Custom: c.Custom, V6Bracket: c.V6Bracket, V6BracketSet: true, GitHub: c.GitHub, GitHubSet: true, GHRepo: c.GHRepo, GHBranch: c.GHBranch, GHPath: c.GHPath, GHMessage: c.GHMessage, GHToken: c.GHToken, GHTokenFile: c.GHTokenFile, GHUpload: c.GHUpload}
}

type cliExportConfigTemplate struct {
	ConfigVersion   any              `json:"_config_version"`
	Config          cliFileConfig    `json:"config"`
	Description     string           `json:"_description"`
	Priority        string           `json:"_priority"`
	Usage           string           `json:"_usage"`
	ConfigHelp      []cliConfigHelp  `json:"_config_help"`
	FormatValues    []string         `json:"_format_values"`
	FieldsValues    []string         `json:"_fields_values"`
	AvailableFields []cliResultField `json:"_available_fields"`
}

type cliConfigHelp struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Default     string   `json:"default"`
	Options     []string `json:"options,omitempty"`
}

func mergeCLIExportConfig(dst *cliExportConfig, src cliExportConfig, onlyProvided bool, provided ...map[string]bool) {
	isSet := func(flagName string, value string) bool {
		if onlyProvided {
			return len(provided) > 0 && provided[0][flagName]
		}
		return strings.TrimSpace(value) != ""
	}
	if isSet("config", src.ConfigFile) {
		dst.ConfigFile = src.ConfigFile
	}
	if isSet("format", src.Format) {
		dst.Format = src.Format
	}
	if isSet("fields", src.Fields) {
		dst.Fields = src.Fields
	}
	if isSet("custom", src.Custom) {
		dst.Custom = src.Custom
	}
	if (!onlyProvided && src.V6BracketSet) || (onlyProvided && len(provided) > 0 && provided[0]["v6bracket"]) {
		dst.V6Bracket = src.V6Bracket
		dst.V6BracketSet = true
	}
	if (!onlyProvided && src.GitHubSet) || (onlyProvided && len(provided) > 0 && provided[0]["github"]) {
		dst.GitHub = src.GitHub
		dst.GitHubSet = true
	}
	if isSet("ghrepo", src.GHRepo) {
		dst.GHRepo = src.GHRepo
	}
	if isSet("ghbranch", src.GHBranch) {
		dst.GHBranch = src.GHBranch
	}
	if isSet("ghpath", src.GHPath) {
		dst.GHPath = src.GHPath
	}
	if isSet("ghmessage", src.GHMessage) {
		dst.GHMessage = src.GHMessage
	}
	if isSet("ghtoken", src.GHToken) {
		dst.GHToken = src.GHToken
	}
	if isSet("ghtokenfile", src.GHTokenFile) {
		dst.GHTokenFile = src.GHTokenFile
	}
	if isSet("ghupload", src.GHUpload) {
		dst.GHUpload = src.GHUpload
	}
}

func loadOrCreateCLIConfig(path string) (cliFileConfig, bool, error) {
	path = expandHome(path)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return cliFileConfig{}, false, err
		}
		if err := writeCLIConfigTemplate(path, defaultCLIFileConfig()); err != nil {
			return cliFileConfig{}, false, err
		}
		return cliFileConfig{}, true, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return cliFileConfig{}, false, err
	}
	cfg := defaultCLIFileConfig()
	if len(strings.TrimSpace(string(data))) == 0 {
		return cfg, false, nil
	}

	needsRewrite := false
	var rawMap map[string]interface{}
	if err := json.Unmarshal(data, &rawMap); err == nil {
		inner, hasNested := rawMap["config"]
		if hasNested {
			if m, ok := inner.(map[string]interface{}); ok {
				if migrateConfigKeys(m) {
					needsRewrite = true
				}
			}
		} else {
			if migrateConfigKeys(rawMap) {
				needsRewrite = true
			}
		}
		if needsRewrite {
			data, _ = json.Marshal(rawMap)
		}
	}

	template := cliExportConfigTemplate{Config: defaultCLIFileConfig()}
	needsRewrite = false
	if err := json.Unmarshal(data, &template); err == nil && (template.Config.ScanMode != "" || template.Config.Format != "" || template.Description != "") {
		cfg = template.Config
		needsRewrite = cliConfigVersionIsOld(template.ConfigVersion)
	} else if err := json.Unmarshal(data, &cfg); err != nil {
		return cliFileConfig{}, false, fmt.Errorf("解析配置文件失败 %s: %w", path, err)
	} else {
		needsRewrite = true
	}
	cfg = migrateCLIFileConfig(cfg, template.ConfigVersion)
	if needsRewrite {
		if err := writeCLIConfigTemplate(path, cfg); err != nil {
			return cliFileConfig{}, false, err
		}
	}
	return cfg, false, nil
}

func newCLIConfigTemplate(cfg cliFileConfig) cliExportConfigTemplate {
	return cliExportConfigTemplate{
		ConfigVersion:   appVersion,
		Config:          cfg,
		Description:     "CFData 官方优选 CLI 配置；真正配置项在 config 内。",
		Priority:        "命令行参数 > 配置文件 > 环境变量 > 默认值",
		Usage:           "首次生成后建议退出并编辑本文件，再重新运行。定时任务请设置 skipgeo=true。debug 支持 false、error、all、true。",
		ConfigHelp:      buildCLIConfigHelp(),
		FormatValues:    []string{"csv", "txt"},
		FieldsValues:    []string{"compact", "all", "ipport", "ipport,dc,loc", "ipport,latency,dc,loc"},
		AvailableFields: cliResultFields,
	}
}

func writeCLIConfigTemplate(path string, cfg cliFileConfig) error {
	template := newCLIConfigTemplate(cfg)
	var buf strings.Builder
	encoder := json.NewEncoder(&buf)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(template); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(buf.String()), 0600)
}

func migrateCLIFileConfig(cfg cliFileConfig, version any) cliFileConfig {
	if cliConfigVersionIsOld(version) {
		if v, ok := cfg.Debug.(bool); ok && v {
			cfg.Debug = "error"
		}
	}
	return cfg
}

func migrateConfigKeys(m map[string]interface{}) bool {
	migrations := map[string]string{
		"iptype":     "offiptype",
		"testport":   "offport",
		"speedlimit": "offspeedlimit",
		"speedmin":   "offspeedmin",
		"dc":         "offdc",
	}
	sharedKeys := []string{"threads", "delay", "out", "url"}
	changed := false
	for oldKey, newKey := range migrations {
		if v, ok := m[oldKey]; ok {
			if _, exists := m[newKey]; !exists {
				m[newKey] = v
				changed = true
			}
		}
	}
	for _, key := range sharedKeys {
		if v, ok := m[key]; ok {
			offKey := "off" + key
			if _, exists := m[offKey]; !exists {
				m[offKey] = v
				changed = true
			}
		}
	}
	return changed
}

func cliConfigVersionIsOld(version any) bool {
	switch v := version.(type) {
	case string:
		return strings.TrimSpace(v) == "" || strings.TrimSpace(v) != appVersion
	case float64:
		return v < 2
	case int:
		return v < 2
	default:
		return true
	}
}

func buildCLIConfigHelp() []cliConfigHelp {
	return []cliConfigHelp{
		{Name: "skipgeo", Description: "跳过地区/代理环境验证；定时任务/无人值守必须设为 true，否则检测到非直连环境时会等待交互输入", Default: "false", Options: []string{"true", "false"}},
		{Name: "scanmode", Description: "扫描方式；tcping：仅测量 TCP 握手延迟（默认），httping：测量 HTTP TTFB 全链路延迟，延迟比 tcping 高属正常，不同模式数据不可互相比较", Default: "tcping", Options: []string{"tcping", "httping"}},
		{Name: "offiptype", Description: "IP 类型", Default: "4", Options: []string{"4", "6"}},
		{Name: "offthreads", Description: "扫描并发数；路由器/N1 内存较小建议 50 左右", Default: "100"},
		{Name: "offout", Description: "本地输出文件名（只含测速合格 IP）", Default: "ip.csv"},
		{Name: "progress", Description: "输出进度日志；日志重定向到文件时建议 false", Default: "true", Options: []string{"true", "false"}},
		{Name: "nocolor", Description: "禁用 ANSI 颜色输出；日志重定向到文件时建议 true", Default: "false", Options: []string{"true", "false"}},
		{Name: "offurl", Description: "测速下载地址；auto 表示由程序自动选择内置测速源", Default: autoSpeedURLValue},
		{Name: "dns", Description: "自定义 DNS 服务器；默认系统 DNS 优先，失败回退内置 DNS；显式设置时强制使用指定 DNS", Default: defaultDNSServers},
		{Name: "debug", Description: "调试输出等级；error 记录程序错误和下载/API 异常，all 额外包含测速失败等全部明细", Default: "false", Options: []string{"false", "error", "all", "true"}},
		{Name: "compactipv4", Description: "精简本地 IPv4 地址库并覆盖 ips-v4.txt", Default: "false", Options: []string{"true", "false"}},
		{Name: "offport", Description: "详细测试与测速端口", Default: "443"},
		{Name: "offdelay", Description: "延迟阈值，单位毫秒", Default: "500"},
		{Name: "offdc", Description: "指定数据中心；留空自动选择最低延迟数据中心", Default: ""},
		{Name: "offspeedlimit", Description: "测速达标结果上限，必须大于 0；只导出/上传达标 IP", Default: "5"},
		{Name: "offspeedmin", Description: "测速达标下限，单位 MB/s", Default: "0.1"},
		{Name: "format", Description: "导出/上传内容格式", Default: "txt", Options: []string{"csv", "txt"}},
		{Name: "fields", Description: "导出字段；支持 compact、all、ipport 或逗号分隔字段 key；自定义字段可写在这里排序", Default: "compact", Options: []string{"compact", "all", "ipport", "ipport,dc,loc", "ipport,latency,dc,loc"}},
		{Name: "custom", Description: "自定义导出字段，格式 标题:内容，多项用逗号分隔；未在 fields 中排序时默认追加到最后", Default: ""},
		{Name: "v6bracket", Description: "TXT 导出时对 IPv6 地址加方括号（[IPv6]:端口），仅对 IPv6 行生效", Default: "true", Options: []string{"true", "false"}},
		{Name: "github", Description: "导出后上传到 GitHub（仅上传测速合格 IP）", Default: "false", Options: []string{"true", "false"}},
		{Name: "ghrepo", Description: "GitHub 仓库，格式 owner/repo", Default: ""},
		{Name: "ghbranch", Description: "GitHub 分支", Default: "main"},
		{Name: "ghpath", Description: "GitHub 目标路径；留空时按 format 自动使用 results/ip.csv 或 results/ip.txt；文件不存在会新建，存在会覆盖", Default: "自动按 format 生成"},
		{Name: "ghmessage", Description: "GitHub 提交信息", Default: "update cfdata results"},
		{Name: "ghtoken", Description: "GitHub token；不推荐直接写入配置，强烈建议使用仅限制指定仓库读写权限的 token", Default: ""},
		{Name: "ghtokenfile", Description: "GitHub token 文件路径，文件内 token 建议仅限制指定仓库读写权限", Default: ""},
		{Name: "ghupload", Description: "快速上传指定文件到 GitHub，不执行测试；需 github=true", Default: ""},
	}
}

func applyCLIFileConfig(cfg *cliConfig, fileCfg cliFileConfig, provided map[string]bool) {
	setString := func(name string, target *string, value string) {
		if !provided[name] && strings.TrimSpace(value) != "" {
			*target = value
		}
	}
	setInt := func(name string, target *int, value int) {
		if !provided[name] {
			*target = value
		}
	}
	setFloat := func(name string, target *float64, value float64) {
		if !provided[name] {
			*target = value
		}
	}
	if !provided["skipgeo"] && fileCfg.SkipGeo {
		skipGeoCheck = true
	}
	setString("scanmode", &cfg.scanMode, fileCfg.ScanMode)
	setInt("offiptype", &cfg.ipType, fileCfg.IPType)
	setInt("offthreads", &cfg.threads, fileCfg.Threads)
	setString("offout", &cfg.outFile, fileCfg.Out)
	if !provided["progress"] {
		cfg.showProgress = fileCfg.Progress
	}
	if !provided["nocolor"] {
		cfg.noColor = fileCfg.NoColor
	}
	if !provided["offurl"] && !provided["url"] && strings.TrimSpace(fileCfg.URL) != "" {
		speedTestURL = fileCfg.URL
	}
	if !provided["dns"] && strings.TrimSpace(fileCfg.DNS) != "" {
		customDNSServer = fileCfg.DNS
	}
	if provided["dns"] {
		customDNSForced = true
	}
	if !provided["debug"] {
		applyConfigDebug(fileCfg.Debug)
	}
	if !provided["compactipv4"] {
		cfg.compactIPv4 = fileCfg.CompactIPv4
	}
	setInt("offport", &cfg.port, fileCfg.TestPort)
	setInt("offdelay", &cfg.delay, fileCfg.Delay)
	if !provided["offdc"] {
		cfg.dc = fileCfg.DC
	}
	setInt("offspeedlimit", &cfg.speedLimit, fileCfg.SpeedLimit)
	setFloat("offspeedmin", &cfg.speedMin, fileCfg.SpeedMin)
}

func defaultCLIConfigPath() string {
	exe, err := os.Executable()
	if err != nil || exe == "" {
		return "cfdata-config.json"
	}
	return filepath.Join(filepath.Dir(exe), "cfdata-config.json")
}

func expandHome(path string) string {
	path = strings.TrimSpace(path)
	if path == "~" {
		home, _ := os.UserHomeDir()
		return home
	}
	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func parseBoolEnv(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func applyConfigDebug(value any) {
	switch v := value.(type) {
	case bool:
		if v {
			_ = setDebugFlag("error")
		} else {
			_ = setDebugFlag("false")
		}
	case string:
		_ = setDebugFlag(v)
	default:
		_ = setDebugFlag("false")
	}
}

func newCLISession(cfg *cliConfig) *appSession {
	session := &appSession{progressState: map[string][2]int{}}
	session.emit = func(msgType string, data interface{}) {
		handleCLIMessage(cfg, session, msgType, data)
	}
	return session
}

func handleCLIMessage(cfg *cliConfig, session *appSession, msgType string, data interface{}) {
	debugWrongType := func(expected string) {
		if debugMode {
			fmt.Fprintf(os.Stderr, "%s[cli-debug]%s 消息 %s 类型断言失败，期望 %s，实际 %T\n", ansiYellow, ansiReset, msgType, expected, data)
		}
	}
	switch msgType {
	case "log":
		fmt.Println(data)
	case "error":
		recordProgramDebugError("cli_message_error", data)
		fmt.Fprintln(os.Stderr, colorize(fmt.Sprint(data), ansiRed))
	case "scan_progress":
		if cfg.showProgress {
			m, ok := data.(map[string]interface{})
			if !ok {
				debugWrongType("map[string]interface{}")
				break
			}
			current := asInt(m["current"])
			total := asInt(m["total"])
			setCLIProgress(session, "scan", current, total)
		}
	case "test_progress":
		if cfg.showProgress {
			m, ok := data.(map[string]interface{})
			if !ok {
				debugWrongType("map[string]interface{}")
				break
			}
			current := asInt(m["current"])
			total := asInt(m["total"])
			setCLIProgress(session, "test", current, total)
		}

	case "scan_result":
		res, ok := data.(ScanResult)
		if !ok {
			debugWrongType("ScanResult")
			break
		}
		fmt.Printf("%s[scan-result]%s %s %s:%d %s %s %s\n", ansiMagenta, ansiReset, advanceCLIProgress(session, "scan"), res.IP, res.Port, res.DataCenter, res.City, colorizeLatencyString(res.LatencyStr))

	case "test_result":
		res, ok := data.(TestResult)
		if !ok {
			debugWrongType("TestResult")
			break
		}
		fmt.Printf("%s[test-result]%s %s %s loss=%s avg=%s\n", ansiMagenta, ansiReset, advanceCLIProgress(session, "test"), res.IP, colorizeLossRate(res.LossRate), colorizeLatencyMS(int(res.AvgLatency/time.Millisecond)))
	case "test_complete":
		results, ok := data.([]TestResult)
		if !ok {
			debugWrongType("[]TestResult")
			break
		}
		session.testMutex.Lock()
		session.testResults = append([]TestResult(nil), results...)
		session.testMutex.Unlock()
		fmt.Printf("%s[test-complete]%s %d results\n", ansiCyan, ansiReset, len(results))

	case "speed_test_result":
		m, ok := data.(map[string]string)
		if !ok {
			debugWrongType("map[string]string")
			break
		}
		endpoint := m["endpoint"]
		if endpoint == "" {
			endpoint = m["ip"]
		}
		fmt.Printf("%s[speed]%s %s %s\n", ansiMagenta, ansiReset, endpoint, colorizeSpeedString(m["speed"]))
	case "compact_ipv4_progress":
		if cfg.showProgress {
			m, ok := data.(map[string]interface{})
			if !ok {
				debugWrongType("map[string]interface{}")
				break
			}
			current := asInt(m["current"])
			total := asInt(m["total"])
			setCLIProgress(session, "compact", current, total)
			maybePrintCLIProgress(session, "compact", current, total)
		}
	case "compact_ipv4_hit":
		if !debugMode {
			break
		}
		m, ok := data.(map[string]interface{})
		if !ok {
			debugWrongType("map[string]interface{}")
			break
		}
		fmt.Printf("%s[compact-hit]%s pass=%v %v\n", ansiMagenta, ansiReset, m["pass"], m["ip"])
	case "compact_ipv4_done":
		m, ok := data.(map[string]interface{})
		if !ok {
			debugWrongType("map[string]interface{}")
			break
		}
		fmt.Printf("%s[compact-done]%s 保留 %v 个子网 → %v\n", ansiGreen, ansiReset, m["count"], m["file"])
	}
}

func runOfficialCLI(cfg *cliConfig) error {
	if cfg.ipType != 4 && cfg.ipType != 6 {
		return errors.New("-offiptype 仅支持 4 或 6")
	}
	if cfg.threads <= 0 {
		cfg.threads = 100
	}
	if cfg.port <= 0 {
		cfg.port = 443
	}
	if cfg.delay < 0 {
		cfg.delay = 0
	}
	if cfg.speedLimit <= 0 {
		return errors.New("精简版只导出/上传测速合格的 IP，-offspeedlimit 必须大于 0")
	}
	if cfg.speedMin <= 0 {
		cfg.speedMin = 0.1
	}

	scanMode := cfg.scanMode
	if scanMode == "" {
		scanMode = scanModeTCPing
	}

	session := newCLISession(cfg)
	if err := session.runTaskSync(func(ctx context.Context, session *appSession) {
		runOfficialTask(ctx, session, cfg.ipType, cfg.threads, cfg.port, cfg.delay, scanMode)
	}); err != nil {
		return cliTaskError(err)
	}

	session.scanMutex.Lock()
	scanResults := append([]ScanResult(nil), session.scanResults...)
	session.scanMutex.Unlock()
	if len(scanResults) == 0 {
		return errors.New("未发现有效 IP")
	}

	dc := strings.TrimSpace(cfg.dc)
	if dc == "" {
		dc = pickBestDataCenter(scanResults)
		if dc == "" {
			return errors.New("无法确定数据中心，已跳过导出与上传")
		}
		fmt.Printf("%s[official]%s 自动选择数据中心: %s\n", ansiGreen, ansiReset, colorize(dc, ansiBold+ansiGreen))
	}

	session.testMutex.Lock()
	session.testResults = nil
	session.testMutex.Unlock()
	if err := session.runTaskSync(func(ctx context.Context, session *appSession) {
		runDetailedTest(ctx, session, dc, cfg.port, cfg.delay, scanMode)
	}); err != nil {
		return cliTaskError(err)
	}

	session.testMutex.Lock()
	results := append([]TestResult(nil), session.testResults...)
	session.testMutex.Unlock()
	if len(results) == 0 {
		return errors.New("没有可用的详细测试结果，已跳过测速、导出与上传")
	}

	sortOfficialTestResults(results)
	setCLIProgress(session, "speed", 0, cfg.speedLimit)
	fmt.Printf("%s[official]%s 开始测速：目标上限=%d，测速阈值=%.2fMB/s\n", ansiGreen, ansiReset, cfg.speedLimit, cfg.speedMin)
	results = runOfficialSpeedTests(context.Background(), session, results, cfg.port, cfg.speedLimit, cfg.speedMin)

	rows := filterQualifiedRows(officialResultRows(scanResults, results, scanMode), cfg.speedMin)
	if len(rows) == 0 {
		return fmt.Errorf("没有测速合格的 IP（阈值 %.2fMB/s），已跳过导出与上传，远端文件保持不变", cfg.speedMin)
	}
	fmt.Printf("%s[official]%s 测速合格 %d 个 IP\n", ansiGreen, ansiReset, len(rows))
	return writeCLIExportAndMaybeUpload(cfg, rows, "official")
}

func scanModeLabel(scanMode string) string {
	if scanMode == scanModeHTTPing {
		return "HTTPing"
	}
	return "TCPing"
}

func filterQualifiedRows(rows []cliResultRow, speedMin float64) []cliResultRow {
	filtered := make([]cliResultRow, 0, len(rows))
	for _, row := range rows {
		if value, ok := parseSpeedMBForSort(row["speed"]); ok && value >= speedMin {
			filtered = append(filtered, row)
		}
	}
	return filtered
}

func cliTaskError(err error) error {
	if errors.Is(err, context.Canceled) {
		return errors.New("已有任务正在运行，请等待完成后再试")
	}
	return err
}

func pickBestDataCenter(scanResults []ScanResult) string {
	dcLatency := map[string]time.Duration{}
	for _, res := range scanResults {
		current, ok := dcLatency[res.DataCenter]
		if !ok || res.TCPDuration < current {
			dcLatency[res.DataCenter] = res.TCPDuration
		}
	}
	type item struct {
		dc      string
		latency time.Duration
	}
	items := make([]item, 0, len(dcLatency))
	for dc, latency := range dcLatency {
		items = append(items, item{dc: dc, latency: latency})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].latency < items[j].latency })
	if len(items) == 0 {
		return ""
	}
	return items[0].dc
}

func runOfficialSpeedTests(ctx context.Context, session *appSession, results []TestResult, port int, limit int, speedMinMB float64) []TestResult {
	updated, _ := runOfficialSpeedTestsCore(ctx, results, port, limit, speedMinMB, speedTestURL, func(current, total, qualifiedCount int, result TestResult) {
		setCLIProgress(session, "speed", min(qualifiedCount, limit), limit)
		fmt.Printf("%s[speed]%s %s %s:%d %s\n", ansiMagenta, ansiReset, renderCLIProgress(session, "speed"), result.IP, port, colorizeSpeedString(result.Speed))
		if speedMB, ok := parseSpeedMBForSort(result.Speed); ok && speedMB >= speedMinMB {
			fmt.Printf("%s[official]%s 达标 %d/%d\n", ansiGreen, ansiReset, qualifiedCount, limit)
		}
	}, func() {
		fmt.Printf("%s[official]%s %s\n", ansiYellow, ansiReset, speedRateLimitMessage)
	})
	return updated
}

func printCLIConfig(cfg *cliConfig) {
	type item struct {
		name         string
		description  string
		value        string
		defaultValue string
	}
	printGroup := func(title string, rows []item) {
		fmt.Println(colorize("----------------------------------------", ansiCyan))
		fmt.Println(colorize(title, ansiBold+ansiCyan))
		for _, row := range rows {
			fmt.Printf("%s-%s%s %s\n", ansiBold, row.name, ansiReset, colorizeCLIParamValue(row.value, row.defaultValue))
			fmt.Printf("  %s %s\n", colorize("说明:", ansiYellow), row.description)
			fmt.Printf("  %s %s\n", colorize("默认:", ansiYellow), colorizeCLIDefaultValue(row.defaultValue))
		}
	}

	fmt.Printf("%s %s\n", colorize("CFData 官方优选 CLI 版本:", ansiBold+ansiGreen), appVersion)
	fmt.Println(colorize("[cli-config] 当前命令参数", ansiBold+ansiGreen))
	printGroup("通用参数", []item{
		{"skipgeo", lookupCLIFlagDescription(cliCommonFlags, "skipgeo"), strconv.FormatBool(skipGeoCheck), "false"},
		{"offout", lookupCLIFlagDescription(cliCommonFlags, "offout"), cfg.outFile, "ip.csv"},
		{"progress", lookupCLIFlagDescription(cliCommonFlags, "progress"), strconv.FormatBool(cfg.showProgress), "true"},
		{"nocolor", lookupCLIFlagDescription(cliCommonFlags, "nocolor"), strconv.FormatBool(cfg.noColor), "false"},
		{"debug", lookupCLIFlagDescription(cliCommonFlags, "debug"), debugFlagValue{}.String(), "false"},
		{"compactipv4", lookupCLIFlagDescription(cliCommonFlags, "compactipv4"), strconv.FormatBool(cfg.compactIPv4), "false"},
		{"config", lookupCLIFlagDescription(cliCommonFlags, "config"), cfg.export.ConfigFile, "二进制目录/cfdata-config.json"},
		{"format", lookupCLIFlagDescription(cliCommonFlags, "format"), cfg.export.Format, "txt"},
		{"fields", lookupCLIFlagDescription(cliCommonFlags, "fields"), cfg.export.Fields, "compact"},
		{"custom", lookupCLIFlagDescription(cliCommonFlags, "custom"), cfg.export.Custom, ""},
		{"github", lookupCLIFlagDescription(cliCommonFlags, "github"), strconv.FormatBool(cfg.export.GitHub), "false"},
		{"ghrepo", lookupCLIFlagDescription(cliCommonFlags, "ghrepo"), cfg.export.GHRepo, ""},
		{"ghbranch", lookupCLIFlagDescription(cliCommonFlags, "ghbranch"), cfg.export.GHBranch, "main"},
		{"ghpath", lookupCLIFlagDescription(cliCommonFlags, "ghpath"), cfg.export.GHPath, "<自动>"},
		{"ghmessage", lookupCLIFlagDescription(cliCommonFlags, "ghmessage"), cfg.export.GHMessage, "update cfdata results"},
		{"ghtoken", lookupCLIFlagDescription(cliCommonFlags, "ghtoken"), maskSecret(cfg.export.GHToken), ""},
		{"ghtokenfile", lookupCLIFlagDescription(cliCommonFlags, "ghtokenfile"), cfg.export.GHTokenFile, ""},
		{"ghupload", lookupCLIFlagDescription(cliCommonFlags, "ghupload"), cfg.export.GHUpload, ""},
	})
	printGroup("官方优选参数", []item{
		{"scanmode", lookupCLIFlagDescription(cliOfficialFlags, "scanmode"), cfg.scanMode, "tcping"},
		{"offiptype", lookupCLIFlagDescription(cliOfficialFlags, "offiptype"), strconv.Itoa(cfg.ipType), "4"},
		{"offthreads", lookupCLIFlagDescription(cliOfficialFlags, "offthreads"), strconv.Itoa(cfg.threads), "100"},
		{"offport", lookupCLIFlagDescription(cliOfficialFlags, "offport"), strconv.Itoa(cfg.port), "443"},
		{"offdelay", lookupCLIFlagDescription(cliOfficialFlags, "offdelay"), strconv.Itoa(cfg.delay), "500"},
		{"offdc", lookupCLIFlagDescription(cliOfficialFlags, "offdc"), cfg.dc, ""},
		{"offurl", lookupCLIFlagDescription(cliOfficialFlags, "offurl"), speedTestURL, autoSpeedURLValue},
		{"offspeedlimit", lookupCLIFlagDescription(cliOfficialFlags, "offspeedlimit"), strconv.Itoa(cfg.speedLimit), "5"},
		{"offspeedmin", lookupCLIFlagDescription(cliOfficialFlags, "offspeedmin"), fmt.Sprintf("%.2f", cfg.speedMin), "0.1"},
	})
	fmt.Println(colorize("----------------------------------------", ansiCyan))
}

func maskSecret(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	if len(value) <= 8 {
		return "********"
	}
	return value[:4] + "..." + value[len(value)-4:]
}

func printCLIUsage() {
	out := flag.CommandLine.Output()
	fmt.Fprintf(out, "%s\n", colorize("CFData 官方优选 CLI 帮助", ansiBold+ansiGreen))
	fmt.Fprintf(out, "版本: %s\n\n", appVersion)
	fmt.Fprintf(out, "用法: ./cfdata-linux-amd64 [参数]\n")
	fmt.Fprintf(out, "首次运行会在二进制目录生成 cfdata-config.json，编辑后重新运行即可。\n")
	fmt.Fprintf(out, "注意: Go 的布尔参数必须写成 -skipgeo 或 -skipgeo=true，不能写成 -skipgeo true。\n\n")
	printCLIUsageGroup("通用参数", cliCommonFlags)
	printCLIUsageGroup("官方优选参数", cliOfficialFlags)
}

func printCLIUsageGroup(title string, rows []cliFlagInfo) {
	fmt.Fprintf(flag.CommandLine.Output(), "%s\n", colorize("----------------------------------------", ansiCyan))
	fmt.Fprintf(flag.CommandLine.Output(), "%s\n", colorize(title, ansiBold+ansiCyan))
	for _, row := range rows {
		fmt.Fprintf(flag.CommandLine.Output(), "%s-%s%s\n", ansiBold, row.name, ansiReset)
		fmt.Fprintf(flag.CommandLine.Output(), "  %s %s\n", colorize("说明:", ansiYellow), row.description)
		fmt.Fprintf(flag.CommandLine.Output(), "  %s %s\n", colorize("默认:", ansiYellow), colorizeCLIDefaultValue(row.defaultValue))
	}
}

func lookupCLIFlagDescription(rows []cliFlagInfo, name string) string {
	for _, row := range rows {
		if row.name == name {
			return row.description
		}
	}
	return ""
}

func colorize(text string, code string) string {
	if text == "" {
		return text
	}
	return code + text + ansiReset
}

func colorizeLatencyString(latency string) string {
	ms, err := strconv.Atoi(strings.TrimSuffix(strings.TrimSpace(latency), " ms"))
	if err != nil {
		return latency
	}
	return colorizeLatencyMS(ms)
}

func colorizeLatencyMS(ms int) string {
	text := fmt.Sprintf("%dms", ms)
	if ms <= 50 {
		return colorize(text, ansiGreen)
	}
	if ms <= 100 {
		return colorize(text, ansiBrightGreen)
	}
	if ms <= 200 {
		return colorize(text, ansiYellow)
	}
	if ms <= 250 {
		return colorize(text, ansiYellow)
	}
	if ms <= 3000 {
		return colorize(text, ansiYellow)
	}
	return colorize(text, ansiRed)
}

func colorizeLossRate(lossRate float64) string {
	text := fmt.Sprintf("%.0f%%", lossRate*100)
	if lossRate <= 0 {
		return colorize(text, ansiGreen)
	}
	if lossRate < 0.5 {
		return colorize(text, ansiYellow)
	}
	return colorize(text, ansiRed)
}

func colorizeSpeedString(speed string) string {
	if strings.Contains(speed, "MB/s") {
		value, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(speed, "MB/s")), 64)
		if err == nil {
			if value > 10 {
				return colorize(speed, ansiGreen)
			}
			return colorize(speed, ansiYellow)
		}
	}
	if strings.Contains(strings.ToLower(speed), "错误") || strings.Contains(speed, "失败") || strings.Contains(speed, "0MB/s") {
		return colorize(speed, ansiRed)
	}
	return speed
}

func colorizeCLIParamValue(value string, defaultValue string) string {
	if value == defaultValue {
		if value == "" {
			return colorize("<空>", ansiGreen)
		}
		return colorize(value, ansiGreen)
	}
	if value == "" {
		return colorize("<空>", ansiYellow)
	}
	if value == "true" {
		return colorize(value, ansiGreen)
	}
	if value == "false" {
		return colorize(value, ansiRed)
	}
	return colorize(value, ansiMagenta)
}

func colorizeCLIDefaultValue(value string) string {
	if value == "" {
		return colorize("<空>", ansiYellow)
	}
	return colorize(value, ansiGreen)
}

func setCLIProgress(session *appSession, phase string, current int, total int) {
	session.progressMutex.Lock()
	defer session.progressMutex.Unlock()
	if session.progressState == nil {
		session.progressState = map[string][2]int{}
	}
	state := session.progressState[phase]
	if total <= 0 {
		total = state[1]
	}
	if current < state[0] {
		current = state[0]
	}
	session.progressState[phase] = [2]int{current, total}
}

func maybePrintCLIProgress(session *appSession, phase string, current, total int) {
	if total <= 0 {
		return
	}
	session.progressMutex.Lock()
	if session.progressPrintTime == nil {
		session.progressPrintTime = map[string]time.Time{}
	}
	if session.progressPrintPercent == nil {
		session.progressPrintPercent = map[string]float64{}
	}
	now := time.Now()
	percent := float64(current) / float64(total) * 100
	lastTime := session.progressPrintTime[phase]
	lastPercent := session.progressPrintPercent[phase]

	shouldPrint := false
	switch {
	case lastTime.IsZero():
		shouldPrint = true
	case current >= total:
		shouldPrint = true
	case percent-lastPercent >= 5.0:
		shouldPrint = true
	case now.Sub(lastTime) >= 3*time.Second:
		shouldPrint = true
	}

	if shouldPrint {
		session.progressPrintTime[phase] = now
		session.progressPrintPercent[phase] = percent
	}
	session.progressMutex.Unlock()

	if !shouldPrint {
		return
	}
	fmt.Printf("%s[%s-progress]%s %s\n", ansiCyan, phase, ansiReset, colorize(fmt.Sprintf("[%d/%d %.2f%%]", current, total, percent), ansiCyan))
}

func renderCLIProgress(session *appSession, phase string) string {
	session.progressMutex.Lock()
	defer session.progressMutex.Unlock()
	if session.progressState == nil {
		return colorize("[0/0]", ansiCyan)
	}
	state, ok := session.progressState[phase]
	if !ok {
		return colorize("[0/0]", ansiCyan)
	}
	if state[1] <= 0 {
		return colorize(fmt.Sprintf("[%d/0]", state[0]), ansiCyan)
	}
	percent := float64(state[0]) / float64(state[1]) * 100
	return colorize(fmt.Sprintf("[%d/%d %.2f%%]", state[0], state[1], percent), ansiCyan)
}

func advanceCLIProgress(session *appSession, phase string) string {
	session.progressMutex.Lock()
	defer session.progressMutex.Unlock()
	if session.progressState == nil {
		session.progressState = map[string][2]int{}
	}
	state := session.progressState[phase]
	if state[1] > 0 && state[0] < state[1] {
		state[0]++
		session.progressState[phase] = state
	}
	if state[1] <= 0 {
		return colorize(fmt.Sprintf("[%d/0]", state[0]), ansiCyan)
	}
	percent := float64(state[0]) / float64(state[1]) * 100
	return colorize(fmt.Sprintf("[%d/%d %.2f%%]", state[0], state[1], percent), ansiCyan)
}

func asInt(v interface{}) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case string:
		value, err := strconv.Atoi(n)
		if err == nil {
			return value
		}
	}
	return 0
}

func writeCLIExportAndMaybeUpload(cfg *cliConfig, rows []cliResultRow, mode string) error {
	if len(rows) == 0 {
		return nil
	}
	content, err := formatCLIResults(rows, cfg.export)
	if err != nil {
		return err
	}
	filename := cfg.outFile
	if strings.TrimSpace(filename) == "" {
		filename = "ip." + cfg.export.Format
	}
	if ext := "." + cfg.export.Format; !strings.HasSuffix(strings.ToLower(filename), ext) {
		filename = strings.TrimSuffix(filename, filepath.Ext(filename)) + ext
	}
	filename = safeFilename(filename)
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := writeUTF8BOM(file); err != nil {
		os.Remove(filename)
		return err
	}

	if _, err := io.WriteString(file, content); err != nil {
		return err
	}
	fmt.Printf("[%s-output] %s (%d rows, %s)\n", mode, filename, len(rows), cfg.export.Format)
	if cfg.export.GitHub {
		if err := uploadCLIExportToGitHub(cfg, content); err != nil {
			return err
		}
	}
	return nil
}

func formatCLIResults(rows []cliResultRow, cfg cliExportConfig) (string, error) {
	customFields := parseCLICustomFields(cfg.Custom)
	fields := resolveCLIFields(cfg.Fields, cfg.Format, rows, customFields)
	rows = applyCLICustomFields(rows, customFields)
	if cfg.Format == "txt" {
		var b strings.Builder
		for _, row := range rows {
			ipport := row["ipport"]
			if ip := strings.TrimSpace(row["ip"]); ip != "" {
				if cfg.V6Bracket && strings.Contains(ip, ":") {
					ipport = "[" + ip + "]:" + row["port"]
				} else {
					ipport = ip + ":" + row["port"]
				}
			} else if ipport == "" {
				ipport = row["ip"] + ":" + row["port"]
			}
			extras := make([]string, 0, len(fields))
			for _, field := range fields {
				if field == "ipport" {
					continue
				}
				if value := strings.TrimSpace(row[field]); value != "" {
					extras = append(extras, value)
				}
			}
			b.WriteString(ipport)
			if len(extras) > 0 {
				b.WriteString("#")
				b.WriteString(strings.Join(extras, "-"))
			}
			b.WriteString("\n")
		}
		return b.String(), nil
	}

	var b strings.Builder
	writer := csv.NewWriter(&b)
	headers := make([]string, 0, len(fields))
	for _, field := range fields {
		headers = append(headers, cliFieldLabel(field, customFields))
	}
	if err := writer.Write(headers); err != nil {
		return "", err
	}
	for _, row := range rows {
		values := make([]string, 0, len(fields))
		for _, field := range fields {
			values = append(values, row[field])
		}
		if err := writer.Write(values); err != nil {
			return "", err
		}
	}
	writer.Flush()
	return b.String(), writer.Error()
}

func resolveCLIFields(spec, format string, rows []cliResultRow, customFields []cliCustomField) []string {
	spec = strings.TrimSpace(strings.ToLower(spec))
	customByKey := map[string]bool{}
	for _, field := range customFields {
		customByKey[field.Key] = true
	}
	appendCustomFields := func(fields []string) []string {
		seen := map[string]bool{}
		result := make([]string, 0, len(fields)+len(customFields))
		for _, field := range fields {
			if field == "" || seen[field] {
				continue
			}
			result = append(result, field)
			seen[field] = true
		}
		for _, field := range customFields {
			if !seen[field.Key] {
				result = append(result, field.Key)
			}
		}
		return result
	}
	if spec == "" || spec == "compact" {
		if format == "txt" {
			return appendCustomFields([]string{"ipport", "dc", "loc"})
		}
		if rowsAreOfficial(rows) {
			if rowsHaveField(rows, "speed") {
				return appendCustomFields([]string{"ip", "port", "latency", "speed", "dc", "region", "city"})
			}
			return appendCustomFields([]string{"ip", "port", "latency", "dc", "region", "city"})
		}
		return appendCustomFields([]string{"ip", "port", "tls", "latency", "speed", "outboundIP", "ipType", "originalInput", "dc", "loc", "region", "city", "asnNumber", "asnOrg"})
	}
	if spec == "ipport" {
		return appendCustomFields([]string{"ipport"})
	}
	if spec == "all" {
		fields := make([]string, 0, len(cliResultFields))
		for _, field := range cliResultFields {
			if field.Key == "ipport" {
				continue
			}
			for _, row := range rows {
				if strings.TrimSpace(row[field.Key]) != "" {
					fields = append(fields, field.Key)
					break
				}
			}
		}
		return appendCustomFields(fields)
	}
	parts := strings.Split(spec, ",")
	fields := make([]string, 0, len(parts))
	valid := map[string]string{"ipport": "ipport"}
	for _, field := range cliResultFields {
		valid[strings.ToLower(field.Key)] = field.Key
	}
	for _, part := range parts {
		field := strings.TrimSpace(part)
		fieldLower := strings.ToLower(field)
		if field != "" && (valid[fieldLower] != "" || customByKey[fieldLower]) {
			if customByKey[fieldLower] {
				field = fieldLower
			} else {
				field = valid[fieldLower]
			}
			fields = append(fields, field)
		}
	}
	if len(fields) == 0 {
		return appendCustomFields([]string{"ipport"})
	}
	return appendCustomFields(fields)
}

func rowsAreOfficial(rows []cliResultRow) bool {
	if len(rows) == 0 {
		return false
	}
	for _, row := range rows {
		if hasAnyCLIField(row, "tls", "outboundIP", "ipType", "loc", "asnNumber", "asnOrg", "visitScheme", "tlsVersion", "sni", "httpVersion", "warp", "gateway", "rbi", "kex", "timestamp") {
			return false
		}
	}
	return true
}

func rowsHaveField(rows []cliResultRow, field string) bool {
	for _, row := range rows {
		if strings.TrimSpace(row[field]) != "" {
			return true
		}
	}
	return false
}

func hasAnyCLIField(row cliResultRow, fields ...string) bool {
	for _, field := range fields {
		if strings.TrimSpace(row[field]) != "" {
			return true
		}
	}
	return false
}

func cliFieldLabel(key string, customFields []cliCustomField) string {
	for _, field := range customFields {
		if field.Key == key {
			return field.Label
		}
	}
	for _, field := range cliResultFields {
		if field.Key == key {
			return field.Label
		}
	}
	return key
}

func parseCLICustomFields(spec string) []cliCustomField {
	parts := strings.Split(spec, ",")
	fields := make([]cliCustomField, 0, len(parts))
	seen := map[string]int{}
	for _, part := range parts {
		item := strings.TrimSpace(part)
		key := ""
		if keyValue := strings.SplitN(item, "=", 2); len(keyValue) == 2 {
			key = strings.ToLower(strings.TrimSpace(keyValue[0]))
			item = strings.TrimSpace(keyValue[1])
		}
		labelValue := strings.SplitN(item, ":", 2)
		if len(labelValue) != 2 || strings.TrimSpace(labelValue[0]) == "" || strings.TrimSpace(labelValue[1]) == "" {
			continue
		}
		label := strings.TrimSpace(labelValue[0])
		if label == "" {
			label = key
		}
		if key == "" {
			key = strings.ToLower(label)
		}
		baseKey := key
		if count := seen[baseKey]; count > 0 {
			for {
				key = fmt.Sprintf("%s%d", baseKey, count)
				if seen[key] == 0 {
					break
				}
				count++
			}
		}
		fields = append(fields, cliCustomField{Key: key, Label: label, Value: strings.TrimSpace(labelValue[1])})
		seen[baseKey]++
		if key != baseKey {
			seen[key]++
		}
	}
	return fields
}

func applyCLICustomFields(rows []cliResultRow, fields []cliCustomField) []cliResultRow {
	if len(fields) == 0 {
		return rows
	}
	for _, row := range rows {
		for _, field := range fields {
			row[field.Key] = field.Value
		}
	}
	return rows
}

func officialScanRows(scanResults []ScanResult, scanMode string) []cliResultRow {
	rows := make([]cliResultRow, 0, len(scanResults))
	modeLabel := scanModeLabel(scanMode)
	for _, res := range scanResults {
		rows = append(rows, cliResultRow{"ip": res.IP, "port": strconv.Itoa(res.Port), "ipport": fmt.Sprintf("%s:%d", res.IP, res.Port), "dc": res.DataCenter, "dcCountry": res.DCCountry, "region": res.Region, "city": res.City, "latency": res.LatencyStr, "scanMode": modeLabel})
	}
	return rows
}

func officialResultRows(scanResults []ScanResult, testResults []TestResult, scanMode string) []cliResultRow {
	if len(testResults) == 0 {
		rows := officialScanRows(scanResults, scanMode)
		sortOfficialRows(rows)
		return rows
	}
	scanByIP := make(map[string]ScanResult, len(scanResults))
	for _, res := range scanResults {
		scanByIP[res.IP] = res
	}
	modeLabel := scanModeLabel(scanMode)
	rows := make([]cliResultRow, 0, len(testResults))
	for _, res := range testResults {
		scan := scanByIP[res.IP]
		port := scan.Port
		if port == 0 {
			port = res.Port
		}
		rows = append(rows, cliResultRow{"ip": res.IP, "port": strconv.Itoa(port), "ipport": fmt.Sprintf("%s:%d", res.IP, port), "dc": scan.DataCenter, "dcCountry": scan.DCCountry, "region": scan.Region, "city": scan.City, "latency": fmt.Sprintf("%dms", res.AvgLatency/time.Millisecond), "speed": res.Speed, "scanMode": modeLabel})
		if rows[len(rows)-1]["dc"] == "" {
			rows[len(rows)-1]["dc"] = res.DataCenter
			rows[len(rows)-1]["dcCountry"] = res.DCCountry
			rows[len(rows)-1]["region"] = res.Region
			rows[len(rows)-1]["city"] = res.City
		}
	}
	sortOfficialRows(rows)
	return rows
}

func sortOfficialRows(rows []cliResultRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		speedI, okI := parseSpeedMBForSort(rows[i]["speed"])
		speedJ, okJ := parseSpeedMBForSort(rows[j]["speed"])
		if okI != okJ {
			return okI
		}
		if okI && speedI != speedJ {
			return speedI > speedJ
		}
		latencyI := parseLatencyMSForSort(rows[i]["latency"])
		latencyJ := parseLatencyMSForSort(rows[j]["latency"])
		if latencyI != latencyJ {
			return latencyI < latencyJ
		}
		return rows[i]["ip"] < rows[j]["ip"]
	})
}

func parseSpeedMBForSort(value string) (float64, bool) {
	value = strings.TrimSpace(value)
	if !strings.Contains(value, "MB/s") {
		return 0, false
	}
	speed, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(value, "MB/s")), 64)
	if err != nil {
		return 0, false
	}
	return speed, true
}

func parseLatencyMSForSort(value string) float64 {
	latency, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(value, "ms")), 64)
	if err != nil {
		return math.MaxFloat64
	}
	return latency
}

func uploadCLIExportToGitHub(cfg *cliConfig, content string) error {
	parts := strings.Split(strings.TrimSpace(cfg.export.GHRepo), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return fmt.Errorf("-ghrepo 必须是 owner/repo")
	}
	if strings.TrimSpace(cfg.export.GHToken) == "" {
		return fmt.Errorf("启用 -github 时需要 -ghtoken、-ghtokenfile、CFDATA_GHTOKEN 或 GITHUB_TOKEN")
	}
	params := githubUploadRequest{Token: cfg.export.GHToken, Owner: parts[0], Repo: parts[1], Branch: cfg.export.GHBranch, Path: cfg.export.GHPath, Message: cfg.export.GHMessage, Content: content}
	downloadURL, err := uploadGitHubContentWithRetry(context.Background(), params, func(attempt, total int, err error) {
		if err == nil {
			fmt.Printf("%s[github]%s upload attempt %d/%d\n", ansiYellow, ansiReset, attempt, total)
			return
		}
		fmt.Printf("%s[github]%s upload attempt %d/%d failed: %s\n", ansiRed, ansiReset, attempt, total, err.Error())
	})
	if err != nil {
		return err
	}
	fmt.Printf("%s[github]%s uploaded %s\n", ansiGreen, ansiReset, downloadURL)
	return nil
}
