package httpapi

import (
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

	body, err := os.ReadFile(filepath.Join(repositoryRoot(t), "api", "openapi.yaml"))
	if err != nil {
		t.Fatalf("api/openapi.yaml を読めない: %v", err)
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
			current = operation{path: m[1]}
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
