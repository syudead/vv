package media

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

func requireEncoder(t *testing.T, encoder string) {
	t.Helper()
	requireFFmpeg(t)
	output, err := exec.Command(seekThumbnailCommand, "-hide_banner", "-encoders").Output()
	if err != nil || !bytes.Contains(output, []byte(" "+encoder+" ")) {
		t.Skipf("ffmpeg に %s が無い", encoder)
	}
}

// makeH264Clock は timeGraySource を 1 秒ごとのキーフレームで H.264 にする。B フレームを
// 含むので、表示時刻は ctts と edit list で決まる。
func makeH264Clock(t *testing.T, name, seconds string, extra ...string) string {
	t.Helper()
	requireEncoder(t, "libx264")
	videoPath := filepath.Join(t.TempDir(), name)
	args := []string{"-f", "lavfi", "-i", timeGraySource("64x64", seconds)}
	args = append(args, extra...)
	args = append(args, "-c:v", "libx264", "-g", "10", "-bf", "2", "-crf", "10", "-y", videoPath)
	runFFmpeg(t, args...)
	return videoPath
}

func TestParseKeyframeTrackReadsKeyframeTimes(t *testing.T) {
	for _, tc := range []struct {
		name  string
		extra []string
	}{
		{"moov at end", nil},
		{"moov at start", []string{"-movflags", "+faststart"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			videoPath := makeH264Clock(t, "clock.mp4", "10", tc.extra...)
			file, err := os.Open(videoPath)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = file.Close() }()
			info, _ := file.Stat()
			movie, err := readMovieBox(context.Background(), file, info.Size())
			if err != nil {
				t.Fatal(err)
			}
			track, err := parseKeyframeTrack(movie)
			if err != nil {
				t.Fatal(err)
			}
			if track.format != "h264" || len(track.parameterSets) < 2 {
				t.Fatalf("形式 %q、パラメータセット %d 個", track.format, len(track.parameterSets))
			}
			if len(track.keyframes) != 10 {
				t.Fatalf("キーフレームが %d 枚（10 枚のはず）", len(track.keyframes))
			}
			// B フレームの遅れは edit list で打ち消され、キーフレームは 0, 1, 2, ... 秒になる。
			for i, frame := range track.keyframes {
				if want := int64(i) * 1_000_000; frame.presentationUs < want-50_000 || frame.presentationUs > want+50_000 {
					t.Errorf("キーフレーム %d の表示時刻 %d µs（%d µs のはず）", i, frame.presentationUs, want)
				}
			}
		})
	}
}

func TestGenerateSeekSpriteFromIndexPicksKeyframeAtIntervalStart(t *testing.T) {
	videoPath := makeH264Clock(t, "clock.mp4", "30")
	layout := domain.NewSeekSpriteLayout(30_000)
	output := t.TempDir()
	if err := generateSeekSpriteFromIndex(context.Background(), videoPath, output, layout); err != nil {
		t.Fatal(err)
	}
	sheets := readSheets(t, output)
	if b := sheets[0].Bounds(); b.Dx() != 576 || b.Dy() != 576 {
		t.Fatalf("シートの大きさ %v（576x576 のはず）", b)
	}
	for k := range layout.FrameCount {
		startSec := float64(k) * float64(layout.IntervalMs) / 1000
		if got := frameSeconds(t, sheets, layout, k); got < startSec-0.5 || got > startSec+0.5 {
			t.Errorf("コマ %d の時刻 %.2f 秒（区間の先頭 %.0f 秒のはず）", k, got, startSec)
		}
	}
	if luma := frameLuma(t, sheets, layout, layout.FrameCount); luma > 20 {
		t.Errorf("空きの升目の輝度が %d", luma)
	}
}

// 最後の区間の開始が映像の終わりと重なる（容器の長さが映像より僅かに長い）と、
// その区間にはフレームが無い。索引からは直前のキーフレームを選び、失敗しない。
func TestGenerateSeekSpriteFromIndexUsesPreviousKeyframeAfterVideoEnds(t *testing.T) {
	videoPath := makeH264Clock(t, "clock.mp4", "10")
	layout := domain.NewSeekSpriteLayout(10_010)
	if layout.FrameCount != 3 {
		t.Fatalf("配置 %+v", layout)
	}
	output := t.TempDir()
	if err := generateSeekSpriteFromIndex(context.Background(), videoPath, output, layout); err != nil {
		t.Fatal(err)
	}
	sheets := readSheets(t, output)
	if got := frameSeconds(t, sheets, layout, 2); got < 8.5 || got > 9.5 {
		t.Fatalf("末尾のコマの時刻が %.2f 秒（最後のキーフレーム 9 秒のはず）", got)
	}
}

// 区間より長い GOP では、キーフレームの無い区間は直前のキーフレームを複製する。
func TestGenerateSeekSpriteFromIndexRepeatsKeyframeForLongGOP(t *testing.T) {
	requireEncoder(t, "libx264")
	videoPath := filepath.Join(t.TempDir(), "long-gop.mp4")
	runFFmpeg(t, "-f", "lavfi", "-i", timeGraySource("64x64", "30"),
		"-c:v", "libx264", "-g", "100", "-keyint_min", "100", "-sc_threshold", "0", "-crf", "10", "-y", videoPath)

	layout := domain.NewSeekSpriteLayout(30_000)
	output := t.TempDir()
	if err := generateSeekSpriteFromIndex(context.Background(), videoPath, output, layout); err != nil {
		t.Fatal(err)
	}
	sheets := readSheets(t, output)
	// キーフレームは 0, 10, 20 秒。区間 5〜10 秒には無いので 0 秒の場面になる。
	for _, check := range []struct {
		frame int
		want  float64
	}{{0, 0}, {1, 0}, {2, 10}, {3, 10}, {4, 20}, {5, 20}} {
		if got := frameSeconds(t, sheets, layout, check.frame); got < check.want-0.5 || got > check.want+0.5 {
			t.Errorf("コマ %d は %.2f 秒（%.0f 秒の場面のはず）", check.frame, got, check.want)
		}
	}
}

func TestGenerateSeekSpriteFromIndexReadsHEVC(t *testing.T) {
	requireEncoder(t, "libx265")
	videoPath := filepath.Join(t.TempDir(), "clock.mov")
	runFFmpeg(t, "-f", "lavfi", "-i", timeGraySource("64x64", "12"),
		"-c:v", "libx265", "-x265-params", "keyint=10:min-keyint=10:log-level=none", "-crf", "10",
		"-tag:v", "hvc1", "-y", videoPath)

	layout := domain.NewSeekSpriteLayout(12_000)
	output := t.TempDir()
	if err := generateSeekSpriteFromIndex(context.Background(), videoPath, output, layout); err != nil {
		t.Fatal(err)
	}
	sheets := readSheets(t, output)
	for k := range layout.FrameCount {
		startSec := float64(k) * float64(layout.IntervalMs) / 1000
		if got := frameSeconds(t, sheets, layout, k); got < startSec-0.5 || got > startSec+0.5 {
			t.Errorf("コマ %d の時刻 %.2f 秒（区間の先頭 %.0f 秒のはず）", k, got, startSec)
		}
	}
}

// 索引から取れない入力（MP4 でない容器や、H.264／HEVC でない映像）は
// errSeekIndexUnsupported になり、GenerateSeekSprite は区間ごとの抽出で作る。
func TestGenerateSeekSpriteFromIndexRejectsUnsupportedInput(t *testing.T) {
	requireEncoder(t, "libx264")
	dir := t.TempDir()
	mpeg4 := filepath.Join(dir, "mpeg4.mp4")
	runFFmpeg(t, "-f", "lavfi", "-i", timeGraySource("64x64", "10"), "-c:v", "mpeg4", "-y", mpeg4)
	matroska := filepath.Join(dir, "h264.mkv")
	runFFmpeg(t, "-f", "lavfi", "-i", timeGraySource("64x64", "10"), "-c:v", "libx264", "-y", matroska)

	for _, videoPath := range []string{mpeg4, matroska} {
		layout := domain.NewSeekSpriteLayout(10_000)
		err := generateSeekSpriteFromIndex(context.Background(), videoPath, t.TempDir(), layout)
		if !errors.Is(err, errSeekIndexUnsupported) {
			t.Errorf("%s: %v（errSeekIndexUnsupported のはず）", filepath.Base(videoPath), err)
		}
		output, layout := generateSprite(t, videoPath, 10_000)
		sheets := readSheets(t, output)
		if got := frameSeconds(t, sheets, layout, 1); got < 4.5 || got > 6 {
			t.Errorf("%s: コマ 1 の時刻 %.2f 秒（5 秒のはず）", filepath.Base(videoPath), got)
		}
	}
}

// 区間ごとの抽出でも、映像の終わりから始まる区間は失敗せず直前のコマを複製する。
func TestGenerateSeekSpriteParallelRepeatsPreviousFrameAfterVideoEnds(t *testing.T) {
	requireFFmpeg(t)
	videoPath := filepath.Join(t.TempDir(), "clock.mp4")
	runFFmpeg(t, "-f", "lavfi", "-i", timeGraySource("64x64", "10"), "-c:v", "mpeg4", "-q:v", "2", "-y", videoPath)

	layout := domain.NewSeekSpriteLayout(10_010)
	output := t.TempDir()
	if err := generateSeekSpriteParallel(context.Background(), videoPath, output, layout); err != nil {
		t.Fatalf("映像の後ろの区間で失敗した: %v", err)
	}
	sheets := readSheets(t, output)
	if got := frameSeconds(t, sheets, layout, 2); got < 4.5 || got > 6 {
		t.Fatalf("末尾のコマの時刻が %.2f 秒（直前のコマの 5 秒のはず）", got)
	}
}

// 壊れた索引（途中で切れた、値が化けた moov）でもパニックせず、エラーか何らかの
// キーフレームの選択で終わる。
func TestParseKeyframeTrackSurvivesDamagedIndex(t *testing.T) {
	videoPath := makeH264Clock(t, "clock.mp4", "10")
	file, err := os.Open(videoPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	info, _ := file.Stat()
	movie, err := readMovieBox(context.Background(), file, info.Size())
	if err != nil {
		t.Fatal(err)
	}
	layout := domain.NewSeekSpriteLayout(10_000)
	parse := func(name string, data []byte) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("%s でパニックした: %v", name, r)
			}
		}()
		track, err := parseKeyframeTrack(data)
		if err != nil {
			return
		}
		for _, pick := range track.pick(layout) {
			if pick < 0 || pick >= len(track.keyframes) {
				t.Fatalf("%s で範囲外のキーフレーム %d を選んだ", name, pick)
			}
		}
	}
	for cut := 0; cut < len(movie); cut += 7 {
		parse("切り詰め", movie[:cut])
	}
	for i := 0; i < len(movie); i += 3 {
		for _, value := range []byte{0x00, 0xff, 0x7f} {
			damaged := bytes.Clone(movie)
			damaged[i] = value
			parse("書き換え", damaged)
		}
	}
}

// slowGraySource は 1 秒あたり 0.5 ずつ明るくなる入力で、400 秒で 216 に届く。
func slowGraySource(seconds string) string {
	return "color=c=black:s=64x64:r=10:d=" + seconds + ",format=yuv420p,geq=lum='16+T/2':cb=128:cr=128"
}

// 離れたキーフレームを続けて復号しても、コマは時刻の順に並ぶ。IDR だけの動画
// （1 本の列で復号する）と、open GOP の CRA や IDR でない I フレームを含む動画
// （1 枚ずつ別の入力にする）の両方を、POC が巻き戻る間隔（90 秒）で確かめる。
func TestGenerateSeekSpriteKeepsOrderOfDistantKeyframes(t *testing.T) {
	for _, tc := range []struct {
		name    string
		encoder string
		args    []string
	}{
		{"hevc open GOP", "libx265", []string{"-c:v", "libx265", "-x265-params", "keyint=25:log-level=none", "-tag:v", "hvc1"}},
		{"hevc closed GOP", "libx265", []string{"-c:v", "libx265", "-x265-params", "keyint=25:no-open-gop=1:log-level=none", "-tag:v", "hvc1"}},
		{"h264 open GOP", "libx264", []string{"-c:v", "libx264", "-g", "25", "-x264-params", "open-gop=1"}},
		{"h264 closed GOP", "libx264", []string{"-c:v", "libx264", "-g", "25"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requireEncoder(t, tc.encoder)
			videoPath := filepath.Join(t.TempDir(), "clock.mp4")
			args := append([]string{"-f", "lavfi", "-i", slowGraySource("400")}, tc.args...)
			runFFmpeg(t, append(args, "-y", videoPath)...)

			layout := domain.SeekSpriteLayout{IntervalMs: 90_000, FrameCount: 5, Columns: 9, Rows: 9, SheetCount: 1}
			output := t.TempDir()
			if err := generateSeekSpriteFromIndex(context.Background(), videoPath, output, layout); err != nil {
				t.Fatal(err)
			}
			sheets := readSheets(t, output)
			previous := -100
			for k := range layout.FrameCount {
				luma := frameLuma(t, sheets, layout, k)
				if luma <= previous+10 {
					t.Errorf("コマ %d の輝度 %d が前のコマ（%d）より十分に明るくない", k, luma, previous)
				}
				previous = luma
			}
		})
	}
}

func editBoxes(t *testing.T, entries ...editEntry) []mp4Box {
	t.Helper()
	elst := editList{entries: entries}.serialize()
	edts, err := serializeBoxes([]mp4Box{{boxType: "elst", payload: elst}})
	if err != nil {
		t.Fatal(err)
	}
	return []mp4Box{{boxType: "edts", payload: edts}}
}

func TestPresentationWindowFollowsEditList(t *testing.T) {
	normal := [4]byte{0, 1, 0, 0}
	// media timescale 90000、movie timescale 1000。
	for _, tc := range []struct {
		name      string
		entries   []editEntry
		wantShift int64
		wantEndUs int64
	}{
		{"no edit list", nil, 0, 0},
		{"media time skips B-frame delay", []editEntry{{segmentDuration: 10_000, mediaTime: 3000, rate: normal}}, 3000, 10_000_000},
		{"empty edit delays start by one second", []editEntry{
			{segmentDuration: 1000, mediaTime: -1, rate: normal},
			{segmentDuration: 10_000, mediaTime: 0, rate: normal},
		}, -90_000, 11_000_000},
		{"edit plays 5 to 12 seconds of the media", []editEntry{{segmentDuration: 7000, mediaTime: 450_000, rate: normal}}, 450_000, 7_000_000},
		{"zero duration leaves the end open", []editEntry{{segmentDuration: 0, mediaTime: 0, rate: normal}}, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var trak []mp4Box
			if tc.entries != nil {
				trak = editBoxes(t, tc.entries...)
			}
			shift, endUs, err := presentationWindow(trak, 90_000, 1000)
			if err != nil || shift != tc.wantShift || endUs != tc.wantEndUs {
				t.Fatalf("presentationWindow = %d, %d, %v（%d, %d のはず）", shift, endUs, err, tc.wantShift, tc.wantEndUs)
			}
		})
	}
	// 途中を切り取った（空でない edit が複数ある）トラックは扱わない。
	cut := editBoxes(t,
		editEntry{segmentDuration: 5000, mediaTime: 0, rate: normal},
		editEntry{segmentDuration: 5000, mediaTime: 900_000, rate: normal})
	if _, _, err := presentationWindow(cut, 90_000, 1000); !errors.Is(err, errSeekIndexUnsupported) {
		t.Fatalf("編集が複数ある edit list で %v（errSeekIndexUnsupported のはず）", err)
	}
}

func fullBox(entries ...[]byte) []byte {
	out := binary.BigEndian.AppendUint32(nil, 0)
	out = binary.BigEndian.AppendUint32(out, uint32(len(entries)))
	for _, entry := range entries {
		out = append(out, entry...)
	}
	return out
}

func be32(values ...uint32) []byte {
	var out []byte
	for _, value := range values {
		out = binary.BigEndian.AppendUint32(out, value)
	}
	return out
}

// 数十バイトの表が 1600 万個の sample を名乗っても、値を取り出すのはキーフレームの
// 分だけで、sample の数だけの配列は作らない。
func TestSampleTableDoesNotAllocatePerSample(t *testing.T) {
	const count = 1 << 24
	stsz := append(be32(0, 1000), be32(count)...)
	boxes := []mp4Box{
		{boxType: "stsz", payload: stsz},
		{boxType: "stts", payload: fullBox(be32(count, 1))},
		{boxType: "stsc", payload: fullBox(be32(1, count, 1))},
		{boxType: "stco", payload: fullBox(be32(4096))},
		{boxType: "stss", payload: fullBox(be32(1), be32(count/2+1))},
	}
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	table, err := readSampleTable(boxes)
	if err != nil {
		t.Fatal(err)
	}
	sync, err := syncSamples(boxes, table.count)
	if err != nil {
		t.Fatal(err)
	}
	times, err := table.decodeTimes(sync)
	if err != nil {
		t.Fatal(err)
	}
	offsets, err := table.offsets(sync)
	if err != nil {
		t.Fatal(err)
	}
	runtime.ReadMemStats(&after)
	if grown := after.TotalAlloc - before.TotalAlloc; grown > 1<<20 {
		t.Errorf("表を読むのに %d バイト確保した", grown)
	}
	if times[1] != count/2 || offsets[1] != 4096+count/2*1000 {
		t.Errorf("2 枚目のキーフレームの時刻 %d・位置 %d", times[1], offsets[1])
	}
}

func TestAnnexBReportsIDR(t *testing.T) {
	for _, tc := range []struct {
		format string
		header []byte
		want   bool
	}{
		{"h264", []byte{0x65, 0x88}, true},  // 型 5（IDR）
		{"h264", []byte{0x41, 0x9a}, false}, // 型 1（IDR でないスライス）
		{"hevc", []byte{0x26, 0x01}, true},  // 型 19（IDR_W_RADL）
		{"hevc", []byte{0x28, 0x01}, true},  // 型 20（IDR_N_LP）
		{"hevc", []byte{0x2a, 0x01}, false}, // 型 21（CRA）
	} {
		track := &keyframeTrack{format: tc.format, lengthSize: 4, parameterSets: [][]byte{{0x67}}}
		sample := append(be32(uint32(len(tc.header))), tc.header...)
		out, idr, err := track.annexB(sample)
		if err != nil || idr != tc.want {
			t.Errorf("%s % x: IDR=%v, %v（%v のはず）", tc.format, tc.header, idr, err, tc.want)
		}
		if want := append([]byte{0, 0, 0, 1, 0x67, 0, 0, 0, 1}, tc.header...); !bytes.Equal(out, want) {
			t.Errorf("%s: 開始コード区切りの並び % x（% x のはず）", tc.format, out, want)
		}
	}
}

// stss の重複や順不同、大きさの異なる sample、複数の stsc の並びでも、キーフレームの
// 位置と時刻を正しく求める。
func TestSampleTableFindsKeyframesAcrossChunks(t *testing.T) {
	// sample 0〜5 の大きさは 10, 20, 30, 40, 50, 60。chunk 0（位置 1000）に 2 個、
	// chunk 1（位置 5000）と chunk 2（位置 9000）に 2 個ずつ入る。
	boxes := []mp4Box{
		{boxType: "stsz", payload: append(be32(0, 0, 6), be32(10, 20, 30, 40, 50, 60)...)},
		{boxType: "stts", payload: fullBox(be32(6, 100))},
		{boxType: "ctts", payload: fullBox(be32(6, 200))},
		{boxType: "stsc", payload: fullBox(be32(1, 2, 1), be32(2, 2, 1))},
		{boxType: "stco", payload: fullBox(be32(1000), be32(5000), be32(9000))},
		{boxType: "stss", payload: fullBox(be32(6), be32(4), be32(4), be32(1))},
	}
	table, err := readSampleTable(boxes)
	if err != nil {
		t.Fatal(err)
	}
	sync, err := syncSamples(boxes, table.count)
	if err != nil {
		t.Fatal(err)
	}
	if want := []int{0, 3, 5}; !slices.Equal(sync, want) {
		t.Fatalf("キーフレーム %v（%v のはず）", sync, want)
	}
	times, err := table.decodeTimes(sync)
	if err != nil {
		t.Fatal(err)
	}
	offsets, err := table.offsets(sync)
	if err != nil {
		t.Fatal(err)
	}
	composition := table.compositionOffsets(sync)
	if want := []int64{0, 300, 500}; !slices.Equal(times, want) {
		t.Errorf("復号時刻 %v（%v のはず）", times, want)
	}
	if want := []int64{1000, 5030, 9050}; !slices.Equal(offsets, want) {
		t.Errorf("位置 %v（%v のはず）", offsets, want)
	}
	if want := []int64{200, 200, 200}; !slices.Equal(composition, want) {
		t.Errorf("表示時刻との差 %v（%v のはず）", composition, want)
	}
}

// 表示行列で回転する動画のコマは、ffmpeg が容器から読んで自動回転したのと同じ向きになる。
// 反転を含む表示行列は索引からは作らず、区間ごとの抽出に任せる。
func TestGenerateSeekSpriteFromIndexAppliesDisplayRotation(t *testing.T) {
	requireEncoder(t, "libx264")
	dir := t.TempDir()
	base := filepath.Join(dir, "base.mp4")
	runFFmpeg(t, "-f", "lavfi", "-i", "testsrc2=s=128x72:r=10:d=12", "-c:v", "libx264", "-g", "10", "-y", base)

	for _, rotation := range []string{"90", "180", "270"} {
		t.Run(rotation, func(t *testing.T) {
			videoPath := filepath.Join(dir, "rotated-"+rotation+".mp4")
			runFFmpeg(t, "-display_rotation", rotation, "-i", base, "-c", "copy", "-y", videoPath)
			layout := domain.NewSeekSpriteLayout(12_000)
			output := t.TempDir()
			if err := generateSeekSpriteFromIndex(context.Background(), videoPath, output, layout); err != nil {
				t.Fatal(err)
			}
			sheet := readSheets(t, output)[0]
			expectedPath := filepath.Join(t.TempDir(), "expected.png")
			runFFmpeg(t, "-ss", "5", "-i", videoPath, "-frames:v", "1", "-vf", seekSpriteFastScale, "-y", expectedPath)
			expected := decodePNG(t, expectedPath)

			w, h := expected.Bounds().Dx(), expected.Bounds().Dy()
			if b := sheet.Bounds(); b.Dx() != w*layout.Columns || b.Dy() != h*layout.Rows {
				t.Fatalf("シートの大きさ %v（コマ %dx%d のはず）", b, w, h)
			}
			var diff, count int
			for y := range h {
				for x := range w {
					got := color.GrayModel.Convert(sheet.At(w+x, y)).(color.Gray).Y
					want := color.GrayModel.Convert(expected.At(x, y)).(color.Gray).Y
					diff += abs(int(got) - int(want))
					count++
				}
			}
			if mean := diff / count; mean > 12 {
				t.Errorf("コマ 1 と自動回転した画像の輝度の差が平均 %d", mean)
			}
		})
	}

	flipped := filepath.Join(dir, "flipped.mp4")
	runFFmpeg(t, "-display_hflip", "-i", base, "-c", "copy", "-y", flipped)
	err := generateSeekSpriteFromIndex(context.Background(), flipped, t.TempDir(), domain.NewSeekSpriteLayout(12_000))
	if !errors.Is(err, errSeekIndexUnsupported) {
		t.Fatalf("反転する表示行列で %v（errSeekIndexUnsupported のはず）", err)
	}
}

func decodePNG(t *testing.T, path string) image.Image {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	img, err := png.Decode(file)
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// stalledReader は読み取りが戻らない置き場の代わりである。
type stalledReader struct{ release chan struct{} }

func (r stalledReader) ReadAt([]byte, int64) (int, error) {
	<-r.release
	return 0, io.EOF
}

// 読み取りが止まっても、期限が来れば索引の読み取りを待たずに戻る。
func TestReadMovieBoxReturnsWhenContextEnds(t *testing.T) {
	reader := stalledReader{release: make(chan struct{})}
	defer close(reader.release)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := readMovieBox(ctx, reader, 1<<20)
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("readMovieBox = %v（期限切れのはず）", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("期限が来ても readMovieBox が戻らなかった")
	}
}
