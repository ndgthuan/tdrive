package mediadl

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fake_digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func TestParseChecksumsReadsSha256SumFormats(t *testing.T) {
	digest := fake_digest([]byte("fake"))
	body := strings.Join([]string{
		"# comment",
		digest + "  yt-dlp.exe",
		strings.ToUpper(digest) + " *ffmpeg-master-latest-win64-gpl.zip",
		"not-a-digest  other.bin",
		"",
	}, "\n")
	sums := parse_checksums(body)
	if sums["yt-dlp.exe"] != digest {
		t.Fatalf("yt-dlp digest missing: %v", sums)
	}
	if sums["ffmpeg-master-latest-win64-gpl.zip"] != digest {
		t.Fatalf("binary-mode marker not stripped: %v", sums)
	}
	if _, ok := sums["other.bin"]; ok {
		t.Fatal("malformed digest must be ignored")
	}
}

func TestVerifyFileSHA256AcceptsMatchAndRejectsTamper(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "asset.exe")
	data := []byte("fake release bytes")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := verify_file_sha256(path, fake_digest(data)); err != nil {
		t.Fatalf("matching digest rejected: %v", err)
	}
	if err := verify_file_sha256(path, fake_digest([]byte("other"))); err == nil {
		t.Fatal("mismatched digest must be rejected")
	}
}

func write_zip(t *testing.T, path string, entries map[string]string) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	writer := zip.NewWriter(file)
	for name, body := range entries {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestExtractFFmpegTakesOnlyTheBinaryFromBinDir(t *testing.T) {
	dir := t.TempDir()
	zip_path := filepath.Join(dir, "release.zip")
	write_zip(t, zip_path, map[string]string{
		"ffmpeg-master/bin/ffmpeg.exe":  "ffmpeg-binary",
		"ffmpeg-master/bin/ffprobe.exe": "ffprobe-binary",
		"ffmpeg-master/doc/readme.txt":  "docs",
		"../../evil/ffmpeg.exe":         "should not be used",
	})
	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(bin, "ffmpeg.exe")
	if err := extract_ffmpeg_binary(zip_path, dest, bin); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(dest)
	if err != nil || string(data) != "ffmpeg-binary" {
		t.Fatalf("extracted binary wrong: %q %v", data, err)
	}
	entries, _ := os.ReadDir(bin)
	if len(entries) != 1 {
		t.Fatalf("only ffmpeg.exe may be written, found %d files", len(entries))
	}
}

func TestExtractFFmpegFailsWithoutBinary(t *testing.T) {
	dir := t.TempDir()
	zip_path := filepath.Join(dir, "release.zip")
	write_zip(t, zip_path, map[string]string{"docs/readme.txt": "docs"})
	if err := extract_ffmpeg_binary(zip_path, filepath.Join(dir, "ffmpeg.exe"), dir); err == nil {
		t.Fatal("archive without ffmpeg.exe must fail")
	}
}
