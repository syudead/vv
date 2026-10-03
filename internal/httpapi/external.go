package httpapi

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/extgen"
)

// 外部連携 API v1（specs/026-external-api/contracts/external-api.md）。正本は
// api/external-v1.yaml で、経路とハンドラの口は internal/httpapi/extgen に生成する。
//
// ハンドラは画面の API の server とは別の型に置く。生成された ServerInterface の名前
// （ListTags・StartScan など）が画面の API とぶつからないようにするためで、依存と
// 応答の書き出しは server と共有する（plan.md の Structure decision）。
//
// 認証は境界（auth.go の bearerBoundary）が済ませていて、ここに届く要求は API トークンで
// 確かめた所有者のものだけである。

// externalServer は生成された extgen.ServerInterface を満たす。契約に操作を足したら
// この型がコンパイルエラーになるため、実装漏れに気付ける。
type externalServer struct {
	s *server
}

// registerExternalAPI は外部連携 API の経路を mux に載せる。
func registerExternalAPI(mux *http.ServeMux, srv *server) {
	extgen.HandlerWithOptions(&externalServer{s: srv}, extgen.StdHTTPServerOptions{
		BaseURL:    externalAPIBase,
		BaseRouter: mux,
		ErrorHandlerFunc: func(w http.ResponseWriter, _ *http.Request, err error) {
			writeExternalError(w, srv, http.StatusBadRequest, extgen.Error{
				Code: extgen.ErrorCodeInvalidRequest, Message: err.Error(),
			})
		},
	})
}

// writeExternalError は外部連携 API の誤りを書き出す。形は画面の API と同じで、
// キャッシュさせない。
func writeExternalError(w http.ResponseWriter, srv *server, status int, body extgen.Error) {
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, status, body, srv.logger)
}

// internalError は予期しない失敗を返す。原因は記録に残し、応答には出さない。
func (e *externalServer) internalError(w http.ResponseWriter, message string, err error) {
	e.s.logger.Error(message, slog.Any("error", err))
	writeExternalError(w, e.s, http.StatusInternalServerError, extgen.Error{
		Code: extgen.ErrorCodeInternal, Message: message,
	})
}

// ListTags はタグの一覧を返す（GET /api/v1/tags、contracts/external-api.md §3）。
// 画面の ListTags の limit を省いた全件（TagListQuery{}）と同じもので、保存先の順
// （名前の自然順）をそのまま返す。ページ・検索・絞り込みは足さない
// （specs/036-tag-admin-scale/contracts/screen-api.md §5）。
func (e *externalServer) ListTags(w http.ResponseWriter, r *http.Request) {
	if e.s.tags == nil {
		e.internalError(w, "Tag storage is not configured.", nil)
		return
	}
	page, err := e.s.tags.ListTags(r.Context(), domain.TagListQuery{})
	if err != nil {
		e.internalError(w, "Could not load tags.", err)
		return
	}
	items := make([]extgen.Tag, 0, len(page.Items))
	for _, tag := range page.Items {
		synonyms := tag.Synonyms
		if synonyms == nil {
			synonyms = []string{}
		}
		items = append(items, extgen.Tag{
			Id: tag.ID, Name: tag.Name, Synonyms: synonyms, VideoCount: tag.VideoCount, Tentative: tag.Tentative,
		})
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, extgen.TagList{Items: items}, e.s.logger)
}

// StartScan はスキャンを始める（POST /api/v1/scans、contracts/external-api.md §5）。
// 新しく始めたら 201、実行中のものを返したら 200 にする。どちらかは、行を作るか実行中の
// 行を返すかを 1 つのトランザクションで決めた app.Scans.StartScan の started で決め、
// 応答の前に状態を読み直して決めることはしない（読む間に走査が始まる・終わる）。
func (e *externalServer) StartScan(w http.ResponseWriter, r *http.Request) {
	if e.s.scans == nil {
		e.internalError(w, "Scanning is not configured.", nil)
		return
	}
	scan, started, err := e.s.scans.StartScan(r.Context())
	if errors.Is(err, domain.ErrNoMediaFolders) {
		writeExternalError(w, e.s, http.StatusConflict, extgen.Error{
			Code: extgen.ErrorCodeMediaFoldersNotConfigured, Message: "Add a media folder first.",
		})
		return
	}
	if err != nil {
		e.internalError(w, "Could not start the scan.", err)
		return
	}
	status := http.StatusOK
	if started {
		status = http.StatusCreated
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, status, toExternalScan(scan), e.s.logger)
}

// GetCurrentScan は直近のスキャンの状態を返す（GET /api/v1/scans/current）。
func (e *externalServer) GetCurrentScan(w http.ResponseWriter, r *http.Request) {
	if e.s.scans == nil {
		e.internalError(w, "Scanning is not configured.", nil)
		return
	}
	scan, err := e.s.scans.CurrentScan(r.Context())
	if errors.Is(err, domain.ErrNotFound) {
		reason := extgen.NoScan
		writeExternalError(w, e.s, http.StatusNotFound, extgen.Error{
			Code: extgen.ErrorCodeNotFound, Reason: &reason, Message: "No scan has been run yet.",
		})
		return
	}
	if err != nil {
		e.internalError(w, "Could not load the scan status.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, toExternalScan(scan), e.s.logger)
}

// toExternalScan は domain.Scan を外部連携 API の形へ写す。本数は internal/app が
// 数え終えたとき（Import.Counted）だけ入れ、finding のあいだは両方 null にする。画面の
// API が videos を省くのと同じ判定で、対象 0 本で終わった走査の 0 と区別できる。
func toExternalScan(scan domain.Scan) extgen.ExternalScan {
	out := extgen.ExternalScan{
		Id:        scan.ID,
		Status:    extgen.ExternalScanStatus(scan.Import.Status),
		StartedAt: scan.StartedAt.UTC(),
	}
	if !scan.FinishedAt.IsZero() {
		finishedAt := scan.FinishedAt.UTC()
		out.FinishedAt = &finishedAt
	}
	if scan.Import.Counted {
		videos, settled := scan.Import.Total, scan.Import.Settled
		out.Videos, out.SettledVideos = &videos, &settled
	}
	// コードは failed の行だけに出す。アップグレード前の失敗にはコードが無い（画面の API と同じ）。
	if scan.State == domain.ScanFailed && scan.ErrorCode != "" {
		code := string(scan.ErrorCode)
		out.ErrorCode = &code
	}
	return out
}
