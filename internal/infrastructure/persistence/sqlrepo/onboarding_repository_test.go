package sqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	ob "gos/internal/domain/onboarding"
	_ "modernc.org/sqlite"
)

func TestOnboardingRepositoryVersionReplayAndLeaseFencing(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	repo := NewOnboardingRepository(db, "sqlite")
	if err := repo.InitSchema(ctx); err != nil {
		t.Fatal(err)
	}
	if err := repo.InitSchema(ctx); err != nil {
		t.Fatalf("migration is not repeatable: %v", err)
	}
	s := ob.Session{ID: "session", OwnerUserID: "owner", Version: 1, UpdatedAt: time.Now()}
	if err := repo.Create(ctx, s); err != nil {
		t.Fatal(err)
	}
	saved, err := repo.Save(ctx, s, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Save(ctx, s, 1); !errors.Is(err, ob.ErrConflict) {
		t.Fatalf("stale draft: %v", err)
	}
	oldWorker, replay, err := repo.BeginOperation(ctx, s.ID, "step-key", "hash", "identity", saved.Version)
	if err != nil || replay {
		t.Fatalf("claim: %v / %v", err, replay)
	}
	if _, err := repo.Save(ctx, oldWorker, oldWorker.Version); !errors.Is(err, ob.ErrConflict) {
		t.Fatalf("save during lease: %v", err)
	}
	if _, _, err := repo.BeginOperation(ctx, s.ID, "other-key", "hash", "identity", oldWorker.Version); !errors.Is(err, ob.ErrConflict) {
		t.Fatalf("concurrent claim: %v", err)
	}
	if _, err := db.Exec(`UPDATE onboarding_sessions SET locked_until=0 WHERE id='session'`); err != nil {
		t.Fatal(err)
	}
	current, _, err := repo.BeginOperation(ctx, s.ID, "step-key", "hash", "identity", oldWorker.Version)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.FinishOperation(ctx, oldWorker, "step-key", true); !errors.Is(err, ob.ErrConflict) {
		t.Fatalf("expired worker completed new lease: %v", err)
	}
	current.Refs.ApplicationID = "application"
	done, err := repo.FinishOperation(ctx, current, "step-key", true)
	if err != nil {
		t.Fatal(err)
	}
	replayed, replay, err := repo.BeginOperation(ctx, s.ID, "step-key", "hash", "identity", 1)
	if err != nil || !replay || replayed.Refs.ApplicationID != "application" || replayed.Version != done.Version {
		t.Fatalf("replay: %+v, %v, %v", replayed, replay, err)
	}
	if _, _, err := repo.BeginOperation(ctx, s.ID, "step-key", "different", "identity", done.Version); !errors.Is(err, ob.ErrConflict) {
		t.Fatalf("changed payload: %v", err)
	}
	hidden, err := repo.List(ctx, "another-owner")
	if err != nil || len(hidden) != 0 {
		t.Fatalf("owner isolation: %v, %v", hidden, err)
	}
}
