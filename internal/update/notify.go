package update

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	notifyCheckEvery = 24 * time.Hour
	notifyHTTPTO     = 10 * time.Second
	// EnvNoUpdateNotifier disables the background update check when set to any non-empty value.
	EnvNoUpdateNotifier = "TRICOUNT_NO_UPDATE_NOTIFIER"
)

// NotifyState is the JSON structure persisted to the state file between runs.
type NotifyState struct {
	CheckedForUpdateAt time.Time `json:"checked_for_update_at"`
	LatestVersion      string    `json:"latest_version"`
}

// CheckForUpdateInBackground starts a background goroutine that fetches the
// latest release at most once every 24 hours and may print a notice to stderr.
//
// Rules:
//   - Never blocks the caller: the network request runs in a goroutine.
//   - Always prints to stderr (never stdout).
//   - Skipped entirely when TRICOUNT_NO_UPDATE_NOTIFIER is set.
//   - Dev and dirty builds are skipped.
//
// The caller must call wg.Wait() before the process exits so the goroutine has
// a chance to finish and the notice is written after all command output.
// The notice string (if any) is stored in noticeOut and printed by the caller
// after wg.Wait() to guarantee it appears after the main command output.
func CheckForUpdateInBackground(currentVersion string, wg *sync.WaitGroup, noticeOut *string) {
	if os.Getenv(EnvNoUpdateNotifier) != "" {
		return
	}
	if !isParseableSemver(currentVersion) {
		return
	}

	state, statePath, _ := loadNotifyState()

	if time.Since(state.CheckedForUpdateAt) < notifyCheckEvery {
		if msg := buildNotice(currentVersion, state.LatestVersion); msg != "" {
			*noticeOut = msg
		}
		return
	}

	wg.Add(1)
	go func() {
		defer wg.Done()

		rel, err := fetchLatestRelease(context.Background())
		if err != nil {
			return
		}

		latest := normalizeTag(rel.TagName)
		newState := NotifyState{
			CheckedForUpdateAt: time.Now().UTC(),
			LatestVersion:      latest,
		}
		_ = saveNotifyState(statePath, newState)

		if msg := buildNotice(currentVersion, latest); msg != "" {
			*noticeOut = msg
		}
	}()
}

// buildNotice returns a non-empty notice string when latest is newer than current.
//
// Two tiers:
//   - Patch update only (< 0.1.0 ahead): soft informational line.
//   - Minor or major update (>= 0.1.0 ahead): stronger outdated warning.
func buildNotice(current, latest string) string {
	if latest == "" || !isParseableSemver(current) || !isParseableSemver(latest) {
		return ""
	}
	if isPrerelease(latest) {
		return ""
	}
	if compareSemver(latest, current) <= 0 {
		return ""
	}
	cv, _ := parseSemver(current)
	lv, _ := parseSemver(latest)
	minorAhead := lv[0] > cv[0] || (lv[0] == cv[0] && lv[1] > cv[1])
	if minorAhead {
		return fmt.Sprintf(
			"\nYou are running an outdated version of tricount (%s). The latest release is %s.\nRun `tricount update` to upgrade.\n\n",
			normalizeTag(current),
			latest,
		)
	}
	return fmt.Sprintf(
		"\nA new version of tricount (%s) is available. Please update for an improved experience.\n\n",
		latest,
	)
}

func fetchLatestRelease(ctx context.Context) (*githubRelease, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, updateAPIBase+"/releases/latest", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "tricount-cli-notifier")

	client := &http.Client{Timeout: notifyHTTPTO}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("github api returned %s", resp.Status)
	}

	var rel githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, err
	}
	return &rel, nil
}

func stateFilePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "tricount", "state.json"), nil
}

func loadNotifyState() (NotifyState, string, error) {
	path, err := stateFilePath()
	if err != nil {
		return NotifyState{}, "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return NotifyState{}, path, nil
	}
	var s NotifyState
	if err := json.Unmarshal(data, &s); err != nil {
		return NotifyState{}, path, nil
	}
	return s, path, nil
}

func saveNotifyState(path string, s NotifyState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
