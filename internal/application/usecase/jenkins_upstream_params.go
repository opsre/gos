package usecase

import (
	"net/url"
	"strings"

	pipelinedomain "gos/internal/domain/pipeline"
	domain "gos/internal/domain/release"
)

const (
	standardParamCIJob   = "ci_job"
	standardParamCIBuild = "ci_build"
)

// defaultJenkinsExecutorParamKey maps the CD contract parameters to platform-owned
// runtime fields. Other Jenkins parameters remain explicitly mapped by users.
func defaultJenkinsExecutorParamKey(executorParamName string) string {
	switch strings.ToUpper(strings.TrimSpace(executorParamName)) {
	case "CI_JOB":
		return standardParamCIJob
	case "CI_BUILD":
		return standardParamCIBuild
	default:
		return ""
	}
}

func isCIJenkinsRuntimeParamKey(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case standardParamCIJob, standardParamCIBuild:
		return true
	default:
		return false
	}
}

func jenkinsUpstreamTemplateParamKey(item domain.ReleaseTemplateParam) string {
	for _, candidate := range []string{item.SourceParamKey, item.ParamKey} {
		switch strings.ToLower(strings.TrimSpace(candidate)) {
		case standardParamCIJob:
			return standardParamCIJob
		case standardParamCIBuild:
			return standardParamCIBuild
		}
	}
	return defaultJenkinsExecutorParamKey(item.ExecutorParamName)
}

func jenkinsUpstreamExecutorParamName(key string) string {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case standardParamCIJob:
		return "CI_JOB"
	case standardParamCIBuild:
		return "CI_BUILD"
	default:
		return ""
	}
}

// resolveCIJenkinsRuntimeValue resolves the upstream CI identity after Jenkins has
// assigned a concrete build URL. CI_JOB is the full Jenkins job path, which is the
// value expected by Copy Artifact's projectName field.
func resolveCIJenkinsRuntimeValue(executions []domain.ReleaseOrderExecution, key string) string {
	key = strings.ToLower(strings.TrimSpace(key))
	if key != standardParamCIJob && key != standardParamCIBuild {
		return ""
	}
	for _, item := range executions {
		if item.PipelineScope != domain.PipelineScopeCI ||
			!strings.EqualFold(strings.TrimSpace(item.Provider), string(pipelinedomain.ProviderJenkins)) {
			continue
		}
		switch key {
		case standardParamCIJob:
			return parseJenkinsJobFullName(item.BuildURL)
		case standardParamCIBuild:
			return parseJenkinsBuildNumber(item.BuildURL)
		}
	}
	return ""
}

// parseJenkinsJobFullName converts Jenkins build URLs such as
// /job/team/job/service/job/main/42/ to team/service/main.
func parseJenkinsJobFullName(buildURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(buildURL))
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.Trim(parsed.EscapedPath(), "/"), "/")
	jobParts := make([]string, 0)
	for index := 0; index+1 < len(parts); index++ {
		if parts[index] != "job" {
			continue
		}
		value, err := url.PathUnescape(parts[index+1])
		if err != nil || strings.TrimSpace(value) == "" {
			return ""
		}
		jobParts = append(jobParts, strings.TrimSpace(value))
		index++
	}
	return strings.Join(jobParts, "/")
}
