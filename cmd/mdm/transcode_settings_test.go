package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// blockingChecker は release が閉じるまで確認を終えない（固まった確認を模す）。
type blockingChecker struct {
	release chan struct{}
}

func (c blockingChecker) CheckEncoder(ctx context.Context, encoder domain.VideoEncoder) domain.EncoderAvailability {
	select {
	case <-c.release:
		return domain.EncoderAvailability{Encoder: encoder, State: domain.EncoderAvailable}
	case <-ctx.Done():
		return domain.EncoderAvailability{Encoder: encoder, State: domain.EncoderUnavailable, Reason: domain.EncoderReasonTimedOut}
	}
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// 起動時の確認は背後で走り、終わる前から HTTP が応答する。起動ログには保存値から決めた
// 方式の行がある（親 Issue #370 要件 5・9）。
func TestTranscodeSettingsCheckDoesNotDelayHTTP(t *testing.T) {
	ctx, db, _ := openTestDB(t)
	if err := db.Settings().SaveTranscodeEncoderChoice(ctx, "nvenc"); err != nil {
		t.Fatal(err)
	}
	logs := &lockedBuffer{}
	logger := slog.New(slog.NewJSONHandler(logs, nil))
	checker := blockingChecker{release: make(chan struct{})}
	checksCtx, stopChecks := context.WithCancel(ctx)
	defer stopChecks()

	started := make(chan error, 1)
	var settingsDone <-chan struct{}
	var handler http.Handler
	go func() {
		settings, err := startTranscodeSettings(checksCtx, db.Settings(), checker, "linux", logger)
		if err == nil {
			settingsDone = settings.Done()
			handler = httpapi.NewRouter(httpapi.Options{
				Pinger: db, TranscodeSettings: settings, Assets: fstest.MapFS{}, Auth: ownerAuth{}, Logger: logger,
			})
		}
		started <- err
	}()
	select {
	case err := <-started:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("確認が終わるのを待って戻らない")
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	res, err := server.Client().Get(server.URL + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("/api/health: status = %d", res.StatusCode)
	}

	res, err = server.Client().Get(server.URL + "/api/settings/transcoding")
	if err != nil {
		t.Fatal(err)
	}
	var current gen.TranscodingSettings
	err = json.NewDecoder(res.Body).Decode(&current)
	_ = res.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if !current.Checking || current.VideoEncoder != gen.VideoEncoderChoiceNvenc || current.EffectiveEncoder != gen.VideoEncoderSoftware {
		t.Errorf("確認中の設定 = %+v", current)
	}

	var line map[string]any
	for _, raw := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		var entry map[string]any
		if json.Unmarshal([]byte(raw), &entry) == nil && strings.HasPrefix(entry["msg"].(string), "transcode video encoder loaded") {
			line = entry
		}
	}
	if line == nil || line["choice"] != "nvenc" || line["effective"] != "software" || line["checking"] != true {
		t.Errorf("起動ログの方式の行 = %v: %s", line, logs)
	}

	// 停止の指示で確認は止まる。
	stopChecks()
	select {
	case <-settingsDone:
	case <-time.After(5 * time.Second):
		t.Fatal("停止の指示で確認が止まらない")
	}
}
