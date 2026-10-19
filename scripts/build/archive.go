package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
)

// archiveWriter adds one directory (source nil) or file to an archive.
type archiveWriter interface {
	add(name string, info fs.FileInfo, source io.Reader) error
	Close() error
}

// zipDir preserves the root folder name and removes partial archives on failure.
func zipDir(root, out string) error {
	return archiveDir(root, out, func(w io.Writer) archiveWriter { return zipArchive{zip.NewWriter(w)} })
}

// tarGzDir preserves the root folder name and file modes and removes partial archives on failure.
func tarGzDir(root, out string) error {
	return archiveDir(root, out, func(w io.Writer) archiveWriter {
		gz := gzip.NewWriter(w)
		return tarGzArchive{tar.NewWriter(gz), gz}
	})
}

func archiveDir(root, out string, newWriter func(io.Writer) archiveWriter) error {
	temp, err := os.CreateTemp(filepath.Dir(out), filepath.Base(out)+".*.part")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(temp.Name()) }()
	writer := newWriter(temp)
	base := filepath.Base(root)
	walkErr := filepath.WalkDir(root, func(name string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		archiveName := path.Join(base, filepath.ToSlash(rel))
		if entry.IsDir() {
			return writer.add(archiveName+"/", info, nil)
		}
		source, err := os.Open(name)
		if err != nil {
			return err
		}
		defer func() { _ = source.Close() }()
		return writer.add(archiveName, info, source)
	})
	if err := errors.Join(walkErr, writer.Close()); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	// CreateTemp uses 0600; make the distribution readable.
	if err := os.Chmod(temp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(temp.Name(), out)
}

type zipArchive struct{ *zip.Writer }

func (z zipArchive) add(name string, info fs.FileInfo, source io.Reader) error {
	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	header.Name = name
	if source == nil {
		_, err := z.CreateHeader(header)
		return err
	}
	header.Method = zip.Deflate
	dest, err := z.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = io.Copy(dest, source)
	return err
}

type tarGzArchive struct {
	tw *tar.Writer
	gz *gzip.Writer
}

func (t tarGzArchive) add(name string, info fs.FileInfo, source io.Reader) error {
	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}
	header.Name = name
	if err := t.tw.WriteHeader(header); err != nil || source == nil {
		return err
	}
	_, err = io.Copy(t.tw, source)
	return err
}

func (t tarGzArchive) Close() error {
	return errors.Join(t.tw.Close(), t.gz.Close())
}
