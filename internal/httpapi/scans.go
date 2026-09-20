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
// 素直だからである（R-108）。
//
// 応答は即座に返り、取り込みは背後で進む。走査中も一覧・再生の経路は通常
// どおり応答する（FR-007）。
func (s *server) StartScan(w http.ResponseWriter, r *http.Request) {
	var body struct{}
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if s.scans == nil {
		s.internalError(w, "取り込みの経路が設定されていません", nil)
		return
	}

	scan, err := s.scans.StartScan(r.Context())
	if errors.Is(err, domain.ErrNoMediaFolders) {
		s.writeError(w, http.StatusConflict, codeMediaFoldersNotConfigured, "メディアフォルダを設定してください")
		return
	}
	if err != nil {
		s.internalError(w, "取り込みを開始できませんでした", err)
		return
	}

	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusAccepted, toAPIScan(scan), s.logger)
}

// GetCurrentScan は直近のスキャンの状態を返す（GET /api/scans/current）。
// 実行中のものがあればそれを、無ければ最後に終わったものを返す。
func (s *server) GetCurrentScan(w http.ResponseWriter, r *http.Request) {
	if s.scans == nil {
		s.internalError(w, "取り込みの経路が設定されていません", nil)
		return
	}

	scan, err := s.scans.CurrentScan(r.Context())
	switch {
	case errors.Is(err, domain.ErrNotFound):
		s.notFound(w, "まだ一度も取り込みを行っていません")
		return
	case err != nil:
		s.internalError(w, "取り込みの状態を取得できませんでした", err)
		return
	}

	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, toAPIScan(scan), s.logger)
}

// toAPIScan は domain.Scan を契約の形へ写す。
func toAPIScan(scan domain.Scan) gen.Scan {
	out := gen.Scan{
		Id:        scan.ID,
		State:     gen.ScanState(scan.State),
		Total:     scan.Total,
		Completed: scan.Completed,
		Failed:    scan.Failed,
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
	return out
}
