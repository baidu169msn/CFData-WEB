package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func isTLSPort(port int) bool {
	return port == 443 || port == 2053 || port == 2083 || port == 2087 || port == 2096 || port == 8443
}

func runWindowedSpeedTest(ctx context.Context, ip string, port int, customURL string) (float64, string) {
	scheme := "http"
	if isTLSPort(port) {
		scheme = "https"
	}

	parsedURL, err := parseSpeedTestURL(customURL, scheme)
	if err != nil {
		return 0, "URL解析错误: " + err.Error()
	}

	transport := &http.Transport{
		DialContext: func(c context.Context, network, addr string) (net.Conn, error) {
			dialer := &net.Dialer{Timeout: 5 * time.Second}
			return dialer.DialContext(c, "tcp", net.JoinHostPort(ip, strconv.Itoa(port)))
		},
		TLSHandshakeTimeout: 10 * time.Second,
		TLSClientConfig:     tlsConfigWithRootCAs(parsedURL.Hostname()),
		DisableCompression:  true,
	}
	client := http.Client{
		Transport: wrapDebugTransport("official-speed", transport),
		Timeout:   15 * time.Second,
	}

	req, err := http.NewRequestWithContext(ctx, "GET", parsedURL.String(), nil)
	if err != nil {
		return 0, "请求构造错误"
	}
	req.Host = parsedURL.Host
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Accept-Encoding", "identity")

	resp, err := client.Do(req)
	if err != nil {
		return 0, "连接错误"
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return 0, formatSpeedHTTPFailure(resp.StatusCode)
	}

	buf := make([]byte, 32*1024)
	var totalBytes int64

	timeout := time.After(6 * time.Second)
	measuredStart := time.Now()

	readerCtx, readerCancel := context.WithCancel(ctx)
	defer readerCancel()

	type readChunk struct {
		n   int
		err error
	}
	chunks := make(chan readChunk, 16)
	readerDone := make(chan struct{})
	safeGo("speed-reader", nil, func() {
		defer close(readerDone)
		for {
			n, err := resp.Body.Read(buf)
			select {
			case chunks <- readChunk{n: n, err: err}:
			case <-readerCtx.Done():
				return
			}
			if err != nil {
				return
			}
		}
	})

	done := false
	canceled := false
loop:
	for !done {
		select {
		case <-ctx.Done():
			canceled = true
			done = true
		case <-timeout:
			done = true
		case chunk := <-chunks:
			if chunk.n > 0 {
				totalBytes += int64(chunk.n)
			}
			if chunk.err != nil {
				done = true
				break loop
			}
		}
	}

	readerCancel()
	resp.Body.Close()
	<-readerDone

	if canceled {
		return 0, "测速任务已终止"
	}

	if totalBytes <= 0 {
		return 0, "0MB/s"
	}

	duration := time.Since(measuredStart).Seconds()
	if duration == 0 {
		duration = 1
	}
	return float64(totalBytes) / duration / 1024 / 1024, ""
}

func formatSpeedHTTPFailure(statusCode int) string {
	if statusCode == http.StatusTooManyRequests {
		return "测速失败（速率限制）"
	}
	return "测速失败"
}

const speedRateLimitMessage = "检测到触发速率限制，请稍后重试或更换网络环境"

func isSpeedRateLimited(speedErr string) bool {
	return strings.Contains(speedErr, "速率限制")
}

func runOfficialSpeedTestsCore(ctx context.Context, results []TestResult, port int, limit int, speedMinMB float64, customURL string, onResult func(current, total, qualified int, result TestResult), onRateLimited func()) ([]TestResult, []TestResult) {
	capacity := len(results)
	if limit > 0 && limit < capacity {
		capacity = limit
	}
	qualified := make([]TestResult, 0, capacity)
	interSpeedPause := func() bool {
		select {
		case <-ctx.Done():
			return false
		case <-time.After(1200 * time.Millisecond):
			return true
		}
	}
	consecutiveRateLimited := 0
	for i := range results {
		select {
		case <-ctx.Done():
			return results, qualified
		default:
		}
		if limit > 0 && len(qualified) >= limit {
			break
		}
		speedMB, speedErr := runWindowedSpeedTest(ctx, results[i].IP, port, customURL)
		if speedErr != "" {
			results[i].Speed = speedErr
			if isSpeedRateLimited(speedErr) {
				consecutiveRateLimited++
			} else {
				consecutiveRateLimited = 0
			}
		} else {
			consecutiveRateLimited = 0
			results[i].Speed = fmt.Sprintf("%.2fMB/s", speedMB)
			if speedMB >= speedMinMB {
				qualified = append(qualified, results[i])
			}
		}
		if onResult != nil {
			if ctx.Err() != nil {
				return results, qualified
			}
			onResult(i+1, len(results), len(qualified), results[i])
		}
		if !interSpeedPause() {
			return results, qualified
		}
		if consecutiveRateLimited >= 3 {
			if onRateLimited != nil {
				onRateLimited()
			}
			break
		}
	}
	return results, qualified
}
