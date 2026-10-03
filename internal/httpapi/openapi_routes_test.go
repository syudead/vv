package httpapi

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// requiresJSONBody は本文を必須にする経路を持っている。その集合の正本は
// api/openapi.yaml で requestBody を持つ操作であり、router.go の実装はその写しで
// ある。写しがずれると、本文検証が本来かかるべき経路から外れたり、かからない
// はずの経路にかかったりする（PR #81）。
//
// ここでは openapi.yaml 側を読み直して突き合わせ、片側だけ変えた状態を落とす。
func TestRequiresJSONBodyMatchesOpenAPI(t *testing.T) {
	withBody, withoutBody := openAPIOperations(t)

	// 走査が何も拾えないまま成功すると、検査が黙って空振りする。
	if len(withBody) == 0 || len(withoutBody) == 0 {
		t.Fatalf("openapi.yaml から操作を読み取れていない (本文あり %d / 本文なし %d)", len(withBody), len(withoutBody))
	}

	for _, op := range withBody {
		if !requiresJSONBody(operationRequest(op)) {
			t.Errorf("%s %s は openapi.yaml で requestBody を持つが、requiresJSONBody が false を返す。"+
				"router.go の経路判定に足すこと", op.method, op.path)
		}
	}
	// 宣言された操作だけを見ると、openapi に無い method と経路の組を router.go が
	// 主張していても素通りする。経路ごとに全 method を当てて、本文を持つと宣言された
	// 組以外はすべて false であることを確かめる。
	required := map[operation]bool{}
	for _, op := range withBody {
		required[op] = true
	}
	for _, op := range append(append([]operation{}, withBody...), withoutBody...) {
		for _, method := range openAPIMethodSet {
			candidate := operation{method: method, path: op.path}
			if required[candidate] {
				continue
			}
			if requiresJSONBody(operationRequest(candidate)) {
				t.Errorf("%s %s は openapi.yaml で requestBody を持たないが、requiresJSONBody が true を返す。"+
					"router.go の経路判定から外すこと", candidate.method, candidate.path)
			}
		}
	}
}

type operation struct {
	method string
	path   string
}

// request は openapi の経路テンプレートを具体的な要求にする。
func operationRequest(op operation) *http.Request {
	concrete := regexp.MustCompile(`\{[^}]+\}`).ReplaceAllString(op.path, "1")
	return httptest.NewRequest(op.method, concrete, nil)
}

var (
	openAPIPath      = regexp.MustCompile(`^  (/\S+):`)
	openAPIMethod    = regexp.MustCompile(`^    (get|post|put|delete|patch):`)
	openAPIHasBody   = regexp.MustCompile(`^      requestBody:`)
	openAPIMethodSet = map[string]string{
		"get": http.MethodGet, "post": http.MethodPost, "put": http.MethodPut,
		"delete": http.MethodDelete, "patch": http.MethodPatch,
	}
)

// openAPIOperations は api/openapi.yaml の操作を、本文を持つものと持たないものに
// 分けて返す。YAML ライブラリを足さずに済ませるため、この文書の固定した字下げを
// 読む。字下げが変わって何も拾えなくなった場合は、呼び出し側が失敗させる。
func openAPIOperations(t *testing.T) (withBody, withoutBody []operation) {
	t.Helper()
	return openAPIOperationsIn(t, "openapi.yaml", "")
}

// openAPIOperationsIn は api/ の下の文書 file の操作を、経路の前に base を付けて返す。
// 外部連携 API（external-v1.yaml）の経路は servers の /api/v1 からの相対で書く。
func openAPIOperationsIn(t *testing.T, file, base string) (withBody, withoutBody []operation) {
	t.Helper()

	body, err := os.ReadFile(filepath.Join(repositoryRoot(t), "api", file))
	if err != nil {
		t.Fatalf("api/%s を読めない: %v", file, err)
	}

	var current operation
	seen := map[operation]bool{}
	flush := func() {
		if current.method != "" && !seen[current] {
			withoutBody = append(withoutBody, current)
		}
	}
	for _, line := range strings.Split(string(body), "\n") {
		if m := openAPIPath.FindStringSubmatch(line); m != nil {
			flush()
			current = operation{path: base + m[1]}
			continue
		}
		if m := openAPIMethod.FindStringSubmatch(line); m != nil {
			flush()
			current.method = openAPIMethodSet[m[1]]
			continue
		}
		if openAPIHasBody.MatchString(line) && current.method != "" && !seen[current] {
			seen[current] = true
			withBody = append(withBody, current)
		}
	}
	flush()
	return withBody, withoutBody
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("テストの位置を解決できない")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("リポジトリ根 %s を確認できない: %v", root, err)
	}
	return root
}

// TestErrorResponsesAreNotCached は API のエラー応答に no-store が付くことを
// 確かめる。付け忘れると、状態が変わったあとも古い失敗が返りうる。PR #74 は
// directory 一覧の 404 についてこれを指摘したが、writeError を通る全経路が
// 同じ状態だったので、経路ごとではなく writeError 側で付けている。
func TestErrorResponsesAreNotCached(t *testing.T) {
	handler := newTestServer(t, Options{})

	cases := []struct {
		name, method, target, body string
	}{
		{"未定義のAPI経路", http.MethodGet, "/api/does-not-exist", ""},
		{"動画が存在しない", http.MethodGet, "/api/videos/999999", ""},
		{"limitが不正", http.MethodGet, "/api/videos?limit=abc", ""},
		{"cursorが壊れている", http.MethodGet, "/api/videos?cursor=%%%", ""},
		{"存在しないディレクトリ", http.MethodGet, "/api/directories?path=/does/not/exist", ""},
		{"相対pathのディレクトリ", http.MethodGet, "/api/directories?path=relative", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var reader io.Reader
			if tc.body != "" {
				reader = strings.NewReader(tc.body)
			}
			req := httptest.NewRequest(tc.method, tc.target, reader)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code < 400 {
				t.Fatalf("エラー応答を期待したが %d が返った: %s", rec.Code, rec.Body)
			}
			if got := rec.Header().Get("Cache-Control"); got != cacheNoStore {
				t.Errorf("%d の応答に Cache-Control: %s が付いていない（得 %q）。"+
					"エラーもキャッシュさせないこと", rec.Code, cacheNoStore, got)
			}
		})
	}
}

// TestAccessClassificationMatchesOpenAPISecurity は、境界の3つの扱い（auth.go の
// classifyRequest）が api/openapi.yaml の各操作の security と一致することを確かめる
// （specs/016-single-account-auth/contracts/auth-api.md §1）。`security: []` は誰でも、
// `[{sessionCookie: []}, {}]` はゲストも、宣言が無いものは全体の既定（所有者だけ）である。
// 宣言の無い method と経路の組が「所有者だけ」に倒れることも確かめる。
func TestAccessClassificationMatchesOpenAPISecurity(t *testing.T) {
	declared := openAPISecurity(t)
	if len(declared) == 0 {
		t.Fatal("openapi.yaml から操作を読み取れていない")
	}
	counts := map[access]int{}
	for op, want := range declared {
		counts[want]++
		got, api := classifyRequest(operationRequest(op))
		if !api {
			t.Errorf("%s %s が /api/ 以下として扱われていない", op.method, op.path)
		}
		if got != want {
			t.Errorf("%s %s: 境界の扱い = %s, openapi.yaml の security = %s", op.method, op.path, got, want)
		}
		for _, method := range openAPIMethodSet {
			candidate := operation{method: method, path: op.path}
			if _, ok := declared[candidate]; ok {
				continue
			}
			if got, _ := classifyRequest(operationRequest(candidate)); got != accessOwner {
				t.Errorf("%s %s は openapi.yaml に無いが、境界の扱いが %s である", method, op.path, got)
			}
		}
	}
	// どれかの扱いが1つも拾えないまま成功すると、検査が黙って空振りする。
	for _, class := range []access{accessPublic, accessGuest, accessOwner} {
		if counts[class] == 0 {
			t.Errorf("openapi.yaml から %s の操作を1つも読み取れていない", class)
		}
	}
	// accessRoutes に openapi.yaml に無い模様が残っていないこと。
	for pattern, class := range accessRoutes {
		method, route, _ := strings.Cut(pattern, " ")
		if class == accessOwner {
			continue
		}
		if _, ok := declared[operation{method: method, path: route}]; !ok {
			t.Errorf("accessRoutes の %q は openapi.yaml に無い", pattern)
		}
	}
}

var (
	openAPISecurityEmpty = regexp.MustCompile(`^      security: \[\]\s*$`)
	openAPISecurityStart = regexp.MustCompile(`^      security:\s*$`)
	openAPISecurityItem  = regexp.MustCompile(`^        - (.+?)\s*$`)
)

// openAPISecurity は api/openapi.yaml の各操作の扱いを、操作ごとの security から読む。
// 字下げの固定した書き方を前提にするのは openAPIOperations と同じである。
func openAPISecurity(t *testing.T) map[operation]access {
	t.Helper()

	body, err := os.ReadFile(filepath.Join(repositoryRoot(t), "api", "openapi.yaml"))
	if err != nil {
		t.Fatalf("api/openapi.yaml を読めない: %v", err)
	}
	result := map[operation]access{}
	var current operation
	var items []string
	inSecurity := false
	flushSecurity := func() {
		if !inSecurity {
			return
		}
		inSecurity = false
		if len(items) == 2 && items[0] == "sessionCookie: []" && items[1] == "{}" {
			result[current] = accessGuest
			return
		}
		t.Errorf("%s %s の security を解釈できない: %q", current.method, current.path, items)
	}
	for _, line := range strings.Split(string(body), "\n") {
		if inSecurity {
			if m := openAPISecurityItem.FindStringSubmatch(line); m != nil {
				items = append(items, m[1])
				continue
			}
			flushSecurity()
		}
		if m := openAPIPath.FindStringSubmatch(line); m != nil {
			current = operation{path: m[1]}
			continue
		}
		if m := openAPIMethod.FindStringSubmatch(line); m != nil {
			current.method = openAPIMethodSet[m[1]]
			result[current] = accessOwner
			continue
		}
		if current.method == "" {
			continue
		}
		if openAPISecurityEmpty.MatchString(line) {
			result[current] = accessPublic
			continue
		}
		if openAPISecurityStart.MatchString(line) {
			inSecurity = true
			items = nil
		}
	}
	flushSecurity()
	return result
}

// TestExternalAPIOperationsAreBearer は、外部連携 API（api/external-v1.yaml）のすべての操作が
// bearerAuth で、境界がそれを Bearer の扱いに分類することを確かめる（specs/026-external-api/
// research.md R-3）。文書の基底が境界の externalAPIBase と同じで、操作ごとの security の上書きが
// 無いことも確かめる。宣言の無い method も Bearer の扱いである（/api/v1 の下はすべて Bearer）。
// 本文を取る操作と requiresJSONBody の一致も、画面の API と同じく確かめる。
func TestExternalAPIOperationsAreBearer(t *testing.T) {
	body, err := os.ReadFile(filepath.Join(repositoryRoot(t), "api", "external-v1.yaml"))
	if err != nil {
		t.Fatalf("api/external-v1.yaml を読めない: %v", err)
	}
	text := string(body)
	if !strings.Contains(text, "\nservers:\n  - url: "+externalAPIBase+"\n") {
		t.Errorf("external-v1.yaml の servers が %s ではない", externalAPIBase)
	}
	if !strings.Contains(text, "\nsecurity:\n  - bearerAuth: []\n") {
		t.Error("external-v1.yaml の全体の security が bearerAuth ではない")
	}
	if !regexp.MustCompile(`(?m)^    bearerAuth:\n      type: http\n      scheme: bearer$`).MatchString(text) {
		t.Error("external-v1.yaml の bearerAuth が type: http・scheme: bearer ではない")
	}
	if regexp.MustCompile(`(?m)^      security:`).MatchString(text) {
		t.Error("external-v1.yaml に操作ごとの security がある。すべての操作は bearerAuth である")
	}

	withBody, withoutBody := openAPIOperationsIn(t, "external-v1.yaml", externalAPIBase)
	operations := append(append([]operation{}, withBody...), withoutBody...)
	if len(operations) == 0 {
		t.Fatal("external-v1.yaml から操作を読み取れていない")
	}
	declaredBody := map[operation]bool{}
	for _, op := range withBody {
		declaredBody[op] = true
	}
	for _, op := range operations {
		for _, method := range openAPIMethodSet {
			candidate := operation{method: method, path: op.path}
			got, api := classifyRequest(operationRequest(candidate))
			if got != accessBearer || !api {
				t.Errorf("%s %s: 境界の扱い = %s (api %v), want bearer", method, op.path, got, api)
			}
			if requiresJSONBody(operationRequest(candidate)) != declaredBody[candidate] {
				t.Errorf("%s %s: requiresJSONBody と external-v1.yaml の requestBody が食い違う", method, op.path)
			}
		}
	}
}

// Bearer の扱いの経路（/api/v1 の下と /mcp）と、そうでない経路の分類を確かめる。
func TestClassifyBearerPaths(t *testing.T) {
	cases := []struct {
		target string
		want   access
		api    bool
	}{
		{"/api/v1", accessBearer, true},
		{"/api/v1/", accessBearer, true},
		{"/api/v1/tags", accessBearer, true},
		{"/api/v1/unknown/route", accessBearer, true},
		{"//api/./v1/tags", accessBearer, true},
		{"/mcp", accessBearer, false},
		// 符号化した区切りで段が食い違う要求は、今までの規則のまま所有者だけに倒す。
		{"/api/v1%2Ftags", accessOwner, true},
		{"/api/v1/x%2F..%2F..%2Fvideos", accessOwner, true},
		// 似た名前は Bearer ではない。
		{"/api/v10/tags", accessOwner, true},
		{"/api/videos", accessGuest, true},
		{"/mcpx", accessPublic, false},
	}
	for _, tc := range cases {
		got, api := classifyRequest(httptest.NewRequest(http.MethodGet, tc.target, nil))
		if got != tc.want || api != tc.api {
			t.Errorf("%s: 扱い = %s (api %v), want %s (api %v)", tc.target, got, api, tc.want, tc.api)
		}
	}
}

// /api/tags/rejected-names は /api/tags/{id} と字面の段で区別される。GET と DELETE の
// この経路が {id} の操作（タグの削除など）に取られないことを確かめる
// （specs/031-tentative-tags/contracts/screen-api.md §3）。
func TestRejectedTagNamesRouteIsNotTagID(t *testing.T) {
	fake := &fakeTags{rejectedNames: []string{"Blocked"}}
	handler := newTestServer(t, Options{Tags: fake})

	rec := do(t, handler, http.MethodGet, "/api/tags/rejected-names")
	if rec.Code != http.StatusOK || fake.operation != "list-rejected" {
		t.Fatalf("GET: status = %d operation = %q: %s", rec.Code, fake.operation, rec.Body)
	}
	// パラメータが無ければ先頭から 100 件まで（specs/036-tag-admin-scale/contracts/screen-api.md §6）。
	if fake.lastCursor != "" || fake.lastLimit != 100 {
		t.Errorf("GET: cursor = %q limit = %d, want 空と 100", fake.lastCursor, fake.lastLimit)
	}

	fake.operation = ""
	rec = do(t, handler, http.MethodDelete, "/api/tags/rejected-names?name=Blocked")
	if rec.Code != http.StatusNoContent || fake.operation != "forget-rejected" || fake.lastName != "Blocked" {
		t.Fatalf("DELETE: status = %d operation = %q name = %q: %s", rec.Code, fake.operation, fake.lastName, rec.Body)
	}

	// name が無ければ 400 で、タグの削除にも却下した名前の操作にも届かない。
	fake.operation = ""
	rec = do(t, handler, http.MethodDelete, "/api/tags/rejected-names")
	if rec.Code != http.StatusBadRequest || fake.operation != "" {
		t.Fatalf("DELETE without name: status = %d operation = %q: %s", rec.Code, fake.operation, rec.Body)
	}
}

// /api/tags/batch と /api/tags/impact は /api/tags/{id} と字面の段で区別される。どちらの POST も
// {id} の操作に取られず、それぞれの経路に届くことを確かめる
// （specs/036-tag-admin-scale/contracts/screen-api.md §1）。
func TestTagBatchRoutesAreNotTagID(t *testing.T) {
	fake := &fakeTags{}
	handler := newTestServer(t, Options{Tags: fake})

	rec := jsonRequest(t, handler, http.MethodPost, "/api/tags/batch", `{"action":"delete","ids":[1]}`)
	if rec.Code != http.StatusOK || fake.operation != "batch" {
		t.Fatalf("POST /api/tags/batch: status = %d operation = %q: %s", rec.Code, fake.operation, rec.Body)
	}

	fake.operation = ""
	rec = jsonRequest(t, handler, http.MethodPost, "/api/tags/impact", `{"action":"delete","ids":[1]}`)
	if rec.Code != http.StatusOK || fake.operation != "impact" {
		t.Fatalf("POST /api/tags/impact: status = %d operation = %q: %s", rec.Code, fake.operation, rec.Body)
	}

	// {id} の経路（PATCH・DELETE）として解釈されない。
	for _, target := range []string{"/api/tags/batch", "/api/tags/impact"} {
		fake.operation = ""
		rec = do(t, handler, http.MethodDelete, target)
		if fake.operation != "" {
			t.Errorf("DELETE %s が %s に届いた (status %d)", target, fake.operation, rec.Code)
		}
	}
}
