package faceruntime

import (
	"archive/tar"
	"archive/zip"
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
	"slices"
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

// verifyRuntime checks that dir holds exactly the pinned files, each with its pinned hash, and that each of
// executables has an execute bit; an added file, a link or a missing file makes it bad, since the helper loads
// libraries from the folder by name. A missing execute bit makes it bad, so downloading again unpacks it anew.
func verifyRuntime(dir string, files map[string]string, executables []string) fileState {
	if _, err := os.Lstat(dir); errors.Is(err, fs.ErrNotExist) {
		return fileAbsent
	}
	seen := 0
	err := filepath.WalkDir(dir, func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(dir, name)
		if err != nil {
			return err
		}
		want, ok := files[filepath.ToSlash(rel)]
		if !ok || !d.Type().IsRegular() {
			return fmt.Errorf("%s is not a pinned runtime file", rel)
		}
		if got, err := hashFile(name); err != nil || got != want {
			return fmt.Errorf("%s does not match its pin", rel)
		}
		if slices.Contains(executables, filepath.ToSlash(rel)) {
			if info, err := d.Info(); err != nil || info.Mode().Perm()&0o111 == 0 {
				return fmt.Errorf("%s is not executable", rel)
			}
		}
		seen++
		return nil
	})
	if err != nil || seen != len(files) {
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
		// Only diskWriter classifies its errors; any other copy error is a read of the response body.
		if f := (*failure)(nil); errors.As(copyErr, &f) {
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

// unpack extracts the archive's root folder root into dir, reading it as a ZIP when zipped and as a .tar.gz
// otherwise; dir must not exist.
func unpack(archive string, zipped bool, root, dir string) error {
	write := func(name string, mode fs.FileMode, r io.Reader) error {
		inner, ok := strings.CutPrefix(name, root+"/")
		if !ok || inner == "" || !filepath.IsLocal(filepath.FromSlash(inner)) {
			return fmt.Errorf("archive entry %q is outside %s", name, root)
		}
		return writeExtracted(filepath.Join(dir, filepath.FromSlash(inner)), mode, r)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return eachArchiveFile(archive, zipped, write)
}

// eachArchiveFile passes each regular file of a ZIP (zipped) or .tar.gz archive to extract.
func eachArchiveFile(archive string, zipped bool, extract extractFunc) error {
	file, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	if zipped {
		info, err := file.Stat()
		if err != nil {
			return err
		}
		return eachZipFile(file, info.Size(), extract)
	}
	return eachTarFile(file, extract)
}

func writeExtracted(target string, mode fs.FileMode, r io.Reader) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	perm := fs.FileMode(0o644)
	if mode&0o111 != 0 {
		perm = 0o755
	}
	out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, perm)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, r)
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	return err
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
