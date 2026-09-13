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

// Library は一覧と詳細の問い合わせ先である。internal/store の *DB がこれを
// 満たす。httpapi は保存の手段を知らないので、必要な操作だけを宣言する。
type Library interface {
	ListVideos(ctx context.Context, q domain.VideoQuery) (domain.VideoPage, error)
	GetVideo(ctx context.Context, id int64) (domain.Video, error)
}

// Scans は取り込みの開始と状態の取得である。
//
// StartScan は実行中なら新しく始めず、実行中のものを返す（R-108）。走査を
// 実際に動かす組み立ては cmd/mdm が行う。
type Scans interface {
	StartScan(ctx context.Context) (domain.Scan, error)
	CurrentScan(ctx context.Context) (domain.Scan, error)
}

// Options は経路の組み立てに必要な依存である。
type Options struct {
	// Build は稼働中のバイナリを特定するための情報。
	Build domain.BuildInfo
	// Pinger は保存層への疎通確認。nil の場合は疎通できないものとして扱う。
	Pinger Pinger
	// Videos は一覧と詳細の問い合わせ先。nil なら該当の経路は 500 を返す。
	Videos Library
	// Scans は取り込みの開始と状態の取得。nil なら該当の経路は 500 を返す。
	Scans Scans
	// MediaDir は配信してよいファイルの根。この配下だけを開く
	// （contracts/http-routes.md の安全性）。
	MediaDir string
	// ThumbnailsDir はサムネイルの置き場所。
	ThumbnailsDir string
	// Assets は SPA のビルド成果物（web/dist に相当）。
	Assets fs.FS
	// Logger は応答の過程で出す記録。nil の場合は slog の既定を使う。
	Logger *slog.Logger
}

// server は生成された gen.ServerInterface を満たす。契約（api/openapi.yaml）に
// 経路を足したらこの型がコンパイルエラーになるため、実装漏れに気付ける。
type server struct {
	build         domain.BuildInfo
	pinger        Pinger
	videos        Library
	scans         Scans
	mediaDir      string
	thumbnailsDir string
	logger        *slog.Logger
}

// NewRouter は経路を分配するハンドラを返す。
//
//	/api/health      → JSON（生成された経路定義から登録する）
//	/api/videos*     → JSON・動画本体・サムネイル（同上）
//	/api/scans*      → JSON（同上）
//	/api/*（未定義） → 404 + Error（index.html を返してはならない）
//	それ以外          → SPA（/videos/{id} を含むクライアント側ルーティング）
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

	srv := &server{
		build:         opts.Build,
		pinger:        opts.Pinger,
		videos:        opts.Videos,
		scans:         opts.Scans,
		mediaDir:      opts.MediaDir,
		thumbnailsDir: opts.ThumbnailsDir,
		logger:        logger,
	}

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

// キャッシュの指示は contracts/http-routes.md の「キャッシュ」表に対応する。
const (
	// cacheNoStore は一覧・詳細・スキャンの状態に付ける。取り込みで内容が
	// 変わり続けるため、中間キャッシュに残してはならない。
	cacheNoStore = "no-store"
	// 動画本体に付ける値は、それを配信する stream.go に置く。
	// cacheImmutable は v 付きのサムネイルに付ける。v は内容由来の識別子で、
	// 内容が変われば URL も変わるので古い画像が残らない（R-112）。
	cacheImmutable = "public, max-age=31536000, immutable"
)

// エラーの code は機械可読な種別である（contracts/http-routes.md「エラー表現」）。
const (
	codeNotFound       = "not_found"
	codeInvalidRequest = "invalid_request"
	codeInternal       = "internal"
)

// writeError は JSON のエラーを書き出す。message は利用者にそのまま提示して
// よい日本語にする（contracts/http-routes.md）。
func (s *server) writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, gen.Error{Code: code, Message: message}, s.logger)
}

// notFound は対象が存在しないことを返す。実体を開けない場合もこれを使う。
// 403 にしないのは、存在そのものを漏らさないためである。
func (s *server) notFound(w http.ResponseWriter, message string) {
	s.writeError(w, http.StatusNotFound, codeNotFound, message)
}

// invalidRequest は要求の形式が不正であることを返す。
func (s *server) invalidRequest(w http.ResponseWriter, message string) {
	s.writeError(w, http.StatusBadRequest, codeInvalidRequest, message)
}

// internalError は予期しない失敗を返す。原因は記録に残し、応答には出さない。
func (s *server) internalError(w http.ResponseWriter, message string, err error) {
	s.logger.Error(message, slog.Any("error", err))
	s.writeError(w, http.StatusInternalServerError, codeInternal, message)
}
