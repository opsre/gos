package usecase

import (
	"context"
	"errors"
	"testing"
)

func TestUpdateSystemManagementSettings(t *testing.T) {
	t.Parallel()

	store := &systemManagementSettingsStoreStub{}
	reader := NewQuerySystemManagementSettings(store)
	updater := NewUpdateSystemManagementSettings(store, reader)

	output, err := updater.Execute(context.Background(), UpdateSystemManagementSettingsInput{
		CurrentSiteURL: " https://gos.example.com/ ",
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if output.CurrentSiteURL != "https://gos.example.com" {
		t.Fatalf("CurrentSiteURL = %q, want %q", output.CurrentSiteURL, "https://gos.example.com")
	}
}

func TestUpdateSystemManagementSettingsRejectsInvalidURL(t *testing.T) {
	t.Parallel()

	store := &systemManagementSettingsStoreStub{}
	reader := NewQuerySystemManagementSettings(store)
	updater := NewUpdateSystemManagementSettings(store, reader)

	_, err := updater.Execute(context.Background(), UpdateSystemManagementSettingsInput{
		CurrentSiteURL: "gos.example.com",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Execute error = %v, want ErrInvalidInput", err)
	}
	if store.currentSiteURL != "" {
		t.Fatalf("invalid URL was persisted: %q", store.currentSiteURL)
	}
}

type systemManagementSettingsStoreStub struct {
	currentSiteURL     string
	metricsAuthEnabled *bool
	saveErr            error
	loadAuthErr        error
}

func (s *systemManagementSettingsStoreStub) LoadCurrentSiteURL(context.Context) (string, error) {
	return s.currentSiteURL, nil
}

func (s *systemManagementSettingsStoreStub) LoadMetricsAuthEnabled(context.Context) (*bool, error) {
	return s.metricsAuthEnabled, s.loadAuthErr
}

func (s *systemManagementSettingsStoreStub) SaveSystemManagementSettings(_ context.Context, currentSiteURL string, metricsAuthEnabled *bool) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	s.currentSiteURL = currentSiteURL
	if metricsAuthEnabled != nil {
		enabled := *metricsAuthEnabled
		s.metricsAuthEnabled = &enabled
	}
	return nil
}

func TestMetricsAuthSwitchPersistsAndChangesActivePolicy(t *testing.T) {
	t.Parallel()
	store := &systemManagementSettingsStoreStub{}
	options := SystemManagementMetricsOptions{Enabled: true, TokenConfigured: true, AuthRequiredByDefault: true}
	reader := NewQuerySystemManagementSettings(store, options)
	updater := NewUpdateSystemManagementSettings(store, reader)
	for _, enabled := range []bool{false, true, false} {
		output, err := updater.Execute(context.Background(), UpdateSystemManagementSettingsInput{CurrentSiteURL: "https://gos.example.com", MetricsAuthEnabled: &enabled})
		if err != nil {
			t.Fatal(err)
		}
		if reader.MetricsAuthRequired() != enabled || output.MetricsAuthEnabled != enabled || store.metricsAuthEnabled == nil || *store.metricsAuthEnabled != enabled {
			t.Fatalf("policy did not change to %v: %+v", enabled, output)
		}
	}
	restarted := NewQuerySystemManagementSettings(store, options)
	if err := restarted.RefreshMetricsAuthPolicy(context.Background()); err != nil {
		t.Fatal(err)
	}
	if restarted.MetricsAuthRequired() {
		t.Fatal("disabled policy lost after restart")
	}
	output, err := updater.Execute(context.Background(), UpdateSystemManagementSettingsInput{CurrentSiteURL: "https://changed.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if output.MetricsAuthEnabled || *store.metricsAuthEnabled {
		t.Fatal("older client unexpectedly changed metrics authentication")
	}
}

func TestMetricsAuthCanBeDisabledWithoutTokenAndCannotBeEnabledWithoutOne(t *testing.T) {
	t.Parallel()
	store := &systemManagementSettingsStoreStub{}
	reader := NewQuerySystemManagementSettings(store, SystemManagementMetricsOptions{Enabled: true, AuthRequiredByDefault: true})
	updater := NewUpdateSystemManagementSettings(store, reader)
	disabled := false
	if _, err := updater.Execute(context.Background(), UpdateSystemManagementSettingsInput{MetricsAuthEnabled: &disabled}); err != nil {
		t.Fatal(err)
	}
	enabled := true
	if _, err := updater.Execute(context.Background(), UpdateSystemManagementSettingsInput{MetricsAuthEnabled: &enabled}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected missing-token validation, got %v", err)
	}
	if reader.MetricsAuthRequired() || *store.metricsAuthEnabled {
		t.Fatal("invalid enable changed active/persisted policy")
	}
}

func TestMetricsAuthFailedWritesAndReadsRetainActivePolicy(t *testing.T) {
	t.Parallel()
	store := &systemManagementSettingsStoreStub{}
	reader := NewQuerySystemManagementSettings(store, SystemManagementMetricsOptions{TokenConfigured: true, AuthRequiredByDefault: true})
	updater := NewUpdateSystemManagementSettings(store, reader)
	store.saveErr = errors.New("database unavailable")
	disabled := false
	if _, err := updater.Execute(context.Background(), UpdateSystemManagementSettingsInput{MetricsAuthEnabled: &disabled}); err == nil {
		t.Fatal("expected save error")
	}
	if !reader.MetricsAuthRequired() || store.metricsAuthEnabled != nil {
		t.Fatal("failed save changed the policy")
	}
	store.loadAuthErr = errors.New("database unavailable")
	if err := reader.RefreshMetricsAuthPolicy(context.Background()); err == nil {
		t.Fatal("expected read error")
	}
	if !reader.MetricsAuthRequired() {
		t.Fatal("failed policy refresh disabled authentication")
	}
}
