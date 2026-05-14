package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	domain "gos/internal/domain/artifactrepo"
)

func TestArtifactRepositoryManagerCreateValidatesAndDefaultsOSSConfig(t *testing.T) {
	repo := newArtifactRepositoryFake()
	manager := NewArtifactRepositoryManager(repo)
	manager.now = func() time.Time { return time.Unix(100, 0).UTC() }

	item, err := manager.Create(context.Background(), CreateArtifactRepositoryInput{
		Name:            "  oa 制品库  ",
		RepositoryType:  domain.RepositoryTypeOSS,
		Endpoint:        "  https://oss.example.com  ",
		Bucket:          "  oa  ",
		Directory:       " /release/jar/ ",
		AccessKeyID:     " ak ",
		AccessKeySecret: " secret ",
		ACL:             domain.ACLPublicRead,
	})
	if err != nil {
		t.Fatalf("Create err = %v", err)
	}

	if item.Name != "oa 制品库" {
		t.Fatalf("Name = %q", item.Name)
	}
	if item.RepositoryType != domain.RepositoryTypeOSS {
		t.Fatalf("RepositoryType = %q", item.RepositoryType)
	}
	if item.Directory != "release/jar" {
		t.Fatalf("Directory = %q", item.Directory)
	}
	if item.Status != domain.StatusEnabled {
		t.Fatalf("Status = %q", item.Status)
	}
	if item.ACL != domain.ACLPublicRead {
		t.Fatalf("ACL = %q", item.ACL)
	}
}

func TestArtifactRepositoryManagerRejectsInvalidACL(t *testing.T) {
	manager := NewArtifactRepositoryManager(newArtifactRepositoryFake())

	_, err := manager.Create(context.Background(), CreateArtifactRepositoryInput{
		Name:            "oa",
		RepositoryType:  domain.RepositoryTypeOSS,
		Endpoint:        "https://oss.example.com",
		Bucket:          "oa",
		AccessKeyID:     "ak",
		AccessKeySecret: "secret",
		ACL:             domain.ACL("public-write"),
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Create err = %v, want ErrInvalidInput", err)
	}
}

func TestArtifactRepositoryManagerUpdateKeepsExistingSecretWhenBlank(t *testing.T) {
	repo := newArtifactRepositoryFake()
	manager := NewArtifactRepositoryManager(repo)
	manager.now = func() time.Time { return time.Unix(200, 0).UTC() }

	created, err := manager.Create(context.Background(), CreateArtifactRepositoryInput{
		Name:            "oa",
		RepositoryType:  domain.RepositoryTypeOSS,
		Endpoint:        "https://oss.example.com",
		Bucket:          "oa",
		AccessKeyID:     "ak",
		AccessKeySecret: "secret-1",
		ACL:             domain.ACLPrivate,
	})
	if err != nil {
		t.Fatalf("Create err = %v", err)
	}

	updated, err := manager.Update(context.Background(), created.ID, domain.UpdateInput{
		Name:            "oa-prod",
		RepositoryType:  domain.RepositoryTypeOSS,
		Endpoint:        "https://oss-prod.example.com",
		Bucket:          "oa-prod",
		Directory:       "",
		AccessKeyID:     "ak-prod",
		AccessKeySecret: "",
		ACL:             domain.ACLPublicRead,
		Status:          domain.StatusDisabled,
	})
	if err != nil {
		t.Fatalf("Update err = %v", err)
	}
	if updated.AccessKeySecret != "secret-1" {
		t.Fatalf("AccessKeySecret = %q, want original secret", updated.AccessKeySecret)
	}
	if updated.Status != domain.StatusDisabled {
		t.Fatalf("Status = %q", updated.Status)
	}
}

func TestArtifactRepositoryManagerTestConnectionNormalizesInputAndCallsTester(t *testing.T) {
	manager := NewArtifactRepositoryManager(newArtifactRepositoryFake())
	tester := &artifactRepositoryConnectionTesterFake{}
	manager.connectionTester = tester

	result, err := manager.TestConnection(context.Background(), CreateArtifactRepositoryInput{
		RepositoryType:  domain.RepositoryTypeOSS,
		Endpoint:        "  https://oss.example.com/  ",
		Bucket:          "  oa  ",
		Directory:       " /release/jar/ ",
		AccessKeyID:     " ak ",
		AccessKeySecret: " secret ",
	})
	if err != nil {
		t.Fatalf("TestConnection err = %v", err)
	}
	if !result.Success {
		t.Fatalf("Success = false, want true")
	}
	if tester.input.Endpoint != "https://oss.example.com/" {
		t.Fatalf("Endpoint = %q", tester.input.Endpoint)
	}
	if tester.input.Bucket != "oa" {
		t.Fatalf("Bucket = %q", tester.input.Bucket)
	}
	if tester.input.Directory != "release/jar" {
		t.Fatalf("Directory = %q", tester.input.Directory)
	}
	if tester.input.AccessKeyID != "ak" {
		t.Fatalf("AccessKeyID = %q", tester.input.AccessKeyID)
	}
	if tester.input.AccessKeySecret != "secret" {
		t.Fatalf("AccessKeySecret = %q", tester.input.AccessKeySecret)
	}
}

func TestArtifactRepositoryManagerTestConnectionRequiresSecret(t *testing.T) {
	manager := NewArtifactRepositoryManager(newArtifactRepositoryFake())

	_, err := manager.TestConnection(context.Background(), CreateArtifactRepositoryInput{
		RepositoryType: domain.RepositoryTypeOSS,
		Endpoint:       "https://oss.example.com",
		Bucket:         "oa",
		AccessKeyID:    "ak",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("TestConnection err = %v, want ErrInvalidInput", err)
	}
}

type artifactRepositoryFake struct {
	items map[string]domain.ArtifactRepository
}

func newArtifactRepositoryFake() *artifactRepositoryFake {
	return &artifactRepositoryFake{items: map[string]domain.ArtifactRepository{}}
}

func (r *artifactRepositoryFake) InitSchema(context.Context) error { return nil }

func (r *artifactRepositoryFake) Create(_ context.Context, item domain.ArtifactRepository) error {
	r.items[item.ID] = item
	return nil
}

func (r *artifactRepositoryFake) GetByID(_ context.Context, id string) (domain.ArtifactRepository, error) {
	item, ok := r.items[id]
	if !ok {
		return domain.ArtifactRepository{}, domain.ErrNotFound
	}
	return item, nil
}

func (r *artifactRepositoryFake) List(_ context.Context, filter domain.ListFilter) ([]domain.ArtifactRepository, int64, error) {
	items := make([]domain.ArtifactRepository, 0, len(r.items))
	for _, item := range r.items {
		items = append(items, item)
	}
	return items, int64(len(items)), nil
}

func (r *artifactRepositoryFake) Update(_ context.Context, id string, input domain.UpdateInput, updatedAt time.Time) (domain.ArtifactRepository, error) {
	item, ok := r.items[id]
	if !ok {
		return domain.ArtifactRepository{}, domain.ErrNotFound
	}
	item.Name = input.Name
	item.RepositoryType = input.RepositoryType
	item.Endpoint = input.Endpoint
	item.Bucket = input.Bucket
	item.Directory = input.Directory
	item.AccessKeyID = input.AccessKeyID
	item.AccessKeySecret = input.AccessKeySecret
	item.ACL = input.ACL
	item.Status = input.Status
	item.UpdatedAt = updatedAt
	r.items[id] = item
	return item, nil
}

func (r *artifactRepositoryFake) Delete(_ context.Context, id string) error {
	if _, ok := r.items[id]; !ok {
		return domain.ErrNotFound
	}
	delete(r.items, id)
	return nil
}

type artifactRepositoryConnectionTesterFake struct {
	input domain.UpdateInput
}

func (t *artifactRepositoryConnectionTesterFake) TestConnection(_ context.Context, input domain.UpdateInput) (ArtifactRepositoryConnectionTestResult, error) {
	t.input = input
	return ArtifactRepositoryConnectionTestResult{Success: true, Message: "连接成功"}, nil
}
