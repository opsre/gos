package sqlrepo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	domain "gos/internal/domain/onboarding"
)

type OnboardingRepository struct {
	db     *sql.DB
	driver string
}

func NewOnboardingRepository(db *sql.DB, driver string) *OnboardingRepository {
	return &OnboardingRepository{db, driver}
}
func (r *OnboardingRepository) InitSchema(ctx context.Context) error {
	if r.driver != "sqlite" && r.driver != "mysql" {
		return fmt.Errorf("unsupported database: %s", r.driver)
	}
	return runSchemaMigrations(ctx, r.db, r.driver, schemaMigration{
		Version: "20260905_01_application_onboarding", Description: "create resumable application onboarding sessions and operation receipts", Up: r.createTables,
	})
}
func (r *OnboardingRepository) createTables(ctx context.Context) error {
	// Portable types keep both databases on the same additive schema. A session is
	// serialized as typed JSON; ownership/version/leases remain indexed SQL data.
	for _, statement := range []string{
		`CREATE TABLE IF NOT EXISTS onboarding_sessions (id VARCHAR(64) PRIMARY KEY, owner_user_id VARCHAR(64) NOT NULL, version BIGINT NOT NULL, payload TEXT NOT NULL, lock_key VARCHAR(100) NOT NULL DEFAULT '', locked_until BIGINT NOT NULL DEFAULT 0, updated_at BIGINT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS onboarding_operations (session_id VARCHAR(64) NOT NULL, request_key VARCHAR(100) NOT NULL, request_hash VARCHAR(64) NOT NULL, step_code VARCHAR(40) NOT NULL, status VARCHAR(16) NOT NULL, PRIMARY KEY(session_id,request_key))`,
	} {
		if r.driver == "mysql" {
			statement = strings.ReplaceAll(statement, "payload TEXT", "payload LONGTEXT")
			statement = strings.ReplaceAll(statement, "updated_at BIGINT NOT NULL)", "updated_at BIGINT NOT NULL, INDEX idx_onboarding_owner (owner_user_id,updated_at))")
		}
		if _, err := r.db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	if r.driver == "sqlite" {
		_, err := r.db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_onboarding_owner ON onboarding_sessions(owner_user_id,updated_at)`)
		return err
	}
	return nil
}
func (r *OnboardingRepository) Create(ctx context.Context, s domain.Session) error {
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `INSERT INTO onboarding_sessions(id,owner_user_id,version,payload,updated_at) VALUES(?,?,?,?,?)`, s.ID, s.OwnerUserID, s.Version, string(data), s.UpdatedAt.UnixMilli())
	return err
}
func decodeOnboarding(data string, err error) (domain.Session, error) {
	var s domain.Session
	if errors.Is(err, sql.ErrNoRows) {
		return s, domain.ErrNotFound
	}
	if err != nil {
		return s, err
	}
	err = json.Unmarshal([]byte(data), &s)
	return s, err
}
func (r *OnboardingRepository) Get(ctx context.Context, id string) (domain.Session, error) {
	var data string
	err := r.db.QueryRowContext(ctx, `SELECT payload FROM onboarding_sessions WHERE id=?`, id).Scan(&data)
	return decodeOnboarding(data, err)
}
func (r *OnboardingRepository) List(ctx context.Context, owner string) ([]domain.Session, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT payload FROM onboarding_sessions WHERE owner_user_id=? ORDER BY updated_at DESC LIMIT 100`, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.Session{}
	for rows.Next() {
		var data string
		if err = rows.Scan(&data); err != nil {
			return nil, err
		}
		s, e := decodeOnboarding(data, nil)
		if e != nil {
			return nil, e
		}
		result = append(result, s)
	}
	return result, rows.Err()
}
func (r *OnboardingRepository) Save(ctx context.Context, s domain.Session, version int64) (domain.Session, error) {
	s.Version = version + 1
	s.UpdatedAt = time.Now().UTC()
	data, err := json.Marshal(s)
	if err != nil {
		return s, err
	}
	res, err := r.db.ExecContext(ctx, `UPDATE onboarding_sessions SET payload=?,version=?,updated_at=? WHERE id=? AND version=? AND locked_until<=?`, string(data), s.Version, s.UpdatedAt.UnixMilli(), s.ID, version, time.Now().UnixMilli())
	if err != nil {
		return s, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return s, err
	}
	if n != 1 {
		return s, domain.ErrConflict
	}
	return s, nil
}
func (r *OnboardingRepository) BeginOperation(ctx context.Context, id, key, hash, step string, version int64) (domain.Session, bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Session{}, false, err
	}
	defer tx.Rollback()
	var oldHash, status string
	err = tx.QueryRowContext(ctx, `SELECT request_hash,status FROM onboarding_operations WHERE session_id=? AND request_key=?`, id, key).Scan(&oldHash, &status)
	exists := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return domain.Session{}, false, err
	}
	if exists && oldHash != hash {
		return domain.Session{}, false, domain.ErrConflict
	}
	if exists && status == "applied" {
		var payload string
		e := tx.QueryRowContext(ctx, `SELECT payload FROM onboarding_sessions WHERE id=?`, id).Scan(&payload)
		s, e := decodeOnboarding(payload, e)
		return s, true, e
	}
	now := time.Now()
	res, err := tx.ExecContext(ctx, `UPDATE onboarding_sessions SET lock_key=?,locked_until=?,version=version+1 WHERE id=? AND version=? AND locked_until<=?`, key, now.Add(2*time.Minute).UnixMilli(), id, version, now.UnixMilli())
	if err != nil {
		return domain.Session{}, false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return domain.Session{}, false, err
	}
	if n != 1 {
		return domain.Session{}, false, domain.ErrConflict
	}
	if exists {
		_, err = tx.ExecContext(ctx, `UPDATE onboarding_operations SET status='prepared' WHERE session_id=? AND request_key=?`, id, key)
	} else {
		_, err = tx.ExecContext(ctx, `INSERT INTO onboarding_operations(session_id,request_key,request_hash,step_code,status) VALUES(?,?,?,?,'prepared')`, id, key, hash, step)
	}
	if err != nil {
		return domain.Session{}, false, err
	}
	var payload string
	err = tx.QueryRowContext(ctx, `SELECT payload FROM onboarding_sessions WHERE id=?`, id).Scan(&payload)
	s, err := decodeOnboarding(payload, err)
	if err != nil {
		return s, false, err
	}
	// A new fencing version prevents an expired worker from completing a lease
	// which was reclaimed with the same idempotency key after a restart.
	s.Version++
	encoded, err := json.Marshal(s)
	if err != nil {
		return s, false, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE onboarding_sessions SET payload=? WHERE id=?`, string(encoded), id); err != nil {
		return s, false, err
	}
	err = tx.Commit()
	return s, false, err
}
func (r *OnboardingRepository) FinishOperation(ctx context.Context, s domain.Session, key string, success bool) (domain.Session, error) {
	version := s.Version
	s.Version++
	s.UpdatedAt = time.Now().UTC()
	data, err := json.Marshal(s)
	if err != nil {
		return s, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return s, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE onboarding_sessions SET payload=?,version=?,updated_at=?,lock_key='',locked_until=0 WHERE id=? AND version=? AND lock_key=?`, string(data), s.Version, s.UpdatedAt.UnixMilli(), s.ID, version, key)
	if err != nil {
		return s, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return s, err
	}
	if n != 1 {
		return s, domain.ErrConflict
	}
	status := "failed"
	if success {
		status = "applied"
	}
	_, err = tx.ExecContext(ctx, `UPDATE onboarding_operations SET status=? WHERE session_id=? AND request_key=?`, status, s.ID, key)
	if err != nil {
		return s, err
	}
	return s, tx.Commit()
}
