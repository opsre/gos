package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	appdomain "gos/internal/domain/application"
	projectdomain "gos/internal/domain/project"
)

func TestCreateApplicationAllowsOptionalArtifactBinding(t *testing.T) {
	repo := newApplicationUsecaseRepoFake()
	creator := NewCreateApplication(repo, newApplicationProjectRepoFake())

	app, err := creator.Execute(context.Background(), CreateInput{
		Name:         "  支付中心  ",
		Key:          " pay-center ",
		ProjectID:    " project-1 ",
		OwnerUserID:  " user-1 ",
		Owner:        "  赵昊宇  ",
		Status:       appdomain.StatusActive,
		ArtifactType: " jar ",
		Language:     " java ",
	})
	if err != nil {
		t.Fatalf("Execute err = %v", err)
	}
	if app.ArtifactRepositoryID != "" {
		t.Fatalf("ArtifactRepositoryID = %q, want empty", app.ArtifactRepositoryID)
	}
	if app.ArtifactDirectory != "" {
		t.Fatalf("ArtifactDirectory = %q, want empty", app.ArtifactDirectory)
	}
}

func TestCreateApplicationNormalizesArtifactBinding(t *testing.T) {
	repo := newApplicationUsecaseRepoFake()
	creator := NewCreateApplication(repo, newApplicationProjectRepoFake())

	app, err := creator.Execute(context.Background(), CreateInput{
		Name:                 "支付中心",
		Key:                  "pay-center",
		ProjectID:            "project-1",
		OwnerUserID:          "user-1",
		Status:               appdomain.StatusActive,
		ArtifactType:         "jar",
		Language:             "java",
		ArtifactRepositoryID: " repo-oss ",
		ArtifactDirectory:    " /release/pay-center/ ",
	})
	if err != nil {
		t.Fatalf("Execute err = %v", err)
	}
	if app.ArtifactRepositoryID != "repo-oss" {
		t.Fatalf("ArtifactRepositoryID = %q", app.ArtifactRepositoryID)
	}
	if app.ArtifactDirectory != "release/pay-center" {
		t.Fatalf("ArtifactDirectory = %q", app.ArtifactDirectory)
	}
}

func TestUpdateApplicationRejectsPartialArtifactBinding(t *testing.T) {
	updater := NewUpdateApplication(newApplicationUsecaseRepoFake(), newApplicationProjectRepoFake())

	_, err := updater.Execute(context.Background(), "app-1", appdomain.UpdateInput{
		Name:              "支付中心",
		Key:               "pay-center",
		ProjectID:         "project-1",
		OwnerUserID:       "user-1",
		Status:            appdomain.StatusActive,
		ArtifactType:      "jar",
		Language:          "java",
		ArtifactDirectory: "release/pay-center",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Execute err = %v, want ErrInvalidInput", err)
	}
}

type applicationUsecaseRepoFake struct {
	items map[string]appdomain.Application
}

func newApplicationUsecaseRepoFake() *applicationUsecaseRepoFake {
	return &applicationUsecaseRepoFake{items: map[string]appdomain.Application{
		"app-1": {
			ID:           "app-1",
			Name:         "支付中心",
			Key:          "pay-center",
			ProjectID:    "project-1",
			OwnerUserID:  "user-1",
			Status:       appdomain.StatusActive,
			ArtifactType: "jar",
			CreatedAt:    time.Unix(100, 0).UTC(),
			UpdatedAt:    time.Unix(100, 0).UTC(),
		},
	}}
}

func (r *applicationUsecaseRepoFake) InitSchema(context.Context) error { return nil }

func (r *applicationUsecaseRepoFake) Create(_ context.Context, app appdomain.Application) error {
	r.items[app.ID] = app
	return nil
}

func (r *applicationUsecaseRepoFake) GetByID(_ context.Context, id string) (appdomain.Application, error) {
	item, ok := r.items[id]
	if !ok {
		return appdomain.Application{}, appdomain.ErrNotFound
	}
	return item, nil
}

func (r *applicationUsecaseRepoFake) List(_ context.Context, _ appdomain.ListFilter) ([]appdomain.Application, int64, error) {
	items := make([]appdomain.Application, 0, len(r.items))
	for _, item := range r.items {
		items = append(items, item)
	}
	return items, int64(len(items)), nil
}

func (r *applicationUsecaseRepoFake) Update(_ context.Context, id string, input appdomain.UpdateInput, updatedAt time.Time) (appdomain.Application, error) {
	item, ok := r.items[id]
	if !ok {
		return appdomain.Application{}, appdomain.ErrNotFound
	}
	item.Name = input.Name
	item.Key = input.Key
	item.ProjectID = input.ProjectID
	item.RepoURL = input.RepoURL
	item.Description = input.Description
	item.OwnerUserID = input.OwnerUserID
	item.Owner = input.Owner
	item.Status = input.Status
	item.ArtifactType = input.ArtifactType
	item.SetLanguage(input.Language)
	item.ArtifactRepositoryID = input.ArtifactRepositoryID
	item.ArtifactDirectory = input.ArtifactDirectory
	item.GitOpsBranchMappings = input.GitOpsBranchMappings
	item.ReleaseBranches = input.ReleaseBranches
	item.UpdatedAt = updatedAt
	r.items[id] = item
	return item, nil
}

func (r *applicationUsecaseRepoFake) Delete(_ context.Context, id string) error {
	delete(r.items, id)
	return nil
}

type applicationProjectRepoFake struct{}

func newApplicationProjectRepoFake() applicationProjectRepoFake {
	return applicationProjectRepoFake{}
}

func (applicationProjectRepoFake) InitSchema(context.Context) error { return nil }
func (applicationProjectRepoFake) Create(context.Context, projectdomain.Project) error {
	return nil
}
func (applicationProjectRepoFake) GetByID(_ context.Context, id string) (projectdomain.Project, error) {
	return projectdomain.Project{ID: "project-1", Name: "默认项目", Key: "default"}, nil
}
func (applicationProjectRepoFake) List(context.Context, projectdomain.ListFilter) ([]projectdomain.Project, int64, error) {
	return nil, 0, nil
}
func (applicationProjectRepoFake) Update(context.Context, string, projectdomain.UpdateInput, time.Time) (projectdomain.Project, error) {
	return projectdomain.Project{}, nil
}
func (applicationProjectRepoFake) Delete(context.Context, string) error { return nil }
