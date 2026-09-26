package media

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// probeTimeout は ffprobe 1回に与える上限である。
//
// 壊れたファイルで ffprobe が戻らないことがあり、そこで取り込み全体が止まると
// 残りの動画が一覧に出てこない。1件を諦めて次へ進む方が損失が小さい。
const probeTimeout = 30 * time.Second

// probeCommand は実行する外部コマンドである。存在は起動前に確認済み
// （Preflight）。
const probeCommand = "ffprobe"

// Probe は1ファイルのメタデータを取得する。
//
// 必要な値は -show_format -show_streams の1回ですべて揃う。値ごとに
// -show_entries で複数回起動すると、プロセス起動が支配的なコストなので遅くなる。
//
// ffmpeg／ffprobe を起こす os/exec はこのパッケージの外へ漏らさない（OS の
// 既定アプリの起動だけは別の責務として internal/opener に閉じ込める）。
// 再生可否の判定規則は internal/domain にあり、外部プロセスに触れずにテストできる。
//
// ffprobe の直前にファイルの大きさと更新時刻を取り、Probe.Source に載せる。
// 保存したライブ変換用の解析情報が、変換で開いたファイルと同じ内容かを比べる鍵になる
// （specs/018-live-transcode-seek/data-model.md §4）。
func Probe(ctx context.Context, path string) (domain.Probe, error) {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	info, err := os.Stat(path)
	if err != nil {
		return domain.Probe{}, fmt.Errorf("解析するファイルを確かめられません (%s): %w", path, err)
	}

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
	probe.Source = domain.FileStampOf(info)
	return probe, nil
}

// probeArgs は ffprobe に渡す1回分の引数を組み立てる。
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

// probeSideData は stream の side_data_list の 1 件である。
type probeSideData struct {
	SideDataType string  `json:"side_data_type"`
	Rotation     float64 `json:"rotation"`
}

// probeOutput は ffprobe の JSON のうち、取り出す部分だけを写す。
type probeOutput struct {
	Streams []probeStream `json:"streams"`
	Format  struct {
		Duration   string `json:"duration"`
		FormatName string `json:"format_name"`
	} `json:"format"`
}

// probeStream は ffprobe の streams の 1 件のうち、取り出す部分だけを写す。
type probeStream struct {
	Index             int    `json:"index"`
	CodecType         string `json:"codec_type"`
	CodecName         string `json:"codec_name"`
	Profile           string `json:"profile"`
	PixelFormat       string `json:"pix_fmt"`
	BitsPerRawSample  string `json:"bits_per_raw_sample"`
	Width             int    `json:"width"`
	Height            int    `json:"height"`
	Level             int    `json:"level"`
	AverageFrameRate  string `json:"avg_frame_rate"`
	RealFrameRate     string `json:"r_frame_rate"`
	SampleAspectRatio string `json:"sample_aspect_ratio"`
	SampleRate        string `json:"sample_rate"`
	Channels          int    `json:"channels"`
	Tags              struct {
		Rotate string `json:"rotate"`
	} `json:"tags"`
	SideDataList []probeSideData `json:"side_data_list"`
	Disposition  struct {
		AttachedPicture int `json:"attached_pic"`
	} `json:"disposition"`
}

// parseProbeOutput は JSON から domain.Probe を組み立てる。取り込みの解析もライブ変換の
// 要求時の解析もこの関数だけで ffprobe の出力を解釈し、同じ出力から同じ
// domain.TranscodeProbe を得る（specs/018-live-transcode-seek/plan.md Structural Decisions 6）。
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
	transcode := domain.TranscodeProbe{FormatName: strings.ToLower(parsed.Format.FormatName)}
	videoFound := false
	for _, stream := range parsed.Streams {
		switch stream.CodecType {
		case "video":
			if !videoFound && stream.Disposition.AttachedPicture == 0 {
				videoFound = true
				transcode.Video = transcodeVideo(stream)
				probe.VideoCodec = stream.CodecName
				// 表示される向きの解像度を記録する。スマートフォンの縦動画は横長で記録し
				// 90 度回転の印を付けていることが多く、そのままでは横長に見えてしまう。
				probe.Width, probe.Height = stream.Width, stream.Height
				rotated := false
				if rotation := streamRotation(stream.Tags.Rotate, stream.SideDataList); rotation == 90 || rotation == 270 {
					probe.Width, probe.Height = stream.Height, stream.Width
					rotated = true
				}
				probe.DisplayAspectRatio = displayAspectRatio(stream.Width, stream.Height, stream.SampleAspectRatio, rotated)
			}
		case "audio":
			if transcode.Audio == nil {
				audio := transcodeAudio(stream)
				transcode.Audio = &audio
				probe.AudioCodec = stream.CodecName
			}
		}
	}
	// 寸法の無い映像はライブ変換できないので、変換用の情報を持たない（保存もしない）。
	if videoFound && transcode.Video.Width > 0 && transcode.Video.Height > 0 {
		probe.Transcode = &transcode
	}

	return probe, nil
}

// transcodeVideo は映像 stream からライブ変換に要る値を取り出す。
func transcodeVideo(stream probeStream) domain.TranscodeVideo {
	sampleAspectNum, sampleAspectDen := parseAspectRatio(stream.SampleAspectRatio)
	return domain.TranscodeVideo{
		Index:            stream.Index,
		CodecName:        strings.ToLower(stream.CodecName),
		Profile:          stream.Profile,
		Level:            stream.Level,
		PixelFormat:      strings.ToLower(stream.PixelFormat),
		BitsPerRawSample: parsePositiveInt(stream.BitsPerRawSample),
		Width:            stream.Width,
		Height:           stream.Height,
		SampleAspectNum:  sampleAspectNum,
		SampleAspectDen:  sampleAspectDen,
		Rotation:         streamRotation(stream.Tags.Rotate, stream.SideDataList),
		FPS:              parseFrameRate(stream.AverageFrameRate),
		RealFPS:          parseFrameRate(stream.RealFrameRate),
	}
}

// transcodeAudio は音声 stream からライブ変換に要る値を取り出す。
func transcodeAudio(stream probeStream) domain.TranscodeAudio {
	return domain.TranscodeAudio{
		Index:      stream.Index,
		CodecName:  strings.ToLower(stream.CodecName),
		Profile:    stream.Profile,
		SampleRate: parsePositiveInt(stream.SampleRate),
		Channels:   stream.Channels,
	}
}

// displayAspectRatio は、符号化した寸法と画素の縦横比（SAR）から表示される横÷縦の比率を
// 返す。720x576・SAR 16:15 のように画素が正方形でない動画は、寸法の比（5:4）と表示の比
// （4:3）が違う。回転していれば逆数にする。寸法が分からなければ 0。
func displayAspectRatio(width, height int, sampleAspectRatio string, rotated bool) float64 {
	if width <= 0 || height <= 0 {
		return 0
	}
	numerator, denominator := parseAspectRatio(sampleAspectRatio)
	if numerator <= 0 || denominator <= 0 {
		numerator, denominator = 1, 1
	}
	ratio := float64(width) * float64(numerator) / (float64(height) * float64(denominator))
	if rotated {
		ratio = 1 / ratio
	}
	return ratio
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
