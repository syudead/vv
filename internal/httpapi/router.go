package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"net/netip"
	"os"
	"slices"
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

// Library は一覧と詳細の問い合わせ先である。httpapi は保存の手段を知らないので、
// 必要な操作だけを宣言する。
//
// 配信と既定アプリで開く操作は、動画の所在と登録フォルダを読んで MediaFiles へ
// 渡す。開いてよい実体かは MediaFiles が確かめる。
//
// 動画を返す読み出しは見る人（domain.Audience）を取り、ゲストには公開の動画だけを
// 返す（specs/016-single-account-auth/data-model.md §3）。見る人は境界（auth.go）が
// 要求の context に載せ、ハンドラはそれを読んで渡す。
type Library interface {
	ListVideos(ctx context.Context, audience domain.Audience, q domain.VideoQuery) (domain.VideoPage, error)
	GetVideo(ctx context.Context, audience domain.Audience, id int64) (domain.Video, error)
	VideoLocations(ctx context.Context, videoID int64) ([]domain.VideoLocation, error)
	ListMediaFolders(ctx context.Context) ([]domain.MediaFolder, error)
	// VideoIDs は listVideos と同じ条件（並び順・カーソル・件数を除く）に合う
	// 全件の id を返す（GET /api/videos/ids、「すべて選択」用。
	// specs/014-video-tags/contracts/tags-api.md §5）。
	VideoIDs(ctx context.Context, q domain.VideoQuery) (ids, missingTagIDs []int64, err error)
}

// Playback は再生位置の保存先である。鍵は content_key（videos.id ではない）なので、
// 動画の行が消えても記録が残る。
type Playback interface {
	SaveProgress(ctx context.Context, contentKey string, progress domain.Progress) (domain.Progress, error)
	ProgressByContentKeys(ctx context.Context, contentKeys []string) (map[string]domain.Progress, error)
}

// Scans は取り込みの開始と状態の取得である。internal/app の *Scans がこれを
// 満たす。
//
// StartScan は実行中なら新しく始めず、実行中のものを返す。走査を実際に
// 動かすのはアプリケーション層である。
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

// Tags はタグ管理画面が操作するタグの保存先である（Plan の Structural
// Decisions 13・14）。どの操作も1つのトランザクションで済むので、
// internal/app は通さず、internal/store の TagStore をここへ直接渡す。
type Tags interface {
	ListTags(ctx context.Context) ([]domain.Tag, error)
	CreateTag(ctx context.Context, name string) (domain.Tag, error)
	RenameTag(ctx context.Context, id int64, name string) (domain.Tag, error)
	DeleteTag(ctx context.Context, id int64) error
	MergeTag(ctx context.Context, targetID, sourceID int64) (domain.Tag, error)
	AddSynonym(ctx context.Context, tagID int64, name string, mergeTagID *int64) (domain.Tag, error)
	RemoveSynonym(ctx context.Context, tagID int64, name string) error

	// 付与・取り外し・要約・一覧の項目のタグ引き（#267、Plan の Structural
	// Decisions 5・14）。どの操作も1つのトランザクションで済むので、こちらも
	// internal/app を通さない。
	AttachTagByID(ctx context.Context, videoIDs []int64, tagID int64) (domain.TagRef, int, error)
	AttachTagByName(ctx context.Context, videoIDs []int64, name string) (domain.TagRef, int, error)
	DetachTag(ctx context.Context, videoIDs []int64, tagID int64) (domain.TagRef, int, error)
	Summary(ctx context.Context, videoIDs []int64) (domain.TagSummary, error)
	// TagsByContentKeys は content_key の集合からそれぞれのタグを引く。
	// progressFor と同じ位置（httpapi）から、一覧・詳細・関連動画・読み取りの
	// やり直しの応答へ Video.tags を載せるために使う。
	TagsByContentKeys(ctx context.Context, contentKeys []string) (map[string][]domain.TagRef, error)
}

// Transcoder は1 request分のfragmented MP4を生成する。
// startupDeadlineはrequest時probeと初期データ生成の共通期限である。
// waitは成功したStartにつきちょうど1回呼び、stopは切断時にprocessを止める。
type Transcoder interface {
	Start(context.Context, string, int64, bool, time.Time) (io.ReadCloser, func() error, func(), error)
}

// ArtifactReader は生成物を配信のために読み出す。internal/artifacts の *Store が
// これを満たす。置き場の並べ方と、生成途中・不完全なものを除く判断はそちらが
// 持ち、無い・不完全・生成中のものには fs.ErrNotExist を包んだ誤りを返す。
type ArtifactReader interface {
	// ThumbnailFile はライブラリ用サムネイルを開く。閉じるのは呼び出し側である。
	ThumbnailFile(contentKey string) (*os.File, error)
	// PreviewFile はホバープレビューの MP4 を開く。閉じるのは呼び出し側である。
	PreviewFile(contentKey string) (*os.File, error)
	// SeekThumbnail は再生位置を含むシーク用プレビューの1枚を読む。
	SeekThumbnail(contentKey string, positionMs int64) ([]byte, error)
}

// VideoCatalog は動画を応答に載せるときの判断と、関連動画の組み立てを行う
// アプリケーション層である。internal/app の *Catalog がこれを満たす。
// httpapi は要求の解釈と契約の形への変換だけを持ち、プレビューの作り直しの
// 予約やシーク用プレビューの状態の導出は行わない。
type VideoCatalog interface {
	// PresentVideos は動画たちを応答に載せる形にする。順序は保つ。
	PresentVideos(ctx context.Context, videos []domain.Video) []domain.VideoView
	SeekThumbnailState(ctx context.Context, video domain.Video) (domain.SeekThumbnailState, error)
	RelatedVideos(ctx context.Context, audience domain.Audience, video domain.Video) (domain.RelatedVideos, error)
	// RetryProbe は読み取りに失敗した動画を読み取り直す。失敗していなければ
	// domain.ErrProbeNotFailed を返す。
	RetryProbe(ctx context.Context, video domain.Video) error
}

// MediaFiles は、メディアファイルとディレクトリへのアクセスを確かめる担当である。
// internal/mediafs の FS がこれを満たす。開いてよいか（登録フォルダの内側にあり、
// symlink を辿った先も内側にあり、通常ファイルであること）の判定はそちらが持ち、
// httpapi は結果を応答へ移すだけである。
type MediaFiles interface {
	// OpenMediaFile は roots のどれかの内側にある通常ファイルだけを、symlink を
	// 辿った先で開く。開けないときは domain.ErrMediaFileUnavailable を包む。
	OpenMediaFile(roots []string, path string) (*os.File, os.FileInfo, error)
	// ResolveMediaFile は OpenMediaFile と同じ規則で、辿った先のパスを返す。
	ResolveMediaFile(roots []string, path string) (string, error)
	// ListDirectories はディレクトリ選択に path の子ディレクトリを返す。
	ListDirectories(path string) (domain.DirectoryListing, error)
	// DirectoryRoots はディレクトリ選択の根を返す。
	DirectoryRoots() domain.DirectoryListing
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
	// Tags はタグの取得と個別操作。nilなら該当経路は500を返す。
	Tags Tags
	// Folders はフォルダ画面の問い合わせ先。nilなら該当経路は500を返す。
	Folders Folders
	// Transcoder は非対応動画をMP4へ変換する。nilなら経路は500を返す。
	Transcoder Transcoder
	// Artifacts は生成物（サムネイル・シーク用プレビュー・ホバープレビュー）の
	// 読み出し。nil ならサムネイルとホバープレビューは 404、シーク用プレビューは
	// 500 を返す。
	Artifacts ArtifactReader
	// Catalog は動画の応答に要る判断・関連動画・読み取りのやり直し。nil なら
	// 関連動画と読み取りのやり直しの経路は 500 を返し、動画の応答にはプレビューの
	// URL とシーク用プレビューの状態が載らない。
	Catalog VideoCatalog
	// Opener はファイルを既定アプリで開く。nil なら開けない環境として扱う。
	Opener FileOpener
	// Files はメディアファイルとディレクトリへのアクセス。nil なら配信・ライブ変換・
	// 既定アプリで開く操作・ディレクトリ選択は 500 を返す。黙って 404 にしないのは、
	// つなぎ忘れを「実体が無い」と見分けられなくなるためである。
	Files MediaFiles
	// Processing は段階ごとの残りの問い合わせ先。nilなら経路は500を返す。
	Processing Processing
	// Events は画面へ送る変化の知らせ。nilなら経路は500を返す。
	Events *Events
	// Assets は SPA のビルド成果物（web/dist に相当）。
	Assets fs.FS
	// Logger は応答の過程で出す記録。nil の場合は slog の既定を使う。
	Logger *slog.Logger
	// Auth は初回設定・ログイン・ログアウト・セッションの確認。nil なら「誰でも」以外の
	// 要求と認証の経路は 500 を返す。所有者とみなして通すことはしない。
	Auth Authenticator
	// SessionRecheck は、所有者として長く続く要求のセッションを確かめ直す間隔である。
	// 0 なら 30 秒。テストが短くする。
	SessionRecheck time.Duration
	// Now は今の時刻を返す（Cookie の Max-Age の計算に使う）。nil なら time.Now。
	Now func() time.Time
	// TrustedProxies は転送ヘッダー（X-Forwarded-For・X-Forwarded-Proto）を信じてよい
	// 直接の接続元である（MDM_TRUSTED_PROXIES）。空ならヘッダーを読まない（client_origin.go）。
	TrustedProxies []netip.Prefix
}

// server は生成された gen.ServerInterface を満たす。契約（api/openapi.yaml）に
// 経路を足したらこの型がコンパイルエラーになるため、実装漏れに気付ける。
type server struct {
	build        domain.BuildInfo
	pinger       Pinger
	videos       Library
	playback     Playback
	scans        Scans
	mediaFolders MediaFolders
	tags         Tags
	folders      Folders
	transcoder   Transcoder
	artifacts    ArtifactReader
	catalog      VideoCatalog
	opener       FileOpener
	files        MediaFiles
	processing   Processing
	events       *Events
	logger       *slog.Logger
	auth         Authenticator
	sessions     *sessionLedger
	now          func() time.Time
	// trustedProxies は転送ヘッダーを信じてよい直接の接続元である。
	trustedProxies trustedProxies
}

// NewRouter は経路を分配するハンドラを返す。
//
//	/api/health      → JSON（生成された経路定義から登録する）
//	/api/videos*     → JSON・動画本体・サムネイル（同上）
//	/api/scans*      → JSON（同上）
//	/api/processing  → JSON（同上）
//	/api/events      → Server-Sent Events（同上）
//	/api/folders*    → JSON（同上）
//	/api/tags*       → JSON（同上）
//	/api/auth/*      → JSON（初回設定・ログイン・ログアウト・状態。同上）
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
		build:        opts.Build,
		pinger:       opts.Pinger,
		videos:       opts.Videos,
		playback:     opts.Playback,
		scans:        opts.Scans,
		mediaFolders: opts.MediaFolders,
		tags:         opts.Tags,
		folders:      opts.Folders,
		transcoder:   opts.Transcoder,
		artifacts:    opts.Artifacts,
		catalog:      opts.Catalog,
		opener:       opts.Opener,
		files:        opts.Files,
		processing:   opts.Processing,
		events:       opts.Events,
		logger:       logger,
		auth:         opts.Auth,
		sessions:     newSessionLedger(opts.SessionRecheck, logger),
		now:          opts.Now,
		// 呼び出し側が後から書き換えても判定が変わらないよう写しを持つ。
		trustedProxies: slices.Clone(opts.TrustedProxies),
	}
	if srv.now == nil {
		srv.now = time.Now
	}
	if opts.Auth != nil {
		srv.sessions.check = func(ctx context.Context, token string) (bool, error) {
			_, valid, err := opts.Auth.CheckSession(ctx, token)
			return valid, err
		}
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
	return noStoreOnError(srv.authBoundary(srv.mutationBoundary(generated)))
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
		switch r.URL.Path {
		case "/api/media-folders", "/api/scans", "/api/tags", "/api/video-tags", "/api/video-tags/summary",
			"/api/auth/setup", "/api/auth/login":
			return true
		}
		if id, ok := strings.CutPrefix(r.URL.Path, "/api/tags/"); ok {
			rest, found := strings.CutSuffix(id, "/merge")
			if found {
				return rest != "" && !strings.Contains(rest, "/")
			}
			rest, found = strings.CutSuffix(id, "/synonyms")
			return found && rest != "" && !strings.Contains(rest, "/")
		}
	case http.MethodPut:
		if id, ok := strings.CutPrefix(r.URL.Path, "/api/media-folders/"); ok {
			return id != "" && !strings.Contains(id, "/")
		}
		if suffix, ok := strings.CutPrefix(r.URL.Path, "/api/videos/"); ok {
			id, rest, found := strings.Cut(suffix, "/")
			return found && id != "" && rest == "progress"
		}
	case http.MethodPatch:
		if id, ok := strings.CutPrefix(r.URL.Path, "/api/tags/"); ok {
			return id != "" && !strings.Contains(id, "/")
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
	// cacheRevalidate はサムネイル・シークプレビュー・ホバープレビューの成功の応答に、
	// 所有者にもゲストにも付ける（specs/016-single-account-auth/contracts/guest-api.md §5）。
	// private で共有キャッシュに所有者の応答を残さず、no-cache で使うたびにサーバーへ
	// 確かめさせる。ログアウト後や非公開にした後はその確かめで 404 になり、ブラウザの
	// キャッシュから出続けない。内容が同じなら ETag で 304 になり、帯域はほぼ増えない。
	cacheRevalidate = "private, no-cache"
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
	codeTagNotFound                 = gen.ErrorCodeTagNotFound
	codeTagNameTaken                = gen.ErrorCodeTagNameTaken
	codeTagMergeRequired            = gen.ErrorCodeTagMergeRequired
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
