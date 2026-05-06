package sqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	announcementdomain "gos/internal/domain/announcement"

	mysqlDriver "github.com/go-sql-driver/mysql"
)

type AnnouncementRepository struct {
	db       *sql.DB
	dbDriver string
}

func NewAnnouncementRepository(db *sql.DB, dbDriver string) *AnnouncementRepository {
	return &AnnouncementRepository{db: db, dbDriver: strings.ToLower(strings.TrimSpace(dbDriver))}
}

func (r *AnnouncementRepository) InitSchema(ctx context.Context) error {
	var schema string
	switch r.dbDriver {
	case "mysql":
		schema = `
	CREATE TABLE IF NOT EXISTS announcements (
		id VARCHAR(64) PRIMARY KEY,
		title VARCHAR(256) NOT NULL,
		content TEXT NOT NULL,
		enabled TINYINT(1) NOT NULL DEFAULT 1,
		start_time BIGINT NOT NULL,
		end_time BIGINT NOT NULL,
		created_by VARCHAR(128) NOT NULL,
		updated_by VARCHAR(128) NOT NULL,
		created_at BIGINT NOT NULL,
		updated_at BIGINT NOT NULL
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`
	case "sqlite":
		schema = `
	CREATE TABLE IF NOT EXISTS announcements (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		content TEXT NOT NULL,
		enabled INTEGER NOT NULL DEFAULT 1,
		start_time INTEGER NOT NULL,
		end_time INTEGER NOT NULL,
		created_by TEXT NOT NULL,
		updated_by TEXT NOT NULL,
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL
	);`
	default:
		return fmt.Errorf("unsupported db driver: %s", r.dbDriver)
	}
	if _, err := r.db.ExecContext(ctx, schema); err != nil {
		return err
	}
	// 旧表兼容：尝试补加 enabled 列，失败忽略（MySQL 5.7 不支持 IF NOT EXISTS）
	if r.dbDriver == "mysql" {
		_, _ = r.db.ExecContext(ctx, `ALTER TABLE announcements ADD COLUMN enabled TINYINT(1) NOT NULL DEFAULT 1`)
	}
	return nil
}

func (r *AnnouncementRepository) Create(ctx context.Context, item announcementdomain.Announcement) (announcementdomain.Announcement, error) {
	const q = `
	INSERT INTO announcements (id, title, content, enabled, start_time, end_time, created_by, updated_by, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`
	_, err := r.db.ExecContext(ctx, q,
		item.ID,
		item.Title,
		item.Content,
		announcementBoolToInt(item.Enabled),
		item.StartTime.UTC().UnixNano(),
		item.EndTime.UTC().UnixNano(),
		item.CreatedBy,
		item.UpdatedBy,
		item.CreatedAt.UTC().UnixNano(),
		item.UpdatedAt.UTC().UnixNano(),
	)
	if err != nil {
		if isMySQLDuplicateError(r.dbDriver, err) {
			return announcementdomain.Announcement{}, err
		}
		return announcementdomain.Announcement{}, err
	}
	return item, nil
}

func (r *AnnouncementRepository) Update(ctx context.Context, item announcementdomain.Announcement) (announcementdomain.Announcement, error) {
	const q = `
	UPDATE announcements
	SET title = ?, content = ?, enabled = ?, start_time = ?, end_time = ?, updated_by = ?, updated_at = ?
	WHERE id = ?;`
	result, err := r.db.ExecContext(ctx, q,
		item.Title,
		item.Content,
		announcementBoolToInt(item.Enabled),
		item.StartTime.UTC().UnixNano(),
		item.EndTime.UTC().UnixNano(),
		item.UpdatedBy,
		item.UpdatedAt.UTC().UnixNano(),
		item.ID,
	)
	if err != nil {
		return announcementdomain.Announcement{}, err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return announcementdomain.Announcement{}, announcementdomain.ErrNotFound
	}
	return item, nil
}

func (r *AnnouncementRepository) GetByID(ctx context.Context, id string) (announcementdomain.Announcement, error) {
	const q = `
	SELECT id, title, content, enabled, start_time, end_time, created_by, updated_by, created_at, updated_at
	FROM announcements
	WHERE id = ?;`
	row := r.db.QueryRowContext(ctx, q, id)
	item, err := scanAnnouncement(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return announcementdomain.Announcement{}, announcementdomain.ErrNotFound
		}
		return announcementdomain.Announcement{}, err
	}
	return item, nil
}

func (r *AnnouncementRepository) List(ctx context.Context, filter announcementdomain.ListFilter) ([]announcementdomain.Announcement, int64, error) {
	args := make([]any, 0, 4)
	where := make([]string, 0, 3)

	if filter.Keyword != "" {
		where = append(where, "title LIKE ?")
		args = append(args, "%"+filter.Keyword+"%")
	}
	if filter.Active != nil {
		now := time.Now().UTC().UnixNano()
		if *filter.Active {
			where = append(where, "enabled = 1 AND start_time <= ? AND end_time > ?")
			args = append(args, now, now)
		} else {
			where = append(where, "end_time <= ?")
			args = append(args, now)
		}
	}

	countSQL := "SELECT COUNT(1) FROM announcements"
	if len(where) > 0 {
		countSQL += " WHERE " + strings.Join(where, " AND ")
	}
	var total int64
	if err := r.db.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `
	SELECT id, title, content, enabled, start_time, end_time, created_by, updated_by, created_at, updated_at
	FROM announcements`
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY created_at DESC"

	if filter.Page > 0 && filter.PageSize > 0 {
		query += fmt.Sprintf(" LIMIT %d OFFSET %d", filter.PageSize, (filter.Page-1)*filter.PageSize)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []announcementdomain.Announcement
	for rows.Next() {
		item, err := scanAnnouncement(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *AnnouncementRepository) Delete(ctx context.Context, id string) error {
	const q = `DELETE FROM announcements WHERE id = ?;`
	result, err := r.db.ExecContext(ctx, q, id)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return announcementdomain.ErrNotFound
	}
	return nil
}

func scanAnnouncement(scanner interface{ Scan(dest ...any) error }) (announcementdomain.Announcement, error) {
	var item announcementdomain.Announcement
	var enabledInt int
	var startNano, endNano, createdNano, updatedNano int64
	err := scanner.Scan(
		&item.ID,
		&item.Title,
		&item.Content,
		&enabledInt,
		&startNano,
		&endNano,
		&item.CreatedBy,
		&item.UpdatedBy,
		&createdNano,
		&updatedNano,
	)
	if err != nil {
		return announcementdomain.Announcement{}, err
	}
	item.Enabled = enabledInt != 0
	item.StartTime = time.Unix(0, startNano).UTC()
	item.EndTime = time.Unix(0, endNano).UTC()
	item.CreatedAt = time.Unix(0, createdNano).UTC()
	item.UpdatedAt = time.Unix(0, updatedNano).UTC()
	return item, nil
}

func announcementBoolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func isMySQLDuplicateError(dbDriver string, err error) bool {
	if strings.ToLower(dbDriver) != "mysql" {
		return false
	}
	var mysqlErr *mysqlDriver.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
}
