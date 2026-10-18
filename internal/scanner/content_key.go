package scanner

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strconv"
)

// Bound reads per file because whole-file hashing is impractical for multi-terabyte libraries;
// headers and trailing container indexes help distinguish content.
const readWindow = 1 << 20 // 1MiB

// ContentKey is hex(sha256(first 1MiB || last 1MiB)) + ":" + size, preserving identity across
// moves. Bounded reads make disk I/O and probing dominate hashing, so standard-library SHA-256
// needs no faster external dependency.
func ContentKey(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("could not compute the content key (%s): %w", path, err)
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("could not compute the content key (%s): %w", path, err)
	}

	key, err := ContentKeyFrom(file, info.Size())
	if err != nil {
		return "", fmt.Errorf("could not compute the content key (%s): %w", path, err)
	}
	return key, nil
}

// ContentKeyFrom accepts ReaderAt to test the formula independently of filesystems.
func ContentKeyFrom(r io.ReaderAt, size int64) (string, error) {
	head, tail, err := readEnds(r, size)
	if err != nil {
		return "", err
	}

	sum := sha256.New()
	sum.Write(head)
	sum.Write(tail)

	return hex.EncodeToString(sum.Sum(nil)) + ":" + strconv.FormatInt(size, 10), nil
}

// For files at most 2 MiB, read once and slice overlapping/adjacent windows without changing the
// hash formula.
func readEnds(r io.ReaderAt, size int64) (head, tail []byte, err error) {
	if size <= 0 {
		return nil, nil, nil
	}

	if size <= 2*readWindow {
		whole := make([]byte, size)
		if _, err := r.ReadAt(whole, 0); err != nil {
			return nil, nil, err
		}
		return whole[:min(size, readWindow)], whole[max(0, size-readWindow):], nil
	}

	head = make([]byte, readWindow)
	if _, err := r.ReadAt(head, 0); err != nil {
		return nil, nil, err
	}

	tail = make([]byte, readWindow)
	if _, err := r.ReadAt(tail, size-readWindow); err != nil {
		return nil, nil, err
	}
	return head, tail, nil
}
