package announcement

import "time"

type Announcement struct {
	ID        string
	Title     string
	Content   string
	Enabled   bool
	StartTime time.Time
	EndTime   time.Time
	CreatedBy string
	UpdatedBy string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// IsActive 公告当前是否在有效期内且已启用。
func (a Announcement) IsActive(now time.Time) bool {
	return a.Enabled && !now.Before(a.StartTime) && now.Before(a.EndTime)
}
