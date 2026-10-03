package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const githubUploadMaxAttempts = 3

func uploadGitHubContentWithRetry(ctx context.Context, params githubUploadRequest, onAttempt func(attempt, total int, err error)) (string, error) {
	var lastErr error
	for attempt := 1; attempt <= githubUploadMaxAttempts; attempt++ {
		if onAttempt != nil {
			onAttempt(attempt, githubUploadMaxAttempts, nil)
		}
		downloadURL, err := uploadGitHubContent(ctx, params)
		if err == nil {
			return downloadURL, nil
		}
		lastErr = err
		recordDebugError("github_upload_attempt", fmt.Sprintf("attempt=%d/%d path=%s err=%v", attempt, githubUploadMaxAttempts, params.Path, err))
		if onAttempt != nil {
			onAttempt(attempt, githubUploadMaxAttempts, err)
		}
		if attempt < githubUploadMaxAttempts {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(time.Duration(attempt) * time.Second):
			}
		}
	}
	return "", lastErr
}

func uploadGitHubContent(ctx context.Context, params githubUploadRequest) (string, error) {
	params.Token = strings.TrimSpace(params.Token)
	params.Owner = strings.TrimSpace(params.Owner)
	params.Repo = strings.TrimSpace(params.Repo)
	params.Branch = strings.TrimSpace(params.Branch)
	params.Path = strings.Trim(strings.TrimSpace(params.Path), "/")
	params.Message = strings.TrimSpace(params.Message)
	if params.Token == "" || params.Owner == "" || params.Repo == "" || params.Path == "" || strings.TrimSpace(params.Content) == "" {
		return "", fmt.Errorf("token、仓库、路径和内容不能为空")
	}
	if params.Branch == "" {
		params.Branch = "main"
	}
	if params.Message == "" {
		params.Message = "update cfdata results"
	}

	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s", url.PathEscape(params.Owner), url.PathEscape(params.Repo), escapeGitHubContentPath(params.Path))
	sha, err := getGitHubContentSHA(ctx, apiURL, params.Token, params.Branch)
	if err != nil {
		return "", err
	}
	payload := map[string]string{
		"message": params.Message,
		"content": base64.StdEncoding.EncodeToString([]byte(params.Content)),
		"branch":  params.Branch,
	}
	if sha != "" {
		payload["sha"] = sha
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, apiURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	setGitHubHeaders(req, params.Token)
	resp, err := upstreamHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("GitHub API 返回 %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	io.Copy(io.Discard, resp.Body)
	return githubRawURL(params), nil
}

func githubRawURL(params githubUploadRequest) string {
	branch := strings.TrimSpace(params.Branch)
	if branch == "" {
		branch = "main"
	}
	path := strings.Trim(strings.TrimSpace(params.Path), "/")
	return fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/refs/heads/%s/%s", url.PathEscape(strings.TrimSpace(params.Owner)), url.PathEscape(strings.TrimSpace(params.Repo)), url.PathEscape(branch), escapeGitHubContentPath(path))
}

func getGitHubContentSHA(ctx context.Context, apiURL, token, branch string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL+"?ref="+url.QueryEscape(branch), nil)
	if err != nil {
		return "", err
	}
	setGitHubHeaders(req, token)
	resp, err := upstreamHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return "", nil
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("查询 GitHub 文件失败 %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	var payload struct {
		SHA string `json:"sha"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	return payload.SHA, nil
}

func setGitHubHeaders(req *http.Request, token string) {
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "cfdata")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
}

func escapeGitHubContentPath(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}
