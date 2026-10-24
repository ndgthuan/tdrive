// Copyright (c) 2026 Nguyễn Dương Gia Thuận

package mediadl

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Release locations. Both projects publish a checksum file next to the asset,
// and the installer refuses any asset whose SHA-256 does not match that file.
const (
	YTDLP_RELEASE_BASE  = "https://github.com/yt-dlp/yt-dlp/releases/latest/download/"
	YTDLP_ASSET         = "yt-dlp.exe"
	YTDLP_SUMS          = "SHA2-256SUMS"
	FFMPEG_RELEASE_BASE = "https://github.com/yt-dlp/FFmpeg-Builds/releases/latest/download/"
	FFMPEG_ASSET        = "ffmpeg-master-latest-win64-gpl.zip"
	FFMPEG_SUMS         = "checksums.sha256"
)

// Size caps keep a bad or hostile response from filling the disk.
const (
	MAX_SUMS_BYTES          = 1 << 20
	MAX_YTDLP_BYTES         = 256 << 20
	MAX_FFMPEG_ZIP_BYTES    = 512 << 20
	MAX_FFMPEG_BINARY_BYTES = 512 << 20
	INSTALL_TIMEOUT         = 15 * time.Minute
)

// Installer downloads yt-dlp and ffmpeg into the app-managed bin directory.
// Base URLs are fields so a test can point them at a local server.
type Installer struct {
	paths       Paths
	client      *http.Client
	ytdlp_base  string
	ffmpeg_base string
}

// NewInstaller builds the production installer for a data directory.
func NewInstaller(paths Paths) *Installer {
	return &Installer{
		paths:       paths,
		client:      &http.Client{Timeout: INSTALL_TIMEOUT, CheckRedirect: require_https_redirect},
		ytdlp_base:  YTDLP_RELEASE_BASE,
		ffmpeg_base: FFMPEG_RELEASE_BASE,
	}
}

func require_https_redirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("too many redirects")
	}
	if req.URL.Scheme != "https" {
		return errors.New("refusing a non-HTTPS redirect")
	}
	return nil
}

// Install downloads and verifies one tool. progress receives a stage name and a
// 0-100 percent (or -1 when the size is unknown).
func (i *Installer) Install(ctx context.Context, tool string, progress func(stage string, percent int)) error {
	if runtime.GOOS != "windows" {
		return errors.New("managed installs are only published for Windows; set a tool path instead")
	}
	if progress == nil {
		progress = func(string, int) {}
	}
	switch tool {
	case TOOL_YTDLP:
		return i.install_ytdlp(ctx, progress)
	case TOOL_FFMPEG:
		return i.install_ffmpeg(ctx, progress)
	}
	return fmt.Errorf("unknown tool %q", tool)
}

func (i *Installer) install_ytdlp(ctx context.Context, progress func(string, int)) error {
	expected, err := i.expected_checksum(ctx, i.ytdlp_base+YTDLP_SUMS, YTDLP_ASSET)
	if err != nil {
		return err
	}
	progress("downloading", 0)
	if err := os.MkdirAll(i.paths.Bin, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(i.paths.Bin, "yt-dlp-*.part")
	if err != nil {
		return err
	}
	tmp_name := tmp.Name()
	defer func() { _ = os.Remove(tmp_name) }()
	digest, err := i.download(ctx, i.ytdlp_base+YTDLP_ASSET, tmp, MAX_YTDLP_BYTES, func(p int) { progress("downloading", p) })
	closeErr := tmp.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err := verify_digest(digest, expected); err != nil {
		return err
	}
	progress("verifying", 100)
	if err := os.Rename(tmp_name, filepath.Join(i.paths.Bin, exe_name(TOOL_YTDLP))); err != nil {
		return err
	}
	progress("done", 100)
	slog.Info("mediadl: installed yt-dlp", "path", filepath.Join(i.paths.Bin, exe_name(TOOL_YTDLP)))
	return nil
}

func (i *Installer) install_ffmpeg(ctx context.Context, progress func(string, int)) error {
	expected, err := i.expected_checksum(ctx, i.ffmpeg_base+FFMPEG_SUMS, FFMPEG_ASSET)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(i.paths.Bin, 0o700); err != nil {
		return err
	}
	zip_file, err := os.CreateTemp(i.paths.Bin, "ffmpeg-*.zip.part")
	if err != nil {
		return err
	}
	zip_name := zip_file.Name()
	defer func() { _ = os.Remove(zip_name) }()
	progress("downloading", 0)
	digest, err := i.download(ctx, i.ffmpeg_base+FFMPEG_ASSET, zip_file, MAX_FFMPEG_ZIP_BYTES, func(p int) { progress("downloading", p) })
	closeErr := zip_file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err := verify_digest(digest, expected); err != nil {
		return err
	}
	progress("extracting", 100)
	dest := filepath.Join(i.paths.Bin, exe_name(TOOL_FFMPEG))
	if err := extract_ffmpeg_binary(zip_name, dest, i.paths.Bin); err != nil {
		return err
	}
	progress("done", 100)
	slog.Info("mediadl: installed ffmpeg", "path", dest)
	return nil
}

// expected_checksum fetches a checksum file and returns the digest for asset.
func (i *Installer) expected_checksum(ctx context.Context, sums_url string, asset string) (string, error) {
	var buf bytes.Buffer
	if _, err := i.download(ctx, sums_url, &buf, MAX_SUMS_BYTES, nil); err != nil {
		return "", fmt.Errorf("cannot fetch %s: %w", path.Base(sums_url), err)
	}
	sums := parse_checksums(buf.String())
	digest, ok := sums[asset]
	if !ok {
		return "", fmt.Errorf("%s has no entry for %s", path.Base(sums_url), asset)
	}
	return digest, nil
}

// download streams url into dst, hashing as it goes. It fails when the body is
// larger than max. The returned digest is the lowercase hex SHA-256.
func (i *Installer) download(ctx context.Context, rawURL string, dst io.Writer, max int64, progress func(int)) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" {
		return "", errors.New("download URL must be HTTPS")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "TDrive-media-download")
	resp, err := i.client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > max {
		return "", errors.New("download is larger than expected")
	}
	hasher := sha256.New()
	counter := &progress_writer{total: resp.ContentLength, report: progress}
	limited := io.LimitReader(resp.Body, max+1)
	written, err := io.Copy(io.MultiWriter(dst, hasher, counter), limited)
	if err != nil {
		return "", err
	}
	if written > max {
		return "", errors.New("download is larger than expected")
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// progress_writer counts bytes and reports a percentage when the total is known.
type progress_writer struct {
	total   int64
	written int64
	last    int
	report  func(int)
}

func (w *progress_writer) Write(p []byte) (int, error) {
	w.written += int64(len(p))
	if w.report == nil || w.total <= 0 {
		return len(p), nil
	}
	percent := int(w.written * 100 / w.total)
	if percent != w.last {
		w.last = percent
		w.report(percent)
	}
	return len(p), nil
}

// parse_checksums reads "<hex>  <name>" (sha256sum) and "<hex> *<name>" lines,
// keyed by the bare file name.
func parse_checksums(body string) map[string]string {
	sums := make(map[string]string)
	scanner := bufio.NewScanner(strings.NewReader(body))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		digest := strings.ToLower(fields[0])
		if len(digest) != sha256.Size*2 {
			continue
		}
		name := strings.TrimPrefix(strings.Join(fields[1:], " "), "*")
		sums[path.Base(strings.ReplaceAll(name, "\\", "/"))] = digest
	}
	return sums
}

// verify_digest compares two hex SHA-256 digests case-insensitively.
func verify_digest(actual string, expected string) error {
	if strings.EqualFold(actual, expected) {
		return nil
	}
	return errors.New("download failed checksum verification and was discarded")
}

// verify_file_sha256 hashes a file and compares it with expected.
func verify_file_sha256(file_path string, expected string) error {
	file, err := os.Open(file_path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return err
	}
	return verify_digest(hex.EncodeToString(hasher.Sum(nil)), expected)
}

// extract_ffmpeg_binary copies only bin/ffmpeg.exe out of the release archive.
// Other entries are never written, and the name check is on the base name, so a
// crafted path cannot escape the bin directory.
func extract_ffmpeg_binary(zip_path string, dest string, bin_dir string) error {
	reader, err := zip.OpenReader(zip_path)
	if err != nil {
		return err
	}
	defer func() { _ = reader.Close() }()
	for _, entry := range reader.File {
		name := strings.ReplaceAll(entry.Name, "\\", "/")
		if path.Base(name) != exe_name(TOOL_FFMPEG) || !strings.Contains(name, "/bin/") {
			continue
		}
		if entry.UncompressedSize64 > MAX_FFMPEG_BINARY_BYTES {
			return errors.New("ffmpeg binary in archive is larger than expected")
		}
		source, err := entry.Open()
		if err != nil {
			return err
		}
		tmp, err := os.CreateTemp(bin_dir, "ffmpeg-*.exe.part")
		if err != nil {
			_ = source.Close()
			return err
		}
		tmp_name := tmp.Name()
		_, copyErr := io.Copy(tmp, io.LimitReader(source, MAX_FFMPEG_BINARY_BYTES+1))
		_ = source.Close()
		closeErr := tmp.Close()
		if copyErr != nil || closeErr != nil {
			_ = os.Remove(tmp_name)
			return firstError(copyErr, closeErr)
		}
		if err := os.Rename(tmp_name, dest); err != nil {
			_ = os.Remove(tmp_name)
			return err
		}
		return nil
	}
	return errors.New("ffmpeg.exe was not found in the release archive")
}

func firstError(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}
