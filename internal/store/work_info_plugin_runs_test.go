package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// 作品情報プラグインの直近の実行の規則（specs/056-plugin-tooling/data-model.md の work_info_plugin_runs・Rules）。

func pluginRun(plugin domain.WorkInfoPlugin, stderr string, ranAt time.Time) domain.WorkInfoPluginRun {
	return domain.WorkInfoPluginRun{
		PluginID: plugin.ID, Generation: plugin.Generation, RanAt: ranAt,
		WorkInfoRunLog: domain.WorkInfoRunLog{Operation: domain.WorkInfoFetch, Stderr: stderr},
	}
}

func saveRun(t *testing.T, s *WorkInfoPluginStore, run domain.WorkInfoPluginRun) {
	t.Helper()
	if err := s.SaveWorkInfoPluginRun(context.Background(), run); err != nil {
		t.Fatalf("SaveWorkInfoPluginRun: %v", err)
	}
}

// wantNoRun はプラグイン id に残っている実行が無いことを確かめる。
func wantNoRun(t *testing.T, s *WorkInfoPluginStore, id int64) {
	t.Helper()
	run, ok, err := s.WorkInfoPluginRun(context.Background(), id)
	if err != nil || ok {
		t.Fatalf("WorkInfoPluginRun = %+v, %v, %v; want no run", run, ok, err)
	}
}

func TestWorkInfoPluginRunIsReplacedByTheNextRun(t *testing.T) {
	ctx := context.Background()
	s := migratedDB(t).WorkInfoPlugins()
	plugin, _ := savePlugin(t, s, pluginUpload("example", "d1"))
	wantNoRun(t, s, plugin.ID)
	if list, err := s.ListWorkInfoPlugins(ctx); err != nil || !list[0].LastRunAt.IsZero() {
		t.Fatalf("List = %+v, %v; want no last run", list, err)
	}

	first := pluginRun(plugin, "first", pluginNow)
	first.Failure = domain.WorkInfoRunFailed
	first.Truncated = true
	saveRun(t, s, first)
	got, ok, err := s.WorkInfoPluginRun(ctx, plugin.ID)
	if err != nil || !ok || got != first {
		t.Fatalf("WorkInfoPluginRun = %+v, %v, %v; want %+v", got, ok, err, first)
	}

	second := pluginRun(plugin, "second", pluginNow.Add(time.Minute))
	saveRun(t, s, second)
	got, ok, err = s.WorkInfoPluginRun(ctx, plugin.ID)
	if err != nil || !ok || got != second {
		t.Fatalf("WorkInfoPluginRun = %+v, %v, %v; want the second run %+v", got, ok, err, second)
	}
	list, err := s.ListWorkInfoPlugins(ctx)
	if err != nil || !list[0].LastRunAt.Equal(second.RanAt) {
		t.Fatalf("List = %+v, %v; want lastRunAt %v", list, err, second.RanAt)
	}
	if read, err := s.WorkInfoPlugin(ctx, plugin.ID); err != nil || !read.LastRunAt.Equal(second.RanAt) {
		t.Fatalf("WorkInfoPlugin = %+v, %v; want lastRunAt %v", read, err, second.RanAt)
	}
}

func TestWorkInfoPluginReplacementDeletesTheLastRun(t *testing.T) {
	s := migratedDB(t).WorkInfoPlugins()
	plugin, _ := savePlugin(t, s, pluginUpload("example", "d1"))
	saveRun(t, s, pluginRun(plugin, "old module", pluginNow))
	savePlugin(t, s, pluginUpload("example", "d2"))
	wantNoRun(t, s, plugin.ID)
}

// 実行の後、保存の前にプラグインが差し替えられた（同じバイト列でも）か、消された実行は何も書かない。
func TestWorkInfoPluginRunOfAnOlderGenerationWritesNothing(t *testing.T) {
	ctx := context.Background()
	s := migratedDB(t).WorkInfoPlugins()

	plugin, _ := savePlugin(t, s, pluginUpload("example", "d1"))
	savePlugin(t, s, pluginUpload("example", "d1"))
	saveRun(t, s, pluginRun(plugin, "ran before the same bytes were uploaded again", pluginNow))
	wantNoRun(t, s, plugin.ID)

	other, _ := savePlugin(t, s, pluginUpload("other", "d3"))
	if err := s.DeleteWorkInfoPlugin(ctx, other.ID); err != nil {
		t.Fatal(err)
	}
	saveRun(t, s, pluginRun(other, "ran before the plugin was deleted", pluginNow))
	if _, _, err := s.WorkInfoPluginRun(ctx, other.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("WorkInfoPluginRun err = %v, want ErrNotFound", err)
	}
	var rows int
	if err := s.sql.QueryRowContext(ctx, `select count(*) from work_info_plugin_runs`).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("work_info_plugin_runs has %d rows, %v; want none", rows, err)
	}
}

func TestWorkInfoPluginDeleteCascadesToTheLastRun(t *testing.T) {
	ctx := context.Background()
	s := migratedDB(t).WorkInfoPlugins()
	plugin, _ := savePlugin(t, s, pluginUpload("example", "d1"))
	saveRun(t, s, pluginRun(plugin, "kept", pluginNow))
	if err := s.DeleteWorkInfoPlugin(ctx, plugin.ID); err != nil {
		t.Fatal(err)
	}
	var rows int
	if err := s.sql.QueryRowContext(ctx, `select count(*) from work_info_plugin_runs`).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("work_info_plugin_runs has %d rows, %v; want none", rows, err)
	}
}
