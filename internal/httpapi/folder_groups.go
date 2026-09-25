package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// フォルダのまとめ方の例外と、グループをタグに変える操作
// （specs/017-folder-groups/contracts/folder-groups-api.md §1・§2）。

// FolderGroups はフォルダのまとめ方の保存先である。internal/store の
// *FolderGroupStore がこれを満たす。どの操作も1つの取引で済むので、internal/app は
// 通さない（specs/017-folder-groups/plan.md の Structural Decisions 8）。
type FolderGroups interface {
	// FolderGroupings はフォルダ（絶対パス）それぞれのまとめ方を、同じ順で返す。
	FolderGroupings(ctx context.Context, folderPaths []string) ([]domain.FolderGrouping, error)
	// SetFolderGrouping はフォルダの例外を mode にし（空なら外し）、変更後のまとめ方を返す。
	SetFolderGrouping(ctx context.Context, folderPath string, mode domain.FolderGroupMode) (domain.FolderGrouping, error)
	// TagFolderGroup はフォルダのグループをタグに変える。グループでなければ
	// domain.ErrNotFolderGroup、フォルダ名がタグ名に使えなければ domain.ErrInvalidTagName。
	TagFolderGroup(ctx context.Context, folderPath string) (domain.FolderGroupTag, error)
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
		s.invalidRequest(w, "modeはauto・ungroup・groupDirectのどれかを指定してください")
		return
	}
	dir, ok := s.existingFolderDir(w, r, root, rel)
	if !ok {
		return
	}
	if s.folderGroups == nil {
		s.internalError(w, "フォルダのまとめ方の保存先が設定されていません", nil)
		return
	}

	grouping, err := s.folderGroups.SetFolderGrouping(r.Context(), dir, domainFolderGroupMode(*body.Mode))
	if err != nil {
		s.folderError(w, r, "フォルダのまとめ方を変えられませんでした", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, apiFolderGrouping(grouping, rel), s.logger)
}

// TagFolderGroup はフォルダのグループをタグに変える（POST /api/folders/{rootId}/grouping/tag）。
// 判定は 400（パス）→ 404（フォルダが無い）→ 409（登録フォルダそのもの・グループでない）
// → 400（フォルダ名がタグ名に使えない）の順に行う。
func (s *server) TagFolderGroup(w http.ResponseWriter, r *http.Request, rootID gen.FolderRootId, params gen.TagFolderGroupParams) {
	root, rel, ok := s.resolveFolderRoot(w, r, rootID, params.Path)
	if !ok {
		return
	}
	dir, ok := s.existingFolderDir(w, r, root, rel)
	if !ok {
		return
	}
	// 登録フォルダの名前はフォルダ由来のタグの照合に入らないので、タグにしても中の
	// 動画にそのタグが付かない（data-model.md §4）。
	if rel == "" {
		s.writeError(w, http.StatusConflict, codeConflict, "登録したフォルダそのもののグループはタグに変えられません")
		return
	}
	if s.folderGroups == nil {
		s.internalError(w, "フォルダのまとめ方の保存先が設定されていません", nil)
		return
	}

	result, err := s.folderGroups.TagFolderGroup(r.Context(), dir)
	switch {
	case errors.Is(err, domain.ErrNotFolderGroup):
		s.writeError(w, http.StatusConflict, codeConflict, "このフォルダは今グループではありません。フォルダを開き直してください")
		return
	case errors.Is(err, domain.ErrInvalidTagName):
		s.invalidRequest(w, "このフォルダ名はタグ名に使えません: "+err.Error())
		return
	case err != nil:
		s.folderError(w, r, "グループをタグに変えられませんでした", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.FolderGroupTagResult{
		Tag:      gen.TagRef{Id: result.Tag.ID, Name: result.Tag.Name},
		Created:  result.Created,
		Grouping: apiFolderGrouping(result.Grouping, rel),
	}, s.logger)
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
		s.folderError(w, r, "フォルダを取得できませんでした", err)
		return "", false
	}
	if !found {
		s.notFound(w, folderNotFoundMessage)
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
		view := apiFolderGrouping(grouping, folders[i].Path)
		out[i] = &view
	}
	return out, nil
}

// apiFolderGrouping はまとめ方を契約の形へ写す。rel は登録フォルダからの相対パスで、
// 空なら登録フォルダそのもの（タグに変えられない）である。
func apiFolderGrouping(grouping domain.FolderGrouping, rel string) gen.FolderGrouping {
	mode := gen.Auto
	switch grouping.Mode {
	case domain.FolderGroupUngroup:
		mode = gen.Ungroup
	case domain.FolderGroupDirect:
		mode = gen.GroupDirect
	}
	return gen.FolderGrouping{
		Mode:     mode,
		Grouped:  grouping.Grouped,
		Taggable: grouping.Grouped && rel != "",
	}
}

// domainFolderGroupMode は契約のまとめ方を保存する例外へ写す。auto は空（例外なし）である。
func domainFolderGroupMode(mode gen.FolderGroupingMode) domain.FolderGroupMode {
	switch mode {
	case gen.Ungroup:
		return domain.FolderGroupUngroup
	case gen.GroupDirect:
		return domain.FolderGroupDirect
	default:
		return ""
	}
}
