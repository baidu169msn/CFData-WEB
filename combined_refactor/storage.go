package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

var upstreamHTTPClient = &http.Client{Timeout: 20 * time.Second}

func configureHTTPClients() {
	initCustomResolver()
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = dialContext
	transport.TLSClientConfig = tlsConfigWithRootCAs("")
	upstreamHTTPClient.Transport = wrapDebugTransport("upstream", transport)
}

func initLocations() {
	filename := "locations.json"
	url := "https://www.baipiao.eu.org/cloudflare/locations"
	var locations []location
	var body []byte
	var err error

	if _, err = os.Stat(filename); os.IsNotExist(err) {
		sendLog("本地 locations.json 不存在，正在从服务器下载...")
		content, err := getURLContent(url)
		if err != nil {
			sendLog("获取位置信息失败: " + err.Error())
			sendLog("⚠️ locationMap 未就绪，后续扫描结果的 城市/地区 列将为空；可删除本地 locations.json 缓存或检查网络后重新运行")
			return
		}
		body = []byte(content)
		if err := saveToFile(filename, content); err != nil {
			sendLog("保存位置信息失败: " + err.Error())
		}
	} else {
		sendLog(fmt.Sprintf("读取本地 %s 文件...", filename))
		body, err = os.ReadFile(filename)
		if err != nil {
			sendLog("读取位置信息失败: " + err.Error())
			sendLog("⚠️ locationMap 未就绪，后续扫描结果的 城市/地区 列将为空")
			return
		}
	}

	if err := json.Unmarshal(body, &locations); err != nil {
		sendLog("解析位置信息失败: " + err.Error())
		sendLog("⚠️ locationMap 未就绪（locations.json 可能损坏），建议删除本地缓存后重启")
		return
	}

	locationMap = make(map[string]location)
	for _, loc := range locations {
		locationMap[loc.Iata] = loc
	}
	fmt.Printf("已加载 %d 个数据中心位置信息\n", len(locationMap))
}

func sendLog(msg string) {
	recordDebugNotice("terminal_log", msg)
	fmt.Println(msg)
}

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}
var utf8BOMString = string(utf8BOM)

func writeUTF8BOM(f *os.File) error {
	_, err := f.Write(utf8BOM)
	return err
}

func getIPListContent(filename, apiURL string) (string, error) {
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		content, err := getURLContent(apiURL)
		if err != nil {
			return "", err
		}
		if err := saveToFile(filename, content); err != nil {
			fmt.Println("保存 IP 列表缓存失败:", err)
		}
		return content, nil
	}
	return getFileContent(filename)
}

func getURLContent(targetURL string) (string, error) {
	return getURLContentWithContext(context.Background(), targetURL)
}

func getURLContentWithContext(ctx context.Context, targetURL string) (string, error) {
	data, err := getURLBytesWithContext(ctx, targetURL)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func getURLBytesWithContext(ctx context.Context, targetURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := upstreamHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("请求失败: %s", resp.Status)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func getFileContent(filename string) (string, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func saveToFile(filename, content string) error {
	return atomicWriteFile(filename, []byte(content), 0644)
}

func atomicWriteFile(filename string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(filename)
	if dir == "" {
		dir = "."
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(filename)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, filename); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}
