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
	// Files is the SHA-256 of every regular file under the archive's root folder, by slash-separated path. The
	// unpacked runtime is verified against it, so the trust root is this binary, not the writable data folder.
	Files map[string]string
}

func (p Platform) windows() bool { return strings.HasPrefix(p.Name, "windows-") }

// zipped reports whether the archive is a ZIP, by its published asset name; the downloaded file's name says
// nothing about its format.
func (p Platform) zipped() bool { return strings.HasSuffix(p.Archive.URL, ".zip") }

// Catalog is everything the runtime can download.
type Catalog struct {
	// Version is the N of face-runtime-v<N>, which names the runtime folder.
	Version int
	// Platform is nil where no runtime is published.
	Platform    *Platform
	Detector    Model
	Recognizers map[domain.FaceRecognizer]Model
}

// runtimeRelease is the published face-runtime-v<N> release; its SHA256SUMS lists the archives' hashes, and each
// platform's Files were hashed from its published archive.
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
		Files: map[string]string{
			"README.txt":                                 "812da5eefa35a71e1fc6a633c25828df965e58d249ee0ed230439bc195fcfcac",
			"lib/ld-linux-x86-64.so.2":                   "c8438e4fde1934e61c88311633f00949ff645d5c04cdb8671fa3d78164d2f307",
			"lib/libc.so.6":                              "9792e3cbb541c8f44c7acf5f14f4022ea62998ecc787d326bed4d8b6547dfd92",
			"lib/libdl.so.2":                             "295fa521a03cd2faa99974f378c9e23dd622021ec7d32bcad4f8ea61aec8a872",
			"lib/libgcc_s.so.1":                          "30c61ab012a4241bed033725a09b61f5fdd3bb7df95ee852d0b096520524c7af",
			"lib/libm.so.6":                              "6d567d53e895273ca14a1f9dc164fc6c8d39aed2f60aa46a733c2784228915f3",
			"lib/libonnxruntime.so":                      "3da6146e14e7b8aaec625dde11d6114c7457c87a5f93d744897da8781e35c673",
			"lib/libpthread.so.0":                        "85e21f7dba0394411d00959176fd18b470e575b0b05f1f4f41e5636802ce0500",
			"lib/librt.so.1":                             "7b7b84d1aedda0e0b2bdfc68844362782132180b8f02be1300e21e77572e514b",
			"lib/libstdc++.so.6":                         "972bb2a18b71140dab0240f8a1f68ab3fb1d56bcd4c4f824a91b70888faf5a00",
			"licenses/libc6-copyright.txt":               "f5788886720a2605a946e81d571e6c8162b09f58d2e2ceb8d36e5768fccd850d",
			"licenses/libgcc-s1-copyright.txt":           "20390f8a6f3b1e4d7cb45dd8652dabb259bbef688cbad839bcdb0b9ba7252f79",
			"licenses/libstdc++6-copyright.txt":          "20390f8a6f3b1e4d7cb45dd8652dabb259bbef688cbad839bcdb0b9ba7252f79",
			"licenses/onnxruntime-LICENSE.txt":           "2f07c72751aed99790b8a4869cf2311df85a860b22ded05fa22803587a48922c",
			"licenses/onnxruntime-ThirdPartyNotices.txt": "e9e90971a8e75a9a8ac0c6412e29c1202d079998389915aa485f46c816c3b4cc",
			"mdm-face":                                   "2b89157837f39ae2f59d17ac4930f3bb064159ea0d5f0aad489da51ced02f273",
		},
	},
	"linux/arm64": {
		Name: "linux-arm64", Loader: "ld-linux-aarch64.so.1",
		Archive: Pin{
			URL:    runtimeRelease + "face-runtime-linux-arm64.tar.gz",
			SHA256: "1da58f3440e562b5566deb7adbef846cb85d813f2ffeda3e67ecdee33f60c872", Size: 10141444,
		},
		Files: map[string]string{
			"README.txt":                                 "6cf005739e01de6f083327d58e73119b0c68f49e89c44128f5e7e22c1ea7aec4",
			"lib/ld-linux-aarch64.so.1":                  "1b882eddacd494b060413a8aa4f0a4754ee3f84775e68cecbf8a86092b59cca6",
			"lib/libc.so.6":                              "70fa4c4cf57eabeb96097cce47582d02379c4cd1de3db3e98ade4dd7089a1b13",
			"lib/libdl.so.2":                             "8023ba7cb6ecabb57d1910ecd24ce347cba5653d1b547d9c7950301ed59716ca",
			"lib/libgcc_s.so.1":                          "9a146967dc38c3820c5dc21c3209812655f66b6551264072b187bd0e787947e0",
			"lib/libm.so.6":                              "a8afb2d1f4a0c5e14c0fc82f794ec839eafe29738907aa92895410f57630bbe2",
			"lib/libonnxruntime.so":                      "0afd69a0ae38c5099fd0e8604dda398ac43dee67cd9c6394b5142b19e82528de",
			"lib/libpthread.so.0":                        "b4f0bcbb029e42861545757592356a490d87ef42cfa8a7f2d4295846e42698b5",
			"lib/librt.so.1":                             "a4b07c4b8bf45da4ec97170e6cd15097c42af1f94124a9ab251551a9a0a79556",
			"lib/libstdc++.so.6":                         "fff1f0050a33ddf29c44634c7639ac16e2573a61e3daab4fcc4c007b432ccf19",
			"licenses/libc6-copyright.txt":               "f5788886720a2605a946e81d571e6c8162b09f58d2e2ceb8d36e5768fccd850d",
			"licenses/libgcc-s1-copyright.txt":           "20390f8a6f3b1e4d7cb45dd8652dabb259bbef688cbad839bcdb0b9ba7252f79",
			"licenses/libstdc++6-copyright.txt":          "20390f8a6f3b1e4d7cb45dd8652dabb259bbef688cbad839bcdb0b9ba7252f79",
			"licenses/onnxruntime-LICENSE.txt":           "2f07c72751aed99790b8a4869cf2311df85a860b22ded05fa22803587a48922c",
			"licenses/onnxruntime-ThirdPartyNotices.txt": "e9e90971a8e75a9a8ac0c6412e29c1202d079998389915aa485f46c816c3b4cc",
			"mdm-face":                                   "ff4ddab65a76dd6365fe02891b8e09374f34e5886078a20550b47db441cabf4e",
		},
	},
	"windows/amd64": {
		Name: "windows-amd64",
		Archive: Pin{
			URL:    runtimeRelease + "face-runtime-windows-amd64.zip",
			SHA256: "bcea639a6423d74be4ed2d618575195cf04cc1c3b0a4e5a59bc7528bf7d371bb", Size: 5942354,
		},
		Files: map[string]string{
			"README.txt":                                 "da53e44fd24a26ae463423bf12bc9820193693b7173f7d852f44e0bb2e67986f",
			"licenses/onnxruntime-LICENSE.txt":           "c250d6278f0b47a6439fb7592b08b58a55eb9f535aa49a1db63211c3f982b674",
			"licenses/onnxruntime-ThirdPartyNotices.txt": "e00f828e0a33de591a355ae6606d2625f5758da7d2c844db7821c9dd3e3647b6",
			"mdm-face.exe":                               "7fa3268cc251c07ca4da9ed03f0c981652641d768eb8678aa8ba59a99e6a27cf",
			"onnxruntime.dll":                            "579b636403983254346a5c1d80bd28f1519cd1e284cd204f8d4ff41f8d711559",
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
