package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/syudead/vv/internal/httpapi/gen"
)

// All videos share mtime; a also has creation time. a, b, and d have overrides; c does not.

func (f *guestFixture) setFileCreatedAt(t *testing.T, name, dir string, createdAt time.Time) {
	t.Helper()
	ctx := context.Background()
	indexed, err := f.env.db.ScanIndex().IndexedVideosByPath(ctx)
	if err != nil {
		t.Fatal(err)
	}
	video, ok := indexed[filepath.Join(f.mediaDir, dir, name+".mp4")]
	if !ok {
		t.Fatalf("%s の所在が索引に無い", name)
	}
	if err := f.env.db.ScanIndex().UpdateLocationCreatedAt(ctx, video.LocationID, createdAt); err != nil {
		t.Fatal(err)
	}
}

func videoIDsOf(videos []gen.Video) []int64 {
	ids := make([]int64, 0, len(videos))
	for _, video := range videos {
		ids = append(ids, video.Id)
	}
	return ids
}

func assertVideoDates(t *testing.T, label string, video map[string]json.RawMessage) {
	t.Helper()
	for _, field := range []string{"updatedAt", "fileCreatedAt"} {
		value, ok := video[field]
		if !ok {
			t.Errorf("%s: %s が無い", label, field)
			continue
		}
		var at time.Time
		if err := json.Unmarshal(value, &at); err != nil || at.IsZero() {
			t.Errorf("%s: %s = %s は日時でない", label, field, value)
		}
	}
}

func TestVideoResponsesCarryUpdatedAtAndFileCreatedAt(t *testing.T) {
	f := newGuestFixture(t, true)
	env := f.env
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	created := base.Add(-24 * time.Hour)
	f.setFileCreatedAt(t, "a", "pub", created)

	detail := decode[gen.Video](t, env.get(f.videoPath("a", ""), f.owner))
	if !detail.FileCreatedAt.Equal(created) {
		t.Errorf("a の fileCreatedAt = %v, want %v", detail.FileCreatedAt, created)
	}
	untouched := decode[gen.Video](t, env.get(f.videoPath("c", ""), f.owner))
	if !untouched.UpdatedAt.Equal(untouched.AddedAt) {
		t.Errorf("編集前の updatedAt = %v, want addedAt %v", untouched.UpdatedAt, untouched.AddedAt)
	}
	if !untouched.FileCreatedAt.Equal(base) {
		t.Errorf("作成日時の無い c の fileCreatedAt = %v, want mtime %v", untouched.FileCreatedAt, base)
	}

	before := time.Now().Truncate(time.Second)
	rec := f.setDisplayName("c", displayNameBody("Renamed"), f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("表示名: status = %d: %s", rec.Code, rec.Body)
	}
	saved := decode[gen.Video](t, rec)
	if saved.UpdatedAt.Before(before) || !saved.UpdatedAt.After(saved.AddedAt) {
		t.Errorf("編集後の updatedAt = %v, want >= %v かつ addedAt %v より後", saved.UpdatedAt, before, saved.AddedAt)
	}
	if !saved.FileCreatedAt.Equal(base) {
		t.Errorf("編集後の fileCreatedAt = %v, want %v", saved.FileCreatedAt, base)
	}
	if got := decode[gen.Video](t, env.get(f.videoPath("c", ""), f.owner)); !got.UpdatedAt.Equal(saved.UpdatedAt) {
		t.Errorf("詳細の updatedAt = %v, want %v", got.UpdatedAt, saved.UpdatedAt)
	}

	if rec := f.bundle(bundleBody([]int64{f.ids["a"], f.ids["b"]}, f.ids["a"]), f.owner); rec.Code != http.StatusOK {
		t.Fatalf("束ねる: status = %d: %s", rec.Code, rec.Body)
	}

	for _, who := range []struct {
		label   string
		cookies []*http.Cookie
	}{{"所有者", []*http.Cookie{f.owner}}, {"ゲスト", nil}} {
		checks := []struct{ label, target, field string }{
			{"一覧", "/api/videos", "items"},
			{"フォルダの一覧", f.folderPath(f.rootID, "/videos?scope=subtree"), "items"},
			{"関連", f.videoPath("d", "/related"), "items"},
			{"バージョン", f.videoPath("a", "/versions"), "items"},
		}
		for _, check := range checks {
			rec := env.get(check.target, who.cookies...)
			if rec.Code != http.StatusOK {
				t.Errorf("%sの%s: status = %d: %s", who.label, check.label, rec.Code, rec.Body)
				continue
			}
			items := rawItems(t, rec.Body.Bytes(), check.field)
			if len(items) == 0 {
				t.Errorf("%sの%s: 項目が無い", who.label, check.label)
			}
			for _, item := range items {
				assertVideoDates(t, who.label+"の"+check.label, item)
			}
		}
		rec := env.get(f.videoPath("a", ""), who.cookies...)
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
			t.Fatalf("%sの詳細: %v: %s", who.label, err, rec.Body)
		}
		assertVideoDates(t, who.label+"の詳細", raw)
	}

	guest := decode[gen.Video](t, env.get(f.videoPath("a", "")))
	if !guest.UpdatedAt.Equal(detail.UpdatedAt) || !guest.FileCreatedAt.Equal(created) {
		t.Errorf("ゲストの詳細: updatedAt = %v, fileCreatedAt = %v, want %v, %v",
			guest.UpdatedAt, guest.FileCreatedAt, detail.UpdatedAt, created)
	}
}

func TestVideoListsAcceptCreatedSort(t *testing.T) {
	f := newGuestFixture(t, true)
	env := f.env
	f.setFileCreatedAt(t, "a", "pub", time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC))
	a := f.ids["a"]

	for _, who := range []struct {
		label   string
		cookies []*http.Cookie
	}{{"所有者", []*http.Cookie{f.owner}}, {"ゲスト", nil}} {
		for _, sort := range []string{"createdAsc", "createdDesc"} {
			for _, target := range []string{
				"/api/videos?sort=" + sort,
				f.folderPath(f.rootID, "/videos?scope=subtree&sort="+sort),
				"/api/library?sort=" + sort,
			} {
				rec := env.get(target, who.cookies...)
				if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), `"invalid_request"`) {
					t.Errorf("%sの %s: status = %d: %s", who.label, target, rec.Code, rec.Body)
				}
			}
			ids := videoIDsOf(decode[gen.VideoPage](t, env.get("/api/videos?sort="+sort, who.cookies...)).Items)
			if len(ids) < 2 {
				t.Fatalf("%sの %s: 項目 = %v", who.label, sort, ids)
			}
			want := ids[0]
			if sort == "createdDesc" {
				want = ids[len(ids)-1]
			}
			if want != a {
				t.Errorf("%sの %s の並び = %v, a (%d) が端に無い", who.label, sort, ids, a)
			}
		}
	}
}
