package domain

import (
	"encoding/json"
	"io/fs"
)

// TranscodeProbeVersion は保存する TranscodeProbe の形の版である。項目を足す・
// 意味を変えるときに上げる。版が違う保存値は「無い」と読まれ、次の変換か再解析で
// 書き直される（specs/018-live-transcode-seek/data-model.md §2・§3）。
const TranscodeProbeVersion = 1

// TranscodeProbe はライブ変換の ffmpeg の引数を決めるのに要る ffprobe の事実である。
// JSON にして video_transcode_probes.probe に保存する。欄名は Go の欄名をそのまま使う。
type TranscodeProbe struct {
	// FormatName は format.format_name（小文字）。MOV の二入力の判定に使う。
	FormatName string
	// Video は最初の非添付の映像 stream。
	Video TranscodeVideo
	// Audio は最初の音声 stream。無ければ nil。
	Audio *TranscodeAudio
}

// TranscodeVideo は選んだ映像 stream の符号化と幾何である。
type TranscodeVideo struct {
	Index            int
	CodecName        string
	Profile          string
	Level            int
	PixelFormat      string
	BitsPerRawSample int
	// Width / Height は符号化された寸法（回転を反映しない）。
	Width           int
	Height          int
	SampleAspectNum int64
	SampleAspectDen int64
	// Rotation は 0/90/180/270（Display Matrix か rotate タグ）。
	Rotation int
	// FPS は avg_frame_rate、RealFPS は r_frame_rate。
	FPS     float64
	RealFPS float64
}

// TranscodeAudio は選んだ音声 stream の符号化である。
type TranscodeAudio struct {
	Index      int
	CodecName  string
	Profile    string
	SampleRate int
	Channels   int
}

// FileStamp はファイルの同一性を比べるための大きさと更新時刻（Unix ナノ秒）である。
type FileStamp struct {
	SizeBytes int64
	ModTimeNs int64
}

// FileStampOf は os.Stat などの結果から FileStamp を作る。
func FileStampOf(info fs.FileInfo) FileStamp {
	return FileStamp{SizeBytes: info.Size(), ModTimeNs: info.ModTime().UnixNano()}
}

// StoredTranscodeProbe は video_transcode_probes の 1 行をそのまま写した値である。
// Probe は解釈前の JSON で、使ってよいかは TranscodeProbeUsable が決める。
type StoredTranscodeProbe struct {
	Version int
	Source  FileStamp
	Probe   string
}

// TranscodeProbeUsable は、保存された解析情報をいま開いたファイルの変換に使ってよいかを
// 判定し、使えるときはその値を返す（data-model.md §3）。行が無い（nil）、版が違う、JSON が
// TranscodeProbe として読めない、大きさか更新時刻が開いたファイルと違う、のいずれかなら
// 使えない。
func TranscodeProbeUsable(stored *StoredTranscodeProbe, opened FileStamp) (TranscodeProbe, bool) {
	if stored == nil || stored.Version != TranscodeProbeVersion || stored.Source != opened {
		return TranscodeProbe{}, false
	}
	var probe TranscodeProbe
	if err := json.Unmarshal([]byte(stored.Probe), &probe); err != nil {
		return TranscodeProbe{}, false
	}
	// "null" や "{}" も JSON としては読めるが、寸法の無い映像は変換できない。
	if probe.Video.Width <= 0 || probe.Video.Height <= 0 {
		return TranscodeProbe{}, false
	}
	return probe, true
}
