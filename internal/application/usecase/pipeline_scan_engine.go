package usecase

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	scandomain "gos/internal/domain/pipelinescan"
)

type PipelineScanEngine struct{}

type pipelineRuleDSL struct {
	Conditions []pipelineRuleMatcher `json:"conditions"`
	Matcher    pipelineRuleMatcher   `json:"matcher"`
}

type pipelineRuleMatcher struct {
	Type               string                 `json:"type"`
	Target             string                 `json:"target"`
	Pattern            string                 `json:"pattern"`
	Expected           *bool                  `json:"expected"`
	StartPattern       string                 `json:"start_pattern"`
	RequiredPatterns   []string               `json:"required_patterns"`
	RequiredParameters []string               `json:"required_parameters"`
	ForbiddenPatterns  []string               `json:"forbidden_patterns"`
	WhenPattern        string                 `json:"when_pattern"`
	ThenPattern        string                 `json:"then_pattern"`
	MaxLines           int                    `json:"max_lines"`
	Format             *pipelineCommandFormat `json:"format"`
}

type pipelineCommandFormat struct {
	Mode             string                      `json:"mode"`
	MaxLines         int                         `json:"max_lines"`
	AllowExtraLines  bool                        `json:"allow_extra_lines"`
	RequireBlock     bool                        `json:"require_block"`
	IgnoreIndent     bool                        `json:"ignore_indent"`
	IgnoreQuoteStyle bool                        `json:"ignore_quote_style"`
	Lines            []pipelineCommandFormatLine `json:"lines"`
}

type pipelineCommandFormatLine struct {
	Name    string `json:"name"`
	Pattern string `json:"pattern"`
}

type pipelineScriptLine struct {
	No   int
	Text string
}

type pipelineCommandBlock struct {
	StartLine int
	Lines     []pipelineScriptLine
}

// NewPipelineScanEngine 创建并返回对应组件实例。
func NewPipelineScanEngine() *PipelineScanEngine {
	return &PipelineScanEngine{}
}

// ScanScript 按规则扫描 Jenkins Pipeline 脚本。
func (e *PipelineScanEngine) ScanScript(
	pipelineID string,
	_ string,
	script string,
	rules []scandomain.Rule,
) ([]scandomain.Finding, error) {
	if e == nil {
		e = NewPipelineScanEngine()
	}
	findings := make([]scandomain.Finding, 0)
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		dsl, err := parsePipelineRuleDSL(rule.RuleDSL)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid pipeline scan rule %s: %v", ErrInvalidInput, rule.RuleCode, err)
		}
		if !matchPipelineConditions(script, dsl.Conditions) {
			continue
		}
		ruleFindings, err := e.applyMatcher(pipelineID, script, rule, dsl.Matcher)
		if err != nil {
			return nil, fmt.Errorf("%w: scan rule %s failed: %v", ErrInvalidInput, rule.RuleCode, err)
		}
		findings = append(findings, ruleFindings...)
	}
	return findings, nil
}

// PipelineScriptHash 计算脚本 hash。
func PipelineScriptHash(script string) string {
	sum := sha256.Sum256([]byte(script))
	return hex.EncodeToString(sum[:])
}

func parsePipelineRuleDSL(raw string) (pipelineRuleDSL, error) {
	var dsl pipelineRuleDSL
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &dsl); err != nil {
		return pipelineRuleDSL{}, err
	}
	if strings.TrimSpace(dsl.Matcher.Type) == "" {
		return pipelineRuleDSL{}, fmt.Errorf("matcher.type is required")
	}
	return dsl, nil
}

func matchPipelineConditions(script string, conditions []pipelineRuleMatcher) bool {
	for _, condition := range conditions {
		matched, err := evaluatePipelineMatcher(script, condition)
		if err != nil || !matched {
			return false
		}
	}
	return true
}

func (e *PipelineScanEngine) applyMatcher(
	pipelineID string,
	script string,
	rule scandomain.Rule,
	matcher pipelineRuleMatcher,
) ([]scandomain.Finding, error) {
	switch strings.TrimSpace(matcher.Type) {
	case "contains", "regex", "forbidden", "paired":
		matched, err := evaluatePipelineMatcher(script, matcher)
		if err != nil {
			return nil, err
		}
		if matched {
			return nil, nil
		}
		return []scandomain.Finding{newPipelineFinding(pipelineID, rule, firstPatternLine(script, matcher), matcherDetails(matcher))}, nil
	case "command_block":
		return e.applyCommandBlockMatcher(pipelineID, script, rule, matcher)
	case "command_format":
		return e.applyCommandFormatMatcher(pipelineID, script, rule, matcher)
	case "pipeline_parameters":
		return e.applyPipelineParametersMatcher(pipelineID, script, rule, matcher)
	default:
		return nil, fmt.Errorf("unsupported matcher type %q", matcher.Type)
	}
}

func evaluatePipelineMatcher(script string, matcher pipelineRuleMatcher) (bool, error) {
	switch strings.TrimSpace(matcher.Type) {
	case "contains":
		expected := true
		if matcher.Expected != nil {
			expected = *matcher.Expected
		}
		return strings.Contains(script, matcher.Pattern) == expected, nil
	case "regex":
		re, err := regexp.Compile(matcher.Pattern)
		if err != nil {
			return false, err
		}
		expected := true
		if matcher.Expected != nil {
			expected = *matcher.Expected
		}
		return re.MatchString(script) == expected, nil
	case "forbidden":
		re, err := regexp.Compile(matcher.Pattern)
		if err != nil {
			return false, err
		}
		return !re.MatchString(script), nil
	case "paired":
		whenRe, err := regexp.Compile(matcher.WhenPattern)
		if err != nil {
			return false, err
		}
		thenRe, err := regexp.Compile(matcher.ThenPattern)
		if err != nil {
			return false, err
		}
		return !whenRe.MatchString(script) || thenRe.MatchString(script), nil
	default:
		return false, fmt.Errorf("unsupported condition type %q", matcher.Type)
	}
}

func (e *PipelineScanEngine) applyCommandBlockMatcher(
	pipelineID string,
	script string,
	rule scandomain.Rule,
	matcher pipelineRuleMatcher,
) ([]scandomain.Finding, error) {
	blocks, err := findPipelineCommandBlocks(script, matcher.StartPattern, normalizedMaxLines(matcher.MaxLines, 20))
	if err != nil {
		return nil, err
	}
	findings := make([]scandomain.Finding, 0)
	for _, block := range blocks {
		blockText := pipelineBlockText(block)
		missing := missingPatterns(blockText, matcher.RequiredPatterns)
		forbidden := presentPatterns(blockText, matcher.ForbiddenPatterns)
		if len(missing) == 0 && len(forbidden) == 0 {
			continue
		}
		details, _ := json.Marshal(map[string][]string{
			"missing_patterns":   missing,
			"forbidden_patterns": forbidden,
		})
		finding := newPipelineFinding(pipelineID, rule, block.Lines[0], string(details))
		finding.MatchedText = blockText
		findings = append(findings, finding)
	}
	return findings, nil
}

func (e *PipelineScanEngine) applyCommandFormatMatcher(
	pipelineID string,
	script string,
	rule scandomain.Rule,
	matcher pipelineRuleMatcher,
) ([]scandomain.Finding, error) {
	if matcher.Format == nil {
		return nil, fmt.Errorf("format is required")
	}
	maxLines := normalizedMaxLines(matcher.Format.MaxLines, 20)
	blocks, err := findPipelineCommandBlocks(script, matcher.StartPattern, maxLines)
	if err != nil {
		return nil, err
	}
	findings := make([]scandomain.Finding, 0)
	if len(blocks) == 0 && matcher.Format.RequireBlock {
		rawDetails, _ := json.Marshal(map[string][]string{"mismatches": []string{"未找到完整命令"}})
		finding := newPipelineFinding(pipelineID, rule, pipelineScriptLine{No: 0}, string(rawDetails))
		finding.Message = "管线中未找到该完整命令"
		return []scandomain.Finding{
			finding,
		}, nil
	}
	for _, block := range blocks {
		details := commandFormatMismatches(block, *matcher.Format)
		if len(details) == 0 {
			continue
		}
		rawDetails, _ := json.Marshal(map[string][]string{"mismatches": details})
		finding := newPipelineFinding(pipelineID, rule, block.Lines[0], string(rawDetails))
		finding.MatchedText = pipelineBlockText(block)
		findings = append(findings, finding)
	}
	return findings, nil
}

func (e *PipelineScanEngine) applyPipelineParametersMatcher(
	pipelineID string,
	script string,
	rule scandomain.Rule,
	matcher pipelineRuleMatcher,
) ([]scandomain.Finding, error) {
	required := normalizeRequiredPipelineParameters(matcher.RequiredParameters)
	if len(required) == 0 {
		return nil, fmt.Errorf("required_parameters is required")
	}
	declared := collectPipelineParameterNames(script)
	missing := make([]string, 0)
	for _, name := range required {
		if _, ok := declared[strings.ToUpper(name)]; !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return nil, nil
	}
	details, _ := json.Marshal(map[string][]string{"missing_parameters": missing})
	finding := newPipelineFinding(pipelineID, rule, firstPipelineParameterLine(script), string(details))
	finding.MatchedText = strings.Join(missing, ", ")
	return []scandomain.Finding{finding}, nil
}

func normalizeRequiredPipelineParameters(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		key := strings.ToUpper(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	return result
}

func collectPipelineParameterNames(script string) map[string]pipelineScriptLine {
	result := make(map[string]pipelineScriptLine)
	re := regexp.MustCompile(`(?i)\bname\s*[:=]\s*['"]([A-Za-z_][A-Za-z0-9_.-]*)['"]`)
	for _, line := range splitPipelineScriptLines(script) {
		matches := re.FindAllStringSubmatch(line.Text, -1)
		for _, match := range matches {
			if len(match) < 2 {
				continue
			}
			name := strings.TrimSpace(match[1])
			if name == "" {
				continue
			}
			result[strings.ToUpper(name)] = line
		}
	}
	return result
}

func firstPipelineParameterLine(script string) pipelineScriptLine {
	re := regexp.MustCompile(`(?i)\bparameters\s*\{`)
	for _, line := range splitPipelineScriptLines(script) {
		if re.MatchString(line.Text) {
			return line
		}
	}
	return pipelineScriptLine{No: 1}
}

func findPipelineCommandBlocks(script string, startPattern string, maxLines int) ([]pipelineCommandBlock, error) {
	re, err := regexp.Compile(startPattern)
	if err != nil {
		return nil, err
	}
	lines := splitPipelineScriptLines(script)
	blocks := make([]pipelineCommandBlock, 0)
	for index, line := range lines {
		if !re.MatchString(line.Text) {
			continue
		}
		block := pipelineCommandBlock{StartLine: line.No}
		for offset := 0; offset < maxLines && index+offset < len(lines); offset++ {
			current := lines[index+offset]
			block.Lines = append(block.Lines, current)
			trimmed := strings.TrimSpace(current.Text)
			if offset > 0 && trimmed == `"""` {
				break
			}
			if offset > 0 && strings.Contains(trimmed, "-f") && !strings.HasSuffix(trimmed, `\`) {
				break
			}
		}
		blocks = append(blocks, block)
	}
	return blocks, nil
}

func splitPipelineScriptLines(script string) []pipelineScriptLine {
	normalized := strings.ReplaceAll(script, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	rawLines := strings.Split(normalized, "\n")
	lines := make([]pipelineScriptLine, 0, len(rawLines))
	for index, line := range rawLines {
		lines = append(lines, pipelineScriptLine{
			No:   index + 1,
			Text: line,
		})
	}
	return lines
}

func missingPatterns(text string, patterns []string) []string {
	result := make([]string, 0)
	for _, pattern := range patterns {
		if strings.TrimSpace(pattern) == "" {
			continue
		}
		re, err := regexp.Compile(pattern)
		if err != nil || !re.MatchString(text) {
			result = append(result, pattern)
		}
	}
	return result
}

func presentPatterns(text string, patterns []string) []string {
	result := make([]string, 0)
	for _, pattern := range patterns {
		if strings.TrimSpace(pattern) == "" {
			continue
		}
		re, err := regexp.Compile(pattern)
		if err == nil && re.MatchString(text) {
			result = append(result, pattern)
		}
	}
	return result
}

func commandFormatMismatches(block pipelineCommandBlock, format pipelineCommandFormat) []string {
	details := make([]string, 0)
	actualLines := trimCommandBlockLines(block.Lines)
	expected := format.Lines
	for index, expectedLine := range expected {
		if index >= len(actualLines) {
			details = append(details, fmt.Sprintf("缺少第 %d 行：%s", index+1, expectedLine.Name))
			continue
		}
		re, err := regexp.Compile(expectedLine.Pattern)
		if err != nil {
			details = append(details, fmt.Sprintf("规则 %s 正则无效：%v", expectedLine.Name, err))
			continue
		}
		actual := actualLines[index].Text
		if !re.MatchString(actual) {
			details = append(details, fmt.Sprintf("第 %d 行期望 %s", index+1, expectedLine.Name))
		}
	}
	if !format.AllowExtraLines && len(actualLines) > len(expected) {
		details = append(details, fmt.Sprintf("存在 %d 行多余命令内容", len(actualLines)-len(expected)))
	}
	return details
}

func trimCommandBlockLines(lines []pipelineScriptLine) []pipelineScriptLine {
	result := make([]pipelineScriptLine, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line.Text)
		if trimmed == "" || trimmed == `"""` {
			continue
		}
		result = append(result, line)
	}
	return result
}

func pipelineBlockText(block pipelineCommandBlock) string {
	parts := make([]string, 0, len(block.Lines))
	for _, line := range block.Lines {
		parts = append(parts, line.Text)
	}
	return strings.Join(parts, "\n")
}

func newPipelineFinding(
	pipelineID string,
	rule scandomain.Rule,
	line pipelineScriptLine,
	detailsJSON string,
) scandomain.Finding {
	return scandomain.Finding{
		PipelineID:  pipelineID,
		RuleID:      rule.ID,
		RuleCode:    rule.RuleCode,
		RuleName:    rule.RuleName,
		Severity:    rule.Severity,
		LineNo:      line.No,
		MatchedText: strings.TrimSpace(line.Text),
		Message:     rule.Message,
		Suggestion:  rule.Suggestion,
		DetailsJSON: detailsJSON,
		Status:      scandomain.FindingStatusOpen,
	}
}

func firstPatternLine(script string, matcher pipelineRuleMatcher) pipelineScriptLine {
	pattern := matcher.Pattern
	if pattern == "" {
		pattern = matcher.WhenPattern
	}
	if pattern == "" {
		return pipelineScriptLine{No: 1}
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return pipelineScriptLine{No: 1}
	}
	for _, line := range splitPipelineScriptLines(script) {
		if re.MatchString(line.Text) {
			return line
		}
	}
	return pipelineScriptLine{No: 1}
}

func matcherDetails(matcher pipelineRuleMatcher) string {
	raw, _ := json.Marshal(map[string]string{"matcher_type": matcher.Type})
	return string(raw)
}

func normalizedMaxLines(value int, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}
