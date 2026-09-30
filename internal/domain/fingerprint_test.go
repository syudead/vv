package domain

import (
	"errors"
	"math"
	"math/bits"
	"math/rand/v2"
	"testing"
)

// sceneLuma は seed で決まる滑らかな場面の輝度を返す。低周波の正弦波を重ねるので、
// 縮めた映像のコマに近い。
func sceneLuma(seed uint64) [FingerprintFrameSize][FingerprintFrameSize]float64 {
	random := rand.New(rand.NewPCG(seed, seed*7+1))
	type wave struct{ fx, fy, phase, amplitude float64 }
	waves := make([]wave, 6)
	for i := range waves {
		waves[i] = wave{
			fx: random.Float64() * 3, fy: random.Float64() * 3,
			phase: random.Float64() * 2 * math.Pi, amplitude: 5 + random.Float64()*10,
		}
	}
	var out [FingerprintFrameSize][FingerprintFrameSize]float64
	for y := range FingerprintFrameSize {
		for x := range FingerprintFrameSize {
			value := 128.0
			for _, w := range waves {
				value += w.amplitude * math.Sin(2*math.Pi*(w.fx*float64(x)+w.fy*float64(y))/FingerprintFrameSize+w.phase)
			}
			out[y][x] = value
		}
	}
	return out
}

// toLuma は輝度を brightness だけ明るくし、平均の周りで contrast 倍にして 0〜255 に収める。
func toLuma(scene [FingerprintFrameSize][FingerprintFrameSize]float64, brightness, contrast float64) [FingerprintFrameSize][FingerprintFrameSize]uint8 {
	var out [FingerprintFrameSize][FingerprintFrameSize]uint8
	for y := range FingerprintFrameSize {
		for x := range FingerprintFrameSize {
			value := (scene[y][x]-128)*contrast + 128 + brightness
			out[y][x] = uint8(math.Round(math.Max(0, math.Min(255, value))))
		}
	}
	return out
}

func hammingDistance(a, b FrameHash) int {
	return bits.OnesCount64(a.Hash ^ b.Hash)
}

// 同じ場面の明るさ・コントラストを変えたものは閾値の内、別の場面は外になる。
func TestHashFrameToleratesBrightnessAndContrast(t *testing.T) {
	for seed := uint64(1); seed <= 20; seed++ {
		scene := sceneLuma(seed)
		original := HashFrame(toLuma(scene, 0, 1))
		if original.Flat {
			t.Fatalf("seed %d: 模様のある場面が単色になった", seed)
		}
		for _, change := range []struct{ brightness, contrast float64 }{
			{30, 1}, {-30, 1}, {0, 0.6}, {0, 1.3}, {20, 0.8},
		} {
			changed := HashFrame(toLuma(scene, change.brightness, change.contrast))
			if d := hammingDistance(original, changed); d > FingerprintMatchMaxDistance {
				t.Errorf("seed %d, 明るさ %+v・コントラスト %v: 距離 %d が閾値 %d を超える",
					seed, change.brightness, change.contrast, d, FingerprintMatchMaxDistance)
			}
		}
		other := HashFrame(toLuma(sceneLuma(seed+100), 0, 1))
		if d := hammingDistance(original, other); d <= FingerprintMatchMaxDistance {
			t.Errorf("seed %d: 別の場面との距離 %d が閾値 %d の内", seed, d, FingerprintMatchMaxDistance)
		}
	}
}

// 輝度の分散が小さいコマ（黒画面や単色）は単色の印が付く。
func TestHashFrameMarksFlatFrames(t *testing.T) {
	var black, gray [FingerprintFrameSize][FingerprintFrameSize]uint8
	for y := range FingerprintFrameSize {
		for x := range FingerprintFrameSize {
			black[y][x] = uint8((x + y) % 3) // JPEG の揺らぎ程度
			gray[y][x] = 120
		}
	}
	if !HashFrame(black).Flat || !HashFrame(gray).Flat {
		t.Error("単色のコマに印が付かない")
	}
}

func TestFingerprintEncodeRoundTrip(t *testing.T) {
	fingerprint := Fingerprint{Version: FingerprintVersion, IntervalMs: 5000, Frames: []FrameHash{
		{Hash: 0x0123456789abcdef}, {Hash: 0, Flat: true}, {Hash: math.MaxUint64},
	}}
	encoded := fingerprint.Encode()
	if len(encoded) != 27 {
		t.Fatalf("長さ %d、27 のはず", len(encoded))
	}
	if encoded[0] != 0 || encoded[1] != 0x01 || encoded[8] != 0xef || encoded[9] != 1 {
		t.Fatalf("保存の形が違う: % x", encoded)
	}
	decoded, err := DecodeFingerprint(encoded, FingerprintVersion, 5000)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Version != fingerprint.Version || decoded.IntervalMs != 5000 || len(decoded.Frames) != 3 {
		t.Fatalf("decoded = %+v", decoded)
	}
	for i := range fingerprint.Frames {
		if decoded.Frames[i] != fingerprint.Frames[i] {
			t.Errorf("コマ %d: %+v, want %+v", i, decoded.Frames[i], fingerprint.Frames[i])
		}
	}
	for _, bad := range [][]byte{encoded[:10], append([]byte{2}, encoded[1:9]...)} {
		if _, err := DecodeFingerprint(bad, FingerprintVersion, 5000); !errors.Is(err, ErrInvalidFingerprint) {
			t.Errorf("% x: err = %v", bad, err)
		}
	}
}

// fingerprintOf は時刻 t ミリ秒の場面が scene(t) の動画を間隔 intervalMs で並べた指紋を返す。
func fingerprintOf(durationMs, intervalMs int64, scene func(positionMs int64) FrameHash) Fingerprint {
	count := (durationMs + intervalMs - 1) / intervalMs
	frames := make([]FrameHash, count)
	for i := range frames {
		frames[i] = scene(int64(i) * intervalMs)
	}
	return Fingerprint{Version: FingerprintVersion, IntervalMs: intervalMs, Frames: frames}
}

// 場面が 20 秒ごとに変わる動画。
func sceneAt(offset uint64) func(int64) FrameHash {
	return func(positionMs int64) FrameHash {
		return HashFrame(toLuma(sceneLuma(offset+uint64(positionMs/20_000)), 0, 1))
	}
}

// 間隔が違う 2 本（405 秒を超える動画の再エンコードで尺が数ミリ秒違う）も、コマを時刻で
// 組にするので一致する。番号で組にすると後ろの方がずれる長さでも同じである。
func TestCompareFingerprintsAlignsFramesByTime(t *testing.T) {
	a := fingerprintOf(3_600_000, NewSeekSpriteLayout(3_600_000).IntervalMs, sceneAt(0))
	b := fingerprintOf(3_600_400, NewSeekSpriteLayout(3_600_400).IntervalMs, sceneAt(0))
	if a.IntervalMs == b.IntervalMs {
		t.Fatalf("間隔が同じ %d", a.IntervalMs)
	}
	for _, pair := range [][2]Fingerprint{{a, b}, {b, a}} {
		distance, ok := CompareFingerprints(pair[0], pair[1])
		if !ok || distance != 0 || !FingerprintsMatch(pair[0], pair[1]) {
			t.Errorf("同じ動画: distance=%d ok=%v", distance, ok)
		}
	}
	short := fingerprintOf(60_000, 5000, sceneAt(0))
	longer := fingerprintOf(3_600_000, NewSeekSpriteLayout(3_600_000).IntervalMs, sceneAt(0))
	if _, ok := CompareFingerprints(short, longer); !ok {
		t.Error("区間の中央を受け持つコマがあるのに比べない")
	}

	other := fingerprintOf(3_600_000, a.IntervalMs, sceneAt(1000))
	if FingerprintsMatch(a, other) {
		t.Error("別の動画が一致する")
	}
}

// 版が違う、単色のコマばかりで比べた組が足りない、b のコマ数の外は比べない。
func TestCompareFingerprintsRefusesWhenNotComparable(t *testing.T) {
	a := fingerprintOf(60_000, 5000, sceneAt(0))
	older := a
	older.Version = FingerprintVersion + 1
	if _, ok := CompareFingerprints(a, older); ok {
		t.Error("版が違うのに比べる")
	}

	flat := fingerprintOf(60_000, 5000, func(positionMs int64) FrameHash {
		if positionMs < 10_000 {
			return sceneAt(0)(positionMs)
		}
		return FrameHash{Flat: true}
	})
	if _, ok := CompareFingerprints(a, flat); ok {
		t.Error("比べた組が 2 つなのに比べる")
	}

	tail := fingerprintOf(10_000, 5000, sceneAt(0))
	if _, ok := CompareFingerprints(a, tail); ok {
		t.Error("b のコマ数の外を組にしている")
	}
	if _, ok := CompareFingerprints(a, Fingerprint{Version: FingerprintVersion}); ok {
		t.Error("間隔の無い指紋と比べる")
	}
}

// 中央値なので、一部のコマだけが違っても一致する。
func TestCompareFingerprintsUsesMedian(t *testing.T) {
	a := fingerprintOf(100_000, 5000, sceneAt(0))
	b := fingerprintOf(100_000, 5000, sceneAt(0))
	for i := range 5 {
		b.Frames[i].Hash = ^b.Frames[i].Hash
	}
	distance, ok := CompareFingerprints(a, b)
	if !ok || distance != 0 {
		t.Errorf("distance=%d ok=%v", distance, ok)
	}
}
