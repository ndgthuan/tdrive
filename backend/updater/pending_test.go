package updater

import (
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPendingUpdateSurvivesRestartOffline(t *testing.T) {
	f := newFixture(t, "3.0.1", "3.0.2")
	f.service.Check(t.Context())
	if err := f.service.StartDownload(); err != nil {
		t.Fatal(err)
	}
	f.waitPhase(PhaseReady)
	f.source.err = errors.New("offline")
	calls := f.source.calls
	opts := f.service.opts
	opts.manifestPublicKeys = []ed25519.PublicKey{f.signingKey.publicKey}
	restarted := New(opts)
	t.Cleanup(restarted.Close)
	if got := restarted.State(); got.Phase != PhaseReady || got.Latest.Version != "3.0.2" {
		t.Fatalf("restart lost verified update: %+v", got)
	}
	if f.source.calls != calls {
		t.Fatal("restart contacted the network")
	}
	if err := restarted.Install(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(restarted.pendingPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("installed update retained pending record: %v", err)
	}
	if len(f.installer.installed) != 1 {
		t.Fatal("pending payload was not installed")
	}
}

func TestPendingUpdateReauthenticatesBeforeUse(t *testing.T) {
	for _, change := range []string{"manifest", "signature", "payload", "target", "asset", "old_version"} {
		t.Run(change, func(t *testing.T) {
			f := newFixture(t, "3.0.1", "3.0.2")
			f.service.Check(t.Context())
			if err := f.service.StartDownload(); err != nil {
				t.Fatal(err)
			}
			f.waitPhase(PhaseReady)
			body, err := os.ReadFile(f.service.pendingPath())
			if err != nil {
				t.Fatal(err)
			}
			var record pendingUpdate
			if err := json.Unmarshal(body, &record); err != nil {
				t.Fatal(err)
			}
			switch change {
			case "manifest":
				record.Manifest[0] ^= 1
			case "signature":
				record.Signature[len(record.Signature)-2] ^= 1
			case "payload":
				if err := os.WriteFile(filepath.Join(f.cacheDir, f.assetName), []byte("modified"), 0600); err != nil {
					t.Fatal(err)
				}
			case "target":
				record.Target = filepath.Join(t.TempDir(), "another.app")
			case "asset":
				record.Asset.Name = "../outside.zip"
			case "old_version":
				record.Release.Tag = "v3.0.0"
			}
			body, err = json.Marshal(record)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(f.service.pendingPath(), body, 0600); err != nil {
				t.Fatal(err)
			}
			opts := f.service.opts
			opts.manifestPublicKeys = []ed25519.PublicKey{f.signingKey.publicKey}
			restarted := New(opts)
			t.Cleanup(restarted.Close)
			if restarted.State().Phase == PhaseReady {
				t.Fatal("untrusted pending update became installable")
			}
			if err := restarted.Install(); !errors.Is(err, ErrNotReady) {
				t.Fatalf("install = %v", err)
			}
			if len(f.installer.installed) != 0 {
				t.Fatal("untrusted pending update reached installer")
			}
		})
	}
}
