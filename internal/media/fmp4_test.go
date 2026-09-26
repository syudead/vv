package media

import (
	"bytes"
	"cmp"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

func testBox(boxType string, payload ...[]byte) []byte {
	body := bytes.Join(payload, nil)
	out := binary.BigEndian.AppendUint32(nil, uint32(boxHeaderSize+len(body)))
	out = append(out, boxType...)
	return append(out, body...)
}

// testInitSegment は ffmpeg の delay_moov の出力と同じ形の先頭部分（ftyp と moov）を作る。
// delays は track ごとの開始時刻（movie timescale）で、正なら先頭に空の edit を置く。
func testInitSegment(timescale uint32, delays []uint32) []byte {
	mvhd := make([]byte, 100)
	binary.BigEndian.PutUint32(mvhd[12:16], timescale)
	moov := [][]byte{testBox("mvhd", mvhd)}
	for index, delay := range delays {
		var entries [][]byte
		if delay > 0 {
			entries = append(entries, testEditEntry(delay, -1))
		}
		entries = append(entries, testEditEntry(0, 0))
		elst := binary.BigEndian.AppendUint32(make([]byte, 4), uint32(len(entries)))
		elst = append(elst, bytes.Join(entries, nil)...)
		tkhd := make([]byte, 84)
		binary.BigEndian.PutUint32(tkhd[12:16], uint32(index+1))
		moov = append(moov, testBox("trak", testBox("tkhd", tkhd), testBox("edts", testBox("elst", elst)), testBox("mdia")))
	}
	moov = append(moov, testBox("mvex"))
	return append(testBox("ftyp", []byte("isom\x00\x00\x02\x00")), testBox("moov", moov...)...)
}

func testEditEntry(duration uint32, mediaTime int32) []byte {
	entry := binary.BigEndian.AppendUint32(nil, duration)
	entry = binary.BigEndian.AppendUint32(entry, uint32(mediaTime))
	return append(entry, 0, 1, 0, 0)
}

func TestRebaseEditListsKeepsTrackOffsets(t *testing.T) {
	tests := []struct {
		name      string
		timescale uint32
		delays    []uint32
		wantMs    int64
		want      []uint32
	}{
		{"video first", 1000, []uint32{25021, 25024}, 25021, []uint32{0, 3}},
		{"audio first", 1000, []uint32{25000, 24981}, 24981, []uint32{19, 0}},
		{"other timescale", 90000, []uint32{900000, 900000}, 10000, []uint32{0, 0}},
		{"starts at zero", 1000, []uint32{0, 21}, 0, []uint32{0, 21}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rewritten, startMs, err := rebaseEditLists(testInitSegment(tc.timescale, tc.delays))
			if err != nil {
				t.Fatal(err)
			}
			if startMs != tc.wantMs {
				t.Errorf("startMs = %d, want %d", startMs, tc.wantMs)
			}
			if want := testInitSegment(tc.timescale, tc.want); !bytes.Equal(rewritten, want) {
				t.Errorf("rewritten = %x\nwant      %x", rewritten, want)
			}
		})
	}
}

func TestRebaseEditListsRejectsMalformedMoov(t *testing.T) {
	valid := testInitSegment(1000, []uint32{1000})
	tests := map[string][]byte{
		"no moov":       testBox("ftyp", []byte("isom")),
		"truncated":     valid[:len(valid)-3],
		"no timescale":  append(testBox("ftyp"), testBox("moov", testBox("mvhd", make([]byte, 100)), testBox("trak"))...),
		"no track":      append(testBox("ftyp"), testBox("moov", testBox("mvhd", append(make([]byte, 12), 0, 0, 3, 232)))...),
		"short elst":    append(testBox("ftyp"), testBox("moov", testBox("mvhd", append(make([]byte, 12), 0, 0, 3, 232)), testBox("trak", testBox("edts", testBox("elst", []byte{0, 0, 0, 0, 0, 0, 0, 2}))))...),
		"elst version2": append(testBox("ftyp"), testBox("moov", testBox("mvhd", append(make([]byte, 12), 0, 0, 3, 232)), testBox("trak", testBox("edts", testBox("elst", []byte{2, 0, 0, 0, 0, 0, 0, 0}))))...),
	}
	for name, segment := range tests {
		t.Run(name, func(t *testing.T) {
			if _, _, err := rebaseEditLists(segment); !errors.Is(err, errInvalidInitSegment) {
				t.Errorf("err = %v", err)
			}
		})
	}
}

func TestReadInitSegmentStopsAfterMoov(t *testing.T) {
	segment := testInitSegment(1000, []uint32{1000, 1003})
	reader := bytes.NewReader(append(append([]byte(nil), segment...), testBox("moof", []byte("fragment"))...))
	got, err := readInitSegment(reader)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, segment) {
		t.Errorf("init = %x", got)
	}
	rest, _ := io.ReadAll(reader)
	if !bytes.Equal(rest, testBox("moof", []byte("fragment"))) {
		t.Errorf("rest = %x", rest)
	}
}

func TestReadInitSegmentFailures(t *testing.T) {
	segment := testInitSegment(1000, []uint32{1000})
	tests := []struct {
		name  string
		input []byte
		want  error
	}{
		{"empty", nil, io.EOF},
		{"ends before moov", testBox("ftyp", []byte("isom")), io.ErrUnexpectedEOF},
		{"truncated moov", segment[:len(segment)-1], io.ErrUnexpectedEOF},
		{"not mp4", []byte("fragment"), errInvalidInitSegment},
		{"moof before moov", testBox("moof"), errInvalidInitSegment},
		{"too large", append(binary.BigEndian.AppendUint32(nil, maxInitSegmentSize+1), "moov"...), errInvalidInitSegment},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := readInitSegment(bytes.NewReader(tc.input)); !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

// recordingTranscoder は本物の ffmpeg/ffprobe を起動し、FFmpeg の引数を記録する。
type recordingTranscoder struct {
	*LiveTranscoder
	mu   sync.Mutex
	args [][]string
}

func newRecordingTranscoder() *recordingTranscoder {
	recording := &recordingTranscoder{LiveTranscoder: NewLiveTranscoder(nil)}
	recording.commandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		if name == transcodeCommand {
			recording.mu.Lock()
			recording.args = append(recording.args, args)
			recording.mu.Unlock()
		}
		return exec.CommandContext(ctx, name, args...)
	}
	return recording
}

func (r *recordingTranscoder) videoCodecs() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var codecs []string
	for _, args := range r.args {
		if index := slices.Index(args, "-c:v"); index >= 0 && index+1 < len(args) {
			codecs = append(codecs, args[index+1])
		}
	}
	return codecs
}

// generateFixture は 30fps の H.264 と AAC の動画を作る。gop はキーフレームの間隔（フレーム）。
func generateFixture(t *testing.T, path string, seconds, gop int) {
	t.Helper()
	generate := exec.Command(transcodeCommand,
		"-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", fmt.Sprintf("testsrc=size=160x90:rate=30:duration=%d", seconds),
		"-f", "lavfi", "-i", fmt.Sprintf("sine=frequency=440:sample_rate=48000:duration=%d", seconds),
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p", "-g", strconv.Itoa(gop),
		"-keyint_min", strconv.Itoa(gop), "-sc_threshold", "0",
		"-c:a", "aac", path,
	)
	if output, err := generate.CombinedOutput(); err != nil {
		t.Fatalf("fixture生成: %v: %s", err, output)
	}
}

func remuxFixture(t *testing.T, input, output string) {
	t.Helper()
	remux := exec.Command(transcodeCommand, "-hide_banner", "-loglevel", "error", "-y", "-i", input, "-c", "copy", output)
	if out, err := remux.CombinedOutput(); err != nil {
		t.Fatalf("fixture変換: %v: %s", err, out)
	}
}

func probeFixture(t *testing.T, path string) domain.TranscodeProbe {
	t.Helper()
	output, err := exec.Command(probeCommand, probeArgs(path)...).Output()
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := parseTranscodeProbe(output)
	if err != nil {
		t.Fatal(err)
	}
	return metadata
}

type mediaPacket struct {
	pts      float64
	keyframe bool
	hash     string
}

// packets は stream の packet を読む。selector は "v:0" か "a:0"。
func packets(t *testing.T, path, selector string) []mediaPacket {
	t.Helper()
	output, err := exec.Command(probeCommand,
		"-v", "error", "-select_streams", selector, "-show_data_hash", "MD5",
		"-show_entries", "packet=pts_time,flags,data_hash", "-of", "csv=p=0", path,
	).Output()
	if err != nil {
		t.Fatal(err)
	}
	var result []mediaPacket
	for line := range strings.SplitSeq(strings.TrimSpace(string(output)), "\n") {
		fields := strings.Split(line, ",")
		if len(fields) < 3 {
			t.Fatalf("packet の行を読めません: %q", line)
		}
		pts, err := strconv.ParseFloat(fields[0], 64)
		if err != nil {
			t.Fatalf("pts を読めません: %q", line)
		}
		result = append(result, mediaPacket{pts: pts, keyframe: strings.Contains(fields[1], "K"), hash: fields[2]})
	}
	slices.SortFunc(result, func(a, b mediaPacket) int { return cmp.Compare(a.pts, b.pts) })
	return result
}

func formatStartTime(t *testing.T, path string) float64 {
	t.Helper()
	output, err := exec.Command(probeCommand, "-v", "error", "-show_entries", "format=start_time", "-of", "csv=p=0", path).Output()
	if err != nil {
		t.Fatal(err)
	}
	start, err := strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
	if err != nil {
		t.Fatal(err)
	}
	return start
}

// transcodeToFile は変換を最後まで読み、出力をファイルに書く。
func transcodeToFile(t *testing.T, transcoder *recordingTranscoder, input string, metadata domain.TranscodeProbe, startMs int64) (string, domain.LiveTranscode) {
	t.Helper()
	info, err := os.Stat(input)
	if err != nil {
		t.Fatal(err)
	}
	started, err := transcoder.Start(context.Background(), domain.LiveTranscodeRequest{
		Path: input, Source: domain.FileStampOf(info), Probe: &metadata, StartMs: startMs,
		StartupDeadline: time.Now().Add(20 * time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "output.mp4")
	file, err := os.Create(output)
	if err != nil {
		t.Fatal(err)
	}
	_, copyErr := io.Copy(file, started.Stream)
	closeErr := file.Close()
	waitErr := started.Wait()
	if copyErr != nil || closeErr != nil || waitErr != nil {
		t.Fatalf("変換: copy=%v close=%v wait=%v", copyErr, closeErr, waitErr)
	}
	return output, started
}

// assertCopySeek は途中からのコピーの出力を元動画と比べる。最初の映像 packet は指定位置の
// 直前のキーフレームそのもので、時間軸は 0 から始まり、出力の時刻に実際の開始位置を
// 足すと、映像も音声も元動画の時刻になる（音声との差が元動画と同じ）。
func assertCopySeek(t *testing.T, input, output string, requestedMs, actualMs int64) {
	t.Helper()
	sourceStart := formatStartTime(t, input)
	var keyframe mediaPacket
	for _, packet := range packets(t, input, "v:0") {
		if packet.keyframe && packet.pts-sourceStart <= float64(requestedMs)/1000 {
			keyframe = packet
		}
	}
	if keyframe.hash == "" {
		t.Fatal("元動画に指定位置より前のキーフレームがありません")
	}
	// 実際の開始位置は最も早く始まる track の時刻で、音声が映像のキーフレームの少し前から
	// 始まることがある。
	if keyframeMs := (keyframe.pts - sourceStart) * 1000; float64(actualMs) > keyframeMs+1 || float64(actualMs) < keyframeMs-100 {
		t.Errorf("実際の開始位置 = %d ms, 直前のキーフレーム = %.0f ms", actualMs, keyframeMs)
	}

	outputVideo := packets(t, output, "v:0")
	if len(outputVideo) == 0 || outputVideo[0].hash != keyframe.hash {
		t.Fatalf("最初の映像 packet が直前のキーフレームではありません")
	}
	outputAudio := packets(t, output, "a:0")
	if len(outputAudio) == 0 {
		t.Fatal("出力に音声がありません")
	}
	if first := math.Min(outputVideo[0].pts, outputAudio[0].pts); math.Abs(first) > 0.0005 {
		t.Errorf("出力の時間軸が %.3f 秒から始まります", first)
	}
	var sourceAudio *mediaPacket
	for _, packet := range packets(t, input, "a:0") {
		if packet.hash == outputAudio[0].hash {
			sourceAudio = &packet
			break
		}
	}
	if sourceAudio == nil {
		t.Fatal("出力の最初の音声 packet が元動画にありません")
	}
	actual := float64(actualMs) / 1000
	for name, pair := range map[string][2]float64{
		"video": {outputVideo[0].pts, keyframe.pts},
		"audio": {outputAudio[0].pts, sourceAudio.pts},
	} {
		if got, want := pair[0]+actual, pair[1]-sourceStart; math.Abs(got-want) > 0.0015 {
			t.Errorf("%s: 出力の時刻 + 実際の開始位置 = %.4f 秒, 元動画 = %.4f 秒", name, got, want)
		}
	}
}

// H.264 の MKV を途中から変換すると映像をコピーし、直前のキーフレームから始まる
// （親 Issue #371 受け入れ条件 1・4 の一部）。
func TestCopySeekStartsAtPreviousKeyframeWithFFmpeg(t *testing.T) {
	requireFFmpeg(t)
	directory := t.TempDir()
	mov := filepath.Join(directory, "source.mov")
	generateFixture(t, mov, 30, 250)
	mkv := filepath.Join(directory, "source.mkv")
	remuxFixture(t, mov, mkv)

	for _, input := range []string{mkv, mov} {
		t.Run(filepath.Ext(input), func(t *testing.T) {
			metadata := probeFixture(t, input)
			if !videoCanCopy(metadata.Video) || metadata.Audio == nil || !audioCanCopy(*metadata.Audio) {
				t.Fatalf("fixture がコピーできません: %+v", metadata)
			}
			transcoder := newRecordingTranscoder()
			output, started := transcodeToFile(t, transcoder, input, metadata, 27000)
			if codecs := transcoder.videoCodecs(); !slices.Equal(codecs, []string{"copy"}) {
				t.Fatalf("映像の扱い = %v", codecs)
			}
			if started.StartMs >= 27000 || started.StartMs < 24000 {
				t.Errorf("StartMs = %d", started.StartMs)
			}
			assertCopySeek(t, input, output, 27000, started.StartMs)
		})
	}
}

// キーフレームの間隔が差の上限より長い動画は、エンコードし直して指定位置から始める
// （受け入れ条件 3）。
func TestCopySeekFallsBackToEncodeForSparseKeyframesWithFFmpeg(t *testing.T) {
	requireFFmpeg(t)
	directory := t.TempDir()
	mov := filepath.Join(directory, "sparse.mov")
	generateFixture(t, mov, 25, 30*int(domain.CopySeekAllowance.Seconds()+5))
	mkv := filepath.Join(directory, "sparse.mkv")
	remuxFixture(t, mov, mkv)

	metadata := probeFixture(t, mkv)
	transcoder := newRecordingTranscoder()
	requestedMs := domain.CopySeekAllowance.Milliseconds() + 2000
	output, started := transcodeToFile(t, transcoder, mkv, metadata, requestedMs)
	if codecs := transcoder.videoCodecs(); !slices.Equal(codecs, []string{"copy", "libx264"}) {
		t.Fatalf("映像の扱い = %v", codecs)
	}
	if started.StartMs != requestedMs {
		t.Errorf("StartMs = %d, want %d", started.StartMs, requestedMs)
	}
	video := packets(t, output, "v:0")
	sourceDuration := 25.0
	if len(video) == 0 || video[0].pts > 0.1 {
		t.Fatalf("出力の映像が 0 秒から始まりません")
	}
	// 指定位置から始めたので、出力の長さは元動画の残りに等しい。
	if last := video[len(video)-1].pts; math.Abs(last-(sourceDuration-float64(requestedMs)/1000)) > 0.1 {
		t.Errorf("出力の最後の映像 = %.3f 秒", last)
	}
}
