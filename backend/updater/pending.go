package updater

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
)

// Only a downloaded, authenticated payload is scheduled. Reopening the app
// reauthenticates the saved manifest and hashes the bytes without network I/O.
// The installation path is bound so a portable copy cannot update another copy.
type pendingUpdate struct {
	Release   Release
	Asset     Asset
	Target    string
	Manifest  []byte
	Signature []byte
}

func (s *Service) pendingPath() string { return filepath.Join(s.opts.CacheDir, "pending.json") }

func (s *Service) savePendingLocked() {
	if s.state.Phase != PhaseReady || s.release == nil || s.asset == nil || len(s.manifest) == 0 {
		return
	}
	record := pendingUpdate{*s.release, *s.asset, s.target.Path, s.manifest, s.signature}
	body, err := json.Marshal(record)
	if err != nil {
		return
	}
	if err = ensureCacheDir(s.opts.CacheDir); err != nil {
		s.pendingErrorLocked(err)
		return
	}
	file, err := os.CreateTemp(s.opts.CacheDir, "pending-*.tmp")
	if err != nil {
		s.pendingErrorLocked(err)
		return
	}
	name := file.Name()
	defer os.Remove(name)
	if _, err = file.Write(body); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(name, s.pendingPath())
	}
	if err != nil {
		s.pendingErrorLocked(err)
	}
}

func (s *Service) pendingErrorLocked(err error) {
	s.state.Error = "The update is ready, but could not be scheduled for next launch: " + userMessage(err)
	s.state.ErrorStage = "install"
}

func (s *Service) restorePending() {
	file, err := os.Open(s.pendingPath())
	if err != nil {
		return
	}
	defer file.Close()
	var record pendingUpdate
	body, err := io.ReadAll(io.LimitReader(file, (2<<20)+1))
	if err != nil || len(body) > 2<<20 || json.Unmarshal(body, &record) != nil {
		return
	}
	version, err := ParseVersion(record.Release.Tag)
	if err != nil || !version.Newer(s.current) {
		_ = os.Remove(s.pendingPath())
		return
	}
	name, ok := appAssetName(record.Release.Tag, s.opts.Platform)
	if !ok || name != record.Asset.Name || record.Asset.Size <= 0 {
		return
	}
	if s.manifestKeys.Verify(record.Manifest, record.Signature) != nil {
		return
	}
	sums, err := parseChecksums(bytes.NewReader(record.Manifest))
	if err != nil || sums[name] == "" {
		return
	}
	target, err := s.resolveTarget()
	if err != nil || target.Path != record.Target || !dirWritable(filepath.Dir(target.Path)) {
		return
	}
	payload := s.payloadPath(name)
	if verifyFile(payload, record.Asset.Size, sums[name]) != nil {
		return
	}
	record.Release.Version = version
	s.applyResolutionLocked(record.Release, resolution{
		asset: &record.Asset, checksums: sums, installable: true,
		payload: payload, manifest: record.Manifest, signature: record.Signature,
	})
}
