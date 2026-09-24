package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// contentTypeJSON はすべての JSON 応答に付けるヘッダ値である。
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

// Playback は再生位置の保存先である。鍵は content_key（videos.id ではない）
// なので、動画の行が消えても記録が残る。
type Playback interface {
	SaveProgress(ctx context.Context, contentKey string, progress domain.Progress) (domain.Progress, error)
	ProgressByContentKeys(ctx context.Context, contentKeys []string) (map[string]domain.Progress, error)
}

// Scans は取り込みの開始と状態の取得である。
//
// StartScan は実行中なら新しく始めず、実行中のものを返す。走査を
// 実際に動かす組み立ては cmd/mdm が行う。
type Scans interface {
	StartScan(ctx context.Context) (domain.Scan, error)
	CurrentScan(ctx context.Context) (domain.Scan, error)
}

// MediaFolders は設定画面が1件ずつ操作するメディアフォルダ保存先である。
type MediaFolders interface {
	ListMediaFolders(ctx context.Context) ([]domain.MediaFolder, error)
	AddMediaFolder(ctx context.Context, path string) (domain.MediaFolder, error)
	ReplaceMediaFolder(ctx context.Context, id, expectedVersion int64, path string) (domain.MediaFolder, error)
	DeleteMediaFolder(ctx context.Context, id, expectedVersion int64) error
}

// Transcoder は1 request分のfragmented MP4を生成する。
// startupDeadlineはrequest時probeと初期データ生成の共通期限である。
// waitは成功したStartにつきちょうど1回呼び、stopは切断時にprocessを止める。
type Transcoder interface {
	Start(context.Context, string, int64, bool, time.Time) (io.ReadCloser, func() error, func(), error)
}

// SeekThumbnailReader はbackground jobが生成したJPEGを読み出す。
type SeekThumbnailReader interface {
	Read(context.Context, string, int64) ([]byte, error)
}

// ThumbnailJobs はサムネイルのジョブの状態の問い合わせ先である。シーク用
// プレビューには DB 上の状態が無いので、動画1件の応答を作るときにこれで導く。
type ThumbnailJobs interface {
	ThumbnailJobActive(ctx context.Context, videoID int64) (bool, error)
}

// RelatedLibrary は関連動画の問い合わせ先である。store は行を読むだけで、
// 並べ方は internal/domain の OrderRelated が決める。
type RelatedLibrary interface {
	DirectVideoPaths(ctx context.Context, dir string) ([]domain.RelatedSibling, error)
	VideosAddedNear(ctx context.Context, id int64, addedAt time.Time, limit int) ([]domain.RelatedNeighbor, error)
	VideosByIDs(ctx context.Context, ids []int64) ([]domain.Video, error)
}

// Reprober は読み取りに失敗した動画を読み取り直す状態へ戻し、ジョブを積む。
// 状態を戻すことと積むことは、保存層が1つの取引で行う。
type Reprober interface {
	RetryProbe(ctx context.Context, id int64, seekThumbnailMissing bool) error
}

// FileOpener はサーバーの PC の既定アプリでファイルを開く。internal/opener の
// *Opener がこれを満たす。起動できる環境かどうかは起動時に決まっている。
type FileOpener interface {
	Available() bool
	Open(path string) error
}

// Options は経路の組み立てに必要な依存である。
type Options struct {
	// Build は稼働中のバイナリを特定するための情報。
	Build domain.BuildInfo
	// Pinger は保存層への疎通確認。nil の場合は疎通できないものとして扱う。
	Pinger Pinger
	// Videos は一覧と詳細の問い合わせ先。nil なら該当の経路は 500 を返す。
	Videos Library
	// Playback は再生位置の保存先。nil なら記録の経路は 500 を返し、
	// 一覧には progress が載らない。
	Playback Playback
	// Scans は取り込みの開始と状態の取得。nil なら該当の経路は 500 を返す。
	Scans Scans
	// MediaFolders は登録rootの取得と個別操作。nilなら該当経路は500を返す。
	MediaFolders MediaFolders
	// Folders はフォルダ画面の問い合わせ先。nilなら該当経路は500を返す。
	Folders Folders
	// ThumbnailsDir はサムネイルの置き場所。
	ThumbnailsDir string
	// Transcoder は非対応動画をMP4へ変換する。nilなら経路は500を返す。
	Transcoder Transcoder
	// SeekThumbnails は生成済みの任意時刻JPEGを読む。nilなら経路は500を返す。
	SeekThumbnails SeekThumbnailReader
	// ThumbnailJobs はシーク用プレビューの状態を導くのに使う。nil ならジョブは
	// 進行中でないものとして導く。
	ThumbnailJobs ThumbnailJobs
	// Related は関連動画の問い合わせ先。nilなら経路は500を返す。
	Related RelatedLibrary
	// Reprobe は読み取りのやり直し。nilなら経路は500を返す。
	Reprobe Reprober
	// Opener はファイルを既定アプリで開く。nil なら開けない環境として扱う。
	Opener FileOpener
	// Assets は SPA のビルド成果物（web/dist に相当）。
	Assets fs.FS
	// Logger は応答の過程で出す記録。nil の場合は slog の既定を使う。
	Logger *slog.Logger
}

// server は生成された gen.ServerInterface を満たす。契約（api/openapi.yaml）に
// 経路を足したらこの型がコンパイルエラーになるため、実装漏れに気付ける。
type server struct {
	build          domain.BuildInfo
	pinger         Pinger
	videos         Library
	playback       Playback
	scans          Scans
	mediaFolders   MediaFolders
	folders        Folders
	thumbnailsDir  string
	transcoder     Transcoder
	seekThumbnails SeekThumbnailReader
	thumbnailJobs  ThumbnailJobs
	related        RelatedLibrary
	reprobe        Reprober
	opener         FileOpener
	logger         *slog.Logger
}

// NewRouter は経路を分配するハンドラを返す。
//
//	/api/health      → JSON（生成された経路定義から登録する）
//	/api/videos*     → JSON・動画本体・サムネイル（同上）
//	/api/scans*      → JSON（同上）
//	/api/folders*    → JSON（同上）
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
		build:          opts.Build,
		pinger:         opts.Pinger,
		videos:         opts.Videos,
		playback:       opts.Playback,
		scans:          opts.Scans,
		mediaFolders:   opts.MediaFolders,
		folders:        opts.Folders,
		thumbnailsDir:  opts.ThumbnailsDir,
		transcoder:     opts.Transcoder,
		seekThumbnails: opts.SeekThumbnails,
		thumbnailJobs:  opts.ThumbnailJobs,
		related:        opts.Related,
		reprobe:        opts.Reprobe,
		opener:         opts.Opener,
		logger:         logger,
	}

	generated := gen.HandlerWithOptions(srv, gen.StdHTTPServerOptions{
		BaseRouter: mux,
		ErrorHandlerFunc: func(w http.ResponseWriter, _ *http.Request, err error) {
			w.Header().Set("Cache-Control", cacheNoStore)
			writeJSON(w, http.StatusBadRequest, gen.Error{
				Code:    codeInvalidRequest,
				Message: err.Error(),
			}, logger)
		},
	})
	return noStoreOnError(srv.mutationBoundary(generated))
}

// noStoreOnError は 4xx と 5xx の応答に no-store を付け直す。
//
// stream と thumbnail は成功用の Cache-Control を設定してから
// http.ServeContent を呼び、ServeContent 自身が不正な Range に 416 を返す。
// その経路は writeError を通らないので、版付きサムネイルでは失敗応答に
// 1年の immutable が残っていた。書き出す直前の状態で判断する。
func noStoreOnError(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(&errorCacheWriter{ResponseWriter: w}, r)
	})
}

type errorCacheWriter struct {
	http.ResponseWriter
}

func (w *errorCacheWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *errorCacheWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *errorCacheWriter) WriteHeader(status int) {
	if status >= http.StatusBadRequest {
		w.Header().Set("Cache-Control", cacheNoStore)
	}
	w.ResponseWriter.WriteHeader(status)
}

func (s *server) mutationBoundary(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			if !s.acceptsSameOrigin(w, r) {
				return
			}
		}
		if requiresJSONBody(r) {
			mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if err != nil || mediaType != "application/json" {
				s.invalidRequest(w, "Content-Typeはapplication/jsonを指定してください")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func requiresJSONBody(r *http.Request) bool {
	switch r.Method {
	case http.MethodPost:
		return r.URL.Path == "/api/media-folders" || r.URL.Path == "/api/scans"
	case http.MethodPut:
		if id, ok := strings.CutPrefix(r.URL.Path, "/api/media-folders/"); ok {
			return id != "" && !strings.Contains(id, "/")
		}
		if suffix, ok := strings.CutPrefix(r.URL.Path, "/api/videos/"); ok {
			id, rest, found := strings.Cut(suffix, "/")
			return found && id != "" && rest == "progress"
		}
	}
	return false
}

// apiNotFound は /api/ 配下の未定義経路に JSON の 404 を返す。
//
// フォールバックを無条件にすると、綴りを誤った API 呼び出しに HTML が 200 で返り、
// クライアント側では「JSON 解析の失敗」としてしか観測できなくなる。原因の切り分けが
// 遅れるため、/api/ 配下だけは必ず JSON のエラーを返す。
func apiNotFound(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusNotFound, gen.Error{
		Code:    codeNotFound,
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

// キャッシュの指示は経路ごとに下で定める。
const (
	// cacheNoStore は一覧・詳細・スキャンの状態に付ける。取り込みで内容が
	// 変わり続けるため、中間キャッシュに残してはならない。
	cacheNoStore = "no-store"
	// 動画本体に付ける値は、それを配信する stream.go に置く。
	// cacheImmutable は v 付きのサムネイルに付ける。v は内容由来の識別子で、
	// 内容が変われば URL も変わるので古い画像が残らない。
	cacheImmutable = "public, max-age=31536000, immutable"
)

// エラーの code の正本は api/openapi.yaml の Error.code である。gen.ErrorCode*
// は task generate の出力なので、新しい種別は openapi.yaml へ足す。ここで別名を
// 与えているのは呼び出し側を短く保つためだけで、値を決めてはいない。
const (
	codeNotFound                    = gen.ErrorCodeNotFound
	codeInvalidRequest              = gen.ErrorCodeInvalidRequest
	codeInternal                    = gen.ErrorCodeInternal
	codeForbidden                   = gen.ErrorCodeForbidden
	codeInvalidMediaDirectory       = gen.ErrorCodeInvalidMediaDirectory
	codeUnsupportedMediaDirectory   = gen.ErrorCodeUnsupportedMediaDirectory
	codeMediaFolderNotFound         = gen.ErrorCodeMediaFolderNotFound
	codeOverlappingMediaDirectories = gen.ErrorCodeOverlappingMediaDirectories
	codeScanInProgress              = gen.ErrorCodeScanInProgress
	codeConflict                    = gen.ErrorCodeConflict
	codeMediaFoldersNotConfigured   = gen.ErrorCodeMediaFoldersNotConfigured
	codeDirectoryUnavailable        = gen.ErrorCodeDirectoryUnavailable
	codeProbeNotFailed              = gen.ErrorCodeProbeNotFailed
	codeOpenUnavailable             = gen.ErrorCodeOpenUnavailable
	codeFileMissing                 = gen.ErrorCodeFileMissing
)

// writeError は JSON のエラーを書き出す。message は利用者にそのまま提示して
// よい日本語にする。
func (s *server) writeError(w http.ResponseWriter, status int, code gen.ErrorCode, message string) {
	// エラーもキャッシュさせない。存在しなかった経路や読めなかったディレクトリの
	// 応答が残ると、状態が変わったあとも古い失敗を返しうる（PR #74 の指摘）。
	// 成功側と違って呼び出し箇所が多いので、ここで一括して付ける。
	w.Header().Set("Cache-Control", cacheNoStore)
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
