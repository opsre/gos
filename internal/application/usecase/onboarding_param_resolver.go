package usecase

import (
	"crypto/sha256"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	ep "gos/internal/domain/executorparam"
	ob "gos/internal/domain/onboarding"
	pp "gos/internal/domain/platformparam"
)

type OnboardingField struct {
	Key     string `json:"key"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Builtin bool   `json:"builtin"`
}
type OnboardingParamRow struct {
	Scope        string                  `json:"scope"`
	PipelineID   string                  `json:"pipeline_id"`
	ID           string                  `json:"id"`
	Name         string                  `json:"name"`
	Description  string                  `json:"description"`
	Type         string                  `json:"type"`
	Required     bool                    `json:"required"`
	DefaultValue string                  `json:"default_value"`
	Choices      []string                `json:"choices"`
	MappedKey    string                  `json:"mapped_key"`
	SuggestedKey string                  `json:"suggested_key"`
	Candidates   []string                `json:"candidates"`
	NewKey       string                  `json:"new_key"`
	Problem      string                  `json:"problem"`
	Runtime      bool                    `json:"runtime"`
	Sensitive    bool                    `json:"sensitive"`
	Live         ep.JenkinsParamSnapshot `json:"-"`
}
type OnboardingInspection struct {
	Rows   []OnboardingParamRow `json:"rows"`
	Fields []OnboardingField    `json:"fields"`
	Issues []ob.Issue           `json:"issues"`
}

var onboardingKeyChars = regexp.MustCompile(`[^a-z0-9_]+`)
var onboardingSensitive = regexp.MustCompile(`(?i)(password|passwd|secret|token|private.?key|credential)`)

func onboardingSuggestedKey(name string) string {
	key := strings.Trim(onboardingKeyChars.ReplaceAllString(strings.ToLower(name), "_"), "_")
	if key == "" {
		key = "parameter"
	}
	if key[0] < 'a' || key[0] > 'z' {
		key = "p_" + key
	}
	if len(key) > 90 {
		sum := sha256.Sum256([]byte(name))
		key = fmt.Sprintf("%s_%x", key[:81], sum[:4])
	}
	return key
}
func onboardingCompatible(actual ep.ParamType, field pp.ParamType) bool {
	return string(actual) == string(field) || (actual == ep.ParamTypeChoice && field == pp.ParamTypeString)
}
func ResolveOnboardingParamSuggestions(scope, pipelineID string, live ep.JenkinsParamSnapshot, stored ep.ExecutorParamDef, fields []pp.PlatformParamDict) OnboardingParamRow {
	row := OnboardingParamRow{Scope: scope, PipelineID: pipelineID, ID: executorParamDefID(pipelineID, "jenkins", live.Name), Name: live.Name, Description: live.Description, Type: string(live.ParamType), Required: live.Required, DefaultValue: live.DefaultValue, Choices: extractChoiceCandidates(live.RawMeta), MappedKey: stored.ParamKey, NewKey: onboardingSuggestedKey(live.Name), Candidates: []string{}, Runtime: defaultJenkinsExecutorParamKey(live.Name) != "", Live: live}
	if onboardingSensitive.MatchString(live.Name) || strings.Contains(strings.ToLower(live.RawMeta), "passwordparameter") || strings.Contains(strings.ToLower(live.RawMeta), "credentialsparameter") {
		row.Sensitive = true
		row.DefaultValue = ""
		row.Live.DefaultValue = ""
		row.Choices = nil
		row.Live.RawMeta = ""
		row.Problem = "敏感参数请使用执行端凭据或受控高级配置"
		return row
	}
	if !live.ParamType.Valid() || (live.ParamType == ep.ParamTypeChoice && (!live.SingleSelect || len(row.Choices) == 0)) {
		row.Problem = "此参数类型或动态候选需要高级配置"
		return row
	}
	byKey := map[string]pp.PlatformParamDict{}
	for _, field := range fields {
		byKey[field.ParamKey] = field
	}
	if row.MappedKey != "" {
		field, ok := byKey[row.MappedKey]
		if row.Runtime && row.MappedKey != defaultJenkinsExecutorParamKey(live.Name) {
			row.Problem = "运行时参数的共享映射不是对应的 CI 内置字段，请在高级配置中修复"
		} else if !ok || field.Status != pp.StatusEnabled || field.CDSelfFill || !onboardingCompatible(live.ParamType, field.ParamType) {
			row.Problem = "已有共享映射失效或类型不兼容，请在标准字段管理中修复"
		} else {
			row.SuggestedKey = row.MappedKey
		}
		return row
	}
	if row.Runtime {
		key := defaultJenkinsExecutorParamKey(live.Name)
		field, ok := byKey[key]
		if ok && field.Status == pp.StatusEnabled && !field.CDSelfFill && onboardingCompatible(live.ParamType, field.ParamType) {
			row.SuggestedKey = key
			return row
		}
	}
	for _, field := range fields {
		if field.Status != pp.StatusEnabled || field.CDSelfFill || !onboardingCompatible(live.ParamType, field.ParamType) {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(live.Name), field.ParamKey) || strings.EqualFold(strings.TrimSpace(live.Name), field.Name) {
			row.Candidates = append(row.Candidates, field.ParamKey)
		}
	}
	if len(row.Candidates) == 1 {
		row.SuggestedKey = row.Candidates[0]
	}
	return row
}
func onboardingValidateValue(row OnboardingParamRow, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s 固定值不能为空", row.Name)
	}
	switch row.Type {
	case "bool":
		if value != "true" && value != "false" {
			return fmt.Errorf("%s 必须为 true 或 false", row.Name)
		}
	case "number":
		if number, err := strconv.ParseFloat(value, 64); err != nil || math.IsNaN(number) || math.IsInf(number, 0) {
			return fmt.Errorf("%s 必须为数值", row.Name)
		}
	case "choice":
		found := false
		for _, v := range row.Choices {
			if value == v {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("%s 不在当前管线候选值中", row.Name)
		}
	}
	return nil
}
