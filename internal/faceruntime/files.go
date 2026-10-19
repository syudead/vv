package faceruntime

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/syudead/vv/internal/domain"
)

// fileState is what verification found for one item.
type fileState int

const (
	fileAbsent fileState = iota
	fileOK
	// fileBad exists but does not match its pin.
	fileBad
)

// manifestName lists, inside the runtime folder, the archive hash and each extracted file's hash, so startup can
// verify the unpacked runtime without keeping the archive.
const manifestName = ".sha256"

// failure is a download or start error with the reason the settings show.
type failure struct {
	reason domain.FaceRuntimeFailure
	err    error
}

func (f *failure) Error() string { return string(f.reason) + ": " + f.err.Error() }
func (f *failure) Unwrap() error { return f.err }

func fail(reason domain.FaceRuntimeFailure, err error) error {
	return &failure{reason: reason, err: err}
}

// reasonOf maps an error to its reason; an unclassified error is a disk error, since only file work is left.
func reasonOf(err error) domain.FaceRuntimeFailure {
	var f *failure
	if errors.As(err, &f) {
		return f.reason
	}
	return domain.FaceRuntimeDisk
}

func hashFile(name string) (string, error) {
	file, err := os.Open(name)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// verifyModel compares a model file with its pin; it creates nothing.
func verifyModel(name, sha string) fileState {
	got, err := hashFile(name)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return fileAbsent
	case err != nil || got != sha:
		return fileBad
	}
	return fileOK
}

// verifyRuntime checks the unpacked runtime against its manifest, whose first line names the archive's hash.
func verifyRuntime(dir, archiveSHA string) fileState {
	file, err := os.Open(filepath.Join(dir, manifestName))
	if errors.Is(err, fs.ErrNotExist) {
		if _, statErr := os.Stat(dir); errors.Is(statErr, fs.ErrNotExist) {
			return fileAbsent
		}
		return fileBad
	}
	if err != nil {
		return fileBad
	}
	defer func() { _ = file.Close() }()
	lines := bufio.NewScanner(file)
	if !lines.Scan() || lines.Text() != "archive "+archiveSHA {
		return fileBad
	}
	for lines.Scan() {
		sha, rel, ok := strings.Cut(lines.Text(), "  ")
		if !ok || !filepath.IsLocal(rel) {
			return fileBad
		}
		if got, err := hashFile(filepath.Join(dir, filepath.FromSlash(rel))); err != nil || got != sha {
			return fileBad
		}
	}
	if lines.Err() != nil {
		return fileBad
	}
	return fileOK
}

// fetch downloads pin into target, adding each received byte to progress, and fails with hash_mismatch when the
// content differs from the pin. The caller removes target on failure.
func fetch(ctx context.Context, client *http.Client, pin Pin, target string, progress func(int64)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pin.URL, nil)
	if err != nil {
		return fail(domain.FaceRuntimeNetwork, err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fail(domain.FaceRuntimeNetwork, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fail(domain.FaceRuntimeNetwork, fmt.Errorf("GET %s: %s", pin.URL, resp.Status))
	}
	file, err := os.Create(target)
	if err != nil {
		return fail(domain.FaceRuntimeDisk, err)
	}
	h := sha256.New()
	// Read one byte past the pinned size, so a longer file is caught without reading all of it.
	_, copyErr := io.Copy(io.MultiWriter(diskWriter{file}, h, progressWriter(progress)), io.LimitReader(resp.Body, pin.Size+1))
	closeErr := file.Close()
	if copyErr != nil {
		if reasonOf(copyErr) == domain.FaceRuntimeDisk {
			return copyErr
		}
		return fail(domain.FaceRuntimeNetwork, copyErr)
	}
	if closeErr != nil {
		return fail(domain.FaceRuntimeDisk, closeErr)
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != pin.SHA256 {
		return fail(domain.FaceRuntimeHashMismatch, fmt.Errorf("%s has SHA-256 %s, want %s", pin.URL, got, pin.SHA256))
	}
	return nil
}

// diskWriter marks write errors as disk errors, so fetch can tell them from a broken connection.
type diskWriter struct{ w io.Writer }

func (d diskWriter) Write(p []byte) (int, error) {
	n, err := d.w.Write(p)
	if err != nil {
		err = fail(domain.FaceRuntimeDisk, err)
	}
	return n, err
}

type progressWriter func(int64)

func (p progressWriter) Write(b []byte) (int, error) {
	p(int64(len(b)))
	return len(b), nil
}

// unpack extracts the archive's root folder root into dir and writes the manifest; dir must not exist.
func unpack(archive, root, dir, archiveSHA string) error {
	var manifest strings.Builder
	manifest.WriteString("archive " + archiveSHA + "\n")
	write := func(name string, mode fs.FileMode, r io.Reader) error {
		inner, ok := strings.CutPrefix(name, root+"/")
		if !ok || inner == "" || !filepath.IsLocal(filepath.FromSlash(inner)) {
			return fmt.Errorf("archive entry %q is outside %s", name, root)
		}
		sha, err := writeExtracted(filepath.Join(dir, filepath.FromSlash(inner)), mode, r)
		if err != nil {
			return err
		}
		manifest.WriteString(sha + "  " + inner + "\n")
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := eachArchiveFile(archive, write); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, manifestName), []byte(manifest.String()), 0o644)
}

// eachArchiveFile passes each regular file of a .zip or .tar.gz archive to extract.
func eachArchiveFile(archive string, extract extractFunc) error {
	file, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	if strings.HasSuffix(archive, ".zip") {
		info, err := file.Stat()
		if err != nil {
			return err
		}
		return eachZipFile(file, info.Size(), extract)
	}
	return eachTarFile(file, extract)
}

func writeExtracted(target string, mode fs.FileMode, r io.Reader) (string, error) {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", err
	}
	perm := fs.FileMode(0o644)
	if mode&0o111 != 0 {
		perm = 0o755
	}
	out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, perm)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	_, err = io.Copy(io.MultiWriter(out, h), r)
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	return hex.EncodeToString(h.Sum(nil)), err
}

type extractFunc func(name string, mode fs.FileMode, r io.Reader) error

func eachTarFile(r io.Reader, extract extractFunc) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	entries := tar.NewReader(gz)
	for {
		header, err := entries.Next()
		switch {
		case errors.Is(err, io.EOF):
			return nil
		case err != nil:
			return err
		case header.Typeflag == tar.TypeReg:
			if err := extract(path.Clean(header.Name), header.FileInfo().Mode(), entries); err != nil {
				return err
			}
		}
	}
}

func eachZipFile(r io.ReaderAt, size int64, extract extractFunc) error {
	files, err := zip.NewReader(r, size)
	if err != nil {
		return err
	}
	for _, f := range files.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if err := extractZipEntry(f, extract); err != nil {
			return err
		}
	}
	return nil
}

func extractZipEntry(f *zip.File, extract extractFunc) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer func() { _ = rc.Close() }()
	return extract(path.Clean(f.Name), f.Mode(), rc)
}
