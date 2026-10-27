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
	"strings"
	"testing"
	"testing/synctest"
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

// B frames require ctts and edit lists to recover presentation times.
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
			movie, _, err := readMovieBox(context.Background(), file, info.Size())
			if err != nil {
				t.Fatal(err)
			}
			track, err := parseKeyframeTrack(movie, nil)
			if err != nil {
				t.Fatal(err)
			}
			if track.format != "h264" || len(track.parameterSets) < 2 {
				t.Fatalf("形式 %q、パラメータセット %d 個", track.format, len(track.parameterSets))
			}
			if len(track.keyframes) != 10 {
				t.Fatalf("キーフレームが %d 枚（10 枚のはず）", len(track.keyframes))
			}
			// The edit list cancels B-frame delay, placing keyframes at whole seconds.
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

// When the final interval starts at video end, reuse the preceding keyframe rather than failing.
func TestGenerateSeekSpriteFromIndexUsesPreviousKeyframeAfterVideoEnds(t *testing.T) {
	videoPath := makeH264Clock(t, "clock.mp4", "10")
	layout := domain.NewSeekSpriteLayout(10_010)
	if layout.FrameCount != 11 {
		t.Fatalf("配置 %+v", layout)
	}
	output := t.TempDir()
	if err := generateSeekSpriteFromIndex(context.Background(), videoPath, output, layout); err != nil {
		t.Fatal(err)
	}
	sheets := readSheets(t, output)
	if got := frameSeconds(t, sheets, layout, 10); got < 8.5 || got > 9.5 {
		t.Fatalf("末尾のコマの時刻が %.2f 秒（最後のキーフレーム 9 秒のはず）", got)
	}
}

func TestGenerateSeekSpriteFromIndexRepeatsKeyframeForLongGOP(t *testing.T) {
	requireEncoder(t, "libx264")
	videoPath := filepath.Join(t.TempDir(), "long-gop.mp4")
	runFFmpeg(t, "-f", "lavfi", "-i", timeGraySource("64x64", "30"),
		"-c:v", "libx264", "-g", "70", "-keyint_min", "70", "-sc_threshold", "0", "-crf", "10", "-y", videoPath)

	layout := domain.SeekSpriteLayout{IntervalMs: 5000, FrameCount: 6, Columns: 9, Rows: 9, SheetCount: 1}
	output := t.TempDir()
	if err := generateSeekSpriteFromIndex(context.Background(), videoPath, output, layout); err != nil {
		t.Fatal(err)
	}
	sheets := readSheets(t, output)
	// Keyframes at 0, 7, 14, 21, and 28 seconds make the 15-20 interval use second 14.
	for _, check := range []struct {
		frame int
		want  float64
	}{{0, 0}, {1, 7}, {2, 14}, {3, 14}, {4, 21}, {5, 28}} {
		if got := frameSeconds(t, sheets, layout, check.frame); got < check.want-0.5 || got > check.want+0.5 {
			t.Errorf("コマ %d は %.2f 秒（%.0f 秒の場面のはず）", check.frame, got, check.want)
		}
	}
}

// Sparse keyframes reject indexed extraction so segment seeks can produce distinct scenes.
func TestGenerateSeekSpriteExtractsPerIntervalForSparseKeyframes(t *testing.T) {
	t.Parallel()

	requireEncoder(t, "libx264")
	videoPath := filepath.Join(t.TempDir(), "sparse-keyframes.mp4")
	runFFmpeg(t, "-f", "lavfi", "-i", timeGraySource("64x64", "30"),
		"-c:v", "libx264", "-g", "100", "-keyint_min", "100", "-sc_threshold", "0", "-crf", "10", "-y", videoPath)

	layout := domain.NewSeekSpriteLayout(30_000)
	err := generateSeekSpriteFromIndex(context.Background(), videoPath, t.TempDir(), layout)
	if !errors.Is(err, errSeekIndexUnsupported) {
		t.Fatalf("キーフレーム 3 枚で 30 コマに %v（errSeekIndexUnsupported のはず）", err)
	}
	output := t.TempDir()
	if err := GenerateSeekSprite(context.Background(), videoPath, output, layout); err != nil {
		t.Fatal(err)
	}
	sheets := readSheets(t, output)
	for _, k := range []int{1, 5, 13, 27} {
		startSec := float64(k) * float64(layout.IntervalMs) / 1000
		if got := frameSeconds(t, sheets, layout, k); got < startSec-0.5 || got > startSec+1 {
			t.Errorf("コマ %d の時刻 %.2f 秒（区間の先頭 %.0f 秒のはず）", k, got, startSec)
		}
	}
}

func TestGenerateSeekSpriteFromIndexReadsHEVC(t *testing.T) {
	t.Parallel()

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

// AV1 must not depend on ffmpeg seeking, which decodes from the start of some AV1 MP4 files.
func TestGenerateSeekSpriteFromIndexReadsAV1(t *testing.T) {
	t.Parallel()

	requireEncoder(t, "libsvtav1")
	videoPath := filepath.Join(t.TempDir(), "clock.mp4")
	runFFmpeg(t, "-f", "lavfi", "-i", timeGraySource("64x64", "12"),
		"-c:v", "libsvtav1", "-g", "10", "-crf", "10", "-svtav1-params", "log-level=0", "-y", videoPath)

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

// Zeroed bytes in one keyframe, as in a partly downloaded file, cost only that interval: it reuses the previous
// frame instead of failing the sprite.
func TestGenerateSeekSpriteReusesPreviousFrameForDamagedInterval(t *testing.T) {
	t.Parallel()

	videoPath := makeH264Clock(t, "clock.mp4", "10")
	file, err := os.OpenFile(videoPath, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	info, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	movie, _, err := readMovieBox(context.Background(), file, info.Size())
	if err != nil {
		t.Fatal(err)
	}
	track, err := parseKeyframeTrack(movie, nil)
	if err != nil {
		t.Fatal(err)
	}
	damaged := track.keyframes[5]
	if _, err := file.WriteAt(make([]byte, damaged.size), damaged.offset); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	output, layout := generateSprite(t, videoPath, 10_000)
	sheets := readSheets(t, output)
	for _, check := range []struct {
		frame int
		want  float64
	}{{4, 4}, {5, 4}, {6, 6}} {
		if got := frameSeconds(t, sheets, layout, check.frame); got < check.want-0.5 || got > check.want+0.5 {
			t.Errorf("コマ %d の時刻 %.2f 秒（%.0f 秒のはず）", check.frame, got, check.want)
		}
	}
}

// ffmpeg 7 and later exit with status 69 when most packets of an interval fail to decode; the limit keeps a damaged
// interval an empty one.
func TestSeekFrameArgsToleratesDecodeErrors(t *testing.T) {
	args := strings.Join(seekFrameArgs("/media/a.mp4", 2, 1500), " ")
	for _, want := range []string{"-max_error_rate 1 ", "-ss 3.000 -t 1.500 -i /media/a.mp4", "trim=end=1.500,"} {
		if !strings.Contains(args, want) {
			t.Errorf("引数に %q が無い: %s", want, args)
		}
	}
}

// Fragmented MP4 without sidx or mfra makes ffmpeg seeking read from the start, so its keyframes come from the
// moof boxes instead.
func TestGenerateSeekSpriteFromIndexReadsFragmentedMP4(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		encoder  string
		encoding []string
	}{
		{"h264", "libx264", []string{"-c:v", "libx264", "-g", "10", "-bf", "2", "-crf", "10"}},
		{"av1", "libsvtav1", []string{"-c:v", "libsvtav1", "-g", "10", "-crf", "10", "-svtav1-params", "log-level=0"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			requireEncoder(t, tc.encoder)
			videoPath := filepath.Join(t.TempDir(), "fragmented.mp4")
			args := append([]string{"-f", "lavfi", "-i", timeGraySource("64x64", "12")}, tc.encoding...)
			args = append(args, "-movflags", "frag_keyframe+empty_moov+default_base_moof+skip_trailer", "-y", videoPath)
			runFFmpeg(t, args...)

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
		})
	}
}

// Fragments past the top-level box limit stay unread, so a partial fragment index is rejected rather than returned.
func TestReadMovieBoxRejectsFragmentsPastBoxLimit(t *testing.T) {
	file := append(be32(16), []byte("moov")...)
	file = append(file, be32(8)...)
	file = append(file, []byte("mvex")...)
	for range maxTopLevelBoxes {
		file = append(file, be32(8)...)
		file = append(file, []byte("free")...)
	}
	_, _, err := readMovieBox(context.Background(), bytes.NewReader(file), int64(len(file)))
	if !errors.Is(err, errSeekIndexUnsupported) {
		t.Fatalf("上限を超えたボックスで %v（errSeekIndexUnsupported のはず）", err)
	}
}

// A trun without per-sample fields takes no bytes per sample, so a huge count of non-sync samples, which never reaches
// the keyframe limit, is rejected before the walk.
func TestReadRunRejectsSampleCountPastLimit(t *testing.T) {
	reader := fragmentReader{trackID: 1}
	trun := be32(0, 0xffffffff)
	header := trackFragment{trackID: 1, defaults: sampleDefaults{flags: sampleIsNonSyncSample}}
	if _, err := reader.readRun(trun, header, 0); !errors.Is(err, errSeekIndexUnsupported) {
		t.Fatalf("サンプル数 0xffffffff で %v（errSeekIndexUnsupported のはず）", err)
	}
}

func TestGenerateSeekSpriteFromIndexRejectsUnsupportedInput(t *testing.T) {
	t.Parallel()

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
		if got := frameSeconds(t, sheets, layout, 5); got < 4.5 || got > 6 {
			t.Errorf("%s: コマ 5 の時刻 %.2f 秒（5 秒のはず）", filepath.Base(videoPath), got)
		}
	}
}

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
	if got := frameSeconds(t, sheets, layout, 10); got < 8.5 || got > 10 {
		t.Fatalf("末尾のコマの時刻が %.2f 秒（直前のコマの 9 秒のはず）", got)
	}
}

func TestParseKeyframeTrackSurvivesDamagedIndex(t *testing.T) {
	videoPath := makeH264Clock(t, "clock.mp4", "10")
	file, err := os.Open(videoPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	info, _ := file.Stat()
	movie, _, err := readMovieBox(context.Background(), file, info.Size())
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
		track, err := parseKeyframeTrack(data, nil)
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

// Brightness increases by 0.5 per second, reaching 216 at four hundred seconds.
func slowGraySource(seconds string) string {
	return "color=c=black:s=64x64:r=10:d=" + seconds + ",format=yuv420p,geq=lum='16+T/2':cb=128:cr=128"
}

// Exercise ninety-second POC wraparound for both IDR streams and separately decoded open-GOP
// keyframes.
func TestGenerateSeekSpriteKeepsOrderOfDistantKeyframes(t *testing.T) {
	t.Parallel()

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
			t.Parallel()

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

// A tiny table claiming sixteen million samples must allocate only for actual keyframes.
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
		{"h264", []byte{0x65, 0x88}, true},  // Type 5: IDR.
		{"h264", []byte{0x41, 0x9a}, false}, // Type 1: non-IDR slice.
		{"hevc", []byte{0x26, 0x01}, true},  // Type 19: IDR_W_RADL.
		{"hevc", []byte{0x28, 0x01}, true},  // Type 20: IDR_N_LP.
		{"hevc", []byte{0x2a, 0x01}, false}, // Type 21: CRA.
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

func TestSampleTableFindsKeyframesAcrossChunks(t *testing.T) {
	// Six samples have sizes 10 through 60; chunks at 1000, 5000, and 9000 each contain two samples.
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

// Match container autorotation; unsupported flip matrices fall back to segment extraction.
func TestGenerateSeekSpriteFromIndexAppliesDisplayRotation(t *testing.T) {
	t.Parallel()

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
			runFFmpeg(t, "-ss", "1", "-i", videoPath, "-frames:v", "1", "-vf", seekSpriteFastScale, "-y", expectedPath)
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

type stalledReader struct{ release chan struct{} }

func (r stalledReader) ReadAt([]byte, int64) (int, error) {
	<-r.release
	return 0, io.EOF
}

func TestReadMovieBoxReturnsWhenContextEnds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reader := stalledReader{release: make(chan struct{})}
		defer close(reader.release)
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		done := make(chan error, 1)
		go func() {
			_, _, err := readMovieBox(ctx, reader, 1<<20)
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
	})
}
