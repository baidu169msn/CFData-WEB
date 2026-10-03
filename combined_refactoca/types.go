package main

import (
	"context"
	"net"
	"sync"
	"time"
)

const (
	requestURL      = "speed.cloudflare.com/cdn-cgi/trace"
	scanModeTCPing  = "tcping"
	scanModeHTTPing = "httping"
)

func latencyMultiplier(scanMode string, isTLS bool) float64 {
	if scanMode != scanModeHTTPing {
		return 1.0
	}
	if isTLS {
		return 4.0
	}
	return 1.3
}

const (
	timeout     = 3 * time.Second
	maxDuration = 5 * time.Second
)

var (
	customDNSServer string
	customDNSForced bool
	customResolver  *net.Resolver
)

const defaultDNSServers = "223.5.5.5,8.8.8.8"

type location struct {
	Iata      string  `json:"iata"`
	Lat       float64 `json:"lat"`
	Lon       float64 `json:"lon"`
	Cca2      string  `json:"cca2"`
	Region    string  `json:"region"`
	City      string  `json:"city"`
	Region_zh string  `json:"region_zh"`
	Country   string  `json:"country"`
	City_zh   string  `json:"city_zh"`
	Emoji     string  `json:"emoji"`
}

type DataCenterInfo struct {
	DataCenter string
	DCCountry  string
	City       string
	IPCount    int
	MinLatency int
}

type ScanResult struct {
	IP          string
	Port        int
	DataCenter  string
	DCCountry   string
	Region      string
	City        string
	LatencyStr  string
	TCPDuration time.Duration
}

type TestResult struct {
	IP         string
	Port       int
	DataCenter string
	DCCountry  string
	Region     string
	City       string
	MinLatency time.Duration
	MaxLatency time.Duration
	AvgLatency time.Duration
	LossRate   float64
	Speed      string
}

type appSession struct {
	emit                 func(msgType string, data interface{})
	taskMutex            sync.Mutex
	isTaskRunning        bool
	taskCancel           context.CancelFunc
	scanMutex            sync.Mutex
	scanResults          []ScanResult
	testMutex            sync.Mutex
	testResults          []TestResult
	progressMutex        sync.Mutex
	progressState        map[string][2]int
	progressPrintTime    map[string]time.Time
	progressPrintPercent map[string]float64
}

type taskStarter func(ctx context.Context, session *appSession)

var (
	locationMap map[string]location

	listenPort       int
	listenHost       string
	speedTestURL     string
	speedTestWorkers = 5
	debugMode        bool
	debugLevel       = "error"
	skipGeoCheck     bool
)
