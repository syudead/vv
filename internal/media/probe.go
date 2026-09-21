package media

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os/exec"
	"strconv"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// probeTimeout は ffprobe 1回に与える上限である。
//
// 壊れたファイルで ffprobe が戻らないことがあり、そこで取り込み全体が止まると
// 残りの動画が一覧に出てこない。1件を諦めて次へ進む方が損失が小さい（FR-008）。
const probeTimeout = 30 * time.Second

// probeCommand は実行する外部コマンドである。存在は起動前に確認済み
// （Preflight）。
const probeCommand = "ffprobe"

// Probe は1ファイルのメタデータを取得する（R-102）。
//
// 必要な値は -show_format -show_streams の1回ですべて揃う。値ごとに
// -show_entries で複数回起動すると、プロセス起動が支配的なコストなので遅くなる。
//
// os/exec はこのパッケージの外へ漏らさない。再生可否の判定規則は
// internal/domain にあり、外部プロセスに触れずにテストできる。
func Probe(ctx context.Context, path string) (domain.Probe, error) {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	output, err := exec.CommandContext(ctx, probeCommand, probeArgs(path)...).Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return domain.Probe{}, fmt.Errorf(
				"%s が失敗しました (%s): %s", probeCommand, path, firstLine(exitErr.Stderr))
		}
		return domain.Probe{}, fmt.Errorf("%s を実行できません (%s): %w", probeCommand, path, err)
	}

	probe, err := parseProbeOutput(output)
	if err != nil {
		return domain.Probe{}, fmt.Errorf("%s の出力を解釈できません (%s): %w", probeCommand, path, err)
	}
	return probe, nil
}

// probeArgs は R-102 が定める1回分の引数を組み立てる。
// パスは -- の後ろに置き、"-" で始まる名前のファイルを選択肢と取り違えない。
func probeArgs(path string) []string {
	return []string{
		"-v", "error",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		"--", path,
	}
}

// probeOutput は ffprobe の JSON のうち、取り出す部分だけを写す。
type probeOutput struct {
	Streams []struct {
		Index            int    `json:"index"`
		CodecType        string `json:"codec_type"`
		CodecName        string `json:"codec_name"`
		Profile          string `json:"profile"`
		PixelFormat      string `json:"pix_fmt"`
		BitsPerRawSample string `json:"bits_per_raw_sample"`
		Width            int    `json:"width"`
		Height           int    `json:"height"`
		Level            int    `json:"level"`
		AverageFrameRate string `json:"avg_frame_rate"`
		RealFrameRate    string `json:"r_frame_rate"`
		SampleRate       string `json:"sample_rate"`
		Channels         int    `json:"channels"`
		Disposition      struct {
			AttachedPicture int `json:"attached_pic"`
		} `json:"disposition"`
	} `json:"streams"`
	Format struct {
		Duration   string `json:"duration"`
		FormatName string `json:"format_name"`
	} `json:"format"`
}

// parseProbeOutput は JSON から domain.Probe を組み立てる。
//
// 外部プロセスを起動しないので、「どの値を取り出すか」を単体テストで固定できる。
// 尺が取れないものは誤りとして返す。尺が分からない動画は一覧で長さを出せず、
// サムネイルの抽出位置も決められないため、取り込めなかったものとして扱う。
func parseProbeOutput(output []byte) (domain.Probe, error) {
	var parsed probeOutput
	if err := json.Unmarshal(output, &parsed); err != nil {
		return domain.Probe{}, fmt.Errorf("JSON として読めません: %w", err)
	}

	seconds, err := strconv.ParseFloat(parsed.Format.Duration, 64)
	if err != nil {
		return domain.Probe{}, fmt.Errorf("尺 (format.duration=%q) を読めません", parsed.Format.Duration)
	}
	if seconds < 0 || math.IsNaN(seconds) || math.IsInf(seconds, 0) {
		return domain.Probe{}, fmt.Errorf("尺 (format.duration=%q) が不正です", parsed.Format.Duration)
	}

	probe := domain.Probe{
		DurationMs: int64(math.Round(seconds * 1000)),
		FormatName: parsed.Format.FormatName,
	}

	// 映像は最初の非添付stream、音声は最初のstreamを採る。アルバムアートや
	// posterはvideoとして現れるため、attached_picを本編にしてはならない。
	for _, stream := range parsed.Streams {
		switch stream.CodecType {
		case "video":
			if probe.VideoCodec == "" && stream.Disposition.AttachedPicture == 0 {
				probe.VideoCodec = stream.CodecName
				probe.Width = stream.Width
				probe.Height = stream.Height
			}
		case "audio":
			if probe.AudioCodec == "" {
				probe.AudioCodec = stream.CodecName
			}
		}
	}

	return probe, nil
}

// firstLine は標準エラーの先頭行を返す。誤りの文面が長大にならないようにする。
func firstLine(raw []byte) string {
	for i, b := range raw {
		if b == '\n' {
			return string(raw[:i])
		}
	}
	if len(raw) == 0 {
		return "（出力なし）"
	}
	return string(raw)
}
