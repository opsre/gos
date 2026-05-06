package bootstrap

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// CheckJenkinsConnection 检查业务状态并返回校验结果。
func CheckJenkinsConnection(cfg Config) error {
	if !cfg.Jenkins.Enabled {
		return nil
	}
	if !cfg.Jenkins.StartupCheckEnabled {
		log.Println("jenkins is enabled, startup check is disabled")
		return nil
	}
	log.Println("jenkins:", cfg.Jenkins.BaseURL)
	baseURL := strings.TrimRight(cfg.Jenkins.BaseURL, "/")
	pingURL := baseURL + "/api/json"
	client := &http.Client{
		Timeout: time.Duration(cfg.Jenkins.TimeoutSec) * time.Second,
	}

	var lastErr error
	for attempt := 1; attempt <= cfg.Jenkins.StartupMaxRetries; attempt++ {
		lastErr = pingJenkins(client, pingURL, cfg.Jenkins.Username, cfg.Jenkins.APIToken)
		if lastErr == nil {
			return nil
		}
		if attempt < cfg.Jenkins.StartupMaxRetries {
			time.Sleep(time.Duration(cfg.Jenkins.StartupRetryIntervalSec) * time.Second)
		}
	}

	return fmt.Errorf(
		"jenkins startup connection check failed after %d attempts (base_url=%s): %w",
		cfg.Jenkins.StartupMaxRetries,
		cfg.Jenkins.BaseURL,
		lastErr,
	)
}

// pingJenkins 封装当前模块的业务处理逻辑。
func pingJenkins(client *http.Client, url, username, apiToken string) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	if username != "" && apiToken != "" {
		req.SetBasicAuth(username, apiToken)
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	return nil
}
