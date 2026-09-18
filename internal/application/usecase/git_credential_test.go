package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	domain "gos/internal/domain/gitcredential"
	"gos/internal/infrastructure/gitlab"
)

type gitCredentialRepositoryFake struct {
	items map[string]domain.Credential
}

func newGitCredentialRepositoryFake() *gitCredentialRepositoryFake {
	return &gitCredentialRepositoryFake{items: make(map[string]domain.Credential)}
}

func (r *gitCredentialRepositoryFake) InitSchema(context.Context) error { return nil }

func (r *gitCredentialRepositoryFake) Create(_ context.Context, item domain.Credential) error {
	for _, existing := range r.items {
		if existing.Name == item.Name {
			return domain.ErrNameDuplicated
		}
	}
	r.items[item.ID] = item
	return nil
}

func (r *gitCredentialRepositoryFake) GetByID(_ context.Context, id string) (domain.Credential, error) {
	item, ok := r.items[id]
	if !ok {
		return domain.Credential{}, domain.ErrNotFound
	}
	return item, nil
}

func (r *gitCredentialRepositoryFake) List(_ context.Context, filter domain.ListFilter) ([]domain.Credential, int64, error) {
	items := make([]domain.Credential, 0, len(r.items))
	for _, item := range r.items {
		if filter.Status != "" && item.Status != filter.Status {
			continue
		}
		items = append(items, item)
	}
	return items, int64(len(items)), nil
}

func (r *gitCredentialRepositoryFake) Update(
	_ context.Context,
	id string,
	input domain.UpdateInput,
	updatedAt time.Time,
) (domain.Credential, error) {
	item, ok := r.items[id]
	if !ok {
		return domain.Credential{}, domain.ErrNotFound
	}
	item.Name = input.Name
	item.Provider = input.Provider
	item.BaseURL = input.BaseURL
	item.Username = input.Username
	item.Secret = input.Secret
	item.AuthType = input.AuthType
	item.Status = input.Status
	item.Remark = input.Remark
	item.UpdatedAt = updatedAt
	r.items[id] = item
	return item, nil
}

func (r *gitCredentialRepositoryFake) Delete(_ context.Context, id string) error {
	if _, ok := r.items[id]; !ok {
		return domain.ErrNotFound
	}
	delete(r.items, id)
	return nil
}

type gitLabUserClientFake struct {
	user   gitlab.User
	err    error
	config gitlab.Config
	calls  int
}

func (f *gitLabUserClientFake) CurrentUser(context.Context) (gitlab.User, error) {
	f.calls++
	if f.err != nil {
		return gitlab.User{}, f.err
	}
	return f.user, nil
}

func TestGitCredentialManagerCreateNormalizesAndDefaults(t *testing.T) {
	repo := newGitCredentialRepositoryFake()
	manager := NewGitCredentialManager(repo)
	manager.now = func() time.Time { return time.Unix(100, 0).UTC() }

	item, err := manager.Create(context.Background(), GitCredentialInput{
		Name:     "  内网 GitLab  ",
		BaseURL:  "git.cloud.local:9080/",
		Username: " release-bot ",
		Secret:   " glpat-1 ",
		Remark:   " 备注 ",
	})
	if err != nil {
		t.Fatalf("Create err = %v", err)
	}
	if !strings.HasPrefix(item.ID, "gc-") {
		t.Fatalf("ID = %q, want a gc prefix", item.ID)
	}
	if item.Name != "内网 GitLab" {
		t.Fatalf("Name = %q", item.Name)
	}
	if item.BaseURL != "http://git.cloud.local:9080" {
		t.Fatalf("BaseURL = %q, want the normalized base url", item.BaseURL)
	}
	if item.Provider != domain.ProviderGitLab {
		t.Fatalf("Provider = %q, want gitlab", item.Provider)
	}
	if item.AuthType != domain.AuthTypeToken {
		t.Fatalf("AuthType = %q, want token", item.AuthType)
	}
	if item.Status != domain.StatusActive {
		t.Fatalf("Status = %q, want active", item.Status)
	}
	if item.Username != "release-bot" || item.Secret != "glpat-1" || item.Remark != "备注" {
		t.Fatalf("Create returned %+v", item)
	}
}

func TestGitCredentialManagerCreateValidatesInput(t *testing.T) {
	tests := []struct {
		name  string
		input GitCredentialInput
	}{
		{name: "name is required", input: GitCredentialInput{BaseURL: "http://git.cloud.local:9080", Secret: "s"}},
		{name: "base url is required", input: GitCredentialInput{Name: "n", Secret: "s"}},
		{name: "secret is required", input: GitCredentialInput{Name: "n", BaseURL: "http://git.cloud.local:9080"}},
		{name: "provider must be known", input: GitCredentialInput{Name: "n", Provider: "gitea", BaseURL: "http://git.cloud.local:9080", Secret: "s"}},
		{name: "auth type must be known", input: GitCredentialInput{Name: "n", BaseURL: "http://git.cloud.local:9080", Secret: "s", AuthType: "ssh"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manager := NewGitCredentialManager(newGitCredentialRepositoryFake())
			if _, err := manager.Create(context.Background(), test.input); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("Create err = %v, want ErrInvalidInput", err)
			}
		})
	}

	manager := NewGitCredentialManager(newGitCredentialRepositoryFake())
	_, err := manager.Create(context.Background(), GitCredentialInput{
		Name:    "n",
		BaseURL: "http://git.cloud.local:9080",
		Secret:  "s",
		Status:  "archived",
	})
	if !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("Create err = %v, want ErrInvalidStatus", err)
	}
}

func TestGitCredentialManagerUpdateKeepsStoredSecretWhenBlank(t *testing.T) {
	repo := newGitCredentialRepositoryFake()
	manager := NewGitCredentialManager(repo)
	manager.now = func() time.Time { return time.Unix(200, 0).UTC() }

	created, err := manager.Create(context.Background(), GitCredentialInput{
		Name:    "内网 GitLab",
		BaseURL: "http://git.cloud.local:9080",
		Secret:  "glpat-original",
	})
	if err != nil {
		t.Fatalf("Create err = %v", err)
	}

	updated, err := manager.Update(context.Background(), created.ID, GitCredentialInput{
		Name:    "内网 GitLab 2",
		BaseURL: "http://192.168.2.34:9080",
		Secret:  "   ",
	})
	if err != nil {
		t.Fatalf("Update err = %v", err)
	}
	if updated.Secret != "glpat-original" {
		t.Fatalf("Secret = %q, want the stored secret", updated.Secret)
	}
	if updated.Name != "内网 GitLab 2" || updated.BaseURL != "http://192.168.2.34:9080" {
		t.Fatalf("Update returned %+v", updated)
	}

	rotated, err := manager.Update(context.Background(), created.ID, GitCredentialInput{
		Name:    "内网 GitLab 2",
		BaseURL: "http://192.168.2.34:9080",
		Secret:  "glpat-rotated",
	})
	if err != nil {
		t.Fatalf("Update err = %v", err)
	}
	if rotated.Secret != "glpat-rotated" {
		t.Fatalf("Secret = %q, want the rotated secret", rotated.Secret)
	}

	if _, err := manager.Update(context.Background(), "gc-missing", GitCredentialInput{Name: "x", BaseURL: "http://git.cloud.local:9080"}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Update missing err = %v, want ErrNotFound", err)
	}
}

func TestGitCredentialManagerRejectsDuplicateName(t *testing.T) {
	repo := newGitCredentialRepositoryFake()
	manager := NewGitCredentialManager(repo)

	if _, err := manager.Create(context.Background(), GitCredentialInput{Name: "重复", BaseURL: "http://git.cloud.local:9080", Secret: "s"}); err != nil {
		t.Fatalf("Create err = %v", err)
	}
	if _, err := manager.Create(context.Background(), GitCredentialInput{Name: "重复", BaseURL: "http://192.168.2.34:9080", Secret: "s"}); !errors.Is(err, domain.ErrNameDuplicated) {
		t.Fatalf("duplicate Create err = %v, want ErrNameDuplicated", err)
	}
}

func TestGitCredentialManagerTestConnection(t *testing.T) {
	repo := newGitCredentialRepositoryFake()
	manager := NewGitCredentialManager(repo)
	created, err := manager.Create(context.Background(), GitCredentialInput{
		Name:     "内网 GitLab",
		BaseURL:  "http://git.cloud.local:9080",
		Username: "release-bot",
		Secret:   "glpat-1",
		AuthType: "token",
	})
	if err != nil {
		t.Fatalf("Create err = %v", err)
	}

	t.Run("success reports the gitlab user", func(t *testing.T) {
		client := &gitLabUserClientFake{user: gitlab.User{ID: 7, Username: "release-bot"}}
		manager.newClient = func(cfg gitlab.Config) GitLabUserClient {
			client.config = cfg
			return client
		}

		result, err := manager.TestConnection(context.Background(), created.ID)
		if err != nil {
			t.Fatalf("TestConnection err = %v", err)
		}
		if !result.OK || result.GitLabUsername != "release-bot" {
			t.Fatalf("result = %+v", result)
		}
		if !strings.Contains(result.Message, "release-bot") {
			t.Fatalf("Message = %q", result.Message)
		}
		if client.config.BaseURL != "http://git.cloud.local:9080" || client.config.Secret != "glpat-1" || client.config.AuthType != "token" {
			t.Fatalf("client config = %+v", client.config)
		}
	})

	t.Run("rejected credential is a result, not an error", func(t *testing.T) {
		manager.newClient = func(gitlab.Config) GitLabUserClient {
			return &gitLabUserClientFake{err: gitlab.ErrUnauthorized}
		}

		result, err := manager.TestConnection(context.Background(), created.ID)
		if err != nil {
			t.Fatalf("TestConnection err = %v, want a result instead", err)
		}
		if result.OK {
			t.Fatalf("result = %+v, want ok=false", result)
		}
		if !strings.Contains(result.Message, "访问令牌") {
			t.Fatalf("Message = %q, want the access token hint", result.Message)
		}
	})

	t.Run("password credential skips the api probe", func(t *testing.T) {
		passwordCredential, err := manager.Create(context.Background(), GitCredentialInput{
			Name:     "内网 GitLab 账号密码",
			BaseURL:  "http://git.cloud.local:9080",
			Username: "client011",
			Secret:   "secret",
			AuthType: "password",
		})
		if err != nil {
			t.Fatalf("Create err = %v", err)
		}
		called := false
		manager.newClient = func(gitlab.Config) GitLabUserClient {
			called = true
			return &gitLabUserClientFake{err: gitlab.ErrUnauthorized}
		}

		result, err := manager.TestConnection(context.Background(), passwordCredential.ID)
		if err != nil {
			t.Fatalf("TestConnection err = %v", err)
		}
		if !result.OK {
			t.Fatalf("result = %+v, want ok=true for the git transport mode", result)
		}
		if called {
			t.Fatal("password credential must not be probed through the GitLab API")
		}
		if !strings.Contains(result.Message, "git 协议") || !strings.Contains(result.Message, "最近提交") {
			t.Fatalf("Message = %q, want the git transport explanation", result.Message)
		}
	})

	t.Run("missing credential is an error", func(t *testing.T) {
		if _, err := manager.TestConnection(context.Background(), "gc-missing"); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("TestConnection err = %v, want ErrNotFound", err)
		}
	})
}

func TestGitCredentialManagerListFiltersAndPages(t *testing.T) {
	repo := newGitCredentialRepositoryFake()
	manager := NewGitCredentialManager(repo)
	if _, err := manager.Create(context.Background(), GitCredentialInput{Name: "a", BaseURL: "http://git.cloud.local:9080", Secret: "s"}); err != nil {
		t.Fatalf("Create err = %v", err)
	}
	if _, err := manager.Create(context.Background(), GitCredentialInput{Name: "b", BaseURL: "http://192.168.2.34:9080", Secret: "s", Status: "disabled"}); err != nil {
		t.Fatalf("Create err = %v", err)
	}

	items, total, err := manager.List(context.Background(), domain.ListFilter{Status: domain.StatusActive})
	if err != nil {
		t.Fatalf("List err = %v", err)
	}
	if total != 1 || len(items) != 1 || items[0].Name != "a" {
		t.Fatalf("List total=%d items=%+v", total, items)
	}
	if _, _, err := manager.List(context.Background(), domain.ListFilter{Status: "archived"}); !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("List err = %v, want ErrInvalidStatus", err)
	}
}
