package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/syudead/vv/internal/httpapi/gen"
)

// 画面の API の Tag.createdAt（specs/036-tag-admin-scale/contracts/screen-api.md §0、research.md R-8）を、
// 本物の保存先で確かめる。外部連携 API の listTags には載せない。
func TestTagCreatedAtOnScreenAPIButNotExternalAPI(t *testing.T) {
	env, cookie := newExternalEnv(t, Options{})

	rec := env.serve(authRequest{
		method: http.MethodPost, target: "/api/tags", body: `{"name":"旅行"}`,
		cookies: []*http.Cookie{cookie},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("作成: status = %d: %s", rec.Code, rec.Body)
	}
	createdAt := rawCreatedAt(t, rec.Body.Bytes())
	var created gen.Tag
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.CreatedAt.IsZero() {
		t.Fatalf("作成の応答の createdAt がゼロ時刻: %s", rec.Body)
	}

	rec = env.get("/api/tags", cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("一覧: status = %d: %s", rec.Code, rec.Body)
	}
	var list struct{ Items []json.RawMessage }
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 1 {
		t.Fatalf("一覧 = %s", rec.Body)
	}
	for _, item := range list.Items {
		if got := rawCreatedAt(t, item); got != createdAt {
			t.Errorf("一覧の createdAt = %q, 作成の応答 = %q", got, createdAt)
		}
	}

	token := env.createAPIToken(cookie, "scraper")
	rec = env.serve(authRequest{method: http.MethodGet, target: "/api/v1/tags", header: bearer(token.Secret)})
	if rec.Code != http.StatusOK {
		t.Fatalf("外部の一覧: status = %d: %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), "createdAt") {
		t.Errorf("外部連携 API の応答に createdAt がある: %s", rec.Body)
	}
}

// rawCreatedAt は Tag 1 件の JSON から createdAt の文字列を取り出し、RFC 3339 であることを確かめる。
func rawCreatedAt(t *testing.T, body []byte) string {
	t.Helper()
	var tag struct {
		CreatedAt *string `json:"createdAt"`
	}
	if err := json.Unmarshal(body, &tag); err != nil {
		t.Fatal(err)
	}
	if tag.CreatedAt == nil {
		t.Fatalf("createdAt が無い: %s", body)
	}
	if _, err := time.Parse(time.RFC3339, *tag.CreatedAt); err != nil {
		t.Fatalf("createdAt = %q は RFC 3339 でない: %v", *tag.CreatedAt, err)
	}
	return *tag.CreatedAt
}
