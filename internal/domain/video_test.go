package domain

import "testing"

// 再生可否は許可リストで決める（R-103 / S3 / SC-007）。未知の値を「再生できる」と
// 誤るより「できない」と誤る方が、利用者の損失が小さいためである。
//
// この表がそのまま仕様であり、一覧は取り込み時に確定した結果を読むだけになる。
func TestEvaluatePlayability(t *testing.T) {
	tests := []struct {
		name       string
		container  string
		probe      Probe
		wantOK     bool
		wantReason UnplayableReason
	}{
		{
			name:      "mp4 + h264 + aac は再生できる",
			container: "mp4",
			probe:     Probe{VideoCodec: "h264", AudioCodec: "aac"},
			wantOK:    true,
		},
		{
			name:      "m4v も許可する",
			container: "m4v",
			probe:     Probe{VideoCodec: "h264", AudioCodec: "mp3"},
			wantOK:    true,
		},
		{
			name:      "webm + vp9 + opus は再生できる",
			container: "webm",
			probe:     Probe{VideoCodec: "vp9", AudioCodec: "opus"},
			wantOK:    true,
		},
		{
			name:      "webm + vp8 + vorbis は再生できる",
			container: "webm",
			probe:     Probe{VideoCodec: "vp8", AudioCodec: "vorbis"},
			wantOK:    true,
		},
		{
			name:      "av1 は許可する",
			container: "mp4",
			probe:     Probe{VideoCodec: "av1", AudioCodec: "aac"},
			wantOK:    true,
		},
		{
			name:      "音声が無くても再生できる",
			container: "mp4",
			probe:     Probe{VideoCodec: "h264", AudioCodec: ""},
			wantOK:    true,
		},
		{
			name:       "mkv はコンテナが理由で再生できない",
			container:  "mkv",
			probe:      Probe{VideoCodec: "h264", AudioCodec: "aac"},
			wantReason: ReasonContainer,
		},
		{
			name:       "mov はコンテナが理由で再生できない",
			container:  "mov",
			probe:      Probe{VideoCodec: "h264", AudioCodec: "aac"},
			wantReason: ReasonContainer,
		},
		{
			name:       "hevc は映像コーデックが理由で再生できない",
			container:  "mp4",
			probe:      Probe{VideoCodec: "hevc", AudioCodec: "aac"},
			wantReason: ReasonVideoCodec,
		},
		{
			name:       "未知の映像コーデックは再生できない側に倒す",
			container:  "mp4",
			probe:      Probe{VideoCodec: "someday_new_codec", AudioCodec: "aac"},
			wantReason: ReasonVideoCodec,
		},
		{
			name:       "映像が無ければ再生できない",
			container:  "mp4",
			probe:      Probe{VideoCodec: "", AudioCodec: "aac"},
			wantReason: ReasonVideoCodec,
		},
		{
			name:       "ac3 は音声コーデックが理由で再生できない",
			container:  "mp4",
			probe:      Probe{VideoCodec: "h264", AudioCodec: "ac3"},
			wantReason: ReasonAudioCodec,
		},
		{
			name:       "コンテナと映像がどちらも駄目ならコンテナを理由にする",
			container:  "avi",
			probe:      Probe{VideoCodec: "mpeg4", AudioCodec: "ac3"},
			wantReason: ReasonContainer,
		},
		{
			name:       "コンテナが通れば次に映像を見る",
			container:  "mp4",
			probe:      Probe{VideoCodec: "mpeg4", AudioCodec: "ac3"},
			wantReason: ReasonVideoCodec,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := EvaluatePlayability(tc.container, tc.probe)

			if got.Playable != tc.wantOK {
				t.Errorf("Playable = %v, want %v", got.Playable, tc.wantOK)
			}
			if got.Reason != tc.wantReason {
				t.Errorf("Reason = %q, want %q", got.Reason, tc.wantReason)
			}
			// 再生できると判定したなら理由は空でなければならない。
			// 両方が立っていると、一覧の表示が自己矛盾する。
			if got.Playable && got.Reason != "" {
				t.Errorf("再生できるのに理由が付いている: %q", got.Reason)
			}
			if !got.Playable && got.Reason == "" {
				t.Error("再生できないのに理由が無い")
			}
		})
	}
}

// 大文字・前後の空白が混ざっても判定が揺れないこと。ffprobe の出力も拡張子も
// 表記が一定とは限らない。
func TestEvaluatePlayabilityNormalizesCase(t *testing.T) {
	got := EvaluatePlayability("MP4", Probe{VideoCodec: "H264", AudioCodec: "AAC"})
	if !got.Playable {
		t.Errorf("大文字で判定が変わった: %+v", got)
	}
}

// コンテナは拡張子から決める。ffprobe の format_name は mp4 と mov を
// 区別しないため、判定と配信の基準を拡張子に揃える（R-103）。
func TestContainerFromPath(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/media/a.mp4", "mp4"},
		{"/media/a.MP4", "mp4"},
		{"/media/a.m4v", "m4v"},
		{"/media/夏休み.webm", "webm"},
		{"/media/a.b.mkv", "mkv"},
		{"/media/noext", ""},
	}

	for _, tc := range tests {
		if got := ContainerFromPath(tc.path); got != tc.want {
			t.Errorf("ContainerFromPath(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

// playable = 1 は probe_state = done のときだけ取り得る（data-model.md）。
// 解析前の動画が「再生できる」と表示されてはならない。
func TestPlayableRequiresCompletedProbe(t *testing.T) {
	tests := []struct {
		state ProbeState
		want  bool
	}{
		{ProbeStatePending, false},
		{ProbeStateFailed, false},
		{ProbeStateDone, true},
	}

	for _, tc := range tests {
		v := Video{ProbeState: tc.state, Playable: true}
		if got := v.PlayableInBrowser(); got != tc.want {
			t.Errorf("probeState=%s: PlayableInBrowser() = %v, want %v", tc.state, got, tc.want)
		}
	}
}

// 状態の値は api/openapi.yaml の enum と一致していなければならない。
// 片方だけ変わると、契約に無い値が応答に混ざる。
func TestStateConstantsMatchContract(t *testing.T) {
	pairs := []struct{ got, want string }{
		{string(ProbeStatePending), "pending"},
		{string(ProbeStateDone), "done"},
		{string(ProbeStateFailed), "failed"},
		{string(ThumbnailStatePending), "pending"},
		{string(ThumbnailStateDone), "done"},
		{string(ThumbnailStateFailed), "failed"},
		{string(ReasonContainer), "container"},
		{string(ReasonVideoCodec), "video_codec"},
		{string(ReasonAudioCodec), "audio_codec"},
	}
	for _, p := range pairs {
		if p.got != p.want {
			t.Errorf("定数 = %q, want %q", p.got, p.want)
		}
	}
}
