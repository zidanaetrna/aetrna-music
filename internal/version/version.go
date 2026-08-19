package version

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const AppVersion = "v2.1.6"

const GitHubRepo = "zidanaetrna/aetrna-music"
const GitHubReleasesURL = "https://api.github.com/repos/" + GitHubRepo + "/releases/latest"
const ReleasesPageURL = "https://github.com/" + GitHubRepo + "/releases"
const DefaultCheckInterval = 20 * time.Minute
const RequestTimeout = 8 * time.Second

type Info struct {
	Current      string    `json:"current"`
	Latest       string    `json:"latest,omitempty"`
	Outdated     bool      `json:"outdated"`
	CheckedAt    time.Time `json:"checkedAt,omitempty"`
	ChangelogURL string    `json:"changelogUrl,omitempty"`
	Error        string    `json:"error,omitempty"`
	CheckEnabled bool      `json:"checkEnabled"`
}

type cachedResult struct {
	latest string
	err    error
	when   time.Time
}

var (
	cacheMu sync.RWMutex
	cache   cachedResult
)

func getenvBool(key string, def bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

func CheckDisabled() bool {
	return getenvBool("DISABLE_VERSION_CHECK", false)
}

type semver struct {
	major, minor, patch int
	pre                 string
}

func ParseSemver(raw string) (semver, error) {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "v")
	s = strings.TrimPrefix(s, "V")
	pre := ""
	if idx := strings.IndexAny(s, "-+"); idx != -1 {
		if s[idx] == '-' {
			pre = s[idx+1:]
		}
		s = s[:idx]
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return semver{}, fmt.Errorf("invalid semver: %q", raw)
	}
	maj, err1 := strconv.Atoi(parts[0])
	min, err2 := strconv.Atoi(parts[1])
	pat, err3 := strconv.Atoi(parts[2])
	if err1 != nil || err2 != nil || err3 != nil {
		return semver{}, fmt.Errorf("invalid numeric version: %q", raw)
	}
	return semver{major: maj, minor: min, patch: pat, pre: pre}, nil
}

func CompareSemver(a, b string) (int, error) {
	sa, errA := ParseSemver(a)
	sb, errB := ParseSemver(b)
	if errA != nil {
		return 0, errA
	}
	if errB != nil {
		return 0, errB
	}
	switch {
	case sa.major < sb.major:
		return -1, nil
	case sa.major > sb.major:
		return 1, nil
	case sa.minor < sb.minor:
		return -1, nil
	case sa.minor > sb.minor:
		return 1, nil
	case sa.patch < sb.patch:
		return -1, nil
	case sa.patch > sb.patch:
		return 1, nil
	}
	if sa.pre == "" && sb.pre != "" {
		return 1, nil
	}
	if sa.pre != "" && sb.pre == "" {
		return -1, nil
	}
	return 0, nil
}

type ghRelease struct {
	TagName     string `json:"tag_name"`
	Prerelease  bool   `json:"prerelease"`
	Draft       bool   `json:"draft"`
	HTMLURL     string `json:"html_url"`
	PublishedAt string `json:"published_at"`
}

func GetRemoteLatest(ctx context.Context) (tag string, changelogURL string, err error) {
	if CheckDisabled() {
		return AppVersion, ReleasesPageURL, nil
	}

	cacheMu.RLock()
	if !cache.when.IsZero() && time.Since(cache.when) < DefaultCheckInterval {
		tag = cache.latest
		err = cache.err
		cacheMu.RUnlock()
		if tag != "" {
			return tag, changelogURLFor(tag), err
		}
		return tag, ReleasesPageURL, err
	}
	cacheMu.RUnlock()

	latest, url, e := fetchRemoteLatest(ctx)

	cacheMu.Lock()
	cache = cachedResult{latest: latest, err: e, when: time.Now()}
	cacheMu.Unlock()

	if latest != "" {
		return latest, url, e
	}
	return AppVersion, url, e
}

func fetchRemoteLatest(ctx context.Context) (tag, url string, err error) {
	ctx2, cancel := context.WithTimeout(ctx, RequestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx2, http.MethodGet, GitHubReleasesURL, nil)
	if err != nil {
		return "", ReleasesPageURL, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "aetrna-music/"+AppVersion)

	if t := os.Getenv("GITHUB_TOKEN"); strings.TrimSpace(t) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(t))
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", ReleasesPageURL, fmt.Errorf("network: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return AppVersion, ReleasesPageURL, nil
	}
	if resp.StatusCode >= 400 {
		return "", ReleasesPageURL, fmt.Errorf("github http %d", resp.StatusCode)
	}

	var rel ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", ReleasesPageURL, fmt.Errorf("decode: %w", err)
	}
	if rel.Draft {
		return AppVersion, ReleasesPageURL, nil
	}
	if rel.Prerelease && !getenvBool("VERSION_CHECK_PRERELEASE", false) {
		return AppVersion, ReleasesPageURL, nil
	}
	tag = strings.TrimSpace(rel.TagName)
	if tag == "" {
		return AppVersion, ReleasesPageURL, nil
	}
	if rel.HTMLURL != "" {
		url = rel.HTMLURL
	} else {
		url = changelogURLFor(tag)
	}
	return tag, url, nil
}

func changelogURLFor(tag string) string {
	return ReleasesPageURL + "/tag/" + tag
}

func StartBackgroundChecker(ctx context.Context) {
	if CheckDisabled() {
		log.Printf("[INFO] [VersionCheck] Version checking is disabled via DISABLE_VERSION_CHECK.")
		return
	}

	go func() {
		time.Sleep(2 * time.Second)
		_ = GetInfo(ctx)
	}()

	go func() {
		ticker := time.NewTicker(DefaultCheckInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = GetInfo(ctx)
			}
		}
	}()
}

func GetInfo(ctx context.Context) Info {
	info := Info{
		Current:      AppVersion,
		ChangelogURL: ReleasesPageURL,
		CheckEnabled: !CheckDisabled(),
	}
	if !info.CheckEnabled {
		return info
	}
	latest, url, err := GetRemoteLatest(ctx)
	info.CheckedAt = time.Now()
	info.Latest = latest
	info.ChangelogURL = url
	if err != nil {
		info.Error = err.Error()
		log.Printf("[WARN] [VersionCheck] Remote latest lookup failed: %v", err)
		return info
	}
	cmp, cErr := CompareSemver(info.Current, latest)
	if cErr != nil {
		info.Error = fmt.Sprintf("version parse: %v", cErr)
		return info
	}
	info.Outdated = cmp < 0
	if info.Outdated {
		log.Printf("[WARN] [VersionCheck] This instance: %s — latest published release: %s. Upgrade recommended: %s", info.Current, latest, url)
	} else {
		log.Printf("[INFO] [VersionCheck] Running %s. Latest published release: %s.", info.Current, latest)
	}
	return info
}
