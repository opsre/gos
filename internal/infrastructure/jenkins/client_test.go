package jenkins

import "testing"

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
