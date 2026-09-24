package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
)

const (
	updateRepoOwner = "agentzhao"
	updateRepoName  = "tricount-cli"
	updateAPIBase   = "https://api.github.com/repos/" + updateRepoOwner + "/" + updateRepoName
)

// githubRelease models the subset of the GitHub release JSON we care about.
type githubRelease struct {
	TagName    string `json:"tag_name"`
	Name       string `json:"name"`
	Body       string `json:"body"`
	HTMLURL    string `json:"html_url"`
	Prerelease bool   `json:"prerelease"`
	Assets     []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
		Size               int64  `json:"size"`
	} `json:"assets"`
}

// ReleaseNote is one GitHub release between the running version and the target.
type ReleaseNote struct {
	Tag   string `json:"tag"`
	Title string `json:"title,omitempty"`
	URL   string `json:"url,omitempty"`
	Body  string `json:"body,omitempty"`
}

func notesFrom(rel githubRelease) ReleaseNote {
	title := strings.TrimSpace(rel.Name)
	tag := normalizeTag(rel.TagName)
	if title == tag {
		title = ""
	}
	return ReleaseNote{
		Tag:   tag,
		Title: title,
		URL:   rel.HTMLURL,
		Body:  strings.TrimSpace(rel.Body),
	}
}

func fetchChangelogBetween(ctx context.Context, currentTag, targetTag string, targetRelease *githubRelease, cmp int) ([]githubRelease, error) {
	if cmp == 0 || !isParseableSemver(currentTag) || !isParseableSemver(targetTag) {
		return []githubRelease{*targetRelease}, nil
	}

	releases, err := fetchReleases(ctx)
	if err != nil {
		return []githubRelease{*targetRelease}, err
	}

	filtered := filterReleaseNotesBetween(releases, currentTag, targetTag, cmp)
	if len(filtered) == 0 {
		return []githubRelease{*targetRelease}, nil
	}
	if !containsReleaseTag(filtered, targetTag) {
		filtered = append(filtered, *targetRelease)
		sortReleaseNotes(filtered, cmp)
	}
	return filtered, nil
}

// fetchRelease talks to the GitHub API to resolve either the latest release
// (when tag is empty) or a specific release by tag.
func fetchRelease(ctx context.Context, tag string) (*githubRelease, error) {
	var url string
	if tag == "" {
		url = updateAPIBase + "/releases/latest"
	} else {
		url = updateAPIBase + "/releases/tags/" + normalizeTag(tag)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "tricount-cli-updater")

	client := &http.Client{Timeout: updateHTTPTO}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to query GitHub: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		if tag == "" {
			return nil, fmt.Errorf("no releases found for %s/%s", updateRepoOwner, updateRepoName)
		}
		return nil, fmt.Errorf("release %s not found", normalizeTag(tag))
	}
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("GitHub API returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var rel githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("failed to decode release metadata: %w", err)
	}
	return &rel, nil
}

func fetchReleases(ctx context.Context) ([]githubRelease, error) {
	client := &http.Client{Timeout: updateHTTPTO}
	var releases []githubRelease
	for page := 1; page <= 10; page++ {
		url := fmt.Sprintf("%s/releases?per_page=100&page=%d", updateAPIBase, page)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("User-Agent", "tricount-cli-updater")

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to query GitHub releases: %w", err)
		}
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		if resp.StatusCode/100 != 2 {
			return nil, fmt.Errorf("GitHub API returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
		}

		var pageReleases []githubRelease
		if err := json.Unmarshal(body, &pageReleases); err != nil {
			return nil, fmt.Errorf("failed to decode release list: %w", err)
		}
		if len(pageReleases) == 0 {
			break
		}
		releases = append(releases, pageReleases...)
		if len(pageReleases) < 100 {
			break
		}
	}
	return releases, nil
}

func filterReleaseNotesBetween(releases []githubRelease, currentTag, targetTag string, cmp int) []githubRelease {
	includePrereleases := isPrerelease(targetTag)
	var out []githubRelease
	for _, rel := range releases {
		tag := normalizeTag(rel.TagName)
		if !isParseableSemver(tag) {
			continue
		}
		if !includePrereleases && isPrereleaseRelease(rel) {
			continue
		}

		relToCurrent := compareSemver(tag, currentTag)
		relToTarget := compareSemver(tag, targetTag)
		if cmp < 0 && relToCurrent > 0 && relToTarget <= 0 {
			out = append(out, rel)
		}
		if cmp > 0 && relToTarget >= 0 && relToCurrent < 0 {
			out = append(out, rel)
		}
	}

	sortReleaseNotes(out, cmp)
	return out
}

func isPrereleaseRelease(rel githubRelease) bool {
	return rel.Prerelease || isPrerelease(rel.TagName)
}

func containsReleaseTag(releases []githubRelease, tag string) bool {
	tag = normalizeTag(tag)
	for _, rel := range releases {
		if normalizeTag(rel.TagName) == tag {
			return true
		}
	}
	return false
}

func sortReleaseNotes(releases []githubRelease, cmp int) {
	sort.SliceStable(releases, func(i, j int) bool {
		cmpSem := compareSemver(releases[i].TagName, releases[j].TagName)
		if cmpSem == 0 {
			left := normalizeTag(releases[i].TagName)
			right := normalizeTag(releases[j].TagName)
			if cmp < 0 {
				return left < right
			}
			return left > right
		}
		if cmp < 0 {
			return cmpSem < 0
		}
		return cmpSem > 0
	})
}
