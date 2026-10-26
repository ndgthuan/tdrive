package updater

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Asset is one downloadable file attached to a release.
type Asset struct {
	Name string
	Size int64
	URL  string
}

// Release is the newest published (non-draft, non-prerelease) release.
type Release struct {
	Tag         string
	Version     Version
	PageURL     string
	PublishedAt time.Time
	Assets      []Asset
}

func (r Release) asset(name string) (Asset, bool) {
	for _, a := range r.Assets {
		if a.Name == name {
			return a, true
		}
	}
	return Asset{}, false
}

// Source resolves the newest release. The GitHub implementation is the only
// production source; tests substitute their own.
type Source interface {
	LatestRelease(ctx context.Context) (Release, error)
}

// ErrNoRelease means the repository has no published release yet.
var ErrNoRelease = errors.New("no published release")

// ErrRateLimited means GitHub refused the unauthenticated request for now.
var ErrRateLimited = errors.New("github rate limit reached")

type rateLimitError struct{ retryAfter time.Duration }

func (e *rateLimitError) Error() string { return ErrRateLimited.Error() }
func (e *rateLimitError) Unwrap() error { return ErrRateLimited }

const (
	githubAPIBase      = "https://api.github.com"
	githubMaxBodyBytes = 1 << 20
	githubTimeout      = 15 * time.Second
)

var releaseTagPattern = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+$`)

// GitHubSource reads /repos/{repo}/releases/latest, which by definition skips
// drafts and pre-releases — publishing the draft stays the release gate.
type GitHubSource struct {
	Repo       string // "owner/name"
	UserAgent  string
	Client     *http.Client
	BaseURL    string // overridable for tests
	WebBaseURL string // overridable for tests; production uses github.com
}

// NewGitHubSource builds a source for repo. client may be nil.
func NewGitHubSource(repo, userAgent string, client *http.Client) *GitHubSource {
	if client == nil {
		client = &http.Client{}
	}
	return &GitHubSource{Repo: repo, UserAgent: userAgent, Client: client}
}

type githubRelease struct {
	TagName     string `json:"tag_name"`
	HTMLURL     string `json:"html_url"`
	PublishedAt string `json:"published_at"`
	Draft       bool   `json:"draft"`
	Prerelease  bool   `json:"prerelease"`
	Assets      []struct {
		Name               string `json:"name"`
		Size               int64  `json:"size"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// LatestRelease fetches and validates the newest release.
func (g *GitHubSource) LatestRelease(ctx context.Context) (Release, error) {
	base := g.BaseURL
	if base == "" {
		base = githubAPIBase
	}
	ctx, cancel := context.WithTimeout(ctx, githubTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/repos/"+g.Repo+"/releases/latest", nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if g.UserAgent != "" {
		req.Header.Set("User-Agent", g.UserAgent)
	}

	resp, err := g.Client.Do(req)
	if err != nil {
		return Release{}, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return Release{}, ErrNoRelease
	case http.StatusForbidden, http.StatusTooManyRequests:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		limited, retryAfter := githubRateLimit(resp, body, time.Now())
		if limited {
			// Published release assets are served separately from the API quota.
			if release, err := g.latestMetadata(ctx); err == nil {
				return release, nil
			}
			// Older releases may predate release.json. The stable latest page
			// redirects to its published tag without consuming the REST API
			// quota; this is enough to report that an already-newer local build
			// is current, and lets newer legacy releases remain visible manually.
			if release, err := g.latestReleasePage(ctx); err == nil {
				return release, nil
			}
			return Release{}, &rateLimitError{retryAfter: retryAfter}
		}
		return Release{}, fmt.Errorf("github responded with status %d", resp.StatusCode)
	default:
		return Release{}, fmt.Errorf("github responded with status %d", resp.StatusCode)
	}

	var payload githubRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, githubMaxBodyBytes)).Decode(&payload); err != nil {
		return Release{}, fmt.Errorf("decode github release: %w", err)
	}
	return parseGitHubRelease(payload)
}

func githubRateLimit(resp *http.Response, body []byte, now time.Time) (bool, time.Duration) {
	if resp == nil {
		return false, 0
	}
	retryAfter := time.Duration(0)
	rawRetryAfter := strings.TrimSpace(resp.Header.Get("Retry-After"))
	if seconds, err := strconv.ParseInt(rawRetryAfter, 10, 64); err == nil && seconds > 0 {
		retryAfter = time.Duration(seconds) * time.Second
	} else if retryAt, err := http.ParseTime(rawRetryAfter); err == nil && retryAt.After(now) {
		retryAfter = retryAt.Sub(now)
	}
	if reset, err := strconv.ParseInt(resp.Header.Get("X-RateLimit-Reset"), 10, 64); err == nil && reset > 0 {
		if untilReset := time.Unix(reset, 0).Sub(now); untilReset > retryAfter {
			retryAfter = untilReset
		}
	}
	message := strings.ToLower(string(body))
	limited := resp.StatusCode == http.StatusTooManyRequests ||
		resp.Header.Get("X-RateLimit-Remaining") == "0" || retryAfter > 0 ||
		strings.Contains(message, "rate limit") || strings.Contains(message, "rate-limit")
	return limited, retryAfter
}

func (g *GitHubSource) latestReleasePage(ctx context.Context) (Release, error) {
	base := g.WebBaseURL
	if base == "" {
		base = "https://github.com"
	}
	parsedBase, err := url.Parse(base)
	if err != nil || parsedBase.Scheme != "https" && parsedBase.Scheme != "http" || parsedBase.Host == "" {
		return Release{}, errors.New("invalid GitHub web base URL")
	}
	ctx, cancel := context.WithTimeout(ctx, githubTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+"/"+g.Repo+"/releases/latest", nil)
	if err != nil {
		return Release{}, err
	}
	if g.UserAgent != "" {
		req.Header.Set("User-Agent", g.UserAgent)
	}
	resp, err := g.Client.Do(req)
	if err != nil {
		return Release{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("latest release page status %d", resp.StatusCode)
	}
	finalURL := resp.Request.URL
	if finalURL == nil || finalURL.Scheme != parsedBase.Scheme || !strings.EqualFold(finalURL.Host, parsedBase.Host) {
		return Release{}, errors.New("latest release redirect left GitHub")
	}
	parts := strings.Split(strings.Trim(finalURL.Path, "/"), "/")
	owner, repoName, ok := strings.Cut(g.Repo, "/")
	if !ok || strings.Contains(repoName, "/") || len(parts) != 5 || parts[0] != owner || parts[1] != repoName || parts[2] != "releases" || parts[3] != "tag" {
		return Release{}, errors.New("latest release redirect did not resolve to a release tag")
	}
	tag := parts[4]
	if !releaseTagPattern.MatchString(tag) {
		return Release{}, errors.New("latest release redirect has an invalid tag")
	}
	version, err := ParseVersion(tag)
	if err != nil {
		return Release{}, err
	}
	pageURL := "https://github.com/" + g.Repo + "/releases/tag/" + tag
	root := "https://github.com/" + g.Repo + "/releases/download/" + tag + "/"
	names := []string{
		"TDrive-" + tag + "-macos-arm64.zip",
		"TDrive-" + tag + "-windows-amd64.zip",
		"TDrive-" + tag + "-x86_64.AppImage",
		"TDrive-" + tag + "-linux-amd64.AppImage",
		checksumsAssetName,
		checksumsSignatureAssetName,
	}
	assets := make([]Asset, 0, len(names))
	for _, name := range names {
		assets = append(assets, Asset{Name: name, URL: root + name})
	}
	return Release{Tag: tag, Version: version, PageURL: pageURL, Assets: assets}, nil
}

// latestMetadata uses the published stable release redirect, so a draft remains
// invisible. Payload installation still requires the signed checksum manifest.
func (g *GitHubSource) latestMetadata(ctx context.Context) (Release, error) {
	base := g.WebBaseURL
	if base == "" {
		base = "https://github.com"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/"+g.Repo+"/releases/latest/download/release.json", nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("User-Agent", g.UserAgent)
	resp, err := g.Client.Do(req)
	if err != nil {
		return Release{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("release metadata status %d", resp.StatusCode)
	}
	var payload githubRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, githubMaxBodyBytes)).Decode(&payload); err != nil {
		return Release{}, err
	}
	release, err := parseGitHubRelease(payload)
	if err != nil {
		return Release{}, err
	}
	root := "https://github.com/" + g.Repo + "/releases/"
	if release.PageURL != root+"tag/"+release.Tag {
		return Release{}, errors.New("release metadata repository mismatch")
	}
	for _, asset := range release.Assets {
		if strings.ContainsAny(asset.Name, "/\\?#") || asset.URL != root+"download/"+release.Tag+"/"+asset.Name ||
			(asset.Size <= 0 && asset.Name != "checksums.txt" && asset.Name != "checksums.txt.sig") {
			return Release{}, errors.New("invalid release metadata asset")
		}
	}
	return release, nil
}

func parseGitHubRelease(payload githubRelease) (Release, error) {
	if payload.Draft || payload.Prerelease {
		return Release{}, ErrNoRelease
	}
	version, err := ParseVersion(payload.TagName)
	if err != nil {
		return Release{}, fmt.Errorf("release tag %q is not a version", payload.TagName)
	}
	if !strings.HasPrefix(payload.HTMLURL, "https://github.com/") {
		return Release{}, fmt.Errorf("release page %q is not on github.com", payload.HTMLURL)
	}

	release := Release{
		Tag:     payload.TagName,
		Version: version,
		PageURL: payload.HTMLURL,
	}
	if payload.PublishedAt != "" {
		if t, err := time.Parse(time.RFC3339, payload.PublishedAt); err == nil {
			release.PublishedAt = t
		}
	}
	for _, a := range payload.Assets {
		if a.Name == "" || !strings.HasPrefix(a.BrowserDownloadURL, "https://") {
			continue
		}
		release.Assets = append(release.Assets, Asset{Name: a.Name, Size: a.Size, URL: a.BrowserDownloadURL})
	}
	return release, nil
}
