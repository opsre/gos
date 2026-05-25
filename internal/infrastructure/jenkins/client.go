package jenkins

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	pipelineparamdomain "gos/internal/domain/executorparam"
	domain "gos/internal/domain/pipeline"
	releasedomain "gos/internal/domain/release"
)

type Config struct {
	BaseURL    string
	Username   string
	APIToken   string
	TimeoutSec int
}

type Client struct {
	baseURL  string
	username string
	apiToken string
	client   *http.Client
}

// NewClient 创建并返回对应组件实例。
func NewClient(cfg Config) *Client {
	timeout := cfg.TimeoutSec
	if timeout <= 0 {
		timeout = 5
	}
	return &Client{
		baseURL:  strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"),
		username: strings.TrimSpace(cfg.Username),
		apiToken: strings.TrimSpace(cfg.APIToken),
		client: &http.Client{
			Timeout: time.Duration(timeout) * time.Second,
		},
	}
}

// ListJobs 查询并返回列表数据。
func (c *Client) ListJobs(ctx context.Context) ([]domain.JenkinsJob, error) {
	endpoint := c.baseURL + "/api/json?tree=jobs[name,url,jobs[name,url,jobs[name,url,jobs[name,url,jobs[name,url]]]]]"
	body, err := c.get(ctx, endpoint)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Jobs []jenkinsJobNode `json:"jobs"`
	}
	if err := decodeJenkinsJSON(body, &resp); err != nil {
		return nil, err
	}

	result := make([]domain.JenkinsJob, 0)
	flattenJenkinsJobs(c.baseURL, "", resp.Jobs, &result)
	return result, nil
}

// GetJob 查询并返回指定资源数据。
func (c *Client) GetJob(ctx context.Context, fullName string) (domain.JenkinsJob, error) {
	fullName = strings.Trim(strings.TrimSpace(fullName), "/")
	if fullName == "" {
		return domain.JenkinsJob{}, fmt.Errorf("job full name is required")
	}
	endpoint := c.baseURL + buildJenkinsJobAPIPath(fullName)
	body, err := c.get(ctx, endpoint)
	if err != nil {
		return domain.JenkinsJob{}, err
	}

	var resp struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	}
	if err := decodeJenkinsJSON(body, &resp); err != nil {
		return domain.JenkinsJob{}, err
	}
	return domain.JenkinsJob{
		Name:     resp.Name,
		FullName: fullName,
		URL:      c.BuildJobURL(fullName),
	}, nil
}

// BuildJobURL 组装业务执行所需的输入数据。
func (c *Client) BuildJobURL(fullName string) string {
	fullName = strings.Trim(strings.TrimSpace(fullName), "/")
	if fullName == "" {
		return ""
	}
	return c.baseURL + buildJenkinsJobPath(fullName) + "/"
}

// GetPipelineScript 查询并返回指定资源数据。
func (c *Client) GetPipelineScript(ctx context.Context, fullName string) (domain.JenkinsPipelineScript, error) {
	fullName = strings.Trim(strings.TrimSpace(fullName), "/")
	if fullName == "" {
		return domain.JenkinsPipelineScript{}, fmt.Errorf("job full name is required")
	}

	body, err := c.GetPipelineConfigXML(ctx, fullName)
	if err != nil {
		return domain.JenkinsPipelineScript{}, err
	}

	var config struct {
		Description string `xml:"description"`
		Definition  struct {
			Class      string `xml:"class,attr"`
			Script     string `xml:"script"`
			Sandbox    bool   `xml:"sandbox"`
			ScriptPath string `xml:"scriptPath"`
		} `xml:"definition"`
	}
	if err := xml.Unmarshal([]byte(body), &config); err != nil {
		return domain.JenkinsPipelineScript{}, err
	}

	definitionClass := strings.TrimSpace(config.Definition.Class)
	script := strings.ReplaceAll(config.Definition.Script, "\r\n", "\n")
	script = strings.ReplaceAll(script, "\r", "\n")
	script = strings.TrimSpace(script)
	scriptPath := strings.TrimSpace(config.Definition.ScriptPath)
	fromSCM := strings.EqualFold(definitionClass, "org.jenkinsci.plugins.workflow.cps.CpsScmFlowDefinition")

	return domain.JenkinsPipelineScript{
		DefinitionClass: definitionClass,
		Description:     strings.TrimSpace(config.Description),
		Script:          script,
		ScriptPath:      scriptPath,
		Sandbox:         config.Definition.Sandbox,
		FromSCM:         fromSCM,
	}, nil
}

// GetPipelineConfigXML 查询并返回指定资源数据。
func (c *Client) GetPipelineConfigXML(ctx context.Context, fullName string) (string, error) {
	fullName = strings.Trim(strings.TrimSpace(fullName), "/")
	if fullName == "" {
		return "", fmt.Errorf("job full name is required")
	}

	endpoint := c.baseURL + buildJenkinsJobConfigPath(fullName)
	body, err := c.get(ctx, endpoint)
	if err != nil {
		return "", err
	}
	body = normalizeXMLVersion(body)
	return string(body), nil
}

// CreateRawPipeline 创建业务资源并返回处理结果。
func (c *Client) CreateRawPipeline(ctx context.Context, fullName string, cfg domain.JenkinsRawPipelineConfig) error {
	fullName = strings.Trim(strings.TrimSpace(fullName), "/")
	if fullName == "" {
		return fmt.Errorf("job full name is required")
	}
	jobName, parentPath := splitJenkinsJobFullName(fullName)
	if jobName == "" {
		return fmt.Errorf("job name is required")
	}
	endpoint := c.baseURL + buildJenkinsCreateItemPath(parentPath) + "?name=" + url.QueryEscape(jobName)
	return c.postXML(ctx, endpoint, buildRawPipelineConfigXML(cfg))
}

// UpdateRawPipeline 更新业务资源并返回处理结果。
func (c *Client) UpdateRawPipeline(ctx context.Context, fullName string, cfg domain.JenkinsRawPipelineConfig) error {
	fullName = strings.Trim(strings.TrimSpace(fullName), "/")
	if fullName == "" {
		return fmt.Errorf("job full name is required")
	}
	endpoint := c.baseURL + buildJenkinsJobConfigPath(fullName)
	return c.postXML(ctx, endpoint, buildRawPipelineConfigXML(cfg))
}

// DeletePipeline 删除业务资源并返回处理结果。
func (c *Client) DeletePipeline(ctx context.Context, fullName string) error {
	fullName = strings.Trim(strings.TrimSpace(fullName), "/")
	if fullName == "" {
		return fmt.Errorf("job full name is required")
	}
	endpoint := buildJenkinsActionEndpoint(c.baseURL, buildJenkinsJobPath(fullName), "doDelete")
	if strings.TrimSpace(endpoint) == "" {
		return fmt.Errorf("job full name is required")
	}
	return c.postAction(ctx, endpoint)
}

// RenderRawPipelineConfigXML 封装当前模块的业务处理逻辑。
func (c *Client) RenderRawPipelineConfigXML(cfg domain.JenkinsRawPipelineConfig) (string, error) {
	if strings.TrimSpace(cfg.Script) == "" {
		return "", fmt.Errorf("raw pipeline script is required")
	}
	return buildRawPipelineConfigXML(cfg), nil
}

// TriggerBuild 组装业务执行所需的输入数据。
func (c *Client) TriggerBuild(ctx context.Context, fullName string, params map[string]string) (string, error) {
	fullName = strings.Trim(strings.TrimSpace(fullName), "/")
	if fullName == "" {
		return "", fmt.Errorf("job full name is required")
	}

	path := buildJenkinsJobPath(fullName)
	buildEndpoint := c.baseURL + path + "/build"
	buildWithParamsEndpoint := c.baseURL + path + "/buildWithParameters"
	form := url.Values{}
	for k, v := range params {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		// 处理多值参数：如果值包含逗号分隔符，拆分为多个同名参数
		if strings.Contains(v, ",") {
			values := strings.Split(v, ",")
			for _, val := range values {
				val = strings.TrimSpace(val)
				if val != "" {
					form.Add(key, val)
				}
			}
		} else {
			form.Set(key, v)
		}
	}
	body := form.Encode()

	endpoints := make([]string, 0, 2)
	if len(form) > 0 {
		endpoints = append(endpoints, buildWithParamsEndpoint)
	} else {
		// Jenkins 参数化任务在 /build 空提交时会返回 400，兜底尝试 /buildWithParameters。
		endpoints = append(endpoints, buildEndpoint, buildWithParamsEndpoint)
	}

	var lastErr error
	for _, endpoint := range endpoints {
		queueURL, statusCode, err := c.postTriggerBuild(ctx, endpoint, body, crumbHeader{})
		if err == nil {
			return queueURL, nil
		}

		if statusCode == http.StatusForbidden {
			crumbField, crumbValue, crumbErr := c.getCrumb(ctx)
			if crumbErr == nil {
				queueURL, _, crumbPostErr := c.postTriggerBuild(ctx, endpoint, body, crumbHeader{field: crumbField, value: crumbValue})
				if crumbPostErr == nil {
					return queueURL, nil
				}
				lastErr = crumbPostErr
				continue
			}
		}

		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("trigger jenkins build failed")
	}
	return "", lastErr
}

// postTriggerBuild 组装业务执行所需的输入数据。
func (c *Client) postTriggerBuild(ctx context.Context, endpoint string, encodedForm string, crumb crumbHeader) (string, int, error) {
	queueURL, statusCode, _, err := c.doPost(ctx, endpoint, encodedForm, crumb, false)
	if err != nil {
		return "", statusCode, err
	}
	if strings.TrimSpace(queueURL) == "" {
		return "", statusCode, fmt.Errorf("jenkins build triggered but queue location is empty")
	}
	return strings.TrimSpace(queueURL), statusCode, nil
}

// GetQueueItem 查询并返回指定资源数据。
func (c *Client) GetQueueItem(
	ctx context.Context,
	queueURL string,
) (executableURL string, cancelled bool, why string, err error) {
	endpoint := buildJenkinsAPIEndpoint(c.baseURL, queueURL, "cancelled,why,executable[url]")
	if endpoint == "" {
		return "", false, "", fmt.Errorf("queue url is required")
	}

	body, err := c.get(ctx, endpoint)
	if err != nil {
		return "", false, "", err
	}

	var payload struct {
		Cancelled  bool   `json:"cancelled"`
		Why        string `json:"why"`
		Executable struct {
			URL string `json:"url"`
		} `json:"executable"`
	}
	if err := decodeJenkinsJSON(body, &payload); err != nil {
		return "", false, "", err
	}
	buildURL := strings.TrimSpace(payload.Executable.URL)
	if buildURL != "" {
		if normalized := resolveJenkinsResourcePrefix(c.baseURL, buildURL); normalized != "" {
			buildURL = strings.TrimRight(normalized, "/") + "/"
		}
	}
	return buildURL, payload.Cancelled, strings.TrimSpace(payload.Why), nil
}

// GetBuildStatus 组装业务执行所需的输入数据。
func (c *Client) GetBuildStatus(ctx context.Context, buildURL string) (building bool, result string, err error) {
	endpoint := buildJenkinsAPIEndpoint(c.baseURL, buildURL, "building,result")
	if endpoint == "" {
		return false, "", fmt.Errorf("build url is required")
	}

	body, err := c.get(ctx, endpoint)
	if err != nil {
		return false, "", err
	}

	var payload struct {
		Building bool   `json:"building"`
		Result   string `json:"result"`
	}
	if err := decodeJenkinsJSON(body, &payload); err != nil {
		return false, "", err
	}
	return payload.Building, strings.TrimSpace(payload.Result), nil
}

// GetBuildStages 组装业务执行所需的输入数据。
func (c *Client) GetBuildStages(
	ctx context.Context,
	buildURL string,
) ([]releasedomain.ReleaseOrderPipelineStage, error) {
	endpoint := buildJenkinsWFAPIEndpoint(c.baseURL, buildURL, "describe")
	if endpoint == "" {
		return nil, fmt.Errorf("build url is required")
	}

	body, err := c.get(ctx, endpoint)
	if err != nil {
		return nil, err
	}

	var payload struct {
		Stages []struct {
			ID              string `json:"id"`
			Name            string `json:"name"`
			Status          string `json:"status"`
			StartTimeMillis int64  `json:"startTimeMillis"`
			DurationMillis  int64  `json:"durationMillis"`
		} `json:"stages"`
	}
	if err := decodeJenkinsJSON(body, &payload); err != nil {
		return nil, err
	}

	result := make([]releasedomain.ReleaseOrderPipelineStage, 0, len(payload.Stages))
	for index, item := range payload.Stages {
		stageKey := strings.TrimSpace(item.ID)
		if stageKey == "" {
			stageKey = strings.TrimSpace(item.Name)
		}
		if stageKey == "" {
			continue
		}

		startedAt := jenkinsMillisToTime(item.StartTimeMillis)
		finishedAt := deriveStageFinishedAt(startedAt, item.DurationMillis, item.Status)
		result = append(result, releasedomain.ReleaseOrderPipelineStage{
			StageKey:       stageKey,
			StageName:      strings.TrimSpace(item.Name),
			Status:         mapJenkinsStageStatus(item.Status),
			RawStatus:      strings.TrimSpace(item.Status),
			SortNo:         index + 1,
			DurationMillis: maxInt64(item.DurationMillis, 0),
			StartedAt:      startedAt,
			FinishedAt:     finishedAt,
		})
	}
	return result, nil
}

// GetBuildStageLog 组装业务执行所需的输入数据。
func (c *Client) GetBuildStageLog(
	ctx context.Context,
	buildURL string,
	stageKey string,
) (log releasedomain.ReleaseOrderPipelineStageLog, err error) {
	return c.getBuildStageLog(ctx, buildURL, stageKey, "")
}

// GetBuildStageLogWithName 查询并返回指定资源数据。
func (c *Client) GetBuildStageLogWithName(
	ctx context.Context,
	buildURL string,
	stageKey string,
	stageName string,
) (log releasedomain.ReleaseOrderPipelineStageLog, err error) {
	return c.getBuildStageLog(ctx, buildURL, stageKey, stageName)
}

func (c *Client) getBuildStageLog(
	ctx context.Context,
	buildURL string,
	stageKey string,
	stageName string,
) (log releasedomain.ReleaseOrderPipelineStageLog, err error) {
	stageKey = strings.TrimSpace(stageKey)
	if stageKey == "" {
		return releasedomain.ReleaseOrderPipelineStageLog{}, fmt.Errorf("stage key is required")
	}
	stageName = strings.TrimSpace(stageName)

	detail, err := c.getBuildStageDetail(ctx, buildURL, stageKey)
	if err != nil {
		return c.getBuildStageLogFromConsole(ctx, buildURL, stageKey, stageName, "", err)
	}
	stageName = firstNonEmptyString(strings.TrimSpace(detail.Name), stageName, stageKey)

	log = releasedomain.ReleaseOrderPipelineStageLog{
		StageName: stageName,
		RawStatus: strings.TrimSpace(detail.Status),
		HasMore:   false,
		FetchedAt: time.Now().UTC(),
	}

	nodes := detail.StageFlowNodes
	if len(nodes) == 0 {
		text, hasMore, logErr := c.getBuildNodeLog(ctx, buildURL, stageKey)
		if logErr != nil {
			return c.getBuildStageLogFromConsole(ctx, buildURL, stageKey, stageName, detail.Status, logErr)
		}
		log.Content = text
		log.HasMore = hasMore
		return log, nil
	}

	var builder strings.Builder
	for _, node := range nodes {
		text, hasMore, logErr := c.getBuildNodeLog(ctx, buildURL, node.ID)
		if logErr != nil {
			return c.getBuildStageLogFromConsole(ctx, buildURL, stageKey, stageName, detail.Status, logErr)
		}
		if hasMore {
			log.HasMore = true
		}
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		if builder.Len() > 0 {
			builder.WriteString("\n")
		}
		if len(nodes) > 1 {
			title := strings.TrimSpace(node.Name)
			if title == "" {
				title = "阶段节点"
			}
			builder.WriteString(">>> ")
			builder.WriteString(title)
			builder.WriteString("\n")
		}
		builder.WriteString(text)
		if !strings.HasSuffix(text, "\n") {
			builder.WriteString("\n")
		}
	}
	log.Content = strings.TrimSpace(builder.String())
	return log, nil
}

// GetBuildConsoleText 组装业务执行所需的输入数据。
func (c *Client) GetBuildConsoleText(
	ctx context.Context,
	buildURL string,
	start int64,
) (content string, nextStart int64, moreData bool, err error) {
	if start < 0 {
		start = 0
	}
	endpoint := buildJenkinsProgressiveTextEndpoint(c.baseURL, buildURL, start)
	if endpoint == "" {
		return "", start, false, fmt.Errorf("build url is required")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", start, false, err
	}
	if c.username != "" && c.apiToken != "" {
		req.SetBasicAuth(c.username, c.apiToken)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return "", start, false, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", start, false, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", start, false, buildJenkinsHTTPError(resp.StatusCode, body)
	}

	nextStart = start
	if textSize := strings.TrimSpace(resp.Header.Get("X-Text-Size")); textSize != "" {
		if parsed, parseErr := strconv.ParseInt(textSize, 10, 64); parseErr == nil && parsed >= 0 {
			nextStart = parsed
		}
	}
	moreData = parseJenkinsMoreData(resp.Header.Get("X-More-Data"))

	return normalizeJenkinsLogContent(string(body)), nextStart, moreData, nil
}

// AbortQueueItem 封装当前模块的业务处理逻辑。
func (c *Client) AbortQueueItem(ctx context.Context, queueURL string) error {
	endpoint := buildJenkinsActionEndpoint(c.baseURL, queueURL, "cancelQueue")
	if endpoint == "" {
		return fmt.Errorf("queue url is required")
	}
	return c.postAction(ctx, endpoint)
}

// AbortBuild 组装业务执行所需的输入数据。
func (c *Client) AbortBuild(ctx context.Context, buildURL string) error {
	endpoint := buildJenkinsActionEndpoint(c.baseURL, buildURL, "stop")
	if endpoint == "" {
		return fmt.Errorf("build url is required")
	}
	return c.postAction(ctx, endpoint)
}

// ListJobParamSets 查询并返回列表数据。
func (c *Client) ListJobParamSets(ctx context.Context) ([]pipelineparamdomain.JenkinsJobParamSet, error) {
	jobs, err := c.ListJobs(ctx)
	if err != nil {
		return nil, err
	}
	if len(jobs) == 0 {
		return nil, nil
	}

	type paramJob struct {
		index    int
		fullName string
	}
	type paramResult struct {
		index    int
		fullName string
		item     pipelineparamdomain.JenkinsJobParamSet
		err      error
	}

	workerCount := 32
	if len(jobs) < workerCount {
		workerCount = len(jobs)
	}
	if workerCount <= 0 {
		workerCount = 1
	}

	jobsCh := make(chan paramJob, len(jobs))
	resultsCh := make(chan paramResult, len(jobs))
	var wg sync.WaitGroup

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobsCh {
				item, itemErr := c.getJobParamSet(ctx, job.fullName)
				resultsCh <- paramResult{
					index:    job.index,
					fullName: job.fullName,
					item:     item,
					err:      itemErr,
				}
			}
		}()
	}

	for index, job := range jobs {
		jobsCh <- paramJob{index: index, fullName: job.FullName}
	}
	close(jobsCh)

	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	collected := make([]pipelineparamdomain.JenkinsJobParamSet, len(jobs))
	var firstErr error
	for result := range resultsCh {
		if result.err != nil && firstErr == nil {
			firstErr = fmt.Errorf("load job params failed for %s: %w", strings.TrimSpace(result.fullName), result.err)
			continue
		}
		collected[result.index] = result.item
	}
	if firstErr != nil {
		return nil, firstErr
	}

	items := make([]pipelineparamdomain.JenkinsJobParamSet, 0, len(collected))
	for _, item := range collected {
		if strings.TrimSpace(item.JobFullName) == "" {
			continue
		}
		items = append(items, item)
	}
	return items, nil
}

// GetJobParamSet 查询并返回指定 Jenkins Job 的参数定义。
func (c *Client) GetJobParamSet(ctx context.Context, fullName string) (pipelineparamdomain.JenkinsJobParamSet, error) {
	return c.getJobParamSet(ctx, fullName)
}

// getJobParamSet 查询并返回指定资源数据。
func (c *Client) getJobParamSet(ctx context.Context, fullName string) (pipelineparamdomain.JenkinsJobParamSet, error) {
	fullName = strings.Trim(strings.TrimSpace(fullName), "/")
	if fullName == "" {
		return pipelineparamdomain.JenkinsJobParamSet{}, fmt.Errorf("job full name is required")
	}

	endpoint := c.baseURL + buildJenkinsJobAPIPath(fullName) +
		"?tree=name,fullName," +
		"actions[parameterDefinitions[name,description,_class,choices,value,type,multiSelectDelimiter,defaultValue,defaultParameterValue[value],propertyFile,propertyKey,descriptionPropertyFile,descriptionPropertyKey,quoteValue,saveJSONParameterToFile,visibleItemCount]]," +
		"property[parameterDefinitions[name,description,_class,choices,value,type,multiSelectDelimiter,defaultValue,defaultParameterValue[value],propertyFile,propertyKey,descriptionPropertyFile,descriptionPropertyKey,quoteValue,saveJSONParameterToFile,visibleItemCount]]"
	body, err := c.get(ctx, endpoint)
	if err != nil {
		return pipelineparamdomain.JenkinsJobParamSet{}, err
	}

	var resp struct {
		Name     string `json:"name"`
		FullName string `json:"fullName"`
		Actions  []struct {
			ParameterDefinitions []json.RawMessage `json:"parameterDefinitions"`
		} `json:"actions"`
		Properties []struct {
			ParameterDefinitions []json.RawMessage `json:"parameterDefinitions"`
		} `json:"property"`
	}
	if err := decodeJenkinsJSON(body, &resp); err != nil {
		return pipelineparamdomain.JenkinsJobParamSet{}, err
	}

	params := make([]pipelineparamdomain.JenkinsParamSnapshot, 0)
	seen := make(map[string]struct{})
	for _, action := range resp.Actions {
		if err := appendParsedJenkinsParams(action.ParameterDefinitions, &params, seen); err != nil {
			return pipelineparamdomain.JenkinsJobParamSet{}, err
		}
	}
	for _, property := range resp.Properties {
		if err := appendParsedJenkinsParams(property.ParameterDefinitions, &params, seen); err != nil {
			return pipelineparamdomain.JenkinsJobParamSet{}, err
		}
	}
	if len(params) == 0 {
		fallbackParams, fallbackErr := c.loadScriptParamFallback(ctx, fullName)
		if fallbackErr == nil {
			params = append(params, fallbackParams...)
			for _, item := range fallbackParams {
				seen[item.Name] = struct{}{}
			}
		}
	}

	if fallback, err := c.loadExtendedChoiceFallback(ctx, fullName); err == nil {
		for idx, item := range params {
			choices, ok := fallback[item.Name]
			if !ok {
				pf, pk := readRawMetaPropertyFile(item.RawMeta)
				if pf == "" {
					continue
				}
				resolved := c.resolveExtendedChoiceValues(ctx, fullName, item.Name, extendedChoiceFallback{
					values:       nil,
					propertyFile: pf,
					propertyKey:  pk,
				})
				params[idx].RawMeta = mergeChoiceValuesIntoRawMeta(item.RawMeta, extendedChoiceFallback{
					values:       resolved.values,
					options:      resolved.options,
					propertyFile: pf,
					propertyKey:  pk,
				})
				params[idx].SingleSelect = inferPipelineSingleSelectFromRawMeta(params[idx].RawMeta, params[idx].SingleSelect)
				continue
			}
			choices = c.resolveExtendedChoiceValues(ctx, fullName, item.Name, choices)
			params[idx].RawMeta = mergeChoiceValuesIntoRawMeta(item.RawMeta, choices)
			if strings.TrimSpace(params[idx].DefaultValue) == "" && strings.TrimSpace(choices.defaultValue) != "" {
				params[idx].DefaultValue = strings.TrimSpace(choices.defaultValue)
			}
			params[idx].SingleSelect = inferPipelineSingleSelectFromRawMeta(params[idx].RawMeta, params[idx].SingleSelect)
		}
		c.appendMissingExtendedChoiceFallbackParams(ctx, fullName, fallback, &params, seen)
	}
	if err := c.loadGitParameterChoicesIntoParams(ctx, fullName, params); err == nil {
		// no-op: params are updated in place
	}

	return pipelineparamdomain.JenkinsJobParamSet{
		JobName:     strings.TrimSpace(resp.Name),
		JobFullName: fullName,
		Params:      params,
	}, nil
}

// loadScriptParamFallback 封装当前模块的业务处理逻辑。
func (c *Client) loadScriptParamFallback(ctx context.Context, fullName string) ([]pipelineparamdomain.JenkinsParamSnapshot, error) {
	script, err := c.GetPipelineScript(ctx, fullName)
	if err != nil {
		return nil, err
	}
	return parsePipelineScriptParamSnapshots(script.Script), nil
}

// appendParsedJenkinsParams 解析输入内容并返回结构化结果。
func appendParsedJenkinsParams(
	rawItems []json.RawMessage,
	target *[]pipelineparamdomain.JenkinsParamSnapshot,
	seen map[string]struct{},
) error {
	for index, raw := range rawItems {
		param, ok, parseErr := parseJenkinsParamDefinition(raw, index+1)
		if parseErr != nil {
			return parseErr
		}
		if !ok {
			continue
		}
		if _, exists := seen[param.Name]; exists {
			continue
		}
		seen[param.Name] = struct{}{}
		*target = append(*target, param)
	}
	return nil
}

// parseJenkinsParamDefinition 解析输入内容并返回结构化结果。
func parseJenkinsParamDefinition(raw json.RawMessage, sortNo int) (pipelineparamdomain.JenkinsParamSnapshot, bool, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return pipelineparamdomain.JenkinsParamSnapshot{}, false, err
	}

	name := strings.TrimSpace(readJSONString(fields["name"]))
	if name == "" {
		return pipelineparamdomain.JenkinsParamSnapshot{}, false, nil
	}

	className := strings.TrimSpace(readJSONString(fields["_class"]))
	typeName := strings.TrimSpace(readJSONString(fields["type"]))
	description := strings.TrimSpace(readJSONString(fields["description"]))
	choices := parseChoiceValues(fields["choices"])
	if len(choices) == 0 {
		choices = parseChoiceValues(fields["value"])
	}

	defaultValueAny, defaultValue, err := parseDefaultValue(fields["defaultValue"])
	if err != nil {
		return pipelineparamdomain.JenkinsParamSnapshot{}, false, err
	}
	if len(fields["defaultParameterValue"]) > 0 {
		var defaultParam struct {
			Value any `json:"value"`
		}
		if err := json.Unmarshal(fields["defaultParameterValue"], &defaultParam); err == nil {
			defaultValue = stringifyDefaultValue(defaultParam.Value)
			defaultValueAny = defaultParam.Value
		}
	}

	rawMeta := "{}"
	if trimmed := strings.TrimSpace(string(raw)); trimmed != "" {
		rawMeta = trimmed
	}

	return pipelineparamdomain.JenkinsParamSnapshot{
		Name:         name,
		ParamType:    inferExecutorParamType(className, choices, defaultValueAny, defaultValue),
		SingleSelect: inferPipelineSingleSelect(name, className, typeName, fields, len(choices)),
		Required:     false,
		DefaultValue: defaultValue,
		Description:  description,
		RawMeta:      rawMeta,
		SortNo:       sortNo,
	}, true, nil
}

// parsePipelineScriptParamSnapshots 解析输入内容并返回结构化结果。
func parsePipelineScriptParamSnapshots(script string) []pipelineparamdomain.JenkinsParamSnapshot {
	block, ok := extractGroovyNamedBlock(script, "parameters")
	if !ok {
		return nil
	}

	calls := extractGroovyTopLevelCalls(block)
	result := make([]pipelineparamdomain.JenkinsParamSnapshot, 0, len(calls))
	for index, item := range calls {
		param, ok := parsePipelineScriptParamCall(item.name, item.args, index+1)
		if !ok {
			continue
		}
		result = append(result, param)
	}
	return result
}

type groovyTopLevelCall struct {
	name string
	args string
}

// extractGroovyNamedBlock 封装当前模块的业务处理逻辑。
func extractGroovyNamedBlock(script string, keyword string) (string, bool) {
	source := script
	for idx := 0; idx < len(source); idx++ {
		pos := strings.Index(source[idx:], keyword)
		if pos < 0 {
			return "", false
		}
		pos += idx
		if (pos > 0 && isGroovyIdentByte(source[pos-1])) ||
			(pos+len(keyword) < len(source) && isGroovyIdentByte(source[pos+len(keyword)])) {
			idx = pos + len(keyword)
			continue
		}
		next := skipGroovySpaces(source, pos+len(keyword))
		if next >= len(source) || source[next] != '{' {
			idx = pos + len(keyword)
			continue
		}
		end, ok := findGroovyMatching(source, next, '{', '}')
		if !ok || end <= next {
			return "", false
		}
		return source[next+1 : end], true
	}
	return "", false
}

// extractGroovyTopLevelCalls 封装当前模块的业务处理逻辑。
func extractGroovyTopLevelCalls(block string) []groovyTopLevelCall {
	result := make([]groovyTopLevelCall, 0)
	for idx := 0; idx < len(block); {
		idx = skipGroovyNoise(block, idx)
		if idx >= len(block) {
			break
		}
		if !isGroovyIdentStart(block[idx]) {
			idx++
			continue
		}
		start := idx
		idx++
		for idx < len(block) && isGroovyIdentByte(block[idx]) {
			idx++
		}
		name := strings.TrimSpace(block[start:idx])
		argStart := skipGroovySpaces(block, idx)
		if argStart >= len(block) {
			break
		}

		if block[argStart] == '(' {
			end, ok := findGroovyMatching(block, argStart, '(', ')')
			if !ok {
				break
			}
			result = append(result, groovyTopLevelCall{
				name: name,
				args: strings.TrimSpace(block[argStart+1 : end]),
			})
			idx = end + 1
			continue
		}

		statementEnd := findGroovyStatementEnd(block, argStart)
		args := strings.TrimSpace(block[argStart:statementEnd])
		if strings.Contains(args, ":") {
			result = append(result, groovyTopLevelCall{name: name, args: args})
		}
		idx = statementEnd + 1
	}
	return result
}

// parsePipelineScriptParamCall 解析输入内容并返回结构化结果。
func parsePipelineScriptParamCall(name string, args string, sortNo int) (pipelineparamdomain.JenkinsParamSnapshot, bool) {
	argsMap := parseGroovyNamedArgs(args)
	paramName := strings.TrimSpace(parseGroovyStringLike(argsMap["name"]))
	if paramName == "" {
		return pipelineparamdomain.JenkinsParamSnapshot{}, false
	}

	description := strings.TrimSpace(parseGroovyStringLike(argsMap["description"]))
	defaultValue := strings.TrimSpace(parseGroovyStringLike(argsMap["defaultValue"]))
	lowerName := strings.ToLower(strings.TrimSpace(name))
	switch lowerName {
	case "choice":
		choices := parseGroovyChoices(argsMap["choices"], ",")
		return buildScriptParamSnapshot(
			paramName,
			description,
			defaultValue,
			pipelineparamdomain.ParamTypeChoice,
			true,
			sortNo,
			map[string]any{
				"_class":  "hudson.model.ChoiceParameterDefinition",
				"type":    "ChoiceParameterDefinition",
				"choices": choices,
			},
		), true
	case "gitparameter":
		return buildScriptParamSnapshot(
			paramName,
			description,
			defaultValue,
			pipelineparamdomain.ParamTypeChoice,
			true,
			sortNo,
			map[string]any{
				"_class": "net.uaznia.lukanus.hudson.plugins.gitparameter.GitParameterDefinition",
				"type":   "GitParameterDefinition",
			},
		), true
	case "extendedchoice":
		typeName := strings.TrimSpace(parseGroovyStringLike(argsMap["type"]))
		delimiter := strings.TrimSpace(parseGroovyStringLike(argsMap["multiSelectDelimiter"]))
		if delimiter == "" {
			delimiter = ","
		}
		choices := parseGroovyChoices(argsMap["value"], delimiter)
		if defaultValue == "" {
			defaultValue = strings.TrimSpace(parseGroovyStringLike(argsMap["defaultValue"]))
		}
		propertyFile := strings.TrimSpace(parseGroovyStringLike(argsMap["propertyFile"]))
		propertyKey := strings.TrimSpace(parseGroovyStringLike(argsMap["propertyKey"]))
		descriptionPropertyFile := strings.TrimSpace(parseGroovyStringLike(argsMap["descriptionPropertyFile"]))
		descriptionPropertyKey := strings.TrimSpace(parseGroovyStringLike(argsMap["descriptionPropertyKey"]))
		quoteValue := strings.TrimSpace(parseGroovyStringLike(argsMap["quoteValue"]))
		saveJSONParameterToFile := strings.TrimSpace(parseGroovyStringLike(argsMap["saveJSONParameterToFile"]))
		visibleItemCount := strings.TrimSpace(parseGroovyStringLike(argsMap["visibleItemCount"]))
		meta := map[string]any{
			"_class":               "com.cwctravel.hudson.plugins.extended_choice_parameter.ExtendedChoiceParameterDefinition",
			"type":                 defaultGroovyString(typeName, "PT_SINGLE_SELECT"),
			"choices":              choices,
			"multiSelectDelimiter": delimiter,
		}
		if propertyFile != "" {
			meta["propertyFile"] = propertyFile
		}
		if propertyKey != "" {
			meta["propertyKey"] = propertyKey
		}
		if descriptionPropertyFile != "" {
			meta["descriptionPropertyFile"] = descriptionPropertyFile
		}
		if descriptionPropertyKey != "" {
			meta["descriptionPropertyKey"] = descriptionPropertyKey
		}
		if quoteValue != "" {
			meta["quoteValue"] = quoteValue
		}
		if saveJSONParameterToFile != "" {
			meta["saveJSONParameterToFile"] = saveJSONParameterToFile
		}
		if visibleItemCount != "" {
			meta["visibleItemCount"] = visibleItemCount
		}
		return buildScriptParamSnapshot(
			paramName,
			description,
			defaultValue,
			pipelineparamdomain.ParamTypeChoice,
			!looksLikeGroovyMultiSelect(typeName),
			sortNo,
			meta,
		), true
	case "text":
		return buildScriptParamSnapshot(
			paramName,
			description,
			defaultValue,
			pipelineparamdomain.ParamTypeString,
			false,
			sortNo,
			map[string]any{
				"_class": "hudson.model.TextParameterDefinition",
				"type":   "TextParameterDefinition",
			},
		), true
	case "string":
		return buildScriptParamSnapshot(
			paramName,
			description,
			defaultValue,
			pipelineparamdomain.ParamTypeString,
			false,
			sortNo,
			map[string]any{
				"_class": "hudson.model.StringParameterDefinition",
				"type":   "StringParameterDefinition",
			},
		), true
	case "booleanparam":
		return buildScriptParamSnapshot(
			paramName,
			description,
			defaultValue,
			pipelineparamdomain.ParamTypeBool,
			false,
			sortNo,
			map[string]any{
				"_class": "hudson.model.BooleanParameterDefinition",
				"type":   "BooleanParameterDefinition",
			},
		), true
	default:
		return pipelineparamdomain.JenkinsParamSnapshot{}, false
	}
}

// buildScriptParamSnapshot 组装业务执行所需的输入数据。
func buildScriptParamSnapshot(
	name string,
	description string,
	defaultValue string,
	paramType pipelineparamdomain.ParamType,
	singleSelect bool,
	sortNo int,
	meta map[string]any,
) pipelineparamdomain.JenkinsParamSnapshot {
	meta["name"] = name
	if description != "" {
		meta["description"] = description
	}
	if defaultValue != "" {
		meta["defaultValue"] = defaultValue
	}
	rawMetaBytes, err := json.Marshal(meta)
	rawMeta := "{}"
	if err == nil {
		rawMeta = string(rawMetaBytes)
	}
	return pipelineparamdomain.JenkinsParamSnapshot{
		Name:         name,
		ParamType:    paramType,
		SingleSelect: singleSelect,
		Required:     false,
		DefaultValue: defaultValue,
		Description:  description,
		RawMeta:      rawMeta,
		SortNo:       sortNo,
	}
}

// parseGroovyNamedArgs 解析输入内容并返回结构化结果。
func parseGroovyNamedArgs(args string) map[string]string {
	result := make(map[string]string)
	for _, part := range splitGroovyTopLevel(args, ',') {
		key, value, ok := splitGroovyNamedArg(part)
		if !ok {
			continue
		}
		result[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return result
}

// splitGroovyNamedArg 封装当前模块的业务处理逻辑。
func splitGroovyNamedArg(part string) (string, string, bool) {
	text := strings.TrimSpace(part)
	if text == "" {
		return "", "", false
	}
	for idx := 0; idx < len(text); idx++ {
		if text[idx] != ':' {
			continue
		}
		if hasGroovyOpeners(text[:idx]) || hasGroovyQuotes(text[:idx]) {
			continue
		}
		return text[:idx], text[idx+1:], true
	}
	return "", "", false
}

// splitGroovyTopLevel 封装当前模块的业务处理逻辑。
func splitGroovyTopLevel(text string, sep byte) []string {
	result := make([]string, 0)
	start := 0
	var (
		parenDepth   int
		bracketDepth int
		braceDepth   int
		inSingle     bool
		inDouble     bool
		inLineCmt    bool
		inBlockCmt   bool
	)
	for idx := 0; idx < len(text); idx++ {
		ch := text[idx]
		next := byte(0)
		if idx+1 < len(text) {
			next = text[idx+1]
		}
		if inLineCmt {
			if ch == '\n' {
				inLineCmt = false
			}
			continue
		}
		if inBlockCmt {
			if ch == '*' && next == '/' {
				inBlockCmt = false
				idx++
			}
			continue
		}
		if inSingle {
			if ch == '\\' && idx+1 < len(text) {
				idx++
				continue
			}
			if ch == '\'' {
				inSingle = false
			}
			continue
		}
		if inDouble {
			if ch == '\\' && idx+1 < len(text) {
				idx++
				continue
			}
			if ch == '"' {
				inDouble = false
			}
			continue
		}
		switch {
		case ch == '/' && next == '/':
			inLineCmt = true
			idx++
		case ch == '/' && next == '*':
			inBlockCmt = true
			idx++
		case ch == '\'':
			inSingle = true
		case ch == '"':
			inDouble = true
		case ch == '(':
			parenDepth++
		case ch == ')':
			if parenDepth > 0 {
				parenDepth--
			}
		case ch == '[':
			bracketDepth++
		case ch == ']':
			if bracketDepth > 0 {
				bracketDepth--
			}
		case ch == '{':
			braceDepth++
		case ch == '}':
			if braceDepth > 0 {
				braceDepth--
			}
		case ch == sep && parenDepth == 0 && bracketDepth == 0 && braceDepth == 0:
			result = append(result, text[start:idx])
			start = idx + 1
		}
	}
	if start <= len(text) {
		result = append(result, text[start:])
	}
	return result
}

// parseGroovyChoices 解析输入内容并返回结构化结果。
func parseGroovyChoices(raw string, delimiter string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if strings.HasPrefix(raw, "[") && strings.HasSuffix(raw, "]") {
		inner := strings.TrimSpace(raw[1 : len(raw)-1])
		if inner == "" {
			return nil
		}
		items := splitGroovyTopLevel(inner, ',')
		values := make([]string, 0, len(items))
		for _, item := range items {
			value := strings.TrimSpace(parseGroovyStringLike(item))
			if value != "" {
				values = append(values, value)
			}
		}
		return normalizeChoiceValues(values)
	}
	return splitChoiceValueByDelimiter(parseGroovyStringLike(raw), delimiter)
}

// parseGroovyStringLike 解析输入内容并返回结构化结果。
func parseGroovyStringLike(raw string) string {
	text := strings.TrimSpace(raw)
	if text == "" {
		return ""
	}
	if len(text) >= 2 {
		if (text[0] == '\'' && text[len(text)-1] == '\'') || (text[0] == '"' && text[len(text)-1] == '"') {
			text = text[1 : len(text)-1]
		}
	}
	text = strings.ReplaceAll(text, "\\'", "'")
	text = strings.ReplaceAll(text, "\\\"", "\"")
	text = strings.ReplaceAll(text, "\\n", "\n")
	text = strings.ReplaceAll(text, "\\r", "\r")
	return strings.TrimSpace(text)
}

// looksLikeGroovyMultiSelect 封装当前模块的业务处理逻辑。
func looksLikeGroovyMultiSelect(typeName string) bool {
	lower := strings.ToLower(strings.TrimSpace(typeName))
	return strings.Contains(lower, "checkbox") || strings.Contains(lower, "multi")
}

// skipGroovyNoise 封装当前模块的业务处理逻辑。
func skipGroovyNoise(text string, idx int) int {
	for idx < len(text) {
		switch {
		case text[idx] == ' ' || text[idx] == '\t' || text[idx] == '\n' || text[idx] == '\r':
			idx++
		case text[idx] == '/' && idx+1 < len(text) && text[idx+1] == '/':
			idx += 2
			for idx < len(text) && text[idx] != '\n' {
				idx++
			}
		case text[idx] == '/' && idx+1 < len(text) && text[idx+1] == '*':
			idx += 2
			for idx+1 < len(text) && !(text[idx] == '*' && text[idx+1] == '/') {
				idx++
			}
			if idx+1 < len(text) {
				idx += 2
			}
		default:
			return idx
		}
	}
	return idx
}

// skipGroovySpaces 封装当前模块的业务处理逻辑。
func skipGroovySpaces(text string, idx int) int {
	for idx < len(text) && (text[idx] == ' ' || text[idx] == '\t' || text[idx] == '\n' || text[idx] == '\r') {
		idx++
	}
	return idx
}

// findGroovyStatementEnd 封装当前模块的业务处理逻辑。
func findGroovyStatementEnd(text string, start int) int {
	var (
		parenDepth   int
		bracketDepth int
		braceDepth   int
		inSingle     bool
		inDouble     bool
		inLineCmt    bool
		inBlockCmt   bool
	)
	for idx := start; idx < len(text); idx++ {
		ch := text[idx]
		next := byte(0)
		if idx+1 < len(text) {
			next = text[idx+1]
		}
		if inLineCmt {
			if ch == '\n' {
				return idx
			}
			continue
		}
		if inBlockCmt {
			if ch == '*' && next == '/' {
				inBlockCmt = false
				idx++
			}
			continue
		}
		if inSingle {
			if ch == '\\' && idx+1 < len(text) {
				idx++
				continue
			}
			if ch == '\'' {
				inSingle = false
			}
			continue
		}
		if inDouble {
			if ch == '\\' && idx+1 < len(text) {
				idx++
				continue
			}
			if ch == '"' {
				inDouble = false
			}
			continue
		}
		switch {
		case ch == '/' && next == '/':
			inLineCmt = true
			idx++
		case ch == '/' && next == '*':
			inBlockCmt = true
			idx++
		case ch == '\'':
			inSingle = true
		case ch == '"':
			inDouble = true
		case ch == '(':
			parenDepth++
		case ch == ')':
			if parenDepth > 0 {
				parenDepth--
			}
		case ch == '[':
			bracketDepth++
		case ch == ']':
			if bracketDepth > 0 {
				bracketDepth--
			}
		case ch == '{':
			braceDepth++
		case ch == '}':
			if braceDepth == 0 && parenDepth == 0 && bracketDepth == 0 {
				return idx
			}
			if braceDepth > 0 {
				braceDepth--
			}
		case (ch == '\n' || ch == ';') && parenDepth == 0 && bracketDepth == 0 && braceDepth == 0:
			return idx
		}
	}
	return len(text)
}

// findGroovyMatching 封装当前模块的业务处理逻辑。
func findGroovyMatching(text string, start int, open byte, close byte) (int, bool) {
	var (
		depth      int
		inSingle   bool
		inDouble   bool
		inLineCmt  bool
		inBlockCmt bool
	)
	for idx := start; idx < len(text); idx++ {
		ch := text[idx]
		next := byte(0)
		if idx+1 < len(text) {
			next = text[idx+1]
		}
		if inLineCmt {
			if ch == '\n' {
				inLineCmt = false
			}
			continue
		}
		if inBlockCmt {
			if ch == '*' && next == '/' {
				inBlockCmt = false
				idx++
			}
			continue
		}
		if inSingle {
			if ch == '\\' && idx+1 < len(text) {
				idx++
				continue
			}
			if ch == '\'' {
				inSingle = false
			}
			continue
		}
		if inDouble {
			if ch == '\\' && idx+1 < len(text) {
				idx++
				continue
			}
			if ch == '"' {
				inDouble = false
			}
			continue
		}
		switch {
		case ch == '/' && next == '/':
			inLineCmt = true
			idx++
		case ch == '/' && next == '*':
			inBlockCmt = true
			idx++
		case ch == '\'':
			inSingle = true
		case ch == '"':
			inDouble = true
		case ch == open:
			depth++
		case ch == close:
			depth--
			if depth == 0 {
				return idx, true
			}
		}
	}
	return 0, false
}

// isGroovyIdentStart 封装当前模块的业务处理逻辑。
func isGroovyIdentStart(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_'
}

// isGroovyIdentByte 封装当前模块的业务处理逻辑。
func isGroovyIdentByte(ch byte) bool {
	return isGroovyIdentStart(ch) || (ch >= '0' && ch <= '9')
}

// hasGroovyOpeners 封装当前模块的业务处理逻辑。
func hasGroovyOpeners(text string) bool {
	var (
		parenDepth   int
		bracketDepth int
		braceDepth   int
		inSingle     bool
		inDouble     bool
	)
	for idx := 0; idx < len(text); idx++ {
		ch := text[idx]
		if inSingle {
			if ch == '\\' && idx+1 < len(text) {
				idx++
				continue
			}
			if ch == '\'' {
				inSingle = false
			}
			continue
		}
		if inDouble {
			if ch == '\\' && idx+1 < len(text) {
				idx++
				continue
			}
			if ch == '"' {
				inDouble = false
			}
			continue
		}
		switch ch {
		case '\'':
			inSingle = true
		case '"':
			inDouble = true
		case '(':
			parenDepth++
		case '[':
			bracketDepth++
		case '{':
			braceDepth++
		case ')':
			if parenDepth > 0 {
				parenDepth--
			}
		case ']':
			if bracketDepth > 0 {
				bracketDepth--
			}
		case '}':
			if braceDepth > 0 {
				braceDepth--
			}
		}
	}
	return parenDepth > 0 || bracketDepth > 0 || braceDepth > 0
}

// hasGroovyQuotes 封装当前模块的业务处理逻辑。
func hasGroovyQuotes(text string) bool {
	var (
		inSingle bool
		inDouble bool
	)
	for idx := 0; idx < len(text); idx++ {
		ch := text[idx]
		if inSingle {
			if ch == '\\' && idx+1 < len(text) {
				idx++
				continue
			}
			if ch == '\'' {
				inSingle = false
			}
			continue
		}
		if inDouble {
			if ch == '\\' && idx+1 < len(text) {
				idx++
				continue
			}
			if ch == '"' {
				inDouble = false
			}
			continue
		}
		if ch == '\'' {
			inSingle = true
			continue
		}
		if ch == '"' {
			inDouble = true
		}
	}
	return inSingle || inDouble
}

// defaultGroovyString 封装当前模块的业务处理逻辑。
func defaultGroovyString(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value != "" {
		return value
	}
	return fallback
}

// readJSONString 封装当前模块的业务处理逻辑。
func readJSONString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var value string
	if err := json.Unmarshal(raw, &value); err == nil {
		return value
	}
	return ""
}

// parseDefaultValue 解析输入内容并返回结构化结果。
func parseDefaultValue(raw json.RawMessage) (any, string, error) {
	if len(raw) == 0 {
		return nil, "", nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, "", err
	}
	return value, stringifyDefaultValue(value), nil
}

// parseChoiceValues 解析输入内容并返回结构化结果。
func parseChoiceValues(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}

	var direct []string
	if err := json.Unmarshal(raw, &direct); err == nil {
		return normalizeChoiceValues(direct)
	}

	var anyArray []any
	if err := json.Unmarshal(raw, &anyArray); err == nil {
		values := make([]string, 0, len(anyArray))
		for _, item := range anyArray {
			values = append(values, stringifyDefaultValue(item))
		}
		return normalizeChoiceValues(values)
	}

	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return normalizeChoiceValues(splitChoiceText(text))
	}

	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err == nil {
		for _, key := range []string{"values", "choices", "items", "list"} {
			if values := parseChoiceValues(object[key]); len(values) > 0 {
				return values
			}
		}
		if valueText := readJSONString(object["value"]); strings.TrimSpace(valueText) != "" {
			return normalizeChoiceValues(splitChoiceText(valueText))
		}
	}

	return nil
}

// splitChoiceText 封装当前模块的业务处理逻辑。
func splitChoiceText(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if strings.Contains(value, "\n") || strings.Contains(value, "\r") {
		normalized := strings.ReplaceAll(value, "\r\n", "\n")
		normalized = strings.ReplaceAll(normalized, "\r", "\n")
		return strings.Split(normalized, "\n")
	}
	if strings.Contains(value, ",") {
		return strings.Split(value, ",")
	}
	return []string{value}
}

// normalizeChoiceValues 标准化输入值，保证后续逻辑使用统一格式。
func normalizeChoiceValues(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, item := range values {
		value := strings.TrimSpace(item)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

type extendedChoiceFallback struct {
	className    string
	description  string
	values       []string
	options      []choiceOptionFallback
	delimiter    string
	typeName     string
	defaultValue string
	propertyFile string
	propertyKey  string
}

type choiceOptionFallback struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// loadGitParameterChoicesIntoParams 封装当前模块的业务处理逻辑。
func (c *Client) loadGitParameterChoicesIntoParams(
	ctx context.Context,
	fullName string,
	params []pipelineparamdomain.JenkinsParamSnapshot,
) error {
	for idx := range params {
		paramName := strings.TrimSpace(params[idx].Name)
		if paramName == "" {
			continue
		}
		if !isGitParameterRawMeta(params[idx].RawMeta) {
			continue
		}

		choices, err := c.loadGitParameterChoices(ctx, fullName, paramName)
		if err != nil || len(choices) == 0 {
			continue
		}

		params[idx].RawMeta = mergeChoiceValuesIntoRawMeta(
			params[idx].RawMeta,
			extendedChoiceFallback{
				values:    choices,
				delimiter: ",",
				typeName:  "GitParameterDefinition",
			},
		)
		params[idx].SingleSelect = true
	}
	return nil
}

func parseExtendedChoiceFillValueItems(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var wrapper struct {
		Values json.RawMessage `json:"values"`
	}
	if err := json.Unmarshal(raw, &wrapper); err == nil && len(wrapper.Values) > 0 {
		return parseGitParameterChoiceValues(wrapper.Values)
	}
	return parseGitParameterChoiceValues(raw)
}

// isGitParameterRawMeta 封装当前模块的业务处理逻辑。
func isGitParameterRawMeta(rawMeta string) bool {
	trimmed := strings.TrimSpace(rawMeta)
	if trimmed == "" {
		return false
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(trimmed), &fields); err != nil {
		return false
	}
	className := strings.ToLower(strings.TrimSpace(readJSONString(fields["_class"])))
	typeName := strings.ToLower(strings.TrimSpace(readJSONString(fields["type"])))
	return strings.Contains(className, "gitparameterdefinition") || strings.Contains(typeName, "gitparameterdefinition")
}

func (c *Client) appendMissingExtendedChoiceFallbackParams(
	ctx context.Context,
	fullName string,
	fallback map[string]extendedChoiceFallback,
	params *[]pipelineparamdomain.JenkinsParamSnapshot,
	seen map[string]struct{},
) {
	names := make([]string, 0, len(fallback))
	for name := range fallback {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		item := fallback[name]
		paramName := strings.TrimSpace(name)
		if paramName == "" {
			continue
		}
		if _, exists := seen[paramName]; exists {
			continue
		}
		item = c.resolveExtendedChoiceValues(ctx, fullName, paramName, item)
		rawMeta := mergeChoiceValuesIntoRawMeta("{}", item)
		*params = append(*params, pipelineparamdomain.JenkinsParamSnapshot{
			Name:         paramName,
			ParamType:    pipelineparamdomain.ParamTypeChoice,
			SingleSelect: inferPipelineSingleSelectFromRawMeta(rawMeta, !looksLikeGroovyMultiSelect(item.typeName)),
			Required:     false,
			DefaultValue: strings.TrimSpace(item.defaultValue),
			Description:  strings.TrimSpace(item.description),
			RawMeta:      rawMeta,
			SortNo:       len(*params) + 1,
		})
		seen[paramName] = struct{}{}
	}
}

func (c *Client) resolveExtendedChoiceValues(
	ctx context.Context,
	fullName string,
	paramName string,
	item extendedChoiceFallback,
) extendedChoiceFallback {
	if len(item.options) > 0 {
		item.options = normalizeChoiceOptions(item.options)
		if len(item.values) == 0 {
			item.values = choiceValuesFromOptions(item.options)
		}
		return item
	}
	if item.propertyFile != "" && len(item.values) == 0 {
		resolved, err := c.loadExtendedChoicePropertyFileValues(ctx, fullName, paramName, item.propertyFile, item.propertyKey)
		if err == nil && len(resolved) > 0 {
			item.values = resolved
		}
	}
	options, err := c.loadExtendedChoiceBuildFormChoiceOptions(ctx, fullName, paramName)
	if err == nil && len(options) > 0 {
		item.options = options
		item.values = choiceValuesFromOptions(options)
	}
	return item
}

// loadGitParameterChoices 封装当前模块的业务处理逻辑。
func (c *Client) loadGitParameterChoices(ctx context.Context, fullName string, paramName string) ([]string, error) {
	escapedParam := url.QueryEscape(strings.TrimSpace(paramName))
	endpoint := c.baseURL + buildJenkinsJobPath(fullName) +
		"/descriptorByName/net.uaznia.lukanus.hudson.plugins.gitparameter.GitParameterDefinition/fillValueItems?param=" +
		escapedParam

	body, err := c.get(ctx, endpoint)
	if err != nil {
		return nil, err
	}

	var response struct {
		Values json.RawMessage `json:"values"`
	}
	if err := decodeJenkinsJSON(body, &response); err != nil {
		return nil, err
	}
	return parseGitParameterChoiceValues(response.Values), nil
}

// parseGitParameterChoiceValues 解析输入内容并返回结构化结果。
func parseGitParameterChoiceValues(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}

	var direct []string
	if err := json.Unmarshal(raw, &direct); err == nil {
		return normalizeChoiceValues(direct)
	}

	var objects []map[string]any
	if err := json.Unmarshal(raw, &objects); err == nil {
		values := make([]string, 0, len(objects))
		for _, item := range objects {
			value := strings.TrimSpace(stringifyDefaultValue(item["value"]))
			if value == "" {
				value = strings.TrimSpace(stringifyDefaultValue(item["name"]))
			}
			if value == "" {
				continue
			}
			values = append(values, value)
		}
		return normalizeChoiceValues(values)
	}

	var anyArray []any
	if err := json.Unmarshal(raw, &anyArray); err == nil {
		values := make([]string, 0, len(anyArray))
		for _, item := range anyArray {
			switch typed := item.(type) {
			case map[string]any:
				value := strings.TrimSpace(stringifyDefaultValue(typed["value"]))
				if value == "" {
					value = strings.TrimSpace(stringifyDefaultValue(typed["name"]))
				}
				if value == "" {
					continue
				}
				values = append(values, value)
			default:
				value := strings.TrimSpace(stringifyDefaultValue(typed))
				if value != "" {
					values = append(values, value)
				}
			}
		}
		return normalizeChoiceValues(values)
	}
	return nil
}

func parseJenkinsBuildFormChoiceValues(raw string, paramName string) []string {
	return choiceValuesFromOptions(parseJenkinsBuildFormChoiceOptions(raw, paramName))
}

func parseJenkinsBuildFormChoiceOptions(raw string, paramName string) []choiceOptionFallback {
	paramName = strings.TrimSpace(paramName)
	if strings.TrimSpace(raw) == "" || paramName == "" {
		return nil
	}

	inputName := paramName + ".value"
	block := extractJenkinsBuildFormParameterBlock(raw, paramName)
	searchText := block
	if searchText == "" {
		searchText = raw
	}

	options := make([]choiceOptionFallback, 0)
	inputIndexes := htmlInputTagPattern.FindAllStringIndex(searchText, -1)
	for idx, index := range inputIndexes {
		tag := searchText[index[0]:index[1]]
		if readHTMLAttribute(tag, "name") != inputName {
			continue
		}
		value := strings.TrimSpace(readHTMLAttribute(tag, "value"))
		if value == "" {
			value = strings.TrimSpace(readHTMLAttribute(tag, "json"))
		}
		if value != "" && value != "<DEFAULT>" {
			options = append(options, choiceOptionFallback{
				Label: extractJenkinsInputChoiceLabel(searchText, index[1], inputIndexes, idx),
				Value: value,
			})
		}
	}
	if len(options) > 0 {
		return normalizeChoiceOptions(options)
	}

	if block == "" {
		return nil
	}
	for _, match := range htmlOptionPattern.FindAllStringSubmatch(block, -1) {
		if len(match) < 2 {
			continue
		}
		value := strings.TrimSpace(readHTMLAttribute(match[0], "value"))
		label := normalizeHTMLText(match[1])
		if value == "" {
			value = label
		}
		if value != "" && value != "<DEFAULT>" {
			options = append(options, choiceOptionFallback{
				Label: label,
				Value: value,
			})
		}
	}
	return normalizeChoiceOptions(options)
}

func extractJenkinsInputChoiceLabel(block string, inputEnd int, inputIndexes [][]int, currentIndex int) string {
	if inputEnd < 0 || inputEnd > len(block) {
		return ""
	}
	end := len(block)
	if currentIndex+1 < len(inputIndexes) && inputIndexes[currentIndex+1][0] > inputEnd {
		end = inputIndexes[currentIndex+1][0]
	}
	lowerTail := strings.ToLower(block[inputEnd:end])
	for _, marker := range []string{"</label>", "</td>", "</tr>", "<br", "</div>"} {
		if markerIndex := strings.Index(lowerTail, marker); markerIndex >= 0 && inputEnd+markerIndex < end {
			end = inputEnd + markerIndex
		}
	}
	return normalizeHTMLText(block[inputEnd:end])
}

func normalizeChoiceOptions(options []choiceOptionFallback) []choiceOptionFallback {
	result := make([]choiceOptionFallback, 0, len(options))
	seen := make(map[string]struct{}, len(options))
	for _, item := range options {
		value := strings.TrimSpace(item.Value)
		label := strings.TrimSpace(item.Label)
		if value == "" {
			value = label
		}
		if value == "" || value == "<DEFAULT>" {
			continue
		}
		if label == "" {
			label = value
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, choiceOptionFallback{
			Label: label,
			Value: value,
		})
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func choiceValuesFromOptions(options []choiceOptionFallback) []string {
	values := make([]string, 0, len(options))
	for _, item := range normalizeChoiceOptions(options) {
		values = append(values, item.Value)
	}
	return normalizeChoiceValues(values)
}

func extractJenkinsBuildFormParameterBlock(raw string, paramName string) string {
	paramName = strings.TrimSpace(paramName)
	if paramName == "" {
		return ""
	}
	indexes := htmlInputTagPattern.FindAllStringIndex(raw, -1)
	for _, index := range indexes {
		tag := raw[index[0]:index[1]]
		if readHTMLAttribute(tag, "name") != "name" || readHTMLAttribute(tag, "value") != paramName {
			continue
		}
		start := index[0]
		end := len(raw)
		lowerRemainder := strings.ToLower(raw[start:])
		for _, marker := range []string{"</tbody>", "<div class=\"jenkins-form-item", "<div class='jenkins-form-item"} {
			markerIndex := strings.Index(lowerRemainder[1:], marker)
			if markerIndex >= 0 && start+1+markerIndex < end {
				end = start + 1 + markerIndex
			}
		}
		return raw[start:end]
	}
	return ""
}

func readHTMLAttribute(tag string, name string) string {
	name = strings.TrimSpace(name)
	if tag == "" || name == "" {
		return ""
	}
	pattern := regexp.MustCompile(`(?is)(?:^|\s)` + regexp.QuoteMeta(name) + `\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s>]+))`)
	match := pattern.FindStringSubmatch(tag)
	if len(match) == 0 {
		return ""
	}
	for _, value := range match[1:] {
		if value != "" {
			return html.UnescapeString(strings.TrimSpace(value))
		}
	}
	return ""
}

// loadExtendedChoiceFallback 封装当前模块的业务处理逻辑。
func (c *Client) loadExtendedChoiceFallback(ctx context.Context, fullName string) (map[string]extendedChoiceFallback, error) {
	endpoint := c.baseURL + buildJenkinsJobConfigPath(fullName)
	body, err := c.get(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	body = normalizeXMLVersion(body)

	type extendedChoiceXMLItem struct {
		Name                 string `xml:"name"`
		Description          string `xml:"description"`
		Type                 string `xml:"type"`
		Value                string `xml:"value"`
		MultiSelectDelimiter string `xml:"multiSelectDelimiter"`
		DefaultValue         string `xml:"defaultValue"`
		PropertyFile         string `xml:"propertyFile"`
		PropertyKey          string `xml:"propertyKey"`
	}
	var config struct {
		ClassicItems []extendedChoiceXMLItem `xml:"properties>hudson.model.ParametersDefinitionProperty>parameterDefinitions>com.cwctravel.hudson.plugins.extended__choice__parameter.ExtendedChoiceParameterDefinition"`
		DynamicItems []extendedChoiceXMLItem `xml:"properties>hudson.model.ParametersDefinitionProperty>parameterDefinitions>com.moded.extendedchoiceparameter.ExtendedChoiceParameterDefinition"`
	}
	if err := xml.Unmarshal(body, &config); err != nil {
		return nil, err
	}

	result := make(map[string]extendedChoiceFallback)
	appendItem := func(item extendedChoiceXMLItem, className string) {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			return
		}
		delimiter := strings.TrimSpace(item.MultiSelectDelimiter)
		if delimiter == "" {
			delimiter = ","
		}
		result[name] = extendedChoiceFallback{
			className:    className,
			description:  strings.TrimSpace(item.Description),
			values:       splitChoiceValueByDelimiter(item.Value, delimiter),
			delimiter:    delimiter,
			typeName:     strings.TrimSpace(item.Type),
			defaultValue: strings.TrimSpace(item.DefaultValue),
			propertyFile: strings.TrimSpace(item.PropertyFile),
			propertyKey:  strings.TrimSpace(item.PropertyKey),
		}
	}
	for _, item := range config.ClassicItems {
		appendItem(item, "com.cwctravel.hudson.plugins.extended_choice_parameter.ExtendedChoiceParameterDefinition")
	}
	for _, item := range config.DynamicItems {
		appendItem(item, "com.moded.extendedchoiceparameter.ExtendedChoiceParameterDefinition")
	}
	return result, nil
}

func (c *Client) loadExtendedChoicePropertyFileValues(
	ctx context.Context,
	fullName string,
	paramName string,
	propertyFile string,
	propertyKey string,
) ([]string, error) {
	endpoint := c.baseURL + buildJenkinsJobPath(fullName) +
		"/descriptorByName/com.cwctravel.hudson.plugins.extended_choice_parameter.ExtendedChoiceParameterDefinition/fillValueItems"

	form := url.Values{}
	form.Set("param", strings.TrimSpace(paramName))
	if propertyFile != "" {
		form.Set("propertyFile", propertyFile)
	}
	if propertyKey != "" {
		form.Set("propertyKey", propertyKey)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	if c.username != "" && c.apiToken != "" {
		req.SetBasicAuth(c.username, c.apiToken)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, buildJenkinsHTTPError(resp.StatusCode, body)
	}

	return parseExtendedChoiceFillValueItems(body), nil
}

func (c *Client) loadExtendedChoiceBuildFormValues(
	ctx context.Context,
	fullName string,
	paramName string,
) ([]string, error) {
	options, err := c.loadExtendedChoiceBuildFormChoiceOptions(ctx, fullName, paramName)
	if err != nil {
		return nil, err
	}
	return choiceValuesFromOptions(options), nil
}

func (c *Client) loadExtendedChoiceBuildFormChoiceOptions(
	ctx context.Context,
	fullName string,
	paramName string,
) ([]choiceOptionFallback, error) {
	endpoint := c.baseURL + buildJenkinsJobPath(fullName) + "/build?delay=0sec"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if c.username != "" && c.apiToken != "" {
		req.SetBasicAuth(c.username, c.apiToken)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	options := parseJenkinsBuildFormChoiceOptions(string(body), paramName)
	if len(options) > 0 {
		return options, nil
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, buildJenkinsHTTPError(resp.StatusCode, body)
	}
	return nil, fmt.Errorf("jenkins build form has no choices for parameter %q", strings.TrimSpace(paramName))
}

// normalizeXMLVersion 标准化输入值，保证后续逻辑使用统一格式。
func normalizeXMLVersion(body []byte) []byte {
	trimmed := bytes.TrimSpace(body)
	if !bytes.HasPrefix(trimmed, []byte("<?xml")) {
		return body
	}
	normalized := bytes.Replace(body, []byte("version='1.1'"), []byte("version='1.0'"), 1)
	normalized = bytes.Replace(normalized, []byte("version=\"1.1\""), []byte("version=\"1.0\""), 1)
	return normalized
}

// splitChoiceValueByDelimiter 封装当前模块的业务处理逻辑。
func splitChoiceValueByDelimiter(value string, delimiter string) []string {
	text := strings.TrimSpace(value)
	if text == "" {
		return nil
	}
	if strings.Contains(text, "\n") || strings.Contains(text, "\r") {
		normalized := strings.ReplaceAll(text, "\r\n", "\n")
		normalized = strings.ReplaceAll(normalized, "\r", "\n")
		return normalizeChoiceValues(strings.Split(normalized, "\n"))
	}
	if delimiter != "" && strings.Contains(text, delimiter) {
		return normalizeChoiceValues(strings.Split(text, delimiter))
	}
	return normalizeChoiceValues(splitChoiceText(text))
}

func readRawMetaPropertyFile(rawMeta string) (propertyFile string, propertyKey string) {
	trimmed := strings.TrimSpace(rawMeta)
	if trimmed == "" {
		return "", ""
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(trimmed), &fields); err != nil {
		return "", ""
	}
	return strings.TrimSpace(readJSONString(fields["propertyFile"])), strings.TrimSpace(readJSONString(fields["propertyKey"]))
}

// mergeChoiceValuesIntoRawMeta 封装当前模块的业务处理逻辑。
func mergeChoiceValuesIntoRawMeta(rawMeta string, fallback extendedChoiceFallback) string {
	meta := make(map[string]any)
	trimmed := strings.TrimSpace(rawMeta)
	if trimmed != "" && trimmed != "{}" {
		if err := json.Unmarshal([]byte(trimmed), &meta); err != nil {
			meta = make(map[string]any)
		}
	}

	values := fallback.values
	if len(values) == 0 && len(fallback.options) > 0 {
		values = choiceValuesFromOptions(fallback.options)
	}
	meta["choices"] = values
	if len(fallback.options) > 0 {
		meta["choiceOptions"] = normalizeChoiceOptions(fallback.options)
	}
	if fallback.className != "" {
		meta["_class"] = fallback.className
	}
	if fallback.propertyFile != "" {
		meta["propertyFile"] = fallback.propertyFile
	}
	if fallback.propertyKey != "" {
		meta["propertyKey"] = fallback.propertyKey
	}
	if fallback.delimiter != "" {
		meta["multiSelectDelimiter"] = fallback.delimiter
	}
	if fallback.typeName != "" {
		meta["type"] = fallback.typeName
	}
	if fallback.defaultValue != "" {
		if _, ok := meta["defaultValue"]; !ok {
			meta["defaultValue"] = fallback.defaultValue
		}
	}

	bytes, err := json.Marshal(meta)
	if err != nil {
		if trimmed == "" {
			return "{}"
		}
		return trimmed
	}
	return string(bytes)
}

// inferPipelineSingleSelect 封装当前模块的业务处理逻辑。
func inferPipelineSingleSelect(
	paramName string,
	className string,
	typeName string,
	fields map[string]json.RawMessage,
	choiceCount int,
) bool {
	lowerClass := strings.ToLower(strings.TrimSpace(className))
	lowerType := strings.ToLower(strings.TrimSpace(typeName))
	lowerName := strings.ToLower(strings.TrimSpace(paramName))

	if lowerName == "branch" {
		return true
	}

	if readJSONBool(fields["multiSelect"]) || readJSONBool(fields["multi_select"]) || readJSONBool(fields["isMulti"]) {
		return false
	}
	if strings.Contains(lowerType, "multi") ||
		strings.Contains(lowerType, "checkbox") ||
		strings.Contains(lowerClass, "multiselect") ||
		strings.Contains(lowerClass, "checkbox") {
		return false
	}
	if strings.Contains(lowerClass, "gitparameter") ||
		strings.Contains(lowerType, "single") ||
		strings.Contains(lowerType, "radio") ||
		strings.Contains(lowerClass, "choiceparameterdefinition") {
		return true
	}
	if choiceCount > 1 {
		return false
	}
	return false
}

// inferPipelineSingleSelectFromRawMeta 封装当前模块的业务处理逻辑。
func inferPipelineSingleSelectFromRawMeta(rawMeta string, fallback bool) bool {
	trimmed := strings.TrimSpace(rawMeta)
	if trimmed == "" {
		return fallback
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(trimmed), &fields); err != nil {
		return fallback
	}

	className := strings.TrimSpace(readJSONString(fields["_class"]))
	typeName := strings.TrimSpace(readJSONString(fields["type"]))
	lowerClass := strings.ToLower(className)
	lowerType := strings.ToLower(typeName)
	choices := parseChoiceValues(fields["choices"])
	if len(choices) == 0 {
		choices = parseChoiceValues(fields["value"])
	}

	if readJSONBool(fields["multiSelect"]) || readJSONBool(fields["multi_select"]) || readJSONBool(fields["isMulti"]) {
		return false
	}
	if strings.Contains(lowerType, "multi") ||
		strings.Contains(lowerType, "checkbox") ||
		strings.Contains(lowerClass, "multiselect") ||
		strings.Contains(lowerClass, "checkbox") {
		return false
	}
	if strings.Contains(lowerClass, "gitparameter") ||
		strings.Contains(lowerType, "single") ||
		strings.Contains(lowerType, "radio") ||
		strings.Contains(lowerClass, "choiceparameterdefinition") {
		return true
	}
	if len(choices) > 1 {
		return false
	}
	return fallback
}

// readJSONBool 封装当前模块的业务处理逻辑。
func readJSONBool(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	var value bool
	if err := json.Unmarshal(raw, &value); err == nil {
		return value
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		parsed, parseErr := strconv.ParseBool(strings.TrimSpace(text))
		if parseErr == nil {
			return parsed
		}
	}
	return false
}

// inferExecutorParamType 封装当前模块的业务处理逻辑。
func inferExecutorParamType(class string, choices []string, defaultValue any, defaultValueStr string) pipelineparamdomain.ParamType {
	lowerClass := strings.ToLower(strings.TrimSpace(class))
	switch {
	case len(choices) > 0 ||
		strings.Contains(lowerClass, "choice") ||
		strings.Contains(lowerClass, "gitparameter"):
		return pipelineparamdomain.ParamTypeChoice
	case strings.Contains(lowerClass, "boolean"):
		return pipelineparamdomain.ParamTypeBool
	case strings.Contains(lowerClass, "number"),
		strings.Contains(lowerClass, "float"),
		strings.Contains(lowerClass, "int"):
		return pipelineparamdomain.ParamTypeNumber
	}

	switch defaultValue.(type) {
	case float64, float32, int, int64, int32, uint, uint64:
		return pipelineparamdomain.ParamTypeNumber
	case bool:
		return pipelineparamdomain.ParamTypeBool
	}
	if defaultValueStr != "" {
		if _, err := strconv.ParseFloat(defaultValueStr, 64); err == nil {
			return pipelineparamdomain.ParamTypeNumber
		}
		if _, err := strconv.ParseBool(defaultValueStr); err == nil {
			return pipelineparamdomain.ParamTypeBool
		}
	}
	return pipelineparamdomain.ParamTypeString
}

// stringifyDefaultValue 封装当前模块的业务处理逻辑。
func stringifyDefaultValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case bool:
		if typed {
			return "true"
		}
		return "false"
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(typed), 'f', -1, 32)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case int32:
		return strconv.FormatInt(int64(typed), 10)
	case uint:
		return strconv.FormatUint(uint64(typed), 10)
	case uint64:
		return strconv.FormatUint(typed, 10)
	default:
		bytes, err := json.Marshal(typed)
		if err != nil {
			return fmt.Sprintf("%v", typed)
		}
		return string(bytes)
	}
}

type jenkinsJobNode struct {
	Name string           `json:"name"`
	URL  string           `json:"url"`
	Jobs []jenkinsJobNode `json:"jobs"`
}

// flattenJenkinsJobs 封装当前模块的业务处理逻辑。
func flattenJenkinsJobs(baseURL string, prefix string, jobs []jenkinsJobNode, result *[]domain.JenkinsJob) {
	for _, job := range jobs {
		fullName := job.Name
		if prefix != "" {
			fullName = prefix + "/" + job.Name
		}
		if len(job.Jobs) > 0 {
			flattenJenkinsJobs(baseURL, fullName, job.Jobs, result)
			continue
		}
		*result = append(*result, domain.JenkinsJob{
			Name:     job.Name,
			FullName: fullName,
			URL:      strings.TrimSpace(buildJenkinsOriginalURL(baseURL, fullName, job.URL)),
		})
	}
}

// buildJenkinsOriginalURL 组装业务执行所需的输入数据。
func buildJenkinsOriginalURL(baseURL string, fullName string, fallback string) string {
	fullName = strings.Trim(strings.TrimSpace(fullName), "/")
	if fullName != "" {
		return strings.TrimRight(strings.TrimSpace(baseURL), "/") + buildJenkinsJobPath(fullName) + "/"
	}
	return strings.TrimSpace(resolveJenkinsResourcePrefix(baseURL, fallback))
}

// buildJenkinsJobAPIPath 组装业务执行所需的输入数据。
func buildJenkinsJobAPIPath(fullName string) string {
	return buildJenkinsJobPath(fullName) + "/api/json"
}

// buildJenkinsAPIEndpoint 组装业务执行所需的输入数据。
func buildJenkinsAPIEndpoint(baseURL string, resourceURL string, tree string) string {
	prefix := resolveJenkinsResourcePrefix(baseURL, resourceURL)
	if prefix == "" {
		return ""
	}
	if strings.TrimSpace(tree) == "" {
		return prefix + "/api/json"
	}
	return prefix + "/api/json?tree=" + tree
}

// buildJenkinsWFAPIEndpoint 组装业务执行所需的输入数据。
func buildJenkinsWFAPIEndpoint(baseURL string, resourceURL string, suffix string) string {
	prefix := resolveJenkinsResourcePrefix(baseURL, resourceURL)
	if prefix == "" {
		return ""
	}
	suffix = strings.Trim(strings.TrimSpace(suffix), "/")
	if suffix == "" {
		return prefix + "/wfapi"
	}
	return prefix + "/wfapi/" + suffix
}

// buildJenkinsProgressiveTextEndpoint 组装业务执行所需的输入数据。
func buildJenkinsProgressiveTextEndpoint(baseURL string, buildURL string, start int64) string {
	prefix := resolveJenkinsResourcePrefix(baseURL, buildURL)
	if prefix == "" {
		return ""
	}
	if start < 0 {
		start = 0
	}
	return fmt.Sprintf("%s/logText/progressiveText?start=%d", prefix, start)
}

// getBuildStageDetail 组装业务执行所需的输入数据。
func (c *Client) getBuildStageDetail(
	ctx context.Context,
	buildURL string,
	stageKey string,
) (struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Status         string `json:"status"`
	StageFlowNodes []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"stageFlowNodes"`
}, error) {
	var payload struct {
		ID             string `json:"id"`
		Name           string `json:"name"`
		Status         string `json:"status"`
		StageFlowNodes []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"stageFlowNodes"`
	}

	resourceURL := resolveJenkinsResourcePrefix(c.baseURL, buildURL)
	if resourceURL == "" {
		return payload, fmt.Errorf("build url is required")
	}
	endpoint := strings.TrimRight(resourceURL, "/") + "/execution/node/" + url.PathEscape(strings.TrimSpace(stageKey)) + "/wfapi/describe"
	body, err := c.get(ctx, endpoint)
	if err != nil {
		return payload, err
	}
	if err := decodeJenkinsJSON(body, &payload); err != nil {
		return payload, err
	}
	return payload, nil
}

// getBuildNodeLog 组装业务执行所需的输入数据。
func (c *Client) getBuildNodeLog(
	ctx context.Context,
	buildURL string,
	nodeID string,
) (content string, hasMore bool, err error) {
	resourceURL := resolveJenkinsResourcePrefix(c.baseURL, buildURL)
	if resourceURL == "" {
		return "", false, fmt.Errorf("build url is required")
	}
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return "", false, fmt.Errorf("node id is required")
	}
	endpoint := strings.TrimRight(resourceURL, "/") + "/execution/node/" + url.PathEscape(nodeID) + "/wfapi/log"
	body, err := c.get(ctx, endpoint)
	if err != nil {
		return "", false, err
	}

	var payload struct {
		Text    string `json:"text"`
		HasMore bool   `json:"hasMore"`
	}
	if err := decodeJenkinsJSON(body, &payload); err != nil {
		return "", false, err
	}
	return normalizeJenkinsLogContent(payload.Text), payload.HasMore, nil
}

func (c *Client) getBuildStageLogFromConsole(
	ctx context.Context,
	buildURL string,
	stageKey string,
	stageName string,
	rawStatus string,
	originalErr error,
) (releasedomain.ReleaseOrderPipelineStageLog, error) {
	content, _, hasMore, err := c.GetBuildConsoleText(ctx, buildURL, 0)
	if err != nil {
		if originalErr != nil {
			return releasedomain.ReleaseOrderPipelineStageLog{}, originalErr
		}
		return releasedomain.ReleaseOrderPipelineStageLog{}, err
	}
	content = normalizeJenkinsLogContent(content)
	content = extractJenkinsStageConsoleSection(content, firstNonEmptyString(stageName, stageKey))
	return releasedomain.ReleaseOrderPipelineStageLog{
		StageName: firstNonEmptyString(stageName, stageKey),
		RawStatus: strings.TrimSpace(rawStatus),
		Content:   content,
		HasMore:   hasMore,
		FetchedAt: time.Now().UTC(),
	}, nil
}

func extractJenkinsStageConsoleSection(content string, stageName string) string {
	content = strings.TrimSpace(content)
	stageName = strings.TrimSpace(stageName)
	if content == "" || stageName == "" {
		return content
	}

	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	lines := strings.Split(content, "\n")
	stageNeedle := strings.ToLower(stageName)

	start := -1
	for index, line := range lines {
		normalizedLine := strings.ToLower(strings.TrimSpace(line))
		if !strings.Contains(normalizedLine, stageNeedle) {
			continue
		}
		if !strings.Contains(normalizedLine, "[pipeline] {") && !strings.Contains(normalizedLine, "stage") {
			continue
		}
		start = index
		if index > 0 && strings.Contains(strings.ToLower(strings.TrimSpace(lines[index-1])), "[pipeline] stage") {
			start = index - 1
		}
		break
	}
	if start < 0 {
		return content
	}

	end := len(lines)
	for index := start + 1; index < len(lines); index++ {
		normalizedLine := strings.ToLower(strings.TrimSpace(lines[index]))
		if strings.Contains(normalizedLine, "[pipeline] // stage") {
			end = index + 1
			break
		}
		if index > start+1 && strings.Contains(normalizedLine, "[pipeline] {") && !strings.Contains(normalizedLine, stageNeedle) {
			end = index
			break
		}
	}
	return strings.TrimSpace(strings.Join(lines[start:end], "\n"))
}

func decodeJenkinsJSON(body []byte, target any) error {
	if err := json.Unmarshal(body, target); err != nil {
		if looksLikeHTMLResponse(body) {
			message := extractJenkinsErrorMessage(string(body))
			if message == "" {
				message = "返回 HTML 页面，可能是登录失效、权限不足或 Jenkins 插件页面不可用"
			}
			return fmt.Errorf("Jenkins 返回了非 JSON 响应：%s", message)
		}
		return err
	}
	return nil
}

func looksLikeHTMLResponse(body []byte) bool {
	text := strings.ToLower(strings.TrimSpace(string(body)))
	if text == "" {
		return false
	}
	return strings.HasPrefix(text, "<!doctype html") ||
		strings.HasPrefix(text, "<html") ||
		strings.Contains(text, "<body") ||
		strings.Contains(text, "</html>")
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// buildJenkinsActionEndpoint 组装业务执行所需的输入数据。
func buildJenkinsActionEndpoint(baseURL string, resourceURL string, action string) string {
	prefix := resolveJenkinsResourcePrefix(baseURL, resourceURL)
	if prefix == "" {
		return ""
	}

	action = strings.Trim(strings.TrimSpace(action), "/")
	if action == "" {
		return prefix
	}
	return prefix + "/" + action
}

// resolveJenkinsResourcePrefix 解析上下文数据，得到后续流程需要的结果。
func resolveJenkinsResourcePrefix(baseURL string, resourceURL string) string {
	trimmed := strings.TrimSpace(resourceURL)
	if trimmed == "" {
		return ""
	}
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")

	if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
		parsedResource, resourceErr := url.Parse(trimmed)
		parsedBase, baseErr := url.Parse(base)
		if resourceErr == nil && baseErr == nil && parsedBase.Scheme != "" && parsedBase.Host != "" {
			parsedResource.Scheme = parsedBase.Scheme
			parsedResource.Host = parsedBase.Host
			parsedResource.User = parsedBase.User
			parsedResource.Fragment = ""
			return strings.TrimRight(parsedResource.String(), "/")
		}
		return strings.TrimRight(trimmed, "/")
	}
	if strings.HasPrefix(trimmed, "/") {
		return base + strings.TrimRight(trimmed, "/")
	}
	return base + "/" + strings.Trim(trimmed, "/")
}

// jenkinsMillisToTime 查询并返回列表数据。
func jenkinsMillisToTime(value int64) *time.Time {
	if value <= 0 {
		return nil
	}
	t := time.UnixMilli(value).UTC()
	return &t
}

// deriveStageFinishedAt 封装当前模块的业务处理逻辑。
func deriveStageFinishedAt(startedAt *time.Time, durationMillis int64, rawStatus string) *time.Time {
	if startedAt == nil || durationMillis <= 0 {
		return nil
	}
	status := strings.ToUpper(strings.TrimSpace(rawStatus))
	switch status {
	case "IN_PROGRESS", "PAUSED_PENDING_INPUT":
		return nil
	default:
		t := startedAt.Add(time.Duration(durationMillis) * time.Millisecond)
		return &t
	}
}

// maxInt64 封装当前模块的业务处理逻辑。
func maxInt64(value int64, minimum int64) int64 {
	if value < minimum {
		return minimum
	}
	return value
}

// mapJenkinsStageStatus 封装当前模块的业务处理逻辑。
func mapJenkinsStageStatus(raw string) releasedomain.PipelineStageStatus {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "SUCCESS":
		return releasedomain.PipelineStageStatusSuccess
	case "FAILED", "FAILURE", "ERROR", "UNSTABLE":
		return releasedomain.PipelineStageStatusFailed
	case "ABORTED":
		return releasedomain.PipelineStageStatusCancelled
	case "NOT_EXECUTED":
		return releasedomain.PipelineStageStatusSkipped
	case "IN_PROGRESS", "PAUSED_PENDING_INPUT":
		return releasedomain.PipelineStageStatusRunning
	case "PENDING", "QUEUED":
		return releasedomain.PipelineStageStatusPending
	default:
		return releasedomain.PipelineStageStatusPending
	}
}

// buildJenkinsJobConfigPath 组装业务执行所需的输入数据。
func buildJenkinsJobConfigPath(fullName string) string {
	return buildJenkinsJobPath(fullName) + "/config.xml"
}

// buildJenkinsCreateItemPath 组装业务执行所需的输入数据。
func buildJenkinsCreateItemPath(parentFullName string) string {
	parentFullName = strings.Trim(strings.TrimSpace(parentFullName), "/")
	if parentFullName == "" {
		return "/createItem"
	}
	return buildJenkinsJobPath(parentFullName) + "/createItem"
}

// buildJenkinsJobPath 组装业务执行所需的输入数据。
func buildJenkinsJobPath(fullName string) string {
	parts := strings.Split(strings.Trim(fullName, "/"), "/")
	var builder strings.Builder
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			continue
		}
		builder.WriteString("/job/")
		builder.WriteString(part)
	}
	return builder.String()
}

// splitJenkinsJobFullName 封装当前模块的业务处理逻辑。
func splitJenkinsJobFullName(fullName string) (jobName string, parentPath string) {
	fullName = strings.Trim(strings.TrimSpace(fullName), "/")
	if fullName == "" {
		return "", ""
	}
	parts := strings.Split(fullName, "/")
	jobName = strings.TrimSpace(parts[len(parts)-1])
	if len(parts) > 1 {
		parentPath = strings.Join(parts[:len(parts)-1], "/")
	}
	return jobName, strings.Trim(strings.TrimSpace(parentPath), "/")
}

type crumbHeader struct {
	field string
	value string
}

// getCrumb 查询并返回指定资源数据。
func (c *Client) getCrumb(ctx context.Context) (string, string, error) {
	endpoint := c.baseURL + "/crumbIssuer/api/json"
	body, err := c.get(ctx, endpoint)
	if err != nil {
		return "", "", err
	}

	var payload struct {
		CrumbRequestField string `json:"crumbRequestField"`
		Crumb             string `json:"crumb"`
	}
	if err := decodeJenkinsJSON(body, &payload); err != nil {
		return "", "", err
	}
	field := strings.TrimSpace(payload.CrumbRequestField)
	value := strings.TrimSpace(payload.Crumb)
	if field == "" || value == "" {
		return "", "", fmt.Errorf("jenkins crumb is empty")
	}
	return field, value, nil
}

// post 封装当前模块的业务处理逻辑。
func (c *Client) post(ctx context.Context, endpoint string, encodedForm string, crumb crumbHeader) (string, int, error) {
	queueURL, statusCode, _, err := c.doPost(ctx, endpoint, encodedForm, crumb, true)
	return queueURL, statusCode, err
}

// doPost 封装当前模块的业务处理逻辑。
func (c *Client) doPost(
	ctx context.Context,
	endpoint string,
	encodedForm string,
	crumb crumbHeader,
	followRedirect bool,
) (string, int, []byte, error) {
	var bodyReader io.Reader
	if encodedForm != "" {
		bodyReader = strings.NewReader(encodedForm)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bodyReader)
	if err != nil {
		return "", 0, nil, err
	}
	if c.username != "" && c.apiToken != "" {
		req.SetBasicAuth(c.username, c.apiToken)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if crumb.field != "" && crumb.value != "" {
		req.Header.Set(crumb.field, crumb.value)
	}

	client := c.client
	if !followRedirect {
		client = &http.Client{
			Timeout: c.client.Timeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", 0, nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	responseBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if readErr != nil {
		return "", resp.StatusCode, nil, readErr
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusBadRequest {
		return "", resp.StatusCode, responseBody, buildJenkinsHTTPError(resp.StatusCode, responseBody)
	}
	queueURL := strings.TrimSpace(resp.Header.Get("Location"))
	return queueURL, resp.StatusCode, responseBody, nil
}

// postXML 封装当前模块的业务处理逻辑。
func (c *Client) postXML(ctx context.Context, endpoint string, payload string) error {
	statusCode, err := c.postXMLOnce(ctx, endpoint, payload, crumbHeader{})
	if err == nil {
		return nil
	}
	if statusCode == http.StatusForbidden {
		crumbField, crumbValue, crumbErr := c.getCrumb(ctx)
		if crumbErr == nil {
			if _, retryErr := c.postXMLOnce(ctx, endpoint, payload, crumbHeader{field: crumbField, value: crumbValue}); retryErr == nil {
				return nil
			} else {
				err = retryErr
			}
		}
	}
	return err
}

// postXMLOnce 封装当前模块的业务处理逻辑。
func (c *Client) postXMLOnce(ctx context.Context, endpoint string, payload string, crumb crumbHeader) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(payload))
	if err != nil {
		return 0, err
	}
	if c.username != "" && c.apiToken != "" {
		req.SetBasicAuth(c.username, c.apiToken)
	}
	req.Header.Set("Content-Type", "application/xml; charset=UTF-8")
	if crumb.field != "" && crumb.value != "" {
		req.Header.Set(crumb.field, crumb.value)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if readErr != nil {
		return resp.StatusCode, readErr
	}
	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusBadRequest {
		return resp.StatusCode, nil
	}
	return resp.StatusCode, buildJenkinsHTTPError(resp.StatusCode, body)
}

// buildRawPipelineConfigXML 组装业务执行所需的输入数据。
func buildRawPipelineConfigXML(cfg domain.JenkinsRawPipelineConfig) string {
	sandbox := "false"
	if cfg.Sandbox {
		sandbox = "true"
	}

	var descriptionBuilder strings.Builder
	_ = xml.EscapeText(&descriptionBuilder, []byte(strings.TrimSpace(cfg.Description)))

	scriptText := strings.ReplaceAll(cfg.Script, "\r\n", "\n")
	scriptText = strings.ReplaceAll(scriptText, "\r", "\n")
	var scriptBuilder strings.Builder
	_ = xml.EscapeText(&scriptBuilder, []byte(scriptText))

	return fmt.Sprintf(`<?xml version='1.0' encoding='UTF-8'?>
<flow-definition plugin="workflow-job">
  <actions/>
  <description>%s</description>
  <keepDependencies>false</keepDependencies>
  <properties/>
  <definition class="org.jenkinsci.plugins.workflow.cps.CpsFlowDefinition" plugin="workflow-cps">
    <script>%s</script>
    <sandbox>%s</sandbox>
  </definition>
  <triggers/>
  <disabled>false</disabled>
</flow-definition>`, descriptionBuilder.String(), scriptBuilder.String(), sandbox)
}

// postAction 封装当前模块的业务处理逻辑。
func (c *Client) postAction(ctx context.Context, endpoint string) error {
	statusCode, err := c.postActionOnce(ctx, endpoint, crumbHeader{})
	if err == nil {
		return nil
	}
	if statusCode == http.StatusForbidden {
		crumbField, crumbValue, crumbErr := c.getCrumb(ctx)
		if crumbErr == nil {
			statusCode, err = c.postActionOnce(ctx, endpoint, crumbHeader{field: crumbField, value: crumbValue})
			if err == nil {
				return nil
			}
		}
	}
	if statusCode == http.StatusNotFound || statusCode == http.StatusGone {
		// Already gone/finished, treat as idempotent success.
		return nil
	}
	return err
}

// postActionOnce 封装当前模块的业务处理逻辑。
func (c *Client) postActionOnce(ctx context.Context, endpoint string, crumb crumbHeader) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return 0, err
	}
	if c.username != "" && c.apiToken != "" {
		req.SetBasicAuth(c.username, c.apiToken)
	}
	if crumb.field != "" && crumb.value != "" {
		req.Header.Set(crumb.field, crumb.value)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if readErr != nil {
		return resp.StatusCode, readErr
	}

	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusBadRequest {
		return resp.StatusCode, nil
	}
	return resp.StatusCode, buildJenkinsHTTPError(resp.StatusCode, body)
}

// get 查询并返回指定资源数据。
func (c *Client) get(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if c.username != "" && c.apiToken != "" {
		req.SetBasicAuth(c.username, c.apiToken)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, buildJenkinsHTTPError(resp.StatusCode, body)
	}
	return body, nil
}

var (
	htmlParagraphPattern = regexp.MustCompile(`(?is)<p[^>]*>(.*?)</p>`)
	htmlH2Pattern        = regexp.MustCompile(`(?is)<h2[^>]*>(.*?)</h2>`)
	htmlInputTagPattern  = regexp.MustCompile(`(?is)<input\b[^>]*>`)
	htmlOptionPattern    = regexp.MustCompile(`(?is)<option\b[^>]*>(.*?)</option>`)
	htmlBreakPattern     = regexp.MustCompile(`(?is)<br\\s*/?>`)
	htmlStylePattern     = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	htmlScriptPattern    = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	htmlTagPattern       = regexp.MustCompile(`(?is)<[^>]+>`)
	multiSpacePattern    = regexp.MustCompile(`\s+`)
	paramInvalidPattern  = regexp.MustCompile(`(?i)parameter\s+[^\n]{1,300}?\s+is\s+invalid`)
	httpErrorPattern     = regexp.MustCompile(`(?i)http error\s+\d+\s+[^\n]{1,120}`)
)

// buildJenkinsHTTPError 组装业务执行所需的输入数据。
func buildJenkinsHTTPError(statusCode int, body []byte) error {
	message := extractJenkinsErrorMessage(string(body))
	if message == "" {
		return fmt.Errorf("jenkins request failed: status=%d", statusCode)
	}
	return fmt.Errorf("jenkins request failed: status=%d message=%s", statusCode, message)
}

// extractJenkinsErrorMessage 封装当前模块的业务处理逻辑。
func extractJenkinsErrorMessage(raw string) string {
	text := strings.TrimSpace(raw)
	if text == "" {
		return ""
	}

	for _, matcher := range []*regexp.Regexp{htmlParagraphPattern, htmlH2Pattern} {
		matches := matcher.FindAllStringSubmatch(text, 3)
		for _, match := range matches {
			if len(match) < 2 {
				continue
			}
			candidate := normalizeHTMLText(match[1])
			if reason := extractKnownJenkinsReason(candidate); reason != "" {
				return reason
			}
			if looksLikeMeaningfulMessage(candidate) {
				return candidate
			}
		}
	}

	candidate := normalizeHTMLText(text)
	if reason := extractKnownJenkinsReason(candidate); reason != "" {
		return reason
	}
	if candidate == "" {
		return ""
	}
	if len(candidate) > 220 {
		return candidate[:220] + "..."
	}
	return candidate
}

// normalizeHTMLText 标准化输入值，保证后续逻辑使用统一格式。
func normalizeHTMLText(raw string) string {
	decoded := html.UnescapeString(strings.TrimSpace(raw))
	if decoded == "" {
		return ""
	}
	decoded = stripHTMLNoise(decoded)
	decoded = htmlTagPattern.ReplaceAllString(decoded, " ")
	decoded = multiSpacePattern.ReplaceAllString(decoded, " ")
	return strings.TrimSpace(decoded)
}

// normalizeJenkinsLogContent 标准化输入值，保证后续逻辑使用统一格式。
func normalizeJenkinsLogContent(raw string) string {
	decoded := html.UnescapeString(strings.TrimSpace(raw))
	if decoded == "" {
		return ""
	}
	decoded = stripHTMLNoise(decoded)
	decoded = strings.ReplaceAll(decoded, "\r\n", "\n")
	decoded = strings.ReplaceAll(decoded, "\r", "\n")
	decoded = htmlBreakPattern.ReplaceAllString(decoded, "\n")
	decoded = htmlTagPattern.ReplaceAllString(decoded, "")
	return strings.TrimSpace(decoded)
}

func stripHTMLNoise(raw string) string {
	cleaned := htmlStylePattern.ReplaceAllString(raw, "")
	cleaned = htmlScriptPattern.ReplaceAllString(cleaned, "")
	return cleaned
}

// looksLikeMeaningfulMessage 封装当前模块的业务处理逻辑。
func looksLikeMeaningfulMessage(message string) bool {
	if message == "" {
		return false
	}
	lower := strings.ToLower(message)
	if lower == "jenkins - jenkins" {
		return false
	}
	if strings.Contains(lower, "skip to content") {
		return false
	}
	if len(message) > 220 {
		return false
	}
	return true
}

// extractKnownJenkinsReason 封装当前模块的业务处理逻辑。
func extractKnownJenkinsReason(text string) string {
	if text == "" {
		return ""
	}
	if match := paramInvalidPattern.FindString(text); match != "" {
		return strings.TrimSpace(match)
	}
	if match := httpErrorPattern.FindString(text); match != "" {
		return strings.TrimSpace(match)
	}
	return ""
}

// parseJenkinsMoreData 解析输入内容并返回结构化结果。
func parseJenkinsMoreData(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "1", "yes":
		return true
	default:
		return false
	}
}
