package httpapi

import (
	"context"
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// contentTypeJSON はすべての JSON 応答に付けるヘッダ値である
// （contracts/http-routes.md）。
const contentTypeJSON = "application/json; charset=utf-8"

// Pinger は保存層への疎通を確認する。internal/store の *DB がこれを満たす。
// 稼働確認が ok / degraded を判定するために使う。
type Pinger interface {
	Ping(context.Context) error
}

// Options は経路の組み立てに必要な依存である。
type Options struct {
	// Build は稼働中のバイナリを特定するための情報。
	Build domain.BuildInfo
	// Pinger は保存層への疎通確認。nil の場合は疎通できないものとして扱う。
	Pinger Pinger
	// Assets は SPA のビルド成果物（web/dist に相当）。
	Assets fs.FS
	// Logger は応答の過程で出す記録。nil の場合は slog の既定を使う。
	Logger *slog.Logger
}

// server は生成された gen.ServerInterface を満たす。契約（api/openapi.yaml）に
// 経路を足したらこの型がコンパイルエラーになるため、実装漏れに気付ける。
type server struct {
	build  domain.BuildInfo
	pinger Pinger
	logger *slog.Logger
}

// NewRouter は経路を分配するハンドラを返す。
//
//	/api/health      → JSON（生成された経路定義から登録する）
//	/api/*（未定義） → 404 + Error（index.html を返してはならない）
//	それ以外          → SPA
func NewRouter(opts Options) http.Handler {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	mux := http.NewServeMux()

	// 先に広い経路を登録する。net/http の ServeMux はより具体的な模様を優先するため、
	// 生成された "GET /api/health" が下の "/api/" より先に一致する。
	mux.Handle("/api/", http.HandlerFunc(apiNotFound))
	mux.Handle("/", newSPAHandler(opts.Assets, logger))

	srv := &server{build: opts.Build, pinger: opts.Pinger, logger: logger}

	return gen.HandlerWithOptions(srv, gen.StdHTTPServerOptions{
		BaseRouter: mux,
		ErrorHandlerFunc: func(w http.ResponseWriter, _ *http.Request, err error) {
			writeJSON(w, http.StatusBadRequest, gen.Error{
				Code:    "invalid_request",
				Message: err.Error(),
			}, logger)
		},
	})
}

// apiNotFound は /api/ 配下の未定義経路に JSON の 404 を返す。
//
// フォールバックを無条件にすると、綴りを誤った API 呼び出しに HTML が 200 で返り、
// クライアント側では「JSON 解析の失敗」としてしか観測できなくなる。原因の切り分けが
// 遅れるため、/api/ 配下だけは必ず JSON のエラーを返す（contracts/http-routes.md）。
func apiNotFound(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusNotFound, gen.Error{
		Code:    "not_found",
		Message: "そのような API 経路はありません: " + r.URL.Path,
	}, slog.Default())
}

// writeJSON は JSON 応答を書き出す。Content-Type は契約で固定されている。
func writeJSON(w http.ResponseWriter, status int, payload any, logger *slog.Logger) {
	body, err := json.Marshal(payload)
	if err != nil {
		logger.Error("応答を JSON へ変換できません", slog.Any("error", err))
		http.Error(w, `{"code":"internal","message":"応答を組み立てられませんでした"}`,
			http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", contentTypeJSON)
	w.WriteHeader(status)
	if _, err := w.Write(append(body, '\n')); err != nil {
		logger.Debug("応答を書き出せませんでした", slog.Any("error", err))
	}
}
