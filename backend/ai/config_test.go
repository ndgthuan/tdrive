// Copyright (c) 2026 Nguyễn Dương Gia Thuận

package ai

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func new_test_manager(t *testing.T) (*Manager, *fake_secrets, string) {
	t.Helper()
	isolate_data_dir(t)
	dir := t.TempDir()
	path := filepath.Join(dir, models_file_name)
	secrets := new_fake_secrets()
	return NewManager(path, secrets), secrets, path
}

func TestModelConfigAddListUpdateDelete(t *testing.T) {
	manager, secrets, path := new_test_manager(t)

	first, err := manager.AddModel(ModelInput{
		DisplayName:   "GPT vision",
		Provider:      ProviderOpenAI,
		Model:         "gpt-4o",
		VisionEnabled: true,
		APIKey:        "sk-secret-value-123",
	})
	if err != nil {
		t.Fatalf("add first model: %v", err)
	}
	if !first.IsDefault || !first.HasAPIKey {
		t.Fatalf("first model should be default with key, got %+v", first)
	}

	second, err := manager.AddModel(ModelInput{
		DisplayName: "Local",
		Provider:    ProviderCustom,
		BaseURL:     "http://localhost:11434/v1/",
		Model:       "llama",
	})
	if err != nil {
		t.Fatalf("add custom model without key: %v", err)
	}
	if second.IsDefault || second.HasAPIKey {
		t.Fatalf("second custom model should not be default and has no key: %+v", second)
	}
	if second.BaseURL != "http://localhost:11434/v1" {
		t.Fatalf("base URL should be normalized, got %q", second.BaseURL)
	}

	views, err := manager.ListModels()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(views) != 2 {
		t.Fatalf("want 2 models, got %d", len(views))
	}

	if err := manager.SetDefaultModel(second.ID); err != nil {
		t.Fatalf("set default: %v", err)
	}
	runtime, err := manager.Resolve("")
	if err != nil {
		t.Fatalf("resolve default: %v", err)
	}
	if runtime.Profile.ID != second.ID {
		t.Fatalf("default should be the custom model, got %s", runtime.Profile.ID)
	}

	updated, err := manager.UpdateModel(first.ID, ModelInput{
		DisplayName:   "GPT vision v2",
		Provider:      ProviderOpenAI,
		Model:         "gpt-4.1",
		VisionEnabled: true,
	})
	if err != nil {
		t.Fatalf("update keeping key: %v", err)
	}
	if !updated.HasAPIKey || updated.Model != "gpt-4.1" || updated.IsDefault {
		t.Fatalf("update should keep key and default flag, got %+v", updated)
	}

	if err := manager.DeleteModel(second.ID); err != nil {
		t.Fatalf("delete default: %v", err)
	}
	views, _ = manager.ListModels()
	if len(views) != 1 || !views[0].IsDefault {
		t.Fatalf("remaining model should become default, got %+v", views)
	}
	if _, ok := secrets.keys[second.ID]; ok {
		t.Fatalf("deleted model key should be removed")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config file: %v", err)
	}
	if strings.Contains(string(data), "sk-secret-value-123") {
		t.Fatalf("API key must never be written to the profile file")
	}
}

func TestModelConfigKeyRotationAndClear(t *testing.T) {
	manager, secrets, _ := new_test_manager(t)
	view, err := manager.AddModel(ModelInput{DisplayName: "Claude", Provider: ProviderAnthropic, Model: "claude", APIKey: "old"})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, err := manager.UpdateModel(view.ID, ModelInput{DisplayName: "Claude", Provider: ProviderAnthropic, Model: "claude", APIKey: "new"}); err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if secrets.keys[view.ID] != "new" {
		t.Fatalf("key should be rotated, got %q", secrets.keys[view.ID])
	}
	if _, err := manager.UpdateModel(view.ID, ModelInput{DisplayName: "Claude", Provider: ProviderAnthropic, Model: "claude", ClearAPIKey: true}); err == nil {
		t.Fatalf("clearing the key of a cloud provider should be refused")
	}
	if secrets.keys[view.ID] != "new" {
		t.Fatalf("a refused clear must leave the stored key untouched, got %q", secrets.keys[view.ID])
	}
}

func TestModelConfigValidation(t *testing.T) {
	manager, _, _ := new_test_manager(t)
	cases := map[string]ModelInput{
		"missing name":       {Provider: ProviderOpenAI, Model: "m", APIKey: "k"},
		"unknown provider":   {DisplayName: "x", Provider: "mystery", Model: "m", APIKey: "k"},
		"missing model":      {DisplayName: "x", Provider: ProviderOpenAI, APIKey: "k"},
		"custom without url": {DisplayName: "x", Provider: ProviderCustom, Model: "m"},
		"bad scheme":         {DisplayName: "x", Provider: ProviderCustom, BaseURL: "ftp://host", Model: "m"},
		"query in base url":  {DisplayName: "x", Provider: ProviderCustom, BaseURL: "https://h/v1?key=1", Model: "m"},
		"cloud without key":  {DisplayName: "x", Provider: ProviderGemini, Model: "m"},
		"url without host":   {DisplayName: "x", Provider: ProviderCustom, BaseURL: "https://", Model: "m"},
	}
	for name, input := range cases {
		if _, err := manager.AddModel(input); err == nil {
			t.Errorf("%s: expected validation error", name)
		}
	}
	views, _ := manager.ListModels()
	if len(views) != 0 {
		t.Fatalf("rejected inputs must not persist, got %d models", len(views))
	}
}

func TestModelConfigMissingKeyIsReported(t *testing.T) {
	manager, secrets, _ := new_test_manager(t)
	view, err := manager.AddModel(ModelInput{DisplayName: "G", Provider: ProviderGemini, Model: "gemini", APIKey: "k"})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	delete(secrets.keys, view.ID)
	if _, err := manager.Resolve(view.ID); err == nil || errors.Is(err, ErrNoModel) {
		t.Fatalf("missing key on a cloud model should be an explicit error, got %v", err)
	}
	if _, err := manager.Resolve("does-not-exist"); !errors.Is(err, ErrNoModel) {
		t.Fatalf("unknown id should be ErrNoModel, got %v", err)
	}
}

func TestModelConfigFilePermissionsAreOwnerOnly(t *testing.T) {
	if os.PathSeparator == '\\' {
		t.Skip("Windows does not carry POSIX permission bits")
	}
	manager, _, path := new_test_manager(t)
	if _, err := manager.AddModel(ModelInput{DisplayName: "C", Provider: ProviderCustom, BaseURL: "http://127.0.0.1:1", Model: "m"}); err != nil {
		t.Fatalf("add: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("config perms = %v, want 0600", info.Mode().Perm())
	}
}
