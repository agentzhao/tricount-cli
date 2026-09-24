package update

import (
	"context"
	"fmt"
	"io"
)

// Outcome is the result of planning or applying a CLI update.
// Asset URLs stay unexported so JSON views do not repeat download links.
type Outcome struct {
	Current     string
	Target      string
	Action      string
	Applied     bool
	Executable  string
	Asset       string
	ReleaseURL  string
	Notes       []ReleaseNote
	NotesError  string
	Requested   string
	archiveURL  string
	archiveSize int64
	checksumURL string
}

// Plan resolves the GitHub release and the asset for this OS and architecture.
// It does not download or replace the binary.
func Plan(ctx context.Context, targetVersion, currentVersion string) (Outcome, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	release, err := fetchRelease(ctx, targetVersion)
	if err != nil {
		return Outcome{}, err
	}

	currentTag := normalizeTag(currentVersion)
	targetTag := normalizeTag(release.TagName)
	action := Action(currentTag, targetTag)
	exe, _ := currentExecutable()
	out := Outcome{
		Current:    DisplayVersion(currentVersion),
		Target:     targetTag,
		Action:     action,
		Executable: exe,
		ReleaseURL: release.HTMLURL,
		Requested:  targetVersion,
	}
	if action == "none" {
		return out, nil
	}

	archiveName, err := expectedArchiveName()
	if err != nil {
		return Outcome{}, err
	}
	var archiveURL string
	var archiveSize int64
	for _, a := range release.Assets {
		if a.Name == archiveName {
			archiveURL = a.BrowserDownloadURL
			archiveSize = a.Size
			break
		}
	}
	if archiveURL == "" {
		return Outcome{}, fmt.Errorf("release %s has no asset matching %s", targetTag, archiveName)
	}
	checksumURL := checksumAssetURL(release)
	if checksumURL == "" {
		return Outcome{}, fmt.Errorf("release %s has no checksums file", targetTag)
	}

	cmp := compareSemver(currentTag, targetTag)
	notes, notesErr := fetchChangelogBetween(ctx, currentTag, targetTag, release, cmp)
	out.Asset = archiveName
	out.archiveURL = archiveURL
	out.archiveSize = archiveSize
	out.checksumURL = checksumURL
	out.Notes = make([]ReleaseNote, 0, len(notes))
	for _, rel := range notes {
		out.Notes = append(out.Notes, notesFrom(rel))
	}
	if notesErr != nil {
		out.NotesError = notesErr.Error()
	}
	return out, nil
}

// Apply downloads the planned release, checks its checksum, and replaces the
// running binary. progress receives the download bar; nil discards it.
func Apply(ctx context.Context, progress io.Writer, plan Outcome) (Outcome, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if plan.Action == "none" {
		return plan, nil
	}
	if plan.archiveURL == "" || plan.checksumURL == "" || plan.Asset == "" {
		return Outcome{}, fmt.Errorf("update plan has no archive to install")
	}
	if progress == nil {
		progress = io.Discard
	}

	fmt.Fprintf(progress, "> Downloading %s ", plan.Target)
	archiveBytes, err := downloadWithProgress(ctx, progress, plan.archiveURL, plan.archiveSize)
	if err != nil {
		return Outcome{}, fmt.Errorf("download failed: %w", err)
	}

	fmt.Fprint(progress, "> Verifying checksum... ")
	if err := verifyChecksum(ctx, archiveBytes, plan.Asset, plan.checksumURL); err != nil {
		fmt.Fprintln(progress, "FAILED")
		return Outcome{}, err
	}
	fmt.Fprintln(progress, "Done.")

	fmt.Fprint(progress, "> Applying update... ")
	if err := applyUpdate(archiveBytes, plan.Asset, plan.Executable); err != nil {
		fmt.Fprintln(progress, "FAILED")
		return Outcome{}, err
	}
	fmt.Fprintln(progress, "Done.")

	plan.Applied = true
	return plan, nil
}
