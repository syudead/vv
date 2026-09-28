package media

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

const helperEncoderList = `Encoders:
 V..... = Video
 ------
 V....D libx264              libx264 H.264 / AVC / MPEG-4 AVC / MPEG-4 part 10 (codec h264)
 V....D h264_nvenc           NVIDIA NVENC H.264 encoder (codec h264)
 V..... h264_qsv             H.264 / AVC / MPEG-4 AVC / MPEG-4 part 10 (Intel Quick Sync Video acceleration) (codec h264)
 V....D h264_vaapi           H.264/AVC (VAAPI) (codec h264)
`

// scriptedEncoderCheck は -encoders を listMode、確認用のエンコードを -c:v の値ごとの
// encodeModes で helper process に代わりに動かし、起動した引数を記録する。
type scriptedEncoderCheck struct {
	*EncoderCheck
	mu          sync.Mutex
	args        [][]string
	listMode    string
	encodeModes map[string]string
}

func newScriptedEncoderCheck(listMode string, encodeModes map[string]string) *scriptedEncoderCheck {
	s := &scriptedEncoderCheck{EncoderCheck: NewEncoderCheck(), listMode: listMode, encodeModes: encodeModes}
	s.commandContext = func(ctx context.Context, _ string, args ...string) *exec.Cmd {
		s.mu.Lock()
		s.args = append(s.args, args)
		s.mu.Unlock()
		mode := s.listMode
		if !slices.Contains(args, "-encoders") {
			mode = "encode-ok"
			if index := slices.Index(args, "-c:v"); index >= 0 {
				if m, ok := s.encodeModes[args[index+1]]; ok {
					mode = m
				}
			}
		}
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestEncoderCheckHelperProcess", "--", mode)
		cmd.Env = append(os.Environ(), "VV_ENCODER_CHECK_HELPER=1")
		return cmd
	}
	return s
}

// encodes は起動した確認用のエンコードの -c:v の値を返す。
func (s *scriptedEncoderCheck) encodes() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var codecs []string
	for _, args := range s.args {
		if index := slices.Index(args, "-c:v"); index >= 0 {
			codecs = append(codecs, args[index+1])
		}
	}
	return codecs
}

func (s *scriptedEncoderCheck) listings() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	for _, args := range s.args {
		if slices.Contains(args, "-encoders") {
			count++
		}
	}
	return count
}

func TestEncoderCheckHelperProcess(t *testing.T) {
	if os.Getenv("VV_ENCODER_CHECK_HELPER") != "1" {
		return
	}
	switch mode := os.Args[len(os.Args)-1]; mode {
	case "list":
		_, _ = io.WriteString(os.Stdout, helperEncoderList)
		os.Exit(0)
	case "encode-ok":
		os.Exit(0)
	case "encode-fail":
		_, _ = fmt.Fprintln(os.Stderr, "Cannot load libcuda.so.1")
		os.Exit(1)
	case "hang":
		for {
			time.Sleep(time.Second)
		}
	default:
		_, _ = fmt.Fprintln(os.Stderr, "unknown helper mode", mode)
		os.Exit(2)
	}
}

func checkAll(check *EncoderCheck, encoders ...domain.VideoEncoder) map[domain.VideoEncoder]domain.EncoderAvailability {
	results := make([]domain.EncoderAvailability, len(encoders))
	var wg sync.WaitGroup
	for i, encoder := range encoders {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = check.CheckEncoder(context.Background(), encoder)
		}()
	}
	wg.Wait()
	byEncoder := map[domain.VideoEncoder]domain.EncoderAvailability{}
	for _, r := range results {
		byEncoder[r.Encoder] = r
	}
	return byEncoder
}

// 成功は available、失敗は check_failed で標準エラーの末尾を残し、固まるエンコードは
// 上限時間で timed_out。-encoders は並行する確認で 1 回だけ読む。
func TestEncoderCheckRunsCheckEncodes(t *testing.T) {
	scripted := newScriptedEncoderCheck("list", map[string]string{
		"h264_nvenc": "encode-fail",
		"h264_qsv":   "encode-ok",
		"h264_vaapi": "hang",
	})
	scripted.timeout = 500 * time.Millisecond
	began := time.Now()
	got := checkAll(scripted.EncoderCheck, domain.VideoEncoderNVENC, domain.VideoEncoderQSV, domain.VideoEncoderVAAPI)
	if elapsed := time.Since(began); elapsed > 3*time.Second {
		t.Errorf("確認に %s かかった", elapsed)
	}

	if r := got[domain.VideoEncoderQSV]; r.State != domain.EncoderAvailable || r.Reason != "" {
		t.Errorf("qsv = %+v", r)
	}
	nvenc := got[domain.VideoEncoderNVENC]
	if nvenc.State != domain.EncoderUnavailable || nvenc.Reason != domain.EncoderReasonCheckFailed ||
		!strings.Contains(nvenc.Detail, "Cannot load libcuda.so.1") {
		t.Errorf("nvenc = %+v", nvenc)
	}
	if r := got[domain.VideoEncoderVAAPI]; r.State != domain.EncoderUnavailable || r.Reason != domain.EncoderReasonTimedOut {
		t.Errorf("vaapi = %+v", r)
	}
	if n := scripted.listings(); n != 1 {
		t.Errorf("-encoders を %d 回読んだ", n)
	}
}

// -encoders に無い方式は確認用のエンコードを実行せずに encoder_missing。
func TestEncoderCheckSkipsMissingEncoder(t *testing.T) {
	scripted := newScriptedEncoderCheck("list", nil)
	got := scripted.CheckEncoder(context.Background(), domain.VideoEncoderVideoToolbox)
	if got.State != domain.EncoderUnavailable || got.Reason != domain.EncoderReasonEncoderMissing {
		t.Errorf("videotoolbox = %+v", got)
	}
	if codecs := scripted.encodes(); len(codecs) != 0 {
		t.Errorf("確認用のエンコードを実行した: %v", codecs)
	}
}

// -encoders が固まると、確認対象がすべて timed_out になって結果が返る。
func TestEncoderCheckTimesOutWhenEncoderListHangs(t *testing.T) {
	scripted := newScriptedEncoderCheck("hang", nil)
	scripted.timeout = 300 * time.Millisecond
	began := time.Now()
	got := checkAll(scripted.EncoderCheck, domain.VideoEncoderNVENC, domain.VideoEncoderQSV, domain.VideoEncoderVAAPI)
	if elapsed := time.Since(began); elapsed > 3*time.Second {
		t.Errorf("結果まで %s かかった", elapsed)
	}
	for _, encoder := range []domain.VideoEncoder{domain.VideoEncoderNVENC, domain.VideoEncoderQSV, domain.VideoEncoderVAAPI} {
		if r := got[encoder]; r.State != domain.EncoderUnavailable || r.Reason != domain.EncoderReasonTimedOut {
			t.Errorf("%s = %+v", encoder, r)
		}
	}
	if codecs := scripted.encodes(); len(codecs) != 0 {
		t.Errorf("確認用のエンコードを実行した: %v", codecs)
	}
}

// 確認用のエンコードは lavfi の合成入力を 8 フレームだけ、ライブ変換と同じ符号化器の
// 指定で符号化して捨てる。
func TestEncoderCheckEncodeArgs(t *testing.T) {
	for _, tc := range []struct {
		encoder domain.VideoEncoder
		want    string
	}{
		{domain.VideoEncoderNVENC, "-hide_banner -loglevel error -nostdin -f lavfi -i testsrc2=size=256x144:rate=30 -frames:v 8 -c:v h264_nvenc "},
		{domain.VideoEncoderVAAPI, "-hide_banner -loglevel error -nostdin -vaapi_device /dev/dri/renderD128 -f lavfi -i testsrc2=size=256x144:rate=30 -frames:v 8 -c:v h264_vaapi "},
	} {
		scripted := newScriptedEncoderCheck("list", nil)
		if got := scripted.CheckEncoder(context.Background(), tc.encoder); got.State != domain.EncoderAvailable {
			t.Fatalf("%s = %+v", tc.encoder, got)
		}
		args := strings.Join(scripted.args[len(scripted.args)-1], " ")
		if !strings.HasPrefix(args, tc.want) || !strings.HasSuffix(args, " -f null -") ||
			!strings.Contains(args, "-force_key_frames expr:gte(t,n_forced*2)") {
			t.Errorf("%s: args = %s", tc.encoder, args)
		}
	}
}
