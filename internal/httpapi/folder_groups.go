package httpapi

import (
	"context"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// フォルダのまとめ方の例外（specs/017-folder-groups/contracts/folder-groups-api.md §1）。
// フォルダの自動グループをタグに変えるのは POST /api/groups/{id}/tag である
// （specs/048-groups/research.md R-11）。

// FolderGroups はフォルダのまとめ方の保存先である。internal/store の
// *FolderGroupStore がこれを満たす。どの操作も1つの取引で済むので、internal/app は
// 通さない。
type FolderGroups interface {
	// FolderGroupings はフォルダ（絶対パス）それぞれのまとめ方を、同じ順で返す。
	FolderGroupings(ctx context.Context, folderPaths []string) ([]domain.FolderGrouping, error)
	// SetFolderGrouping はフォルダの例外を mode にし（空なら外し）、変更後のまとめ方を返す。
	SetFolderGrouping(ctx context.Context, folderPath string, mode domain.FolderGroupMode) (domain.FolderGrouping, error)
}

// SetFolderGrouping はフォルダのまとめ方の例外を付け外しする
// （PUT /api/folders/{rootId}/grouping）。
func (s *server) SetFolderGrouping(w http.ResponseWriter, r *http.Request, rootID gen.FolderRootId, params gen.SetFolderGroupingParams) {
	root, rel, ok := s.resolveFolderRoot(w, r, rootID, params.Path)
	if !ok {
		return
	}
	// gen.FolderGroupingRequest の mode は string なので、欠けると空の値に読めてしまう。
	// 欠けた本文を誤りにするよう、有無を区別して読む。
	var body struct {
		Mode *gen.FolderGroupingMode `json:"mode"`
	}
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if body.Mode == nil || !body.Mode.Valid() {
		s.invalidRequest(w, "mode must be one of auto, ungroup, or groupDirect.")
		return
	}
	dir, ok := s.existingFolderDir(w, r, root, rel)
	if !ok {
		return
	}
	if s.folderGroups == nil {
		s.internalError(w, "Folder grouping storage is not configured.", nil)
		return
	}

	grouping, err := s.folderGroups.SetFolderGrouping(r.Context(), dir, domainFolderGroupMode(*body.Mode))
	if err != nil {
		s.folderError(w, r, "Could not change the folder grouping.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, apiFolderGrouping(grouping), s.logger)
}

// existingFolderDir は、フォルダが無ければ 404 を書いて false を返し、あれば絶対パスを
// 返す。判定は getFolder と同じで、登録フォルダそのものは動画が無くてもあり、それ以外は
// 所在があるときだけある（listFolderVideos と同じ）。
func (s *server) existingFolderDir(w http.ResponseWriter, r *http.Request, root domain.MediaFolder, rel string) (string, bool) {
	dir := domain.FolderDir(root.Path, rel)
	if rel == "" {
		return dir, true
	}
	found, err := s.folders.HasFolderLocations(r.Context(), audienceFrom(r.Context()), dir)
	if err != nil {
		s.folderError(w, r, "Could not load folders.", err)
		return "", false
	}
	if !found {
		s.notFoundReason(w, reasonFolderNotFound, folderNotFoundMessage)
		return "", false
	}
	return dir, true
}

// folderGroupings は、所有者に返すフォルダそれぞれのまとめ方を folders と同じ順で
// 引く。ゲストと、まとめ方の保存先が無いときは nil を返し、応答の grouping を省く。
func (s *server) folderGroupings(ctx context.Context, audience domain.Audience, folders []domain.FolderSummary) ([]*gen.FolderGrouping, error) {
	if !audience.IsOwner() || s.folderGroups == nil {
		return nil, nil
	}
	dirs := make([]string, len(folders))
	for i, folder := range folders {
		dirs[i] = domain.FolderDir(folder.RootPath, folder.Path)
	}
	groupings, err := s.folderGroups.FolderGroupings(ctx, dirs)
	if err != nil {
		return nil, err
	}
	out := make([]*gen.FolderGrouping, len(folders))
	for i, grouping := range groupings {
		view := apiFolderGrouping(grouping)
		out[i] = &view
	}
	return out, nil
}

// apiFolderGrouping はまとめ方を契約の形へ写す。自動グループが無ければ groupId を省く。
func apiFolderGrouping(grouping domain.FolderGrouping) gen.FolderGrouping {
	mode := gen.FolderGroupingModeAuto
	switch grouping.Mode {
	case domain.FolderGroupUngroup:
		mode = gen.FolderGroupingModeUngroup
	case domain.FolderGroupDirect:
		mode = gen.FolderGroupingModeGroupDirect
	}
	out := gen.FolderGrouping{Mode: mode}
	if grouping.GroupID != 0 {
		out.GroupId = &grouping.GroupID
	}
	return out
}

// domainFolderGroupMode は契約のまとめ方を保存する例外へ写す。auto は空（例外なし）である。
func domainFolderGroupMode(mode gen.FolderGroupingMode) domain.FolderGroupMode {
	switch mode {
	case gen.FolderGroupingModeUngroup:
		return domain.FolderGroupUngroup
	case gen.FolderGroupingModeGroupDirect:
		return domain.FolderGroupDirect
	default:
		return ""
	}
}
