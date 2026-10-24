package autobackup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeviceFolderNameMapsKnownDevices(t *testing.T) {
	cases := []struct {
		name                                string
		platform, manufacturer, model, want string
	}{
		{"iphone", PlatformIOS, "Apple", "iPhone", "iPhone"},
		{"ipad", PlatformIOS, "Apple", "iPad", "iPad"},
		{"ipad mixed case", PlatformIOS, "", "iPAD Pro", "iPad"},
		{"ios unknown model defaults to iPhone", PlatformIOS, "", "", "iPhone"},
		{"oppo", PlatformAndroid, "OPPO", "CPH2415", "OPPO"},
		{"samsung lower", PlatformAndroid, "samsung", "SM-S928B", "Samsung"},
		{"xiaomi", PlatformAndroid, "Xiaomi", "2201117TY", "Xiaomi"},
		{"huawei", PlatformAndroid, "HUAWEI", "ALN-L29", "HUAWEI"},
		{"oneplus", PlatformAndroid, "OnePlus", "CPH2581", "OnePlus"},
		{"realme", PlatformAndroid, "realme", "RMX3700", "Realme"},
		{"vivo", PlatformAndroid, "vivo", "V2316", "Vivo"},
		{"google pixel", PlatformAndroid, "Google", "Pixel 8", "Pixel"},
		{"unknown android", PlatformAndroid, "Fairphone", "FP5", "Phone"},
		{"unknown platform", "windows", "", "", "Phone"},
		{"brand substring is not a match", PlatformAndroid, "vivoxyz", "", "Phone"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := DeviceFolderName(tc.platform, tc.manufacturer, tc.model); got != tc.want {
				t.Fatalf("DeviceFolderName(%q,%q,%q) = %q, want %q", tc.platform, tc.manufacturer, tc.model, got, tc.want)
			}
		})
	}
}

func TestResolveFolderNameUsesValidOverrideOnly(t *testing.T) {
	if got := ResolveFolderName("", PlatformIOS, "", "iPhone"); got != "iPhone" {
		t.Fatalf("empty override = %q", got)
	}
	if got := ResolveFolderName("  Work phone ", PlatformIOS, "", "iPhone"); got != "Work phone" {
		t.Fatalf("valid override = %q", got)
	}
	if got := ResolveFolderName("bad/name", PlatformAndroid, "Samsung", ""); got != "Samsung" {
		t.Fatalf("invalid override must fall back to device default, got %q", got)
	}
}

func TestSanitizeFolderName(t *testing.T) {
	valid := map[string]string{
		"iPhone":          "iPhone",
		"  My phone  ":    "My phone",
		"Trailing dots..": "Trailing dots",
		"Tiếng Việt":      "Tiếng Việt",
	}
	for input, want := range valid {
		got, err := SanitizeFolderName(input)
		if err != nil || got != want {
			t.Fatalf("SanitizeFolderName(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
	invalid := []string{"", "   ", ".", "..", "a/b", `a\b`, "a:b", "a*b", "tab\there", strings.Repeat("x", 201)}
	for _, input := range invalid {
		if _, err := SanitizeFolderName(input); !errors.Is(err, ErrInvalidFolderName) {
			t.Fatalf("SanitizeFolderName(%q) error = %v, want ErrInvalidFolderName", input, err)
		}
	}
}

func TestSettingsDefaultsAndRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "auto_backup.json")

	missing, err := LoadSettings(path)
	if err != nil {
		t.Fatalf("missing file: %v", err)
	}
	if missing.Enabled || !missing.WiFiOnly || missing.ChargingOnly || missing.FolderName != "" {
		t.Fatalf("defaults = %+v", missing)
	}

	// An older file without wifi_only must keep Wi-Fi-only on.
	if err := os.WriteFile(path, []byte(`{"enabled":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	legacy, err := LoadSettings(path)
	if err != nil {
		t.Fatalf("legacy file: %v", err)
	}
	if !legacy.Enabled || !legacy.WiFiOnly {
		t.Fatalf("legacy settings = %+v, want enabled with wifi_only true", legacy)
	}

	want := Settings{Enabled: true, FolderName: "Pixel 8", WiFiOnly: false, ChargingOnly: true}
	if err := SaveSettings(path, want); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := LoadSettings(path)
	if err != nil || got != want {
		t.Fatalf("round trip = %+v, %v; want %+v", got, err, want)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("atomic write left temp files: %d entries", len(entries))
	}
}

func TestLoadSettingsCorruptFileIsOffWithError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auto_backup.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := LoadSettings(path)
	if err == nil {
		t.Fatal("expected decode error")
	}
	if got.Enabled || !got.WiFiOnly {
		t.Fatalf("corrupt file must load as safe defaults, got %+v", got)
	}
}

func TestSaveSettingsRejectsBadName(t *testing.T) {
	err := SaveSettings(filepath.Join(t.TempDir(), "s.json"), Settings{FolderName: "a/b"})
	if !errors.Is(err, ErrInvalidFolderName) {
		t.Fatalf("error = %v", err)
	}
}

func TestDecidePhaseOrdering(t *testing.T) {
	on := Settings{Enabled: true, WiFiOnly: true}
	cases := []struct {
		name string
		in   PhaseInput
		want Phase
	}{
		{"disabled", PhaseInput{Settings: Settings{}, Permission: PermissionGranted, PendingItems: 5}, PhaseOff},
		{"denied beats wifi wait", PhaseInput{Settings: on, Permission: PermissionDenied, WiFiKnown: true}, PhaseNeedsPermission},
		{"undetermined", PhaseInput{Settings: on, Permission: PermissionUndetermined}, PhaseNeedsPermission},
		{"paused beats wifi", PhaseInput{Settings: on, Permission: PermissionGranted, Paused: true, WiFiKnown: true}, PhasePaused},
		{"waiting for wifi", PhaseInput{Settings: on, Permission: PermissionGranted, WiFiKnown: true, OnWiFi: false, PendingItems: 1}, PhaseWaitingForWiFi},
		{"unknown wifi does not block", PhaseInput{Settings: on, Permission: PermissionGranted, PendingItems: 1}, PhaseRunning},
		{"waiting for charging", PhaseInput{Settings: Settings{Enabled: true, WiFiOnly: false, ChargingOnly: true}, Permission: PermissionGranted, ChargingKnown: true, Charging: false, PendingItems: 1}, PhaseWaitingForCharging},
		{"limited still runs", PhaseInput{Settings: on, Permission: PermissionLimited, WiFiKnown: true, OnWiFi: true, PendingItems: 2}, PhaseRunning},
		{"up to date", PhaseInput{Settings: on, Permission: PermissionGranted, WiFiKnown: true, OnWiFi: true}, PhaseUpToDate},
		{"failed when nothing pending", PhaseInput{Settings: on, Permission: PermissionGranted, WiFiKnown: true, OnWiFi: true, FailedItems: 3}, PhaseFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := DecidePhase(tc.in); got != tc.want {
				t.Fatalf("DecidePhase = %q, want %q", got, tc.want)
			}
		})
	}
}

type fakeFolderStore struct {
	folders     map[string]string // parent|name -> id
	createErr   error
	createRace  bool // when true, Create fails after a sibling appeared
	createCalls int
	findCalls   int
	nextID      int
}

func (f *fakeFolderStore) key(parent, name string) string { return parent + "|" + name }

func (f *fakeFolderStore) Find(_ context.Context, parent, name string) (string, bool, error) {
	f.findCalls++
	id, ok := f.folders[f.key(parent, name)]
	return id, ok, nil
}

func (f *fakeFolderStore) Create(_ context.Context, parent, name string) (string, error) {
	f.createCalls++
	if f.createRace {
		f.folders[f.key(parent, name)] = "racer"
		return "", errors.New("already exists")
	}
	if f.createErr != nil {
		return "", f.createErr
	}
	f.nextID++
	id := "folder-" + string(rune('0'+f.nextID))
	f.folders[f.key(parent, name)] = id
	return id, nil
}

func TestEnsureFolderIsIdempotent(t *testing.T) {
	store := &fakeFolderStore{folders: map[string]string{}}
	first, err := EnsureFolder(context.Background(), store, "", "iPhone")
	if err != nil {
		t.Fatal(err)
	}
	second, err := EnsureFolder(context.Background(), store, "", "iPhone")
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("ids differ: %q vs %q", first, second)
	}
	if store.createCalls != 1 {
		t.Fatalf("create calls = %d, want 1", store.createCalls)
	}
}

func TestEnsureFolderResolvesRacingSibling(t *testing.T) {
	store := &fakeFolderStore{folders: map[string]string{}, createRace: true}
	id, err := EnsureFolder(context.Background(), store, "", "Samsung")
	if err != nil {
		t.Fatal(err)
	}
	if id != "racer" {
		t.Fatalf("id = %q, want the sibling that won the race", id)
	}
}

func TestEnsureFolderPropagatesCreateFailure(t *testing.T) {
	store := &fakeFolderStore{folders: map[string]string{}, createErr: errors.New("offline")}
	if _, err := EnsureFolder(context.Background(), store, "", "Pixel"); err == nil {
		t.Fatal("expected error")
	}
}

func TestEnsureFolderRejectsEmptyNameAndNilStore(t *testing.T) {
	if _, err := EnsureFolder(context.Background(), nil, "", "x"); err == nil {
		t.Fatal("nil store must fail")
	}
	store := &fakeFolderStore{folders: map[string]string{}}
	if _, err := EnsureFolder(context.Background(), store, "", ""); !errors.Is(err, ErrInvalidFolderName) {
		t.Fatalf("empty name error = %v", err)
	}
}

func TestEnsureFolderHonoursCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	store := &fakeFolderStore{folders: map[string]string{}}
	if _, err := EnsureFolder(ctx, store, "", "iPad"); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
	if store.createCalls != 0 {
		t.Fatal("no create after cancellation")
	}
}
