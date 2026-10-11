package main

import (
	"runtime/debug"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

func TestResolveBuildInfo(t *testing.T) {
	at := time.Date(2026, 9, 27, 16, 34, 41, 0, time.UTC)
	vcs := []debug.BuildSetting{
		{Key: "vcs.revision", Value: "abc123"},
		{Key: "vcs.time", Value: "2026-09-27T16:34:41Z"},
	}

	tests := []struct {
		name         string
		release      string
		commit, time string
		settings     []debug.BuildSetting
		want         domain.BuildInfo
	}{
		{
			name:     "Go が埋めた値を使う",
			release:  "v1",
			commit:   "ignored",
			time:     "2000-01-01T00:00:00Z",
			settings: vcs,
			want:     domain.BuildInfo{Version: "v1", Commit: "abc123", BuiltAt: at},
		},
		{
			name:    "コミット情報が無いビルドは ldflags の値で補う",
			release: "v1",
			commit:  "def456",
			time:    "2026-09-27T16:34:41Z",
			want:    domain.BuildInfo{Version: "v1", Commit: "def456", BuiltAt: at},
		},
		{
			name:    "どちらも無ければ省く",
			release: "",
			want:    domain.BuildInfo{Version: domain.DefaultVersion},
		},
		{
			name:    "形式の合わない時刻は省く",
			release: "v1",
			commit:  "def456",
			time:    "yesterday",
			want:    domain.BuildInfo{Version: "v1", Commit: "def456"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveBuildInfo(tt.release, tt.commit, tt.time, tt.settings)
			if got.Version != tt.want.Version || got.Commit != tt.want.Commit || !got.BuiltAt.Equal(tt.want.BuiltAt) {
				t.Fatalf("resolveBuildInfo() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
