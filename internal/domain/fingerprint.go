package domain

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"math/bits"
	"slices"
)

// 映像の指紋（specs/030-video-versions/research.md R-6、data-model.md §2・§6）。
//
// 指紋はシーク用スプライトの各コマの pHash を並べたものである。internal/media が各コマを
// 32×32 の輝度に縮め、ここで 2 次元 DCT の低周波からハッシュを作る。2 本の比較は、コマを
// 番号ではなく時刻で組にする。閾値は再エンコードした試験用の動画で決めた値で、規則や値を
// 変えるときは FingerprintVersion を上げ、旧い版の指紋とは比べない。

const (
	// FingerprintVersion は指紋の規則（ハッシュの作り方と閾値）の版である。
	FingerprintVersion = 1
	// FingerprintFrameSize は 1 コマのハッシュを作る輝度の一辺である。
	FingerprintFrameSize = 32
	// FingerprintFlatVariance は、これ未満の輝度の分散（0〜255 の値の 2 乗）のコマを
	// 単色として比較から外す値である。黒画面やフェードはどの動画でも一致してしまう。
	FingerprintFlatVariance = 16.0
	// FingerprintMinComparableFrames は、比べられたとみなす組の最小の数である。
	FingerprintMinComparableFrames = 3
	// FingerprintMatchMaxDistance は、同じ動画かもしれないとみなすハミング距離の
	// 中央値の上限である（63 ビットのうち）。
	FingerprintMatchMaxDistance = 12

	// fingerprintLowFrequency は DCT の係数のうち使う低周波の一辺である。
	fingerprintLowFrequency = 8
	// fingerprintFrameBytes は保存の形の 1 コマの大きさ（印 1 バイト + ハッシュ 8 バイト）である。
	fingerprintFrameBytes = 9
)

// FrameHash はコマ 1 つのハッシュである。Flat のコマは単色で、比較から外す。
type FrameHash struct {
	Hash uint64
	Flat bool
}

// Fingerprint は 1 本の動画の指紋である。Frames[i] はスプライトのコマ i で、
// [i*IntervalMs, (i+1)*IntervalMs) の区間を受け持つ。
type Fingerprint struct {
	Version    int
	IntervalMs int64
	Frames     []FrameHash
}

// ErrInvalidFingerprint は保存の形の指紋が読めないことを表す。
var ErrInvalidFingerprint = errors.New("the fingerprint data is malformed")

// Encode は Frames を保存の形（1 コマ 9 バイト: 単色なら 1・そうでなければ 0 の印、
// 続いてハッシュの big endian）にする。版と間隔は別の列に置く。
func (f Fingerprint) Encode() []byte {
	out := make([]byte, 0, len(f.Frames)*fingerprintFrameBytes)
	for _, frame := range f.Frames {
		var flag byte
		if frame.Flat {
			flag = 1
		}
		out = append(out, flag)
		out = binary.BigEndian.AppendUint64(out, frame.Hash)
	}
	return out
}

// DecodeFingerprint は Encode の形を読む。長さが 9 の倍数でないか、印が 0・1 以外なら
// ErrInvalidFingerprint を返す。
func DecodeFingerprint(data []byte, version int, intervalMs int64) (Fingerprint, error) {
	if len(data)%fingerprintFrameBytes != 0 {
		return Fingerprint{}, fmt.Errorf("%w: %d bytes", ErrInvalidFingerprint, len(data))
	}
	frames := make([]FrameHash, 0, len(data)/fingerprintFrameBytes)
	for offset := 0; offset < len(data); offset += fingerprintFrameBytes {
		flag := data[offset]
		if flag > 1 {
			return Fingerprint{}, fmt.Errorf("%w: flag %d", ErrInvalidFingerprint, flag)
		}
		frames = append(frames, FrameHash{
			Hash: binary.BigEndian.Uint64(data[offset+1 : offset+fingerprintFrameBytes]),
			Flat: flag == 1,
		})
	}
	return Fingerprint{Version: version, IntervalMs: intervalMs, Frames: frames}, nil
}

// fingerprintCos[u][x] は cos((2x+1)uπ/64) で、32 点の DCT-II の低周波 8 個の基底である。
var fingerprintCos = func() (table [fingerprintLowFrequency][FingerprintFrameSize]float64) {
	for u := range fingerprintLowFrequency {
		for x := range FingerprintFrameSize {
			table[u][x] = math.Cos(float64(2*x+1) * float64(u) * math.Pi / float64(2*FingerprintFrameSize))
		}
	}
	return table
}()

// HashFrame は 32×32 の輝度からコマのハッシュを作る。2 次元 DCT の低周波 8×8 のうち
// 直流を除く 63 係数を、その中央値より大きければ 1 のビットにする（係数 (u, v) は
// ビット u*8+v-1）。輝度の分散が FingerprintFlatVariance 未満なら Flat にする。
func HashFrame(luma [FingerprintFrameSize][FingerprintFrameSize]uint8) FrameHash {
	var sum, sumSquares float64
	for _, row := range luma {
		for _, value := range row {
			v := float64(value)
			sum += v
			sumSquares += v * v
		}
	}
	count := float64(FingerprintFrameSize * FingerprintFrameSize)
	mean := sum / count
	variance := sumSquares/count - mean*mean

	// 行ごとに横方向の低周波を求め、続けて縦方向に求める。
	var rows [FingerprintFrameSize][fingerprintLowFrequency]float64
	for y := range FingerprintFrameSize {
		for v := range fingerprintLowFrequency {
			var total float64
			for x := range FingerprintFrameSize {
				total += float64(luma[y][x]) * fingerprintCos[v][x]
			}
			rows[y][v] = total
		}
	}
	coefficients := make([]float64, 0, fingerprintLowFrequency*fingerprintLowFrequency-1)
	for u := range fingerprintLowFrequency {
		for v := range fingerprintLowFrequency {
			if u == 0 && v == 0 {
				continue
			}
			var total float64
			for y := range FingerprintFrameSize {
				total += rows[y][v] * fingerprintCos[u][y]
			}
			coefficients = append(coefficients, total)
		}
	}
	sorted := slices.Clone(coefficients)
	slices.Sort(sorted)
	median := sorted[len(sorted)/2]

	var hash uint64
	for i, coefficient := range coefficients {
		if coefficient > median {
			hash |= 1 << uint(i)
		}
	}
	return FrameHash{Hash: hash, Flat: variance < FingerprintFlatVariance}
}

// CompareFingerprints は 2 本の指紋のハミング距離の中央値を返す。版が違う、間隔が
// 正でない、または比べた組が FingerprintMinComparableFrames 未満なら ok は偽である。
//
// コマは時刻で組にする。a のコマ i の区間の中央（i*a.IntervalMs + a.IntervalMs/2）を
// 受け持つ b のコマ（b の配置の FrameAt と同じ規則）と組にし、b のコマ数の外なら組に
// しない。405 秒を超える動画は間隔が ceil(尺 / 81) で、尺が数ミリ秒違うだけで間隔が
// 変わるため、番号では同じ場面を組にできない。どちらかが単色のコマの組は数えない。
// 組の数が偶数のときは、真ん中の 2 つのうち小さい方を中央値とする。
func CompareFingerprints(a, b Fingerprint) (distance int, ok bool) {
	if a.Version != b.Version || a.IntervalMs <= 0 || b.IntervalMs <= 0 {
		return 0, false
	}
	distances := make([]int, 0, len(a.Frames))
	for i, frame := range a.Frames {
		center := int64(i)*a.IntervalMs + a.IntervalMs/2
		j := center / b.IntervalMs
		if j >= int64(len(b.Frames)) {
			continue
		}
		other := b.Frames[j]
		if frame.Flat || other.Flat {
			continue
		}
		distances = append(distances, bits.OnesCount64(frame.Hash^other.Hash))
	}
	if len(distances) < FingerprintMinComparableFrames {
		return 0, false
	}
	slices.Sort(distances)
	return distances[(len(distances)-1)/2], true
}

// FingerprintsMatch は 2 本の指紋が比べられ、距離が FingerprintMatchMaxDistance 以下かを返す。
func FingerprintsMatch(a, b Fingerprint) bool {
	distance, ok := CompareFingerprints(a, b)
	return ok && distance <= FingerprintMatchMaxDistance
}
