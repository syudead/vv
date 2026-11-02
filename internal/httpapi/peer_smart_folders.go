package httpapi

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// A smart folder lists its own node's items and the other node's videos matching its rule, sent by name. The
// node that holds the folder gives the other one the rule by name in peerRule.

const remoteSmartFolderPrefix = "/api/smart-folders/"

// readsBothNodes answers a read of a link's folder, or of this node's while it reads links, and reports false for
// a read this node answers alone.
func (s *server) readsBothNodes(w http.ResponseWriter, r *http.Request, id int64, cursor string, limit int, query domain.VideoQuery) bool {
	ctx := r.Context()
	remote := domain.IsRemoteID(id) && !isPeerRequest(ctx)
	if !remote && !s.readsPeer(ctx) {
		return false
	}
	read, ok := s.parseSmartFolderRead(w, id, cursor, limit, query)
	read.cursor = read.cursor.Begin(s.readLinks(ctx))
	if remote {
		read.slot = domain.RemoteSlot(id)
	}
	switch {
	case !ok:
	case remote:
		s.readRemoteSmartFolder(w, r, read)
	default:
		s.readMergedSmartFolder(w, r, read)
	}
	return true
}

// smartRuleByName is the path query q of this node's ids by name, or nil when the peer cannot match it: a
// missing entity, or a search longer than a list takes.
func (s *server) smartRuleByName(ctx context.Context, q domain.VideoQuery) (*gen.SmartRuleByName, error) {
	if s.videos == nil || utf8.RuneCountInString(q.Query) > maxQueryLength {
		return nil, nil
	}
	named, found, err := s.videos.NameFilters(ctx, q)
	if err != nil || !found || len(named.TagNames) != len(q.TagIDs) {
		return nil, err
	}
	rule := &gen.SmartRuleByName{
		Q: optionalString(named.Query), TagKindName: optionalString(named.TagKindName),
		PersonName: optionalString(named.PersonName), OrganizationName: optionalString(named.OrganizationName),
	}
	if q.FavoriteOnly {
		rule.Favorite = &q.FavoriteOnly
	}
	if len(named.TagNames) > 0 {
		rule.TagNames = &named.TagNames
	}
	return rule, nil
}

// queryByName is rule as a query of names, which this node resolves to its own entities.
func queryByName(rule gen.SmartRuleByName) domain.VideoQuery {
	q := domain.VideoQuery{}
	for _, field := range []struct {
		value *string
		out   *string
	}{
		{rule.Q, &q.Query}, {rule.TagKindName, &q.TagKindName}, {rule.PersonName, &q.PersonName},
		{rule.OrganizationName, &q.OrganizationName},
	} {
		if field.value != nil {
			*field.out = *field.value
		}
	}
	if rule.TagNames != nil {
		q.TagNames = *rule.TagNames
	}
	q.FavoriteOnly = rule.Favorite != nil && *rule.Favorite
	return q
}

// peerSmartRule is what the peer's request needs to add its own videos to this node's folder with path rules;
// own, when set, also gives the folder's own entities by name and sort its saved order.
func (s *server) peerSmartRule(ctx context.Context, rules []domain.SmartRule, own *domain.SmartFolder) (*gen.PeerSmartRule, error) {
	if !isPeerRequest(ctx) {
		return nil, nil
	}
	out := &gen.PeerSmartRule{Favorite: slices.ContainsFunc(rules, func(rule domain.SmartRule) bool { return rule.Favorite })}
	if q, ok := domain.SmartPathQuery(rules); ok {
		match, err := s.smartRuleByName(ctx, q)
		if err != nil {
			return nil, err
		}
		out.Match = match
	}
	if own == nil {
		return out, nil
	}
	sort := gen.VideoSort(own.Sort)
	names, err := s.smartRuleNames(ctx, own.Rule)
	out.Sort, out.Names = &sort, &names
	return out, err
}

// smartRuleNames gives each entity rule names its canonical name, leaving out a missing one.
func (s *server) smartRuleNames(ctx context.Context, rule domain.SmartRule) ([]gen.SmartRuleRefName, error) {
	out := []gen.SmartRuleRefName{}
	if s.videos == nil {
		return out, nil
	}
	for _, ref := range rule.Refs() {
		var q domain.VideoQuery
		switch ref.Kind {
		case domain.SmartRefTag:
			q.TagIDs = []int64{ref.ID}
		case domain.SmartRefTagKind:
			q.TagKindID = ref.ID
		case domain.SmartRefPerson:
			q.Person = ref.ID
		case domain.SmartRefOrganization:
			q.Organization = ref.ID
		}
		named, found, err := s.videos.NameFilters(ctx, q)
		if err != nil {
			return nil, err
		}
		name := named.TagKindName + named.PersonName + named.OrganizationName
		if len(named.TagNames) == 1 {
			name = named.TagNames[0]
		}
		if found && name != "" {
			out = append(out, gen.SmartRuleRefName{Kind: gen.SmartRuleRefNameKind(ref.Kind), Id: ref.ID, Name: name})
		}
	}
	return out, nil
}

func (s *server) CountPeerRuleMatches(w http.ResponseWriter, r *http.Request) {
	if !s.requirePeerCaller(w, r) || !s.smartFoldersConfigured(w) {
		return
	}
	var body gen.PeerRuleCountsRequest
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if len(body.Rules) == 0 || len(body.Rules) > domain.MaxSmartFolderCards {
		s.invalidRequest(w, fmt.Sprintf("rules must hold between 1 and %d rules.", domain.MaxSmartFolderCards))
		return
	}
	rules := make([]domain.VideoQuery, len(body.Rules))
	for i, rule := range body.Rules {
		rules[i] = queryByName(rule)
		if len(rules[i].TagNames) > domain.SmartRuleMaxTags || utf8.RuneCountInString(rules[i].Query) > maxQueryLength {
			s.invalidRequest(w, "A rule names too many tags or has too long a search.")
			return
		}
	}
	counts, err := s.smartFolders.SmartRuleCounts(r.Context(), audienceFrom(r.Context()), rules)
	if err != nil {
		s.internalError(w, "Could not count the smart rules.", err)
		return
	}
	out := gen.PeerRuleCounts{Items: make([]gen.PeerRuleCount, len(counts))}
	for i, count := range counts {
		out.Items[i] = gen.PeerRuleCount{VideoCount: count.VideoCount, DurationMs: count.DurationMs, SizeBytes: count.SizeBytes}
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, out, s.logger)
}

// withPeerSmartFolders adds every connected link's folders, with remote ids, to the local list in natural-name
// order; a link that does not answer is left out.
func (s *server) withPeerSmartFolders(r *http.Request, local []gen.SmartFolderSummary) []gen.SmartFolderSummary {
	merged := slices.Clone(local)
	for _, answer := range linkAnswers[gen.SmartFolderList](s, r, "/api/smart-folders", nil) {
		join := domain.VocabularyJoin{Slot: answer.slot}
		for _, folder := range answer.value.SmartFolders {
			rewriteRemoteSmartFolderSummary(&folder, join)
			merged = append(merged, folder)
		}
	}
	slices.SortStableFunc(merged, func(a, b gen.SmartFolderSummary) int {
		if order := strings.Compare(domain.NaturalSortKey(a.Name), domain.NaturalSortKey(b.Name)); order != 0 {
			return order
		}
		return cmp.Compare(a.Id, b.Id)
	})
	return merged
}

// rewriteRemoteSmartFolderSummary keeps the peer's favorite, which lives on the node holding the folder.
func rewriteRemoteSmartFolderSummary(folder *gen.SmartFolderSummary, join domain.VocabularyJoin) {
	folder.Id = join.Remote(folder.Id)
	if folder.ParentId != nil {
		*folder.ParentId = join.Remote(*folder.ParentId)
	}
}

// rewriteRemoteSmartFolderCard keeps the peer's watch values and favorite, as a remote group does, and leaves out
// the rule only this node reads.
func rewriteRemoteSmartFolderCard(card *gen.LibrarySmartFolder, join domain.VocabularyJoin) {
	card.Id, card.PeerRule = join.Remote(card.Id), nil
	if card.ParentId != nil {
		*card.ParentId = join.Remote(*card.ParentId)
	}
	if card.OpenVideoId != nil {
		*card.OpenVideoId = join.Remote(*card.OpenVideoId)
	}
	rewriteRemotePreviews(card.Previews, join)
	rewriteRemoteTags(card.Tags, join)
}

// cardCount is a card's video count, duration and size.
func cardCount(card gen.LibrarySmartFolder) domain.SmartRuleCount {
	return domain.SmartRuleCount{VideoCount: card.VideoCount, DurationMs: card.DurationMs, SizeBytes: card.SizeBytes}
}

func setCardCount(card *gen.LibrarySmartFolder, count domain.SmartRuleCount) {
	card.VideoCount, card.DurationMs, card.SizeBytes = count.VideoCount, count.DurationMs, count.SizeBytes
}

// cardRule is a card's rule by name for the links that do not hold its folder; owner is the slot of the node
// that holds it, 0 for this node, whose own count the card already has.
type cardRule struct {
	owner int64
	rule  gen.SmartRuleByName
	card  *gen.LibrarySmartFolder
}

// mergedCards are a cards request's cards of every node and what each adds from the other nodes.
type mergedCards struct {
	cards map[int64]*gen.LibrarySmartFolder
	// linkRules are the cards' rules the links count.
	linkRules []cardRule
	// localRules are the remote cards' rules this node counts, and localCards the cards they add to.
	localRules []domain.VideoQuery
	localCards []*gen.LibrarySmartFolder
}

// listMergedSmartFolderCards answers the cards of the folders of this node and its links, each with the other
// nodes' matches added.
func (s *server) listMergedSmartFolderCards(w http.ResponseWriter, r *http.Request, ids []int64) {
	ctx := r.Context()
	audience := audienceFrom(ctx)
	var local []int64
	remote := map[int64][]int64{}
	var slots []int64
	for _, id := range ids {
		if !domain.IsRemoteID(id) {
			local = append(local, id)
			continue
		}
		slot := domain.RemoteSlot(id)
		if _, ok := remote[slot]; !ok {
			slots = append(slots, slot)
		}
		remote[slot] = append(remote[slot], domain.FromRemoteID(id))
	}
	merged := mergedCards{cards: map[int64]*gen.LibrarySmartFolder{}}
	if len(local) > 0 {
		cards, err := s.smartFolders.SmartFolderCards(ctx, audience, local)
		if err != nil {
			s.internalError(w, "Could not load the smart folder cards.", err)
			return
		}
		if err := s.addLocalCards(ctx, &merged, cards); err != nil {
			s.internalError(w, "Could not load the smart folder cards.", err)
			return
		}
	}
	if err := s.addRemoteCards(r, slots, remote, &merged); err != nil {
		s.internalError(w, "Could not load the smart folder cards.", err)
		return
	}
	if err := s.addCardCounts(r, merged); err != nil {
		s.internalError(w, "Could not load the smart folder cards.", err)
		return
	}
	out := gen.SmartFolderCards{Cards: []gen.LibrarySmartFolder{}}
	seen := map[int64]bool{}
	for _, id := range ids {
		if card, ok := merged.cards[id]; ok && !seen[id] {
			seen[id] = true
			out.Cards = append(out.Cards, *card)
		}
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, out, s.logger)
}

func (s *server) addLocalCards(ctx context.Context, merged *mergedCards, cards []domain.SmartFolderCard) error {
	audience := audienceFrom(ctx)
	for _, card := range cards {
		out := s.toAPISmartFolderCard(ctx, audience, card)
		merged.cards[card.ID] = &out
		q, ok := domain.SmartPathQuery(card.PathRules)
		if !ok {
			continue
		}
		rule, err := s.smartRuleByName(ctx, q)
		if err != nil {
			return err
		}
		if rule != nil {
			merged.linkRules = append(merged.linkRules, cardRule{rule: *rule, card: &out})
		}
	}
	return nil
}

// addRemoteCards reads the cards of each link in slots, its ids in ids, concurrently; a link that does not answer
// leaves its cards out, as a merged list does.
func (s *server) addRemoteCards(r *http.Request, slots []int64, ids map[int64][]int64, merged *mergedCards) error {
	answers, errs := eachLink(slots, func(slot int64) (gen.SmartFolderCards, error) {
		values := make([]string, len(ids[slot]))
		for i, id := range ids[slot] {
			values[i] = strconv.FormatInt(id, 10)
		}
		var answer gen.SmartFolderCards
		err := s.peerGet(r, slot, "/api/smart-folders/cards", url.Values{"ids": {strings.Join(values, ",")}}, &answer)
		return answer, err
	})
	join, err := s.vocabularyJoin(r.Context(), 0)
	if err != nil {
		return err
	}
	for k, slot := range slots {
		if errs[k] != nil {
			s.logger.Info("the smart folder cards leave out a linked VVMDM", slog.Int64("slot", slot), slog.Any("error", errs[k]))
			continue
		}
		for i := range answers[k].Cards {
			card := &answers[k].Cards[i]
			rule := card.PeerRule
			rewriteRemoteSmartFolderCard(card, join.InSlot(slot))
			merged.cards[card.Id] = card
			match := matchWithFavorite(rule)
			if match == nil {
				continue
			}
			q := queryByName(*match)
			merged.localRules, merged.localCards = append(merged.localRules, q), append(merged.localCards, card)
			merged.linkRules = append(merged.linkRules, cardRule{owner: slot, rule: *match, card: card})
		}
	}
	return nil
}

// addCardCounts adds this node's counts to the remote cards, and each connected link's to the cards of the other
// nodes, one request per link sent concurrently; a link that does not answer adds nothing.
func (s *server) addCardCounts(r *http.Request, merged mergedCards) error {
	if len(merged.localRules) > 0 {
		counts, err := s.smartFolders.SmartRuleCounts(r.Context(), audienceFrom(r.Context()), merged.localRules)
		if err != nil {
			return err
		}
		for i, count := range counts {
			setCardCount(merged.localCards[i], cardCount(*merged.localCards[i]).Add(count))
		}
	}
	if len(merged.linkRules) == 0 {
		return nil
	}
	slots := s.connectedSlots(r.Context())
	counted := make([][]cardRule, len(slots))
	for k, slot := range slots {
		for _, rule := range merged.linkRules {
			if rule.owner != slot {
				counted[k] = append(counted[k], rule)
			}
		}
	}
	answers, errs := eachLink(slots, func(slot int64) (gen.PeerRuleCounts, error) {
		rules := counted[slices.Index(slots, slot)]
		var answer gen.PeerRuleCounts
		if len(rules) == 0 {
			return answer, nil
		}
		body := gen.PeerRuleCountsRequest{Rules: make([]gen.SmartRuleByName, len(rules))}
		for i, rule := range rules {
			body.Rules[i] = rule.rule
		}
		err := s.peerCall(r, slot, http.MethodPost, "/api/peer/rule-counts", nil, body, &answer)
		if err == nil && len(answer.Items) != len(rules) {
			err = fmt.Errorf("%w: %d counts for %d rules", errPeerAnswer, len(answer.Items), len(rules))
		}
		return answer, err
	})
	for k, slot := range slots {
		if errs[k] != nil {
			s.logger.Info("the smart folder cards leave out a linked VVMDM's videos", slog.Int64("slot", slot), slog.Any("error", errs[k]))
			continue
		}
		for i, item := range answers[k].Items {
			count := domain.SmartRuleCount{VideoCount: item.VideoCount, DurationMs: item.DurationMs, SizeBytes: item.SizeBytes}
			setCardCount(counted[k][i].card, cardCount(*counted[k][i].card).Add(count))
		}
	}
	return nil
}

// smartFolderRead is one page request of a smart folder; first is the detail, which adds the child folders.
type smartFolderRead struct {
	id int64
	// slot is the folder's link for a remote folder, 0 for this node's.
	slot   int64
	cursor domain.MergedCursor
	limit  int
	query  domain.VideoQuery
	first  bool
}

// smartFolderPage is a merged page: the detail's folder, trail and settings, child folders and the items.
type smartFolderPage struct {
	folder   gen.LibrarySmartFolder
	trail    []gen.GroupRef
	settings *gen.SmartFolderSettings
	children []gen.SmartFolderItem
	items    []gen.SmartFolderItem
	next     string
}

func (s *server) parseSmartFolderRead(w http.ResponseWriter, id int64, cursor string, limit int, query domain.VideoQuery) (smartFolderRead, bool) {
	parsed, err := domain.ParseMergedCursor(cursor)
	if err != nil {
		s.invalidRequestReason(w, reasonInvalidCursor, "Cannot read the cursor. Reload the smart folder.")
		return smartFolderRead{}, false
	}
	return smartFolderRead{id: id, cursor: parsed, limit: limit, query: query, first: cursor == ""}, true
}

// writeSmartFolderPage answers the detail on the first page and the item page after it.
func (s *server) writeSmartFolderPage(w http.ResponseWriter, read smartFolderRead, page smartFolderPage) {
	w.Header().Set("Cache-Control", cacheNoStore)
	if !read.first {
		writeJSON(w, http.StatusOK, gen.SmartFolderItemPage{Items: page.items, NextCursor: optionalCursor(page.next)}, s.logger)
		return
	}
	detail := gen.SmartFolderDetail{
		SmartFolder: page.folder, Trail: page.trail, Settings: page.settings,
		Items: slices.Concat(page.children, page.items), NextCursor: optionalCursor(page.next),
	}
	if detail.Trail == nil {
		detail.Trail = []gen.GroupRef{}
	}
	writeJSON(w, http.StatusOK, detail, s.logger)
}

// readMergedSmartFolder reads one page of this node's folder with every link's videos matching its rule.
func (s *server) readMergedSmartFolder(w http.ResponseWriter, r *http.Request, read smartFolderRead) {
	ctx := r.Context()
	audience := audienceFrom(ctx)
	own := read.cursor.Side(0)
	contents, err := s.smartFolders.SmartFolderContents(ctx, audience, read.id, own.Cursor, sideLimit(read.limit, own.Done), read.query)
	if !s.smartFolderReadOK(w, err) {
		return
	}
	join, err := s.vocabularyJoin(ctx, 0)
	if err != nil {
		s.internalError(w, "Could not load the smart folder.", err)
		return
	}
	folder, items, cursors := s.presentSmartFolderPage(ctx, audience, contents)
	page := smartFolderPage{folder: folder}
	children := slices.IndexFunc(items, func(item gen.SmartFolderItem) bool { return item.Kind != gen.SmartFolderItemKindSmartFolder })
	if children < 0 {
		children = len(items)
	}
	if read.first {
		page.children = items[:children]
		for _, ref := range contents.Trail {
			page.trail = append(page.trail, gen.GroupRef{Id: ref.ID, Name: ref.Name})
		}
		if audience.IsOwner() {
			settings, err := toAPISmartFolderSettings(contents)
			if err != nil {
				s.internalError(w, "Could not load the smart folder.", err)
				return
			}
			page.settings = &settings
		}
	}
	local := domain.MergeSide[gen.SmartFolderItem]{Items: items[children:], Cursors: cursors, Next: contents.NextCursor}
	if own.Done {
		local = domain.MergeSide[gen.SmartFolderItem]{}
	}
	sort := cmp.Or(read.query.Sort, contents.Folder.Sort)
	rule, ruleErr := s.folderRuleByName(ctx, contents)
	sides := s.linkSmartFolderSides(r, &read, linkRule{rule: rule, err: ruleErr, join: join, sort: sort}, domain.MergeSide[gen.SmartFolderItem]{})
	merged, next, more := domain.MergePages(sort, read.cursor, append(sides, local), read.limit)
	page.items = merged
	if more {
		page.next = next.Encode()
	}
	s.writeSmartFolderPage(w, read, page)
}

// folderRuleByName is the rule of this node's folder by name for the links, or nil when no link can match it: an
// entity of the rule is missing, or the rule is not one a video list takes.
func (s *server) folderRuleByName(ctx context.Context, contents domain.SmartFolderPage) (*gen.SmartRuleByName, error) {
	if len(contents.Missing) > 0 {
		return nil, nil
	}
	q, ok := domain.SmartPathQuery(contents.PathRules)
	if !ok {
		return nil, nil
	}
	return s.smartRuleByName(ctx, q)
}

// linkRule is how the links that do not hold a smart folder match it: its rule by name, nil when they cannot, or
// the error that kept it from being named, with this node's names and the folder's order.
type linkRule struct {
	rule *gen.SmartRuleByName
	err  error
	join domain.VocabularyJoin
	sort domain.VideoSort
}

// linkSmartFolderSides reads the side of every link of the listing concurrently: own, the folder's own items, for
// the link holding a remote folder, and every other link's videos matching match. A link that fails is left out for
// the rest of the listing; read's cursor records it.
func (s *server) linkSmartFolderSides(
	r *http.Request, read *smartFolderRead, match linkRule, own domain.MergeSide[gen.SmartFolderItem],
) []domain.MergeSide[gen.SmartFolderItem] {
	parts, cursor := readLinkParts(s, remoteSmartFolderPrefix, read.cursor,
		func(slot int64, side domain.SideCursor) (listPart[gen.SmartFolderItem], error) {
			switch {
			case slot == read.slot:
				return listPart[gen.SmartFolderItem]{side: own}, nil
			case match.err != nil:
				return listPart[gen.SmartFolderItem]{}, match.err
			case side.Done || match.rule == nil:
				return listPart[gen.SmartFolderItem]{}, nil
			}
			videos, err := s.readPeerVideos(r, *match.rule, *read, slot, side.Cursor, match)
			return listPart[gen.SmartFolderItem]{side: videos}, err
		})
	read.cursor = cursor
	sides := make([]domain.MergeSide[gen.SmartFolderItem], len(parts))
	for i, part := range parts {
		sides[i] = part.side
	}
	return sides
}

// readPeerVideos reads the videos of the link in slot matching rule after that link's cursor; a rule and search
// that do not join into one search a list takes leave the link's part empty.
func (s *server) readPeerVideos(
	r *http.Request, rule gen.SmartRuleByName, read smartFolderRead, slot int64, after string, match linkRule,
) (domain.MergeSide[gen.SmartFolderItem], error) {
	values := url.Values{"limit": {strconv.Itoa(read.limit)}, "sort": {string(match.sort)}}
	if after != "" {
		values.Set("cursor", after)
	}
	if match.sort == domain.SortRandom {
		values.Set("seed", strconv.FormatInt(read.query.Seed, 10))
	}
	q := queryByName(rule)
	search, joins := domain.JoinSearches(q.Query, read.query.Query)
	if !joins || utf8.RuneCountInString(search) > maxQueryLength {
		return domain.MergeSide[gen.SmartFolderItem]{}, nil
	}
	q.Query, q.FavoriteOnly = search, q.FavoriteOnly || read.query.FavoriteOnly
	setNameFilters(values, q)
	var answer gen.VideoPage
	if err := s.peerGet(r, slot, "/api/videos", values, &answer); err != nil {
		return domain.MergeSide[gen.SmartFolderItem]{}, err
	}
	rewriteRemoteVideos(answer.Items, match.join.InSlot(slot))
	return peerSide(videoItems(answer.Items), answer.PeerItems, answer.NextCursor)
}

func videoItems(videos []gen.Video) []gen.SmartFolderItem {
	items := make([]gen.SmartFolderItem, len(videos))
	for i := range videos {
		items[i] = gen.SmartFolderItem{Kind: gen.SmartFolderItemKindVideo, Video: &videos[i]}
	}
	return items
}

// errRemoteSmartFolder is the peer's refusal of a remote folder read, answered as the peer answered it.
var errRemoteSmartFolder = errors.New("the other VVMDM refused the smart folder")

// readRemoteSmartFolder reads one page of a link's folder with the videos of this node and of every other link
// matching its rule. The link's detail gives the rule on every page; its later pages continue with the link's own
// cursor.
func (s *server) readRemoteSmartFolder(w http.ResponseWriter, r *http.Request, read smartFolderRead) {
	path := remoteSmartFolderPrefix + strconv.FormatInt(domain.FromRemoteID(read.id), 10)
	values := remoteSmartFolderValues(read)
	// Once a page has taken the link's items, later pages read the detail for the rule alone; until then its
	// items are the link's first page.
	detailValues := maps.Clone(values)
	if read.cursor.Side(read.slot).Cursor != "" {
		detailValues.Set("limit", "1")
	}
	var detail gen.SmartFolderDetail
	if !s.remoteSmartFolderAnswer(w, s.peerGet(r, read.slot, path, detailValues, &detail)) {
		return
	}
	rule := detail.SmartFolder.PeerRule
	if rule == nil || rule.Sort == nil {
		s.internalError(w, "The other VVMDM answered in an unexpected form.", nil)
		return
	}
	join, err := s.vocabularyJoin(r.Context(), 0)
	if err != nil {
		s.internalError(w, "Could not load the smart folder.", err)
		return
	}
	sort := cmp.Or(read.query.Sort, domain.VideoSort(*rule.Sort))
	own, ok := s.remoteFolderSide(w, r, read, path, values, detail, join.InSlot(read.slot))
	if !ok {
		return
	}
	local, err := s.localMatches(r, read, rule, sort)
	if err != nil {
		s.checkListRead(w, err)
		return
	}
	sides := s.linkSmartFolderSides(r, &read, linkRule{rule: matchWithFavorite(rule), join: join, sort: sort}, own)
	merged, next, more := domain.MergePages(sort, read.cursor, append(sides, local), read.limit)
	page := smartFolderPage{items: merged}
	if more {
		page.next = next.Encode()
	}
	if read.first {
		s.remoteFolderDetail(&page, detail, join.InSlot(read.slot))
	}
	s.writeSmartFolderPage(w, read, page)
}

// remoteSmartFolderValues are the query of a remote folder read: the request's order, favorite filter and search.
func remoteSmartFolderValues(read smartFolderRead) url.Values {
	values := url.Values{"limit": {strconv.Itoa(read.limit)}}
	if read.query.Sort != "" {
		values.Set("sort", string(read.query.Sort))
	}
	if read.query.FavoriteOnly {
		values.Set("favorite", "true")
	}
	if read.query.Sort == domain.SortRandom {
		values.Set("seed", strconv.FormatInt(read.query.Seed, 10))
	}
	if read.query.Query != "" {
		values.Set("query", read.query.Query)
	}
	return values
}

// matchWithFavorite is the rule by name of a link's folder for the other nodes, favorite as the folder's rules
// say, or nil when they cannot match it.
func matchWithFavorite(rule *gen.PeerSmartRule) *gen.SmartRuleByName {
	if rule == nil || rule.Match == nil {
		return nil
	}
	match := *rule.Match
	match.Favorite = nil
	if rule.Favorite {
		match.Favorite = &rule.Favorite
	}
	return &match
}

// remoteSmartFolderAnswer answers r when the peer's read failed, keeping a missing folder missing.
func (s *server) remoteSmartFolderAnswer(w http.ResponseWriter, err error) bool {
	var refusal *peerRefusal
	switch {
	case err == nil:
		return true
	case errors.As(err, &refusal) && refusal.status == http.StatusNotFound:
		s.notFound(w, "The smart folder does not exist.")
	case errors.As(err, &refusal) && refusal.reason != nil && *refusal.reason == reasonInvalidCursor:
		s.invalidRequestReason(w, reasonInvalidCursor, "Cannot read the cursor. Reload the smart folder.")
	case errors.Is(err, domain.ErrPeerUnreachable), errors.As(err, &refusal):
		s.peerUnreachable(w)
	default:
		s.internalError(w, "Could not load the smart folder.", fmt.Errorf("%w: %w", errRemoteSmartFolder, err))
	}
	return false
}

// remoteFolderSide is the link's own items of its folder: the detail's until a page has taken one of them, then
// the items after the link's cursor.
func (s *server) remoteFolderSide(w http.ResponseWriter, r *http.Request, read smartFolderRead, path string, values url.Values,
	detail gen.SmartFolderDetail, join domain.VocabularyJoin,
) (domain.MergeSide[gen.SmartFolderItem], bool) {
	own := read.cursor.Side(read.slot)
	if own.Done {
		return domain.MergeSide[gen.SmartFolderItem]{}, true
	}
	items, peerItems, next := detail.Items, detail.PeerItems, detail.NextCursor
	if own.Cursor != "" {
		values.Set("cursor", own.Cursor)
		var page gen.SmartFolderItemPage
		if !s.remoteSmartFolderAnswer(w, s.peerGet(r, read.slot, path+"/items", values, &page)) {
			return domain.MergeSide[gen.SmartFolderItem]{}, false
		}
		items, peerItems, next = page.Items, page.PeerItems, page.NextCursor
	}
	shown := slices.DeleteFunc(slices.Clone(items), func(item gen.SmartFolderItem) bool {
		return item.Kind == gen.SmartFolderItemKindSmartFolder
	})
	for i := range shown {
		rewriteRemoteSmartFolderItem(&shown[i], join)
	}
	side, err := peerSide(shown, peerItems, next)
	if err != nil {
		s.internalError(w, "The other VVMDM answered in an unexpected form.", err)
		return domain.MergeSide[gen.SmartFolderItem]{}, false
	}
	return side, true
}

// localMatches lists this node's videos matching a link's folder after the read's local cursor.
func (s *server) localMatches(r *http.Request, read smartFolderRead, rule *gen.PeerSmartRule, sort domain.VideoSort) (domain.MergeSide[gen.SmartFolderItem], error) {
	local := read.cursor.Side(0)
	if rule.Match == nil || local.Done || s.videos == nil {
		return domain.MergeSide[gen.SmartFolderItem]{}, nil
	}
	q := queryByName(*rule.Match)
	search, joins := domain.JoinSearches(q.Query, read.query.Query)
	if !joins {
		return domain.MergeSide[gen.SmartFolderItem]{}, nil
	}
	q.Query = search
	q.FavoriteOnly = rule.Favorite || read.query.FavoriteOnly
	q.Sort, q.Seed, q.Cursor, q.Limit = sort, read.query.Seed, local.Cursor, read.limit
	ctx := r.Context()
	page, err := s.videos.ListVideos(ctx, audienceFrom(ctx), q)
	if err != nil {
		return domain.MergeSide[gen.SmartFolderItem]{}, err
	}
	return domain.MergeSide[gen.SmartFolderItem]{
		Items: videoItems(s.listItems(ctx, page.Items, s.registeredRoots(ctx))), Cursors: page.ItemCursors, Next: page.NextCursor,
	}, nil
}

// remoteFolderDetail sets the first page's folder, trail, settings and child folders from the peer's detail.
func (s *server) remoteFolderDetail(page *smartFolderPage, detail gen.SmartFolderDetail, join domain.VocabularyJoin) {
	names := detail.SmartFolder.PeerRule.Names
	rewriteRemoteSmartFolderCard(&detail.SmartFolder, join)
	page.folder = detail.SmartFolder
	page.trail = detail.Trail
	for i := range page.trail {
		page.trail[i].Id = join.Remote(page.trail[i].Id)
	}
	if settings := detail.Settings; settings != nil {
		rewriteRemoteSmartRule(&settings.Rule, names, join)
		for i := range settings.Missing {
			settings.Missing[i].Id = join.Remote(settings.Missing[i].Id)
		}
		page.settings = settings
	}
	for _, item := range detail.Items {
		if item.Kind == gen.SmartFolderItemKindSmartFolder && item.SmartFolder != nil {
			rewriteRemoteSmartFolderSummary(item.SmartFolder, join)
			page.children = append(page.children, item)
		}
	}
}

func rewriteRemoteSmartFolderItem(item *gen.SmartFolderItem, join domain.VocabularyJoin) {
	if item.Video != nil {
		rewriteRemoteVideo(item.Video, join)
	}
	if item.Group != nil {
		rewriteRemoteGroup(item.Group, join)
	}
}

// rewriteRemoteSmartRule joins the rule's entities by the names the peer gave; an entity without a name, which
// is missing on the peer, takes the offset.
func rewriteRemoteSmartRule(rule *gen.SmartRule, names *[]gen.SmartRuleRefName, join domain.VocabularyJoin) {
	name := func(kind gen.SmartRuleRefNameKind, id int64) string {
		if names == nil {
			return ""
		}
		for _, ref := range *names {
			if ref.Kind == kind && ref.Id == id {
				return ref.Name
			}
		}
		return ""
	}
	if rule.Tags != nil {
		for i, id := range *rule.Tags {
			(*rule.Tags)[i] = join.Tag(id, name(gen.SmartRuleRefNameKindTag, id))
		}
	}
	for _, ref := range []struct {
		id   *int64
		kind gen.SmartRuleRefNameKind
		join func(int64, string) int64
	}{
		{rule.TagKind, gen.SmartRuleRefNameKindTagKind, join.TagKind},
		{rule.Person, gen.SmartRuleRefNameKindPerson, join.Person},
		{rule.Organization, gen.SmartRuleRefNameKindOrganization, join.Organization},
	} {
		if ref.id != nil {
			*ref.id = ref.join(*ref.id, name(ref.kind, *ref.id))
		}
	}
}

// noPeerEntityError is a rule entity of this node that the peer has none of the same name for.
type noPeerEntityError struct {
	kind domain.SmartRuleRefKind
	name string
}

func (e *noPeerEntityError) Error() string {
	return fmt.Sprintf("the other VVMDM has no %s named %q", e.kind, e.name)
}

var remoteSmartFolderAnswer = remoteOperation{rewrite: rewriteAs(func(folder *gen.SmartFolderSummary, join domain.VocabularyJoin) {
	rewriteRemoteSmartFolderSummary(folder, join)
})}

// peerSmartRuleIDs is rule in the ids of the link in slot: that link's id loses the offset, and this node's entity
// takes that link's entity of the same name.
func (s *server) peerSmartRuleIDs(r *http.Request, slot int64, rule domain.SmartRule) (domain.SmartRule, error) {
	out := rule
	out.Tags = slices.Clone(rule.Tags)
	var err error
	for i, id := range out.Tags {
		if out.Tags[i], err = s.peerEntityID(r, slot, domain.SmartRefTag, id); err != nil {
			return domain.SmartRule{}, err
		}
	}
	for _, ref := range []struct {
		kind domain.SmartRuleRefKind
		id   *int64
	}{
		{domain.SmartRefTagKind, &out.TagKind}, {domain.SmartRefPerson, &out.Person},
		{domain.SmartRefOrganization, &out.Organization},
	} {
		if *ref.id, err = s.peerEntityID(r, slot, ref.kind, *ref.id); err != nil {
			return domain.SmartRule{}, err
		}
	}
	return out, nil
}

func (s *server) peerEntityID(r *http.Request, slot int64, kind domain.SmartRuleRefKind, id int64) (int64, error) {
	switch {
	case id == 0 || (domain.IsRemoteID(id) && domain.RemoteSlot(id) == slot):
		return peerID(id), nil
	case domain.IsRemoteID(id):
		return 0, &noPeerEntityError{kind: kind, name: "#" + strconv.FormatInt(id, 10)}
	}
	names, err := s.smartRuleNames(r.Context(), smartRuleOf(kind, id))
	if err != nil {
		return 0, err
	}
	if len(names) == 0 {
		return 0, &noPeerEntityError{kind: kind, name: "#" + strconv.FormatInt(id, 10)}
	}
	name := names[0].Name
	var found int64
	switch kind {
	case domain.SmartRefTag:
		found, err = peerIDByName(s, r, slot, "/api/tags", name, func(tag gen.Tag) (int64, string) { return tag.Id, tag.Name })
	case domain.SmartRefTagKind:
		found, err = peerIDByName(s, r, slot, "/api/tag-kinds", name, func(kind gen.TagKind) (int64, string) { return kind.Id, kind.Name })
	case domain.SmartRefPerson:
		found, err = peerIDByName(s, r, slot, "/api/people", name, func(person gen.Person) (int64, string) { return person.Id, person.Name })
	case domain.SmartRefOrganization:
		found, err = peerIDByName(s, r, slot, "/api/organizations", name, func(organization gen.Organization) (int64, string) {
			return organization.Id, organization.Name
		})
	}
	if err == nil && found == 0 {
		err = &noPeerEntityError{kind: kind, name: name}
	}
	return found, err
}

func smartRuleOf(kind domain.SmartRuleRefKind, id int64) domain.SmartRule {
	switch kind {
	case domain.SmartRefTag:
		return domain.SmartRule{Tags: []int64{id}}
	case domain.SmartRefTagKind:
		return domain.SmartRule{TagKind: id}
	case domain.SmartRefPerson:
		return domain.SmartRule{Person: id}
	default:
		return domain.SmartRule{Organization: id}
	}
}

// peerIDByName is the lowest id of the peer's entity at path named name, or zero; the kind list takes no
// search and is read whole.
func peerIDByName[T any](s *server, r *http.Request, slot int64, path, name string, entry func(T) (int64, string)) (int64, error) {
	var answer struct {
		Items []T `json:"items"`
	}
	query := url.Values{}
	if path != "/api/tag-kinds" {
		query.Set("q", name)
	}
	if err := s.peerGet(r, slot, path, query, &answer); err != nil {
		return 0, err
	}
	var found int64
	for _, item := range answer.Items {
		if id, itemName := entry(item); itemName == name && (found == 0 || id < found) {
			found = id
		}
	}
	return found, nil
}

// writePeerSmartRuleError answers a rule the peer cannot take, or the peer's failure.
func (s *server) writePeerSmartRuleError(w http.ResponseWriter, err error) {
	var missing *noPeerEntityError
	switch {
	case errors.As(err, &missing):
		s.writeError(w, http.StatusBadRequest, codeInvalidSmartRule, "The other VVMDM has no "+string(missing.kind)+
			" named "+missing.name+". Choose one both VVMDMs have, or one of the other VVMDM.")
	case errors.Is(err, domain.ErrPeerUnreachable), errors.Is(err, errPeerAnswer):
		s.peerUnreachable(w)
	default:
		s.internalError(w, "Could not forward the smart folder to the other VVMDM.", err)
	}
}

// remoteSmartFolderBody is a create or change for the peer, with the fields the request named.
type remoteSmartFolderBody struct {
	Name     *string            `json:"name,omitempty"`
	Rule     *domain.SmartRule  `json:"rule,omitempty"`
	Sort     *domain.VideoSort  `json:"sort,omitempty"`
	ParentID *remoteParentValue `json:"parentId,omitempty"`
}

// remoteParentValue writes a parent id, or null for a move to the top level.
type remoteParentValue struct{ id int64 }

func (v remoteParentValue) MarshalJSON() ([]byte, error) {
	if v.id == 0 {
		return []byte("null"), nil
	}
	return []byte(strconv.FormatInt(v.id, 10)), nil
}

// createsSmartFolderLocally forwards a folder under the peer's parent to the peer, where it is stored.
func (s *server) createsSmartFolderLocally(w http.ResponseWriter, r *http.Request, create domain.SmartFolderCreate) bool {
	if isPeerRequest(r.Context()) || !domain.IsRemoteID(create.ParentID) {
		return true
	}
	slot := domain.RemoteSlot(create.ParentID)
	rule, err := s.peerSmartRuleIDs(r, slot, create.Rule)
	if err != nil {
		s.writePeerSmartRuleError(w, err)
		return false
	}
	body := remoteSmartFolderBody{
		Name: &create.Name, Rule: &rule, ParentID: &remoteParentValue{id: domain.FromRemoteID(create.ParentID)},
	}
	if create.Sort != "" {
		body.Sort = &create.Sort
	}
	s.forwardJSON(w, r, slot, "/api/smart-folders", body, remoteSmartFolderAnswer)
	return false
}

// changesSmartFolderLocally forwards a change of the peer's folder, and refuses a parent of the other node.
func (s *server) changesSmartFolderLocally(w http.ResponseWriter, r *http.Request, id int64, change domain.SmartFolderChange) bool {
	if isPeerRequest(r.Context()) {
		return true
	}
	remote := domain.IsRemoteID(id)
	if change.ParentSet && change.ParentID != 0 && domain.RemoteSlot(change.ParentID) != domain.RemoteSlot(id) {
		s.mixedNodes(w)
		return false
	}
	if !remote {
		return true
	}
	body := remoteSmartFolderBody{Name: change.Name, Sort: change.Sort}
	if change.ParentSet {
		body.ParentID = &remoteParentValue{id: peerID(change.ParentID)}
	}
	if change.Rule != nil {
		rule, err := s.peerSmartRuleIDs(r, domain.RemoteSlot(id), *change.Rule)
		if err != nil {
			s.writePeerSmartRuleError(w, err)
			return false
		}
		body.Rule = &rule
	}
	s.forwardJSON(w, r, domain.RemoteSlot(id), remoteSmartFolderPrefix+strconv.FormatInt(domain.FromRemoteID(id), 10), body,
		remoteSmartFolderAnswer)
	return false
}

// deletesSmartFolderLocally forwards the deletion of the peer's folder.
func (s *server) deletesSmartFolderLocally(w http.ResponseWriter, r *http.Request, id int64) bool {
	if isPeerRequest(r.Context()) || !domain.IsRemoteID(id) {
		return true
	}
	slot := domain.RemoteSlot(id)
	out, err := remoteRequest(r, slot, remoteSmartFolderPrefix+strconv.FormatInt(domain.FromRemoteID(id), 10), url.Values{})
	if err != nil {
		s.invalidRequest(w, "The request cannot be forwarded to the other VVMDM.")
		return false
	}
	s.forward(w, r, slot, out, remoteOperation{})
	return false
}
