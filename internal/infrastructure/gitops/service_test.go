package gitops

import "testing"

// TestBuildCommitMessageUsesConfiguredTemplate 组装业务执行所需的输入数据。
func TestBuildCommitMessageUsesConfiguredTemplate(t *testing.T) {
	service := NewService(Config{
		Enabled:               true,
		LocalRoot:             "/tmp/gitops",
		CommitMessageTemplate: "release: {env} -> {image_version}",
	})

	got := service.BuildCommitMessage(
		map[string]string{
			"order_no":      "RO-20260318-001",
			"app_name":      "南通后端",
			"app_key":       "java_nantong",
			"env":           "dev",
			"image_version": "20260318.1",
			"source_path":   "apps/java_nantong/overlays/dev",
		},
	)

	want := "release: dev -> 20260318.1"
	if got != want {
		t.Fatalf("unexpected commit message: got %q want %q", got, want)
	}
}

// TestBuildCommitMessageFallsBackToDefaultTemplate 组装业务执行所需的输入数据。
func TestBuildCommitMessageFallsBackToDefaultTemplate(t *testing.T) {
	service := NewService(Config{
		Enabled:   true,
		LocalRoot: "/tmp/gitops",
	})

	got := service.BuildCommitMessage(
		map[string]string{
			"order_no":      "RO-20260318-002",
			"app_name":      "南通后端",
			"app_key":       "java_nantong",
			"project_name":  "gateway",
			"env":           "dev",
			"branch":        "master",
			"image_version": "20260318.2",
		},
	)

	want := "chore(release): java_nantong/gateway/dev -> 20260318.2 (master)"
	if got != want {
		t.Fatalf("unexpected default commit message: got %q want %q", got, want)
	}
}

// TestBuildCommitMessageSupportsDynamicPlatformKeys 组装业务执行所需的输入数据。
func TestBuildCommitMessageSupportsDynamicPlatformKeys(t *testing.T) {
	service := NewService(Config{
		Enabled:               true,
		LocalRoot:             "/tmp/gitops",
		CommitMessageTemplate: "release: {env} / {image_version} / {project_name}",
	})

	got := service.BuildCommitMessage(map[string]string{
		"env":           "test",
		"image_version": "20260318.3",
		"project_name":  "gateway",
	})

	want := "release: test / 20260318.3 / gateway"
	if got != want {
		t.Fatalf("unexpected dynamic commit message: got %q want %q", got, want)
	}
}

// TestNormalizeHoistedHelmValuesFilePathTemplate 标准化输入值，保证后续逻辑使用统一格式。
func TestNormalizeHoistedHelmValuesFilePathTemplate(t *testing.T) {
	got := normalizeHoistedHelmValuesFilePathTemplate("apps/java-nantong-test/helm/platform.values-{env}.yaml")
	want := "apps/helm/platform.values-{env}.yaml"
	if got != want {
		t.Fatalf("unexpected hoisted helm values path: got %q want %q", got, want)
	}
}

// TestNormalizeHoistedHelmValuesFilePathTemplateKeepsSharedPath 标准化输入值，保证后续逻辑使用统一格式。
func TestNormalizeHoistedHelmValuesFilePathTemplateKeepsSharedPath(t *testing.T) {
	got := normalizeHoistedHelmValuesFilePathTemplate("apps/helm/platform.values-{env}.yaml")
	want := "apps/helm/platform.values-{env}.yaml"
	if got != want {
		t.Fatalf("unexpected shared helm values path: got %q want %q", got, want)
	}
}
