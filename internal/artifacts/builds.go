package artifacts

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// A forced rebuild of a seek sprite or preview writes build n+1 beside the published build n, switches a per-key
// pointer file in one rename, then removes build n, so a reader never finds the artifact missing. Build 0 is the
// layout at the top of this package and needs no pointer; build n > 0 lives in a dot directory of the shard, which
// no content key can name.
//
//	<root>/<kind>/<p>/.current/<s>      Pointer: the current build number
//	<root>/<kind>/<p>/.b<n>/<s>[ext]    Build n of the artifact
const (
	pointerDirName = ".current"
	buildDirPrefix = ".b"
)

// shard returns the directory holding build 0 of a key and the key's sanitized name.
func (s *Store) shard(dir, contentKey string) (shardDir, name string, ok bool) {
	if s.root == "" {
		return "", "", false
	}
	name, ok = fileName(contentKey)
	if !ok {
		return "", "", false
	}
	prefix := name
	if len(prefix) > 2 {
		prefix = prefix[:2]
	}
	return filepath.Join(s.root, dir, prefix), name, true
}

func (s *Store) buildPath(dir, contentKey, ext string, build int) (string, bool) {
	shardDir, name, ok := s.shard(dir, contentKey)
	if !ok {
		return "", false
	}
	if build > 0 {
		shardDir = filepath.Join(shardDir, buildDirPrefix+strconv.Itoa(build))
	}
	return filepath.Join(shardDir, name+ext), true
}

func (s *Store) pointerPath(dir, contentKey string) (string, bool) {
	shardDir, name, ok := s.shard(dir, contentKey)
	if !ok {
		return "", false
	}
	return filepath.Join(shardDir, pointerDirName, name), true
}

// currentBuild treats a missing or unreadable pointer as build 0; a read that then misses requeues the artifact.
func (s *Store) currentBuild(dir, contentKey string) int {
	path, ok := s.pointerPath(dir, contentKey)
	if !ok {
		return 0
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	build, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || build < 0 {
		return 0
	}
	return build
}

// switchBuild replaces the pointer in one rename so readers see either the old build or the new one.
func (s *Store) switchBuild(dir, contentKey string, build int) error {
	target, ok := s.pointerPath(dir, contentKey)
	if !ok {
		return errInvalidKey
	}
	temporary, err := s.makeTemporaryDir("build-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(temporary) }()
	staged := filepath.Join(temporary, "current")
	if err := os.WriteFile(staged, []byte(strconv.Itoa(build)+"\n"), 0o644); err != nil {
		return fmt.Errorf("cannot write the build pointer: %w", err)
	}
	if err := moveInto(staged, target); err != nil {
		return fmt.Errorf("cannot switch the build pointer: %w", err)
	}
	return nil
}

// readResolved reads the current build, and resolves the pointer again when that build is missing, because a forced
// rebuild may switch the pointer and remove the build between the two steps.
func readResolved[T any](s *Store, dir, contentKey string, read func(build int) (T, error)) (T, error) {
	build := s.currentBuild(dir, contentKey)
	value, err := read(build)
	if err == nil || !errors.Is(err, fs.ErrNotExist) {
		return value, err
	}
	if again := s.currentBuild(dir, contentKey); again != build {
		return read(again)
	}
	return value, err
}

// removeBuild removes one build's paths and, for build n > 0, its directory once empty.
func removeBuild(paths ...string) error {
	var errs []error
	for _, path := range paths {
		if err := os.RemoveAll(path); err != nil {
			errs = append(errs, err)
		}
	}
	if len(paths) > 0 && strings.HasPrefix(filepath.Base(filepath.Dir(paths[0])), buildDirPrefix) {
		// Another key's build may still use the directory; a failed remove leaves it for that key.
		_ = os.Remove(filepath.Dir(paths[0]))
	}
	return errors.Join(errs...)
}

// removeAllBuilds removes every build of a key and its pointer, ignoring missing files.
func (s *Store) removeAllBuilds(dir, contentKey string, exts ...string) error {
	shardDir, _, ok := s.shard(dir, contentKey)
	if !ok {
		return nil
	}
	builds := []int{0}
	entries, err := os.ReadDir(shardDir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	for _, entry := range entries {
		number, found := strings.CutPrefix(entry.Name(), buildDirPrefix)
		if build, err := strconv.Atoi(number); found && err == nil && build > 0 && entry.IsDir() {
			builds = append(builds, build)
		}
	}
	var errs []error
	for _, build := range builds {
		var paths []string
		for _, ext := range exts {
			path, _ := s.buildPath(dir, contentKey, ext, build)
			paths = append(paths, path)
		}
		errs = append(errs, removeBuild(paths...))
	}
	if pointer, ok := s.pointerPath(dir, contentKey); ok {
		errs = append(errs, removeFile(pointer))
	}
	return errors.Join(errs...)
}
