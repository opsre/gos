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
	currentSiteURL string
}

func (s *systemManagementSettingsStoreStub) LoadCurrentSiteURL(context.Context) (string, error) {
	return s.currentSiteURL, nil
}

func (s *systemManagementSettingsStoreStub) SaveCurrentSiteURL(_ context.Context, value string) error {
	s.currentSiteURL = value
	return nil
}
