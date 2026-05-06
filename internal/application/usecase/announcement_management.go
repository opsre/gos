package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	announcementdomain "gos/internal/domain/announcement"
)

type AnnouncementManager struct {
	repo announcementdomain.Repository
	now  func() time.Time
}

func NewAnnouncementManager(repo announcementdomain.Repository) *AnnouncementManager {
	return &AnnouncementManager{
		repo: repo,
		now:  func() time.Time { return time.Now().UTC() },
	}
}

type CreateAnnouncementInput struct {
	Title     string
	Content   string
	Enabled   bool
	StartTime time.Time
	EndTime   time.Time
	CreatedBy string
}

type UpdateAnnouncementInput struct {
	Title     string
	Content   string
	Enabled   bool
	StartTime time.Time
	EndTime   time.Time
	UpdatedBy string
}

func (uc *AnnouncementManager) Create(ctx context.Context, input CreateAnnouncementInput) (announcementdomain.Announcement, error) {
	if uc == nil || uc.repo == nil {
		return announcementdomain.Announcement{}, fmt.Errorf("%w: announcement repository is not configured", ErrInvalidInput)
	}
	title := strings.TrimSpace(input.Title)
	if title == "" {
		return announcementdomain.Announcement{}, fmt.Errorf("%w: title is required", ErrInvalidInput)
	}
	if input.EndTime.Before(input.StartTime) {
		return announcementdomain.Announcement{}, fmt.Errorf("%w: end time must be after start time", ErrInvalidInput)
	}
	now := uc.now()
	item := announcementdomain.Announcement{
		ID:        generateID("ann"),
		Title:     title,
		Content:   strings.TrimSpace(input.Content),
		Enabled:   input.Enabled,
		StartTime: input.StartTime.UTC(),
		EndTime:   input.EndTime.UTC(),
		CreatedBy: input.CreatedBy,
		UpdatedBy: input.CreatedBy,
		CreatedAt: now,
		UpdatedAt: now,
	}
	return uc.repo.Create(ctx, item)
}

func (uc *AnnouncementManager) Update(ctx context.Context, id string, input UpdateAnnouncementInput) (announcementdomain.Announcement, error) {
	if uc == nil || uc.repo == nil {
		return announcementdomain.Announcement{}, fmt.Errorf("%w: announcement repository is not configured", ErrInvalidInput)
	}
	if strings.TrimSpace(id) == "" {
		return announcementdomain.Announcement{}, ErrInvalidID
	}
	title := strings.TrimSpace(input.Title)
	if title == "" {
		return announcementdomain.Announcement{}, fmt.Errorf("%w: title is required", ErrInvalidInput)
	}
	if input.EndTime.Before(input.StartTime) {
		return announcementdomain.Announcement{}, fmt.Errorf("%w: end time must be after start time", ErrInvalidInput)
	}
	existing, err := uc.repo.GetByID(ctx, id)
	if err != nil {
		return announcementdomain.Announcement{}, err
	}
	existing.Title = title
	existing.Content = strings.TrimSpace(input.Content)
	existing.Enabled = input.Enabled
	existing.StartTime = input.StartTime.UTC()
	existing.EndTime = input.EndTime.UTC()
	existing.UpdatedBy = input.UpdatedBy
	existing.UpdatedAt = uc.now()
	return uc.repo.Update(ctx, existing)
}

func (uc *AnnouncementManager) GetByID(ctx context.Context, id string) (announcementdomain.Announcement, error) {
	if uc == nil || uc.repo == nil {
		return announcementdomain.Announcement{}, fmt.Errorf("%w: announcement repository is not configured", ErrInvalidInput)
	}
	if strings.TrimSpace(id) == "" {
		return announcementdomain.Announcement{}, ErrInvalidID
	}
	return uc.repo.GetByID(ctx, id)
}

func (uc *AnnouncementManager) List(ctx context.Context, filter announcementdomain.ListFilter) ([]announcementdomain.Announcement, int64, error) {
	if uc == nil || uc.repo == nil {
		return nil, 0, fmt.Errorf("%w: announcement repository is not configured", ErrInvalidInput)
	}
	return uc.repo.List(ctx, filter)
}

func (uc *AnnouncementManager) Delete(ctx context.Context, id string) error {
	if uc == nil || uc.repo == nil {
		return fmt.Errorf("%w: announcement repository is not configured", ErrInvalidInput)
	}
	if strings.TrimSpace(id) == "" {
		return ErrInvalidID
	}
	return uc.repo.Delete(ctx, id)
}

// ToggleEnabled 切换公告启用状态。
func (uc *AnnouncementManager) ToggleEnabled(ctx context.Context, id string, enabled bool, updatedBy string) (announcementdomain.Announcement, error) {
	existing, err := uc.repo.GetByID(ctx, id)
	if err != nil {
		return announcementdomain.Announcement{}, err
	}
	existing.Enabled = enabled
	existing.UpdatedBy = updatedBy
	existing.UpdatedAt = uc.now()
	return uc.repo.Update(ctx, existing)
}

// ListActive 查询当前有效的公告。
func (uc *AnnouncementManager) ListActive(ctx context.Context) ([]announcementdomain.Announcement, error) {
	active := true
	filter := announcementdomain.ListFilter{
		Active: &active,
		Page:   1,
		PageSize: 50,
	}
	items, _, err := uc.repo.List(ctx, filter)
	return items, err
}
