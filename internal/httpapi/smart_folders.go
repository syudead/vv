package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// スマートフォルダの経路。読み取り（GET）は所有者と
// ゲストが読め（accessRoutes）、書き込みは /api/* の既定どおり所有者だけである。所有者だけの欄
// （settings と favorite と視聴の値）はゲストの応答から外す。

// SmartFolders はスマートフォルダの保存先である。internal/store の *SmartFolderStore がこれを満たす。
// どの操作も 1 つの取引で済むので、internal/app は通さない。
type SmartFolders interface {
	// ListSmartFolders は見る人に見せるフォルダの要約を名前の順に返す。
	ListSmartFolders(ctx context.Context, audience domain.Audience) ([]domain.SmartFolderSummary, error)
	// CreateSmartFolder はフォルダを作る。名前の規則違反は *domain.InvalidGroupNameError、条件か並べ順の
	// 違反は domain.ErrInvalidSmartRule、親が無ければ domain.ErrSmartFolderNotFound である。
	CreateSmartFolder(ctx context.Context, create domain.SmartFolderCreate) (domain.SmartFolder, error)
	// ChangeSmartFolder はフォルダ id を変える。誤りは作成と同じで、加えてフォルダが無ければ
	// domain.ErrNotFound、親が自分か子孫なら domain.ErrSmartFolderCycle である。
	ChangeSmartFolder(ctx context.Context, id int64, change domain.SmartFolderChange) (domain.SmartFolder, error)
	// DeleteSmartFolder はフォルダ id とその子孫を消す。無ければ domain.ErrNotFound である。
	DeleteSmartFolder(ctx context.Context, id int64) error
	// SmartFolderContents はフォルダ id と中身 1 ページを返す。無いか見せないときは domain.ErrNotFound、
	// 読めないカーソルは domain.ErrInvalidCursor である。
	SmartFolderContents(ctx context.Context, audience domain.Audience, id int64, cursor string, limit int) (domain.SmartFolderPage, error)
	// SmartFolderCards はフォルダ ids のカードの値を ids の順に返す。無いか見せないフォルダは飛ばす。
	SmartFolderCards(ctx context.Context, audience domain.Audience, ids []int64) ([]domain.SmartFolderCard, error)
}

// smartFolderItemsDefaultLimit と smartFolderItemsMaxLimit は中身の 1 ページの件数の既定と上限で、
// グループの項目と同じである。
const (
	smartFolderItemsDefaultLimit = domain.GroupItemsDefaultLimit
	smartFolderItemsMaxLimit     = domain.GroupItemsMaxLimit
)

// ListSmartFolders は見る人に見せるフォルダを名前の順に返す（GET /api/smart-folders）。
func (s *server) ListSmartFolders(w http.ResponseWriter, r *http.Request) {
	if !s.smartFoldersConfigured(w) {
		return
	}
	audience := audienceFrom(r.Context())
	folders, err := s.smartFolders.ListSmartFolders(r.Context(), audience)
	if err != nil {
		s.internalError(w, "Could not load the smart folders.", err)
		return
	}
	out := gen.SmartFolderList{SmartFolders: make([]gen.SmartFolderSummary, 0, len(folders))}
	for _, folder := range folders {
		out.SmartFolders = append(out.SmartFolders, toAPISmartFolderSummary(audience, folder))
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, out, s.logger)
}

// smartFolderCreateBody は POST /api/smart-folders の本文である。rule は domain.ParseSmartRule が
// 読むので、生の JSON で受ける（null や知らない項目を invalid_smart_rule にするため）。sort と parentId
// の null は省略と見分けて 400 invalid_request にする（契約の型が null を許さないため）。
type smartFolderCreateBody struct {
	Name     *string         `json:"name"`
	Rule     json.RawMessage `json:"rule"`
	Sort     *string         `json:"sort"`
	ParentID *int64          `json:"parentId"`
}

// CreateSmartFolder はフォルダを作る（POST /api/smart-folders）。
func (s *server) CreateSmartFolder(w http.ResponseWriter, r *http.Request) {
	var body smartFolderCreateBody
	present, ok := s.readJSONBodyFields(w, r, &body)
	if !ok {
		return
	}
	if body.Name == nil || len(body.Rule) == 0 {
		s.invalidRequest(w, "name and rule are required.")
		return
	}
	if (present("sort") && body.Sort == nil) || (present("parentId") && body.ParentID == nil) {
		s.invalidRequest(w, "sort and parentId must not be null.")
		return
	}
	rule, err := domain.ParseSmartRule(body.Rule)
	if err != nil {
		s.writeSmartFolderError(w, err, "")
		return
	}
	create := domain.SmartFolderCreate{Name: *body.Name, Rule: rule}
	if body.Sort != nil {
		create.Sort = domain.VideoSort(*body.Sort)
	}
	if body.ParentID != nil {
		create.ParentID = *body.ParentID
	}
	if !s.smartFoldersConfigured(w) {
		return
	}
	folder, err := s.smartFolders.CreateSmartFolder(r.Context(), create)
	s.writeSmartFolder(w, r, http.StatusCreated, folder, err, "Could not create the smart folder.")
}

// smartFolderChangeBody は PATCH /api/smart-folders/{id} の本文である。parentId は省いたことと null
// （一番上にする）を分けるため、生の JSON で受ける。name と sort の null は省略と見分けて 400
// invalid_request にする（契約の型が null を許さないため）。
type smartFolderChangeBody struct {
	Name     *string         `json:"name"`
	Rule     json.RawMessage `json:"rule"`
	Sort     *string         `json:"sort"`
	ParentID json.RawMessage `json:"parentId"`
}

// ChangeSmartFolder はフォルダの名前・条件・並べ順・親を変える（PATCH /api/smart-folders/{id}）。
func (s *server) ChangeSmartFolder(w http.ResponseWriter, r *http.Request, id gen.SmartFolderId) {
	var body smartFolderChangeBody
	present, ok := s.readJSONBodyFields(w, r, &body)
	if !ok {
		return
	}
	if (present("name") && body.Name == nil) || (present("sort") && body.Sort == nil) {
		s.invalidRequest(w, "name and sort must not be null.")
		return
	}
	change, ok := s.smartFolderChange(w, body)
	if !ok || !s.smartFoldersConfigured(w) {
		return
	}
	folder, err := s.smartFolders.ChangeSmartFolder(r.Context(), id, change)
	s.writeSmartFolder(w, r, http.StatusOK, folder, err, "Could not change the smart folder.")
}

// smartFolderChange は変更の本文を保存先の入力にする。項目が 1 つも無いか parentId が整数でも null
// でもなければ 400 invalid_request、条件が規則に合わなければ 400 invalid_smart_rule を書いて false を
// 返す。
func (s *server) smartFolderChange(w http.ResponseWriter, body smartFolderChangeBody) (domain.SmartFolderChange, bool) {
	change := domain.SmartFolderChange{Name: body.Name}
	if body.Sort != nil {
		sort := domain.VideoSort(*body.Sort)
		change.Sort = &sort
	}
	if len(body.Rule) > 0 {
		rule, err := domain.ParseSmartRule(body.Rule)
		if err != nil {
			s.writeSmartFolderError(w, err, "")
			return domain.SmartFolderChange{}, false
		}
		change.Rule = &rule
	}
	if len(body.ParentID) > 0 {
		var parent *int64
		if err := json.Unmarshal(body.ParentID, &parent); err != nil {
			s.invalidRequest(w, "parentId must be an integer or null.")
			return domain.SmartFolderChange{}, false
		}
		change.ParentSet = true
		if parent != nil {
			change.ParentID = *parent
		}
	}
	if change.Name == nil && change.Sort == nil && change.Rule == nil && !change.ParentSet {
		s.invalidRequest(w, "Specify at least one of name, rule, sort and parentId.")
		return domain.SmartFolderChange{}, false
	}
	return change, true
}

// DeleteSmartFolder はフォルダとその子孫を消す（DELETE /api/smart-folders/{id}）。
func (s *server) DeleteSmartFolder(w http.ResponseWriter, r *http.Request, id gen.SmartFolderId) {
	if !s.smartFoldersConfigured(w) {
		return
	}
	if err := s.smartFolders.DeleteSmartFolder(r.Context(), id); err != nil {
		s.writeSmartFolderError(w, err, "Could not delete the smart folder.")
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	w.WriteHeader(http.StatusNoContent)
}

// GetSmartFolder はフォルダ 1 つと、その祖先と中身の最初のページを返す（GET /api/smart-folders/{id}）。
func (s *server) GetSmartFolder(w http.ResponseWriter, r *http.Request, id gen.SmartFolderId, params gen.GetSmartFolderParams) {
	limit, ok := s.smartFolderItemsLimit(w, params.Limit)
	if !ok || !s.smartFoldersConfigured(w) {
		return
	}
	audience := audienceFrom(r.Context())
	page, err := s.smartFolders.SmartFolderContents(r.Context(), audience, id, "", limit)
	if !s.smartFolderReadOK(w, err) {
		return
	}
	folder, items := s.presentSmartFolderPage(r.Context(), audience, page)
	payload := gen.SmartFolderDetail{
		SmartFolder: folder, Trail: make([]gen.GroupRef, 0, len(page.Trail)), Items: items,
		NextCursor: optionalCursor(page.NextCursor),
	}
	for _, ref := range page.Trail {
		payload.Trail = append(payload.Trail, gen.GroupRef{Id: ref.ID, Name: ref.Name})
	}
	if audience.IsOwner() {
		settings, err := toAPISmartFolderSettings(page)
		if err != nil {
			s.internalError(w, "Could not load the smart folder.", err)
			return
		}
		payload.Settings = &settings
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, payload, s.logger)
}

// ListSmartFolderItems はフォルダの中身の続きのページを返す（GET /api/smart-folders/{id}/items）。
func (s *server) ListSmartFolderItems(w http.ResponseWriter, r *http.Request, id gen.SmartFolderId, params gen.ListSmartFolderItemsParams) {
	limit, ok := s.smartFolderItemsLimit(w, params.Limit)
	if !ok || !s.smartFoldersConfigured(w) {
		return
	}
	if params.Cursor == "" {
		s.invalidRequestReason(w, reasonInvalidCursor, "Cannot read the cursor. Reload the smart folder.")
		return
	}
	audience := audienceFrom(r.Context())
	page, err := s.smartFolders.SmartFolderContents(r.Context(), audience, id, params.Cursor, limit)
	if !s.smartFolderReadOK(w, err) {
		return
	}
	_, items := s.presentSmartFolderPage(r.Context(), audience, page)
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.SmartFolderItemPage{Items: items, NextCursor: optionalCursor(page.NextCursor)}, s.logger)
}

// ListSmartFolderCards は画面に出たフォルダのカードの値を返す（GET /api/smart-folders/cards）。上限は契約どおり、重複を除く前の件数で数える。
func (s *server) ListSmartFolderCards(w http.ResponseWriter, r *http.Request, params gen.ListSmartFolderCardsParams) {
	if len(params.Ids) == 0 || len(params.Ids) > domain.MaxSmartFolderCards {
		s.invalidRequest(w, fmt.Sprintf("ids must name between 1 and %d smart folders.", domain.MaxSmartFolderCards))
		return
	}
	if !s.smartFoldersConfigured(w) {
		return
	}
	audience := audienceFrom(r.Context())
	cards, err := s.smartFolders.SmartFolderCards(r.Context(), audience, params.Ids)
	if err != nil {
		s.internalError(w, "Could not load the smart folder cards.", err)
		return
	}
	out := gen.SmartFolderCards{Cards: make([]gen.LibrarySmartFolder, 0, len(cards))}
	for _, card := range cards {
		out.Cards = append(out.Cards, s.toAPISmartFolderCard(r.Context(), audience, card))
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, out, s.logger)
}

// smartFoldersConfigured は保存先が無ければ 500 を書いて false を返す。
func (s *server) smartFoldersConfigured(w http.ResponseWriter) bool {
	if s.smartFolders == nil {
		s.internalError(w, "Smart folder storage is not configured.", nil)
		return false
	}
	return true
}

// smartFolderItemsLimit は中身の 1 ページの件数を読む。範囲の外なら 400 を書いて false を返す。
func (s *server) smartFolderItemsLimit(w http.ResponseWriter, limit *int) (int, bool) {
	if limit == nil {
		return smartFolderItemsDefaultLimit, true
	}
	if *limit < 1 || *limit > smartFolderItemsMaxLimit {
		s.invalidRequest(w, fmt.Sprintf("limit must be between 1 and %d.", smartFolderItemsMaxLimit))
		return 0, false
	}
	return *limit, true
}

// smartFolderReadOK はフォルダの読み取りの誤りを応答にし、続けてよいかを返す。
func (s *server) smartFolderReadOK(w http.ResponseWriter, err error) bool {
	switch {
	case err == nil:
		return true
	case errors.Is(err, domain.ErrNotFound):
		s.notFound(w, "The smart folder does not exist.")
	case errors.Is(err, domain.ErrInvalidCursor):
		s.invalidRequestReason(w, reasonInvalidCursor, "Cannot read the cursor. Reload the smart folder.")
	default:
		s.internalError(w, "Could not load the smart folder.", err)
	}
	return false
}

// writeSmartFolder は書き込んだあとのフォルダを要約にして返す。err があれば writeSmartFolderError で
// 応答にし、failure はその 500 の文である。書き込みの応答は favorite を載せない（お気に入りは
// PUT /api/favorites が変える）。
func (s *server) writeSmartFolder(w http.ResponseWriter, r *http.Request, status int, folder domain.SmartFolder, err error, failure string) {
	if err != nil {
		s.writeSmartFolderError(w, err, failure)
		return
	}
	out := gen.SmartFolderSummary{Id: folder.ID, Name: folder.Name, ParentId: parentIDRef(folder.ParentID)}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, status, out, s.logger)
}

// writeSmartFolderError は保存層の誤りを応答にする。条件か並べ順の
// 違反は 400 invalid_smart_rule、名前の規則違反は 400 invalid_request、親が無ければ 404
// smart_folder_not_found、フォルダが無ければ 404 not_found、入れ子の循環は 409 smart_folder_cycle、
// それ以外は message の 500 である。
func (s *server) writeSmartFolderError(w http.ResponseWriter, err error, message string) {
	var invalidName *domain.InvalidGroupNameError
	switch {
	case errors.Is(err, domain.ErrInvalidSmartRule):
		s.writeError(w, http.StatusBadRequest, codeInvalidSmartRule, "A condition or the sort is not allowed for a smart folder.")
	case errors.As(err, &invalidName):
		s.invalidRequest(w, groupNameMessage(invalidName.Problem))
	case errors.Is(err, domain.ErrSmartFolderNotFound):
		s.writeError(w, http.StatusNotFound, codeSmartFolderNotFound, "The parent smart folder no longer exists.")
	case errors.Is(err, domain.ErrNotFound):
		s.notFound(w, "The smart folder does not exist.")
	case errors.Is(err, domain.ErrSmartFolderCycle):
		s.writeError(w, http.StatusConflict, codeSmartFolderCycle, "A smart folder cannot be put inside itself or one of its children.")
	default:
		s.internalError(w, message, err)
	}
}

// presentSmartFolderPage はフォルダのカードと中身の項目を応答の形にする。項目の動画はライブラリの
// 動画の項目と同じく再生位置・タグ・フォルダを載せ、グループはライブラリのグループの項目と同じ形、
// 子フォルダは要約にする。見せる動画の無いグループの項目は入れない。
func (s *server) presentSmartFolderPage(ctx context.Context, audience domain.Audience, page domain.SmartFolderPage) (gen.LibrarySmartFolder, []gen.SmartFolderItem) {
	var shown []domain.Video
	tagged := append([]domain.Video{}, page.SmartFolder.Members...)
	for _, item := range page.Items {
		switch {
		case item.Video != nil:
			shown = append(shown, *item.Video)
			tagged = append(tagged, *item.Video)
		case item.Group != nil:
			tagged = append(tagged, item.Group.Members...)
		}
	}
	lookup := s.itemLookups(ctx, shown, tagged)
	items := make([]gen.SmartFolderItem, 0, len(page.Items))
	for _, item := range page.Items {
		switch {
		case item.SmartFolder != nil:
			child := toAPISmartFolderSummary(audience, *item.SmartFolder)
			items = append(items, gen.SmartFolderItem{Kind: gen.SmartFolderItemKindSmartFolder, SmartFolder: &child})
		case item.Video != nil:
			video := lookup.video(ctx, audience, page.Roots, *item.Video)
			items = append(items, gen.SmartFolderItem{Kind: gen.SmartFolderItemKindVideo, Video: &video})
		case item.Group != nil:
			if group, ok := lookup.group(ctx, audience, page.Roots, *item.Group); ok {
				items = append(items, gen.SmartFolderItem{Kind: gen.SmartFolderItemKindGroup, Group: &group})
			}
		}
	}
	return lookup.smartFolder(ctx, audience, page.SmartFolder), items
}

// smartFolder はフォルダのカード（値は再生順から作る）を応答の形にする。値は LibraryGroup と同じく
// 作り、再生順の空のフォルダ（所有者にだけ見せる）は本数 0 で addedAt と openVideoId を省く。
func (l itemLookup) smartFolder(ctx context.Context, audience domain.Audience, folder domain.LibrarySmartFolder) gen.LibrarySmartFolder {
	out := gen.LibrarySmartFolder{
		Id: folder.ID, Name: folder.Name, ParentId: parentIDRef(folder.ParentID),
		Previews: []gen.FolderPreview{}, Tags: []gen.VideoTag{},
	}
	values := smartFolderOwnerValues{state: domain.WatchStateUnwatched, favorite: folder.Favorite}
	// グループの項目の値の作り方をそのまま使う。フォルダの Path は空なので登録フォルダは要らない。
	if group, ok := l.group(ctx, audience, nil, folder.LibraryGroup); ok {
		addedAt, openVideoID := group.AddedAt, group.OpenVideoId
		out.VideoCount, out.DurationMs, out.SizeBytes = group.VideoCount, group.DurationMs, group.SizeBytes
		out.AddedAt, out.OpenVideoId, out.Previews, out.Tags = &addedAt, &openVideoID, group.Previews, group.Tags
		values = smartFolderOwnerValues{
			state: folder.WatchState, watched: folder.WatchedCount, lastPlayedAt: folder.LastPlayedAt, favorite: folder.Favorite,
		}
	}
	values.apply(audience, &out)
	return out
}

// toAPISmartFolderCard はカードの読み出しのフォルダ 1 つの値（一致する動画から作る）を応答の形にする。
func (s *server) toAPISmartFolderCard(ctx context.Context, audience domain.Audience, card domain.SmartFolderCard) gen.LibrarySmartFolder {
	out := gen.LibrarySmartFolder{
		Id: card.ID, Name: card.Name, ParentId: parentIDRef(card.ParentID),
		VideoCount: card.VideoCount, DurationMs: card.DurationMs, SizeBytes: card.SizeBytes,
		Previews: s.folderPreviews(ctx, card.Previews), Tags: []gen.VideoTag{},
	}
	if card.VideoCount > 0 {
		addedAt, openVideoID := card.AddedAt, card.OpenVideoID
		out.AddedAt, out.OpenVideoId = &addedAt, &openVideoID
	}
	if audience.IsOwner() {
		for _, tag := range card.Tags {
			out.Tags = append(out.Tags, toAPIVideoTag(tag))
		}
	}
	smartFolderOwnerValues{
		state: card.WatchState, watched: card.WatchedCount, lastPlayedAt: card.LastPlayedAt, favorite: card.Favorite,
	}.apply(audience, &out)
	return out
}

// smartFolderOwnerValues はカードのうち所有者の応答にだけ載せる値である。
type smartFolderOwnerValues struct {
	state        domain.WatchState
	watched      int
	lastPlayedAt *time.Time
	favorite     bool
}

// apply は所有者の応答なら値を out に載せる。ゲストには何も載せない。
func (v smartFolderOwnerValues) apply(audience domain.Audience, out *gen.LibrarySmartFolder) {
	if !audience.IsOwner() {
		return
	}
	state := gen.LibrarySmartFolderWatchState(v.state)
	watched, favorite := v.watched, v.favorite
	out.WatchState, out.WatchedCount, out.Favorite = &state, &watched, &favorite
	if v.lastPlayedAt != nil {
		played := *v.lastPlayedAt
		out.LastPlayedAt = &played
	}
}

// toAPISmartFolderSummary はフォルダの要約を応答の形にする。favorite は所有者の応答にだけ載せる。
func toAPISmartFolderSummary(audience domain.Audience, folder domain.SmartFolderSummary) gen.SmartFolderSummary {
	out := gen.SmartFolderSummary{Id: folder.ID, Name: folder.Name, ParentId: parentIDRef(folder.ParentID)}
	if audience.IsOwner() {
		favorite := folder.Favorite
		out.Favorite = &favorite
	}
	return out
}

// toAPISmartFolderSettings はフォルダの条件・並べ順・欠けた参照を応答の形にする（所有者だけ）。条件は
// domain.SmartRule の JSON が契約の SmartRule と同じ名前なので、JSON を通して写す。
func toAPISmartFolderSettings(page domain.SmartFolderPage) (gen.SmartFolderSettings, error) {
	settings := gen.SmartFolderSettings{Sort: gen.VideoSort(page.Folder.Sort), Missing: make([]gen.SmartRuleRef, 0, len(page.Missing))}
	encoded, err := json.Marshal(page.Folder.Rule)
	if err != nil {
		return gen.SmartFolderSettings{}, fmt.Errorf("cannot encode the smart rule: %w", err)
	}
	if err := json.Unmarshal(encoded, &settings.Rule); err != nil {
		return gen.SmartFolderSettings{}, fmt.Errorf("cannot encode the smart rule: %w", err)
	}
	for _, ref := range page.Missing {
		settings.Missing = append(settings.Missing, gen.SmartRuleRef{Kind: gen.SmartRuleRefKind(ref.Kind), Id: ref.ID})
	}
	return settings, nil
}

// parentIDRef は親の id を応答の形にする。根（0）は省いた値（nil）にする。
func parentIDRef(id int64) *int64 {
	if id == 0 {
		return nil
	}
	return &id
}

// optionalCursor は空のカーソルを省いた値（nil）にする。
func optionalCursor(cursor string) *string {
	if cursor == "" {
		return nil
	}
	return &cursor
}
