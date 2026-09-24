package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// applyUpdate extracts the tricount binary from the archive and replaces exe.
func applyUpdate(archive []byte, archiveName, exe string) error {
	name := binaryName()
	var binary []byte
	var err error
	if strings.HasSuffix(archiveName, ".zip") {
		binary, err = extractZipEntry(archive, name)
	} else {
		binary, err = extractTarGzEntry(archive, name)
	}
	if err != nil {
		return err
	}
	if len(binary) == 0 {
		return fmt.Errorf("archive did not contain %s", name)
	}

	if exe == "" {
		exe, err = currentExecutable()
		if err != nil {
			return err
		}
	}

	dir := filepath.Dir(exe)
	tmp, err := os.CreateTemp(dir, ".tricount-update-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file next to %s: %w", exe, err)
	}
	tmpPath := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpPath) }

	if _, err := tmp.Write(binary); err != nil {
		tmp.Close()
		cleanup()
		return fmt.Errorf("failed to write new binary: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Chmod(tmpPath, 0o755); err != nil {
		cleanup()
		return err
	}

	if runtime.GOOS == "windows" {
		old := exe + ".old"
		_ = os.Remove(old)
		if err := os.Rename(exe, old); err != nil {
			cleanup()
			return fmt.Errorf("failed to move existing binary aside: %w", err)
		}
		if err := os.Rename(tmpPath, exe); err != nil {
			_ = os.Rename(old, exe)
			cleanup()
			return fmt.Errorf("failed to install new binary: %w", err)
		}
		return nil
	}

	if err := os.Rename(tmpPath, exe); err != nil {
		cleanup()
		return fmt.Errorf("failed to install new binary at %s: %w", exe, err)
	}
	return nil
}

func currentExecutable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to resolve current executable: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return exe, nil
}

func extractTarGzEntry(data []byte, entryName string) ([]byte, error) {
	gzr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("invalid gzip: %w", err)
	}
	defer gzr.Close()
	tr := tar.NewReader(gzr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil, fmt.Errorf("entry %s not found in archive", entryName)
		}
		if err != nil {
			return nil, err
		}
		if filepath.Base(hdr.Name) != entryName || hdr.Typeflag != tar.TypeReg {
			continue
		}
		return io.ReadAll(tr)
	}
}

func extractZipEntry(data []byte, entryName string) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("invalid zip: %w", err)
	}
	for _, f := range zr.File {
		if filepath.Base(f.Name) != entryName || f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		body, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, err
		}
		return body, nil
	}
	return nil, fmt.Errorf("entry %s not found in archive", entryName)
}
