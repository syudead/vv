// Package faceruntime downloads and verifies the face runtime and its models under the data folder, and starts
// the face helper mdm-face and talks to it. See docs/design-docs/face-recognition.md.
package faceruntime

import (
	"strings"

	"github.com/syudead/vv/internal/domain"
)

// Pin is one downloaded file: where it comes from and what it must hash to.
type Pin struct {
	URL    string
	SHA256 string
	Size   int64
}

// Model is a model file kept under models/.
type Model struct {
	Pin
	File string
	// License is an SPDX id, or "non-commercial".
	License       string
	NonCommercial bool
}

// Platform is the runtime archive of one platform.
type Platform struct {
	// Name is the archive's platform, also the name of its root folder after "face-runtime-".
	Name    string
	Archive Pin
	// Loader is the bundled glibc loader under lib/ on Linux; empty on Windows.
	Loader string
}

func (p Platform) windows() bool { return strings.HasPrefix(p.Name, "windows-") }

// Catalog is everything the runtime can download.
type Catalog struct {
	// Version is the N of face-runtime-v<N>, which names the runtime folder.
	Version int
	// Platform is nil where no runtime is published.
	Platform    *Platform
	Detector    Model
	Recognizers map[domain.FaceRecognizer]Model
}

// runtimeRelease is the published face-runtime-v<N> release; its SHA256SUMS lists the archives' hashes.
const (
	runtimeVersion = 1
	runtimeRelease = "https://github.com/syudead/vv/releases/download/face-runtime-v1/"
)

var runtimePlatforms = map[string]Platform{
	"linux/amd64": {
		Name: "linux-amd64", Loader: "ld-linux-x86-64.so.2",
		Archive: Pin{
			URL:    runtimeRelease + "face-runtime-linux-amd64.tar.gz",
			SHA256: "8a6c465e9492a6d2e4a0a56cad92e73fdd87ce20b292ae3c925ea6b02263b98f", Size: 11629060,
		},
	},
	"linux/arm64": {
		Name: "linux-arm64", Loader: "ld-linux-aarch64.so.1",
		Archive: Pin{
			URL:    runtimeRelease + "face-runtime-linux-arm64.tar.gz",
			SHA256: "1da58f3440e562b5566deb7adbef846cb85d813f2ffeda3e67ecdee33f60c872", Size: 10141444,
		},
	},
	"windows/amd64": {
		Name: "windows-amd64",
		Archive: Pin{
			URL:    runtimeRelease + "face-runtime-windows-amd64.zip",
			SHA256: "bcea639a6423d74be4ed2d618575195cf04cc1c3b0a4e5a59bc7528bf7d371bb", Size: 5942354,
		},
	},
}

// The models come from their upstream Hugging Face repositories, pinned to a commit and a hash.
var (
	yunet = Model{
		File: "face_detection_yunet_2023mar.onnx", License: "MIT",
		Pin: Pin{
			URL:    "https://huggingface.co/opencv/face_detection_yunet/resolve/3cc26e7f1014a5ee5d74a42acee58bafc9d0a310/face_detection_yunet_2023mar.onnx",
			SHA256: "8f2383e4dd3cfbb4553ea8718107fc0423210dc964f9f4280604804ed2552fa4", Size: 232589,
		},
	}
	sfaceInt8 = Model{
		File: "face_recognition_sface_2021dec_int8.onnx", License: "Apache-2.0",
		Pin: Pin{
			URL:    "https://huggingface.co/opencv/face_recognition_sface/resolve/3d7082438a6e4551e840c9b2bb60b71e8da4b524/face_recognition_sface_2021dec_int8.onnx",
			SHA256: "2b0e941e6f16cc048c20aee0c8e31f569118f65d702914540f7bfdc14048d78a", Size: 9896933,
		},
	}
	// arcFaceR50 is InsightFace buffalo_l's w600k_r50, licensed for non-commercial research only.
	arcFaceR50 = Model{
		File: "arcface_r50_buffalo_l.onnx", License: "non-commercial", NonCommercial: true,
		Pin: Pin{
			URL:    "https://huggingface.co/immich-app/buffalo_l/resolve/d09715916a0778919a770c343533641e250b8699/recognition/model.onnx",
			SHA256: "4c06341c33c2ca1f86781dab0e829f88ad5b64be9fba56e56bc9ebdefc619e43", Size: 174383860,
		},
	}
)

// DefaultCatalog is the published runtime for goos/goarch and the pinned models.
func DefaultCatalog(goos, goarch string) Catalog {
	c := Catalog{
		Version:  runtimeVersion,
		Detector: yunet,
		Recognizers: map[domain.FaceRecognizer]Model{
			domain.FaceRecognizerSFaceInt8:  sfaceInt8,
			domain.FaceRecognizerArcFaceR50: arcFaceR50,
		},
	}
	if p, ok := runtimePlatforms[goos+"/"+goarch]; ok {
		c.Platform = &p
	}
	return c
}
