package jenkins

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestParsePipelineScriptParamSnapshotsBasic 解析输入内容并返回结构化结果。
func TestParsePipelineScriptParamSnapshotsBasic(t *testing.T) {
	script := `
pipeline {
    agent any
    parameters {
        choice(
            description: '发布生产或者测试?',
            name: 'DEPLOY_TO',
            choices: ['dev', 'prod']
        )
        choice(
            description: '选择deploy部署分支最新代码 或者 选择rollback回退历史版本?',
            name: 'deploy_env',
            choices: ['deploy', 'rollback']
        )
        gitParameter (branch:'', branchFilter: 'origin/(release_.*)', defaultValue: 'release_2.0', description: '选择将要构建的分支', name: 'BRANCH', type: 'PT_BRANCH')
        text(name: 'gitchange',
         defaultValue: '请输入SHA值',
         description: '请输入SHA值回退历史版本')
    }
}
`

	items := parsePipelineScriptParamSnapshots(script)
	if len(items) != 4 {
		t.Fatalf("expected 4 params, got %d", len(items))
	}

	if items[0].Name != "DEPLOY_TO" || items[0].ParamType != "choice" || items[0].SingleSelect != true {
		t.Fatalf("unexpected first param: %+v", items[0])
	}
	if items[2].Name != "BRANCH" || items[2].ParamType != "choice" || items[2].SingleSelect != true {
		t.Fatalf("unexpected git param: %+v", items[2])
	}
	if items[3].Name != "gitchange" || items[3].ParamType != "string" {
		t.Fatalf("unexpected text param: %+v", items[3])
	}
}

// TestParsePipelineScriptParamSnapshotsExtendedChoice 解析输入内容并返回结构化结果。
func TestParsePipelineScriptParamSnapshotsExtendedChoice(t *testing.T) {
	script := `
pipeline {
    agent any
    parameters {
        choice(description: '发布环境', name: 'DEPLOY_TO', choices: ['dev', 'prod'])
        extendedChoice description: '请选择需要构建的微服务', multiSelectDelimiter: ',', name: 'project_name', type: 'PT_CHECKBOX', value: 'gateway,auth,robot'
        text(name: 'gitchange', defaultValue: '', description: '回滚SHA')
    }
}
`

	items := parsePipelineScriptParamSnapshots(script)
	if len(items) != 3 {
		t.Fatalf("expected 3 params, got %d", len(items))
	}

	if items[1].Name != "project_name" {
		t.Fatalf("unexpected extendedChoice name: %+v", items[1])
	}
	if items[1].ParamType != "choice" {
		t.Fatalf("unexpected extendedChoice type: %+v", items[1])
	}
	if items[1].SingleSelect {
		t.Fatalf("extendedChoice checkbox should be multi-select: %+v", items[1])
	}
}

func TestGetBuildStageLogWithNameFallsBackToConsoleTextWhenStageDescribeReturnsHTML(t *testing.T) {
	t.Parallel()

	var describeHit bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/job/demo/1/execution/node/7/wfapi/describe":
			describeHit = true
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte("<html><body><h2>Jenkins</h2><p>Please log in</p></body></html>"))
		case "/job/demo/1/logText/progressiveText":
			if r.URL.Query().Get("start") != "0" {
				t.Fatalf("unexpected progressiveText start = %q", r.URL.Query().Get("start"))
			}
			w.Header().Set("X-Text-Size", "320")
			w.Header().Set("X-More-Data", "false")
			_, _ = w.Write([]byte(strings.Join([]string{
				"[Pipeline] stage",
				"[Pipeline] { (Upload OSS)",
				"+ ossutil cp artifact.tar oss://release-bucket/",
				"ERROR: AccessDenied: access denied by bucket policy",
				"[Pipeline] }",
				"[Pipeline] // stage",
				"[Pipeline] stage",
				"[Pipeline] { (Deploy)",
				"deploying service",
				"[Pipeline] }",
				"[Pipeline] // stage",
			}, "\n")))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, TimeoutSec: 2})
	log, err := client.GetBuildStageLogWithName(context.Background(), server.URL+"/job/demo/1/", "7", "Upload OSS")
	if err != nil {
		t.Fatalf("GetBuildStageLogWithName failed: %v", err)
	}
	if !describeHit {
		t.Fatalf("expected stage describe endpoint to be called")
	}
	if log.StageName != "Upload OSS" {
		t.Fatalf("StageName = %q, want Upload OSS", log.StageName)
	}
	if !strings.Contains(log.Content, "AccessDenied") {
		t.Fatalf("fallback log content = %q, want stage error", log.Content)
	}
	if strings.Contains(log.Content, "Deploy") {
		t.Fatalf("fallback log content should be scoped to Upload OSS stage, got %q", log.Content)
	}
}

func TestListJobsReportsNonJSONResponseInsteadOfRawJSONParseError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/json" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<html><body><p>Please log in</p></body></html>"))
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, TimeoutSec: 2})
	_, err := client.ListJobs(context.Background())
	if err == nil {
		t.Fatalf("ListJobs error = nil, want non JSON response error")
	}
	if strings.Contains(err.Error(), "invalid character") {
		t.Fatalf("ListJobs error = %q, should not expose raw JSON parse error", err.Error())
	}
	if !strings.Contains(err.Error(), "Jenkins 返回了非 JSON 响应") {
		t.Fatalf("ListJobs error = %q, want non JSON response message", err.Error())
	}
}

func TestNormalizeJenkinsLogContentCleansGitLabSignInHTML(t *testing.T) {
	t.Parallel()

	raw := `[Pipeline] sh
+ curl https://gitlab.example/releases
<!DOCTYPE html>
<html>
<head>
  <title>Sign in · GitLab</title>
  <style>@keyframes blinking-dot{0%{opacity:1}}body.ui-indigo{background:#292961}</style>
</head>
<body>
  <h1>GitLab</h1>
  <form>Username Password Sign in</form>
</body>
</html>
ERROR: script returned exit code 1`

	got := normalizeJenkinsLogContent(raw)
	if strings.Contains(got, "@keyframes") || strings.Contains(got, "body.ui-indigo") {
		t.Fatalf("normalized log should not contain GitLab CSS, got %q", got)
	}
	if strings.Contains(got, "GitLab 返回登录页") {
		t.Fatalf("normalized log should preserve original log text, got %q", got)
	}
	for _, want := range []string{
		"[Pipeline] sh",
		"curl https://gitlab.example/releases",
		"Sign in · GitLab",
		"Username Password Sign in",
		"ERROR: script returned exit code 1",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("normalized log = %q, want to contain %q", got, want)
		}
	}
}

func TestGetBuildConsoleTextCleansHTMLFromProgressiveLog(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/job/demo/1/logText/progressiveText" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("X-Text-Size", "2048")
		w.Header().Set("X-More-Data", "false")
		_, _ = w.Write([]byte(`[Pipeline] sh
+ curl http://gitlab.example/project
<!DOCTYPE html>
<html>
<head>
  <title>Sign in · GitLab</title>
  <style>@keyframes blinking-dot{0%{opacity:1}}body.ui-indigo{background:#292961}</style>
  <script>window.gon={};</script>
</head>
<body>
  <form>Username Password Sign in</form>
</body>
</html>
ERROR: script returned exit code 1`))
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, TimeoutSec: 2})
	content, nextStart, moreData, err := client.GetBuildConsoleText(context.Background(), server.URL+"/job/demo/1/", 0)
	if err != nil {
		t.Fatalf("GetBuildConsoleText failed: %v", err)
	}
	if nextStart != 2048 {
		t.Fatalf("nextStart = %d, want 2048", nextStart)
	}
	if moreData {
		t.Fatalf("moreData = true, want false")
	}
	if strings.Contains(content, "@keyframes") || strings.Contains(content, "window.gon") || strings.Contains(content, "<html") {
		t.Fatalf("content should not contain raw GitLab HTML noise, got %q", content)
	}
	for _, want := range []string{
		"[Pipeline] sh",
		"curl http://gitlab.example/project",
		"Sign in · GitLab",
		"Username Password Sign in",
		"ERROR: script returned exit code 1",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("content = %q, want to contain %q", content, want)
		}
	}
}
