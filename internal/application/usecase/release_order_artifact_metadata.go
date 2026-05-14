package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	domain "gos/internal/domain/release"
)

type RecordReleaseOrderArtifactMetadataInput struct {
	ExecutionID      string
	PipelineScope    string
	ArtifactName     string
	ArtifactType     string
	ArtifactVersion  string
	ArtifactURL      string
	RepositoryID     string
	RepositoryName   string
	Bucket           string
	ObjectKey        string
	Checksum         string
	ChecksumType     string
	SizeBytes        int64
	BuildNumber      string
	AdditionalFields map[string]any
}

type ReleaseOrderArtifactMetadataOutput struct {
	ID               string
	ReleaseOrderID   string
	ExecutionID      string
	PipelineScope    string
	ArtifactName     string
	ArtifactType     string
	ArtifactVersion  string
	ArtifactURL      string
	RepositoryID     string
	RepositoryName   string
	Bucket           string
	ObjectKey        string
	Checksum         string
	ChecksumType     string
	SizeBytes        int64
	BuildNumber      string
	AdditionalFields map[string]any
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type ListReleaseOrderArtifactMetadataInput struct {
	ProjectID                   string
	ApplicationID               string
	ApplicationIDs              []string
	VisibleApplicationEnvScopes []domain.ApplicationEnvScope
	VisibleToUserID             string
	ReleaseOrderID              string
	Keyword                     string
	ArtifactName                string
	ArtifactType                string
	PipelineScope               string
	RepositoryID                string
	CreatedAtFrom               *time.Time
	CreatedAtTo                 *time.Time
	Page                        int
	PageSize                    int
}

type ReleaseOrderArtifactMetadataSummaryOutput struct {
	ReleaseOrderArtifactMetadataOutput
	ReleaseOrderNo     string
	ReleaseName        string
	ReleaseDisplayName string
	ApplicationID      string
	ApplicationName    string
	ProjectID          string
	ProjectName        string
	ProjectKey         string
	EnvCode            string
	OrderStatus        string
}

func (uc *ReleaseOrderManager) RecordArtifactMetadata(
	ctx context.Context,
	releaseOrderID string,
	input RecordReleaseOrderArtifactMetadataInput,
) (ReleaseOrderArtifactMetadataOutput, error) {
	if uc == nil || uc.repo == nil {
		return ReleaseOrderArtifactMetadataOutput{}, fmt.Errorf("%w: release order manager is not configured", ErrInvalidInput)
	}

	orderID := strings.TrimSpace(releaseOrderID)
	if orderID == "" {
		return ReleaseOrderArtifactMetadataOutput{}, ErrInvalidID
	}
	if _, err := uc.repo.GetByID(ctx, orderID); err != nil {
		return ReleaseOrderArtifactMetadataOutput{}, err
	}

	artifactURL := strings.TrimSpace(input.ArtifactURL)
	if artifactURL == "" {
		return ReleaseOrderArtifactMetadataOutput{}, fmt.Errorf("%w: artifact_url is required", ErrInvalidInput)
	}
	if input.SizeBytes < 0 {
		return ReleaseOrderArtifactMetadataOutput{}, fmt.Errorf("%w: size_bytes must be greater than or equal to 0", ErrInvalidInput)
	}

	scope := domain.PipelineScope(strings.ToLower(strings.TrimSpace(input.PipelineScope)))
	if scope != "" && !scope.Valid() {
		return ReleaseOrderArtifactMetadataOutput{}, fmt.Errorf("%w: invalid pipeline_scope", ErrInvalidInput)
	}

	metadataJSON, err := marshalReleaseOrderArtifactMetadata(input.AdditionalFields)
	if err != nil {
		return ReleaseOrderArtifactMetadataOutput{}, err
	}

	now := uc.now()
	item := domain.ReleaseOrderArtifactMetadata{
		ID:              generateID("roart"),
		ReleaseOrderID:  orderID,
		ExecutionID:     strings.TrimSpace(input.ExecutionID),
		PipelineScope:   scope,
		ArtifactName:    strings.TrimSpace(input.ArtifactName),
		ArtifactType:    strings.TrimSpace(input.ArtifactType),
		ArtifactVersion: strings.TrimSpace(input.ArtifactVersion),
		ArtifactURL:     artifactURL,
		RepositoryID:    strings.TrimSpace(input.RepositoryID),
		RepositoryName:  strings.TrimSpace(input.RepositoryName),
		Bucket:          strings.TrimSpace(input.Bucket),
		ObjectKey:       strings.TrimSpace(input.ObjectKey),
		Checksum:        strings.TrimSpace(input.Checksum),
		ChecksumType:    strings.TrimSpace(input.ChecksumType),
		SizeBytes:       input.SizeBytes,
		BuildNumber:     strings.TrimSpace(input.BuildNumber),
		MetadataJSON:    metadataJSON,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	saved, err := uc.repo.UpsertArtifactMetadata(ctx, item)
	if err != nil {
		return ReleaseOrderArtifactMetadataOutput{}, err
	}
	return toReleaseOrderArtifactMetadataOutput(saved), nil
}

func (uc *ReleaseOrderManager) ListArtifactMetadata(
	ctx context.Context,
	releaseOrderID string,
) ([]ReleaseOrderArtifactMetadataOutput, error) {
	if uc == nil || uc.repo == nil {
		return nil, fmt.Errorf("%w: release order manager is not configured", ErrInvalidInput)
	}
	orderID := strings.TrimSpace(releaseOrderID)
	if orderID == "" {
		return nil, ErrInvalidID
	}
	if _, err := uc.repo.GetByID(ctx, orderID); err != nil {
		return nil, err
	}
	items, err := uc.repo.ListArtifactMetadata(ctx, orderID)
	if err != nil {
		return nil, err
	}
	outputs := make([]ReleaseOrderArtifactMetadataOutput, 0, len(items))
	for _, item := range items {
		outputs = append(outputs, toReleaseOrderArtifactMetadataOutput(item))
	}
	return outputs, nil
}

func (uc *ReleaseOrderManager) DeleteArtifactMetadata(
	ctx context.Context,
	releaseOrderID string,
	artifactID string,
) error {
	if uc == nil || uc.repo == nil {
		return fmt.Errorf("%w: release order manager is not configured", ErrInvalidInput)
	}
	orderID := strings.TrimSpace(releaseOrderID)
	if orderID == "" {
		return ErrInvalidID
	}
	id := strings.TrimSpace(artifactID)
	if id == "" {
		return ErrInvalidID
	}
	if _, err := uc.repo.GetByID(ctx, orderID); err != nil {
		return err
	}
	item, err := uc.repo.GetArtifactMetadataByID(ctx, id)
	if err != nil {
		return err
	}
	if strings.TrimSpace(item.ReleaseOrderID) != orderID {
		return domain.ErrArtifactMetadataNotFound
	}
	if strings.TrimSpace(item.ExecutionID) != "" {
		return fmt.Errorf("%w: 发布过程产出的制品请随发布单删除", ErrInvalidStatus)
	}
	return uc.repo.DeleteArtifactMetadata(ctx, id)
}

func (uc *ReleaseOrderManager) ListArtifactMetadataSummaries(
	ctx context.Context,
	input ListReleaseOrderArtifactMetadataInput,
) ([]ReleaseOrderArtifactMetadataSummaryOutput, int64, error) {
	if uc == nil || uc.repo == nil {
		return nil, 0, fmt.Errorf("%w: release order manager is not configured", ErrInvalidInput)
	}
	const (
		defaultPage     = 1
		defaultPageSize = 20
		maxPageSize     = 100
	)
	input.ProjectID = strings.TrimSpace(input.ProjectID)
	input.ApplicationID = strings.TrimSpace(input.ApplicationID)
	input.ReleaseOrderID = strings.TrimSpace(input.ReleaseOrderID)
	input.Keyword = strings.TrimSpace(input.Keyword)
	input.ArtifactName = strings.TrimSpace(input.ArtifactName)
	input.ArtifactType = strings.TrimSpace(input.ArtifactType)
	input.RepositoryID = strings.TrimSpace(input.RepositoryID)
	if input.CreatedAtFrom != nil && input.CreatedAtTo != nil && input.CreatedAtTo.Before(*input.CreatedAtFrom) {
		return nil, 0, ErrInvalidInput
	}
	scope := domain.PipelineScope(strings.ToLower(strings.TrimSpace(input.PipelineScope)))
	if scope != "" && !scope.Valid() {
		return nil, 0, fmt.Errorf("%w: invalid pipeline_scope", ErrInvalidInput)
	}
	if input.Page <= 0 {
		input.Page = defaultPage
	}
	if input.PageSize <= 0 {
		input.PageSize = defaultPageSize
	}
	if input.PageSize > maxPageSize {
		input.PageSize = maxPageSize
	}

	items, total, err := uc.repo.ListArtifactMetadataSummaries(ctx, domain.ArtifactMetadataListFilter{
		ProjectID:                   input.ProjectID,
		ApplicationID:               input.ApplicationID,
		ApplicationIDs:              normalizeReleaseApplicationIDs(input.ApplicationIDs),
		VisibleApplicationEnvScopes: normalizeReleaseApplicationEnvScopes(input.VisibleApplicationEnvScopes),
		VisibleToUserID:             strings.TrimSpace(input.VisibleToUserID),
		ReleaseOrderID:              input.ReleaseOrderID,
		Keyword:                     input.Keyword,
		ArtifactName:                input.ArtifactName,
		ArtifactType:                input.ArtifactType,
		PipelineScope:               scope,
		RepositoryID:                input.RepositoryID,
		CreatedAtFrom:               input.CreatedAtFrom,
		CreatedAtTo:                 input.CreatedAtTo,
		Page:                        input.Page,
		PageSize:                    input.PageSize,
	})
	if err != nil {
		return nil, 0, err
	}
	outputs := make([]ReleaseOrderArtifactMetadataSummaryOutput, 0, len(items))
	for _, item := range items {
		outputs = append(outputs, toReleaseOrderArtifactMetadataSummaryOutput(item))
	}
	return outputs, total, nil
}

func marshalReleaseOrderArtifactMetadata(values map[string]any) (string, error) {
	if values == nil {
		return "{}", nil
	}
	payload, err := json.Marshal(values)
	if err != nil {
		return "", fmt.Errorf("%w: metadata must be a valid json object", ErrInvalidInput)
	}
	return string(payload), nil
}

func toReleaseOrderArtifactMetadataOutput(item domain.ReleaseOrderArtifactMetadata) ReleaseOrderArtifactMetadataOutput {
	metadata := map[string]any{}
	if strings.TrimSpace(item.MetadataJSON) != "" {
		_ = json.Unmarshal([]byte(item.MetadataJSON), &metadata)
	}
	return ReleaseOrderArtifactMetadataOutput{
		ID:               item.ID,
		ReleaseOrderID:   item.ReleaseOrderID,
		ExecutionID:      item.ExecutionID,
		PipelineScope:    string(item.PipelineScope),
		ArtifactName:     item.ArtifactName,
		ArtifactType:     item.ArtifactType,
		ArtifactVersion:  item.ArtifactVersion,
		ArtifactURL:      item.ArtifactURL,
		RepositoryID:     item.RepositoryID,
		RepositoryName:   item.RepositoryName,
		Bucket:           item.Bucket,
		ObjectKey:        item.ObjectKey,
		Checksum:         item.Checksum,
		ChecksumType:     item.ChecksumType,
		SizeBytes:        item.SizeBytes,
		BuildNumber:      item.BuildNumber,
		AdditionalFields: metadata,
		CreatedAt:        item.CreatedAt,
		UpdatedAt:        item.UpdatedAt,
	}
}

func toReleaseOrderArtifactMetadataSummaryOutput(item domain.ReleaseOrderArtifactMetadataSummary) ReleaseOrderArtifactMetadataSummaryOutput {
	return ReleaseOrderArtifactMetadataSummaryOutput{
		ReleaseOrderArtifactMetadataOutput: toReleaseOrderArtifactMetadataOutput(item.Artifact),
		ReleaseOrderNo:                     item.ReleaseOrderNo,
		ReleaseName:                        item.ReleaseName,
		ReleaseDisplayName:                 buildReleaseArtifactDisplayName(item.ReleaseName, item.ReleaseOrderNo),
		ApplicationID:                      item.ApplicationID,
		ApplicationName:                    item.ApplicationName,
		ProjectID:                          item.ProjectID,
		ProjectName:                        item.ProjectName,
		ProjectKey:                         item.ProjectKey,
		EnvCode:                            item.EnvCode,
		OrderStatus:                        string(item.OrderStatus),
	}
}

func buildReleaseArtifactDisplayName(releaseName, orderNo string) string {
	name := strings.TrimSpace(releaseName)
	no := strings.TrimSpace(orderNo)
	switch {
	case name != "" && no != "":
		return name + " - " + no
	case name != "":
		return name
	case no != "":
		return no
	default:
		return "-"
	}
}
