package domain

import (
	"reflect"
	"testing"
)

func int64Ptr(value int64) *int64 { return &value }

func TestMediaSizesFrom(t *testing.T) {
	tests := []struct {
		name    string
		total   int64
		streams []MediaStreamSize
		want    *MediaSizes
	}{
		{"正確なバイト数を優先し、長さの無い流れはファイルの長さで計算する", 2_000_000, []MediaStreamSize{
			{Kind: MediaStreamVideo, BitRate: 800_000},
			{Kind: MediaStreamAudio, Bytes: 500_000, BitRate: 1},
		}, &MediaSizes{TotalBytes: 2_000_000, VideoBytes: int64Ptr(1_000_000), AudioBytes: int64Ptr(500_000),
			OtherBytes: int64Ptr(500_000)}},
		{"一つでも分からない流れがある種類と、その他は出さない", 2_000_000, []MediaStreamSize{
			{Kind: MediaStreamVideo, BitRate: 800_000},
			{Kind: MediaStreamAudio, BitRate: 128_000},
			{Kind: MediaStreamAudio},
		}, &MediaSizes{TotalBytes: 2_000_000, VideoBytes: int64Ptr(1_000_000)}},
		{"合計がファイルを超える推定ならその他は出さない", 100, []MediaStreamSize{
			{Kind: MediaStreamVideo, BitRate: 800_000},
		}, &MediaSizes{TotalBytes: 100, VideoBytes: int64Ptr(1_000_000)}},
		{"何も分からなければ nil", 0, []MediaStreamSize{{Kind: MediaStreamVideo}}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := MediaSizesFrom(tc.total, 10_000, tc.streams); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("MediaSizesFrom = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestBitsPerPixelSeparatesBitRates(t *testing.T) {
	low, high := BitsPerPixel(2_073_600, 1920, 1080, 10), BitsPerPixel(8_294_400, 1920, 1080, 10)
	if low != 0.1 || high != 0.4 {
		t.Errorf("BitsPerPixel = %v, %v, want 0.1, 0.4", low, high)
	}
	if got := BitsPerPixel(8_294_400, 1920, 1080, 0); got != 0 {
		t.Errorf("フレームレート不明 = %v, want 0", got)
	}
}

func TestVariableFrameRate(t *testing.T) {
	if got := VariableFrameRate(30, 29.97); got == nil || *got {
		t.Errorf("1%% 以内 = %v, want false", got)
	}
	if got := VariableFrameRate(60, 30); got == nil || !*got {
		t.Errorf("基本の半分 = %v, want true", got)
	}
	if got := VariableFrameRate(0, 30); got != nil {
		t.Errorf("基本が不明 = %v, want nil", *got)
	}
}

func TestChromaSubsampling(t *testing.T) {
	for format, want := range map[string]string{
		"yuv420p10le": "4:2:0", "yuvj420p": "4:2:0", "nv12": "4:2:0", "p010le": "4:2:0", "yuv422p": "4:2:2",
		"yuv444p12le": "4:4:4", "gbrp10le": "4:4:4", "gray": "4:0:0", "pal8": "", "": "",
	} {
		if got := ChromaSubsampling(format); got != want {
			t.Errorf("ChromaSubsampling(%q) = %q, want %q", format, got, want)
		}
	}
}

func TestDisplayMatrixMirrored(t *testing.T) {
	const mirrored = "\n00000000:       -65536           0           0\n00000001:            0       65536           0\n00000002:            0           0  1073741824\n"
	const rotated = "\n00000000:            0       65536           0\n00000001:       -65536           0           0\n00000002:            0           0  1073741824\n"
	if !DisplayMatrixMirrored(mirrored) {
		t.Error("左右反転を見逃した")
	}
	if DisplayMatrixMirrored(rotated) || DisplayMatrixMirrored("garbage") {
		t.Error("回転だけ・読めない行列を反転とした")
	}
}

func TestMediaBrandsAndLayers(t *testing.T) {
	if got := MediaBrands("isomiso2avc1mp41"); !reflect.DeepEqual(got, []string{"isom", "iso2", "avc1", "mp41"}) {
		t.Errorf("MediaBrands = %v", got)
	}
	if got := MediaLayerCount("0,1"); got != 2 {
		t.Errorf("MediaLayerCount = %d, want 2", got)
	}
	if got := MediaLayerCount(""); got != 0 {
		t.Errorf("MediaLayerCount(空) = %d, want 0", got)
	}
}
