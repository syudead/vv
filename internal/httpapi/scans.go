package httpapi

import (
	"errors"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// StartScan は取り込みを開始する（POST /api/scans）。
//
// すでに実行中なら新しく始めず、実行中のものを返す。409 にしないのは、
// 利用者の意図が「今の状態を進めたい」であり、進行中ならそれを返すのが
// 素直だからである。
//
// 応答は即座に返り、取り込みは背後で進む。走査中も一覧・再生の経路は通常
// どおり応答する。
func (s *server) StartScan(w http.ResponseWriter, r *http.Request) {
	var body struct{}
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if s.scans == nil {
		s.internalError(w, "Scanning is not configured.", nil)
		return
	}

	scan, err := s.scans.StartScan(r.Context())
	if errors.Is(err, domain.ErrNoMediaFolders) {
		s.writeError(w, http.StatusConflict, codeMediaFoldersNotConfigured, "Add a media folder first.")
		return
	}
	if err != nil {
		s.internalError(w, "Could not start the scan.", err)
		return
	}

	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusAccepted, toAPIScan(scan), s.logger)
}

// GetCurrentScan は直近のスキャンの状態を返す（GET /api/scans/current）。
// 実行中のものがあればそれを、無ければ最後に終わったものを返す。
func (s *server) GetCurrentScan(w http.ResponseWriter, r *http.Request) {
	if s.scans == nil {
		s.internalError(w, "Scanning is not configured.", nil)
		return
	}

	scan, err := s.scans.CurrentScan(r.Context())
	switch {
	case errors.Is(err, domain.ErrNotFound):
		s.notFoundReason(w, reasonNoScan, "No scan has been run yet.")
		return
	case err != nil:
		s.internalError(w, "Could not load the scan status.", err)
		return
	}

	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, toAPIScan(scan), s.logger)
}

// toAPIScan は domain.Scan を契約の形へ写す。
func toAPIScan(scan domain.Scan) gen.Scan {
	out := gen.Scan{
		Id:        scan.ID,
		Status:    gen.ScanStatus(scan.Import.Status),
		State:     gen.ScanState(scan.State),
		Total:     scan.Total,
		Completed: scan.Completed,
		Failed:    scan.Failed,
	}
	// 状態と本数は internal/app が組み立てたものを写すだけにする。
	if scan.Import.Counted {
		out.Videos = &gen.ScanVideos{Total: scan.Import.Total, Settled: scan.Import.Settled}
	}
	if !scan.Import.SettledAt.IsZero() {
		settledAt := scan.Import.SettledAt
		out.SettledAt = &settledAt
	}
	if !scan.StartedAt.IsZero() {
		startedAt := scan.StartedAt
		out.StartedAt = &startedAt
	}
	if !scan.FinishedAt.IsZero() {
		finishedAt := scan.FinishedAt
		out.FinishedAt = &finishedAt
	}
	if scan.Error != "" {
		reason := scan.Error
		out.Error = &reason
	}
	// コードと場所は failed の行だけに出す。アップグレード前の失敗にはコードが無い
	// （specs/023-english-i18n/contracts/error-api.md §3）。
	if scan.State == domain.ScanFailed && scan.ErrorCode != "" {
		code := gen.ScanErrorCode(scan.ErrorCode)
		out.ErrorCode = &code
		if scan.ErrorPath != "" {
			path := scan.ErrorPath
			out.ErrorPath = &path
		}
	}
	return out
}
