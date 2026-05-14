package aiinfra

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"gos/internal/application/usecase"
	aidomain "gos/internal/domain/ai"
	"gos/internal/support/secure"
)

type OpenAICompatibleClientFactory struct {
	httpClient *http.Client
}

func NewOpenAICompatibleClientFactory() *OpenAICompatibleClientFactory {
	return &OpenAICompatibleClientFactory{
		httpClient: http.DefaultClient,
	}
}

func (f *OpenAICompatibleClientFactory) NewClient(config aidomain.ModelConfig) (usecase.AIModelClient, error) {
	if config.Provider != aidomain.ProviderOpenAICompatible {
		return nil, fmt.Errorf("unsupported ai provider: %s", config.Provider)
	}
	apiKey, err := secure.DecryptString(config.APIKeyCipher)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("api key is required")
	}
	timeout := config.TimeoutSec
	if timeout <= 0 {
		timeout = 60
	}
	httpClient := f.httpClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &OpenAICompatibleClient{
		baseURL:     strings.TrimRight(strings.TrimSpace(config.BaseURL), "/"),
		model:       strings.TrimSpace(config.Model),
		apiKey:      apiKey,
		temperature: config.Temperature,
		maxTokens:   config.MaxTokens,
		timeout:     time.Duration(timeout) * time.Second,
		httpClient:  httpClient,
	}, nil
}

type OpenAICompatibleClient struct {
	baseURL     string
	model       string
	apiKey      string
	temperature float64
	maxTokens   int
	timeout     time.Duration
	httpClient  *http.Client
}

func (c *OpenAICompatibleClient) DiagnoseStageLog(ctx context.Context, input usecase.AIChatInput) (json.RawMessage, error) {
	payload, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	requestBody := map[string]any{
		"model": c.model,
		"messages": []map[string]string{
			{
				"role":    "system",
				"content": "你是发布平台的 Jenkins 阶段日志诊断助手。必须严格按用户输入 rules.output_schema 的字段返回一个 JSON object；不要包裹 diagnosis 字段，不要输出 Markdown，不要添加 schema 外的顶层字段。",
			},
			{
				"role":    "user",
				"content": string(payload),
			},
		},
		"temperature": c.temperature,
		"max_tokens":  c.maxTokens,
		"response_format": map[string]string{
			"type": "json_object",
		},
	}
	body, err := json.Marshal(requestBody)
	if err != nil {
		return nil, err
	}

	reqCtx := ctx
	var cancel context.CancelFunc
	if c.timeout > 0 {
		reqCtx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ai model request failed: status %d: %s", resp.StatusCode, string(respBody))
	}

	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, err
	}
	if len(parsed.Choices) == 0 || strings.TrimSpace(parsed.Choices[0].Message.Content) == "" {
		return nil, fmt.Errorf("ai model response is empty")
	}
	return json.RawMessage(strings.TrimSpace(parsed.Choices[0].Message.Content)), nil
}
