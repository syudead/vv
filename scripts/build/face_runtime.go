package main

// Build the face runtime archives that the face-runtime workflow publishes; see docs/design-docs/face-recognition.md.

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/syudead/vv/internal/faceproto"
	"github.com/syudead/vv/scripts/devtools"
)

// Pin onnxruntime to Microsoft's NuGet package, which holds the library of every platform;
// update the version and the SHA-256 together.
const (
	onnxruntimeVersion = "1.22.0"
	ortPackage         = "microsoft.ml.onnxruntime." + onnxruntimeVersion + ".nupkg"
	ortPackageSHA256   = "d571e63a2329baacb713f441e65ad75284de354db6e1ac435fe4bebbb417986a"
	ortPackageURL      = "https://api.nuget.org/v3-flatcontainer/microsoft.ml.onnxruntime/" + onnxruntimeVersion + "/" + ortPackage
)

// debianImage supplies the glibc and C++ runtime the Linux helper runs under, so the helper runs on
// Alpine and on any other host libc. Renovate does not read this constant; update it by hand.
const debianImage = "debian:trixie@sha256:913f6706df59a68922d1dd08f78c2476560a8d367897200a6005b00e5f67c2d5"

// debianLibs are the C runtime files onnxruntime and the helper load, from Debian's multiarch directory.
var debianLibs = []string{"libc.so.6", "libm.so.6", "libdl.so.2", "libpthread.so.0", "librt.so.1", "libstdc++.so.6", "libgcc_s.so.1"}

// debianCopyright are the Debian packages of debianLibs, whose copyright files go into the archive.
var debianCopyright = []string{"libc6", "libstdc++6", "libgcc-s1"}

// facePlatform is one archive: where its onnxruntime comes from and, on Linux, its Debian runtime.
type facePlatform struct {
	goos, goarch string
	// ortLib is the library's path inside the onnxruntime package.
	ortLib string
	// Linux only: Debian's multiarch directory and the dynamic loader in it.
	triplet, loader string
}

var facePlatforms = map[string]facePlatform{
	"linux-amd64": {
		goos: "linux", goarch: "amd64",
		ortLib:  "runtimes/linux-x64/native/libonnxruntime.so",
		triplet: "x86_64-linux-gnu", loader: "ld-linux-x86-64.so.2",
	},
	"linux-arm64": {
		goos: "linux", goarch: "arm64",
		ortLib:  "runtimes/linux-arm64/native/libonnxruntime.so",
		triplet: "aarch64-linux-gnu", loader: "ld-linux-aarch64.so.1",
	},
	"windows-amd64": {
		goos: "windows", goarch: "amd64",
		ortLib: "runtimes/win-x64/native/onnxruntime.dll",
	},
}

func (p facePlatform) linux() bool { return p.goos == "linux" }

func faceRuntimeName(platform string) string { return "face-runtime-" + platform }

// faceRuntimeArchive is the file name the release carries and mdm downloads.
func faceRuntimeArchive(platform string) string {
	if facePlatforms[platform].linux() {
		return faceRuntimeName(platform) + ".tar.gz"
	}
	return faceRuntimeName(platform) + ".zip"
}

// buildFaceRuntime writes dist/face-runtime-<platform>.(tar.gz|zip) and returns its path.
func buildFaceRuntime(root, platform string) (string, error) {
	p, ok := facePlatforms[platform]
	if !ok {
		return "", fmt.Errorf("unknown face runtime platform %q", platform)
	}
	cache := filepath.Join(root, ffmpegCacheDir)
	ortArchive, err := fetchPinned(&http.Client{Timeout: downloadTimeout}, ortPackageURL, cache, ortPackage, ortPackageSHA256)
	if err != nil {
		return "", err
	}
	staging := filepath.Join(root, windowsAppDir, faceRuntimeName(platform))
	if err := os.RemoveAll(staging); err != nil {
		return "", err
	}
	if err := stageFaceRuntime(root, p, ortArchive, staging); err != nil {
		return "", err
	}
	out := filepath.Join(root, windowsAppDir, faceRuntimeArchive(platform))
	if p.linux() {
		err = tarGzDir(staging, out)
	} else {
		err = zipDir(staging, out)
	}
	if err != nil {
		return "", err
	}
	return out, os.RemoveAll(staging)
}

func stageFaceRuntime(root string, p facePlatform, ortArchive, staging string) error {
	libDir, exe := staging, "mdm-face.exe"
	if p.linux() {
		libDir, exe = filepath.Join(staging, "lib"), "mdm-face"
	}
	licenses := filepath.Join(staging, "licenses")
	for _, dir := range []string{libDir, licenses} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	ortFiles := map[string]string{
		p.ortLib:                filepath.Join(libDir, path.Base(p.ortLib)),
		"LICENSE":               filepath.Join(licenses, "onnxruntime-LICENSE.txt"),
		"ThirdPartyNotices.txt": filepath.Join(licenses, "onnxruntime-ThirdPartyNotices.txt"),
	}
	// A NuGet package is a zip without a root directory.
	wholeName := func(name string) (string, bool) { return name, true }
	if err := extractZipEntries(ortArchive, ortFiles, wholeName); err != nil {
		return err
	}
	if p.linux() {
		if err := copyDebianRuntime(p, libDir, licenses); err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(staging, "README.txt"), []byte(faceRuntimeReadme(p)), 0o644); err != nil {
		return err
	}
	// The helper needs no cgo: purego loads onnxruntime, so one Linux host builds every archive.
	return devtools.RunEnv(root, []string{"GOOS=" + p.goos, "GOARCH=" + p.goarch, "CGO_ENABLED=0"}, "go", "build",
		"-trimpath", "-ldflags", "-s -w", "-o", filepath.Join(staging, exe), "./cmd/mdm-face")
}

func requireFound(archive string, want map[string]string, found map[string]bool) error {
	for inner := range want {
		if !found[inner] {
			return fmt.Errorf("%s に %s がありません", filepath.Base(archive), inner)
		}
	}
	return nil
}

func writeFile(target string, source io.Reader, mode os.FileMode) error {
	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, source); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// copyDebianRuntime copies the loader and libraries out of a created, never started, Debian container.
func copyDebianRuntime(p facePlatform, libDir, licenses string) error {
	id, err := exec.Command("docker", "create", "--platform", p.goos+"/"+p.goarch, debianImage).Output()
	if err != nil {
		return fmt.Errorf("docker create %s: %w", debianImage, err)
	}
	container := strings.TrimSpace(string(id))
	defer func() { _ = exec.Command("docker", "rm", container).Run() }()
	copies := map[string]string{}
	for _, lib := range append([]string{p.loader}, debianLibs...) {
		copies["/usr/lib/"+p.triplet+"/"+lib] = filepath.Join(libDir, lib)
	}
	for _, pkg := range debianCopyright {
		copies["/usr/share/doc/"+pkg+"/copyright"] = filepath.Join(licenses, pkg+"-copyright.txt")
	}
	for from, to := range copies {
		// -L follows the soname symlinks, so the archive holds plain files.
		if out, err := exec.Command("docker", "cp", "-L", container+":"+from, to).CombinedOutput(); err != nil {
			return fmt.Errorf("docker cp %s: %w: %s", from, err, out)
		}
	}
	return nil
}

func faceRuntimeReadme(p facePlatform) string {
	text := fmt.Sprintf(`vv face runtime for %s/%s (helper protocol version %d)

vv's server (mdm) downloads this archive when face recognition is turned on,
and starts mdm-face to detect and embed faces. It is not meant to be run by hand.

onnxruntime %s: Microsoft's NuGet package %s
  License: licenses/onnxruntime-LICENSE.txt (MIT), licenses/onnxruntime-ThirdPartyNotices.txt
`, p.goos, p.goarch, faceproto.Version, onnxruntimeVersion, ortPackageURL)
	if p.linux() {
		text += fmt.Sprintf(`
lib/ holds the GNU C library, libstdc++ and libgcc_s from the Debian image
  %s
  mdm starts the helper with lib/%s --library-path lib mdm-face, so it runs
  under these libraries on any Linux, Alpine included. Their licenses are in
  licenses/*-copyright.txt; the sources are Debian's glibc and gcc-14 source
  packages: https://sources.debian.org/
`, debianImage, p.loader)
	}
	if !p.linux() {
		return crlf(text)
	}
	return text
}
