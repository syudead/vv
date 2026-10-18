package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func newFavoritesFixture(t *testing.T) *libraryFixture {
	t.Helper()
	return newLibraryFixture(t)
}

func favoritesBody(favorite bool, videoIDs []int64, groupIDs ...int64) string {
	body := map[string]any{"favorite": favorite}
	if videoIDs != nil {
		body["videoIds"] = videoIDs
	}
	if groupIDs != nil {
		body["groupIds"] = groupIDs
	}
	out, err := json.Marshal(body)
	if err != nil {
		panic(err)
	}
	return string(out)
}

func (f *libraryFixture) putFavorites(body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	return f.env.serve(authRequest{method: http.MethodPut, target: "/api/favorites", body: body, cookies: cookies})
}

// Guest read responses omit favorite even when storage marks the targets as favorites.
func TestFavoritesHiddenFromGuests(t *testing.T) {
	f := newFavoritesFixture(t)
	show := f.groupID(t, "show")

	if rec := f.putFavorites(favoritesBody(true, []int64{f.ids["ep1"], f.ids["solo"]}, show), f.owner); rec.Code != http.StatusOK {
		t.Fatalf("付ける: status = %d: %s", rec.Code, rec.Body)
	}

	rec := f.env.get("/api/videos/" + strconv.FormatInt(f.ids["solo"], 10))
	if rec.Code != http.StatusOK {
		t.Fatalf("ゲストの詳細: status = %d: %s", rec.Code, rec.Body)
	}
	var detail map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	assertGuestVideo(t, "ゲストの詳細", detail)

	rec = f.env.get(f.groupPath(t, "show"))
	if rec.Code != http.StatusOK {
		t.Fatalf("ゲストのグループ: status = %d: %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), `"favorite"`) {
		t.Errorf("ゲストのグループに favorite がある: %s", rec.Body)
	}

	rec = f.env.get("/api/library")
	if rec.Code != http.StatusOK {
		t.Fatalf("ゲストのライブラリ: status = %d: %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), `"favorite"`) {
		t.Errorf("ゲストのライブラリに favorite がある: %s", rec.Body)
	}
}

func TestFavoriteInOwnerVideoResponses(t *testing.T) {
	f := newFavoritesFixture(t)
	if rec := f.putFavorites(favoritesBody(true, []int64{f.ids["solo"]}), f.owner); rec.Code != http.StatusOK {
		t.Fatalf("付ける: status = %d: %s", rec.Code, rec.Body)
	}

	solo := strconv.FormatInt(f.ids["solo"], 10)
	for _, target := range []string{
		"/api/videos",
		"/api/folders/" + strconv.FormatInt(f.rootID, 10) + "/videos",
		"/api/videos/" + solo + "/versions",
	} {
		rec := f.env.get(target, f.owner)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d: %s", target, rec.Code, rec.Body)
		}
		items := rawItems(t, rec.Body.Bytes(), "items")
		if len(items) == 0 {
			t.Fatalf("%s: 項目が無い: %s", target, rec.Body)
		}
		for _, item := range items {
			value, ok := item["favorite"]
			if !ok {
				t.Errorf("%s: favorite が無い: %v", target, item)
				continue
			}
			var id int64
			if err := json.Unmarshal(item["id"], &id); err != nil {
				t.Fatal(err)
			}
			if want := strconv.FormatBool(id == f.ids["solo"]); string(value) != want {
				t.Errorf("%s: id %d の favorite = %s, want %s", target, id, value, want)
			}
		}
	}

	rec := f.env.get("/api/library", f.owner)
	for _, item := range rawItems(t, rec.Body.Bytes(), "items") {
		field := "video"
		if string(item["kind"]) == `"group"` {
			field = "group"
		}
		var payload map[string]json.RawMessage
		if err := json.Unmarshal(item[field], &payload); err != nil {
			t.Fatal(err)
		}
		if _, ok := payload["favorite"]; !ok {
			t.Errorf("所有者のライブラリの %s に favorite が無い: %s", field, item[field])
		}
	}
}

func TestFavoriteInRelatedVideos(t *testing.T) {
	f := newGuestFixture(t, true)
	rec := f.env.serve(authRequest{
		method: http.MethodPut, target: "/api/favorites",
		body: favoritesBody(true, []int64{f.ids["d"]}), cookies: []*http.Cookie{f.owner},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("付ける: status = %d: %s", rec.Code, rec.Body)
	}

	rec = f.env.get(f.videoPath("a", "/related"), f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("所有者の関連: status = %d: %s", rec.Code, rec.Body)
	}
	items := rawItems(t, rec.Body.Bytes(), "items")
	if len(items) == 0 {
		t.Fatalf("所有者の関連に項目が無い: %s", rec.Body)
	}
	for _, item := range items {
		var id int64
		if err := json.Unmarshal(item["id"], &id); err != nil {
			t.Fatal(err)
		}
		if want := strconv.FormatBool(id == f.ids["d"]); string(item["favorite"]) != want {
			t.Errorf("所有者の関連の id %d の favorite = %s, want %s", id, item["favorite"], want)
		}
	}

	rec = f.env.get(f.videoPath("a", "/related"))
	if rec.Code != http.StatusOK {
		t.Fatalf("ゲストの関連: status = %d: %s", rec.Code, rec.Body)
	}
	for _, item := range rawItems(t, rec.Body.Bytes(), "items") {
		assertGuestVideo(t, "ゲストの関連の項目", item)
	}
}
