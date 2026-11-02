package domain

import (
	"cmp"
	"context"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"
)

// RelatedClues are what the node holding the open video sends so the other node scores its own videos
// against it. They carry no directory, since two disks share none; when the other node holds the open video,
// it finds the directory by the video's id.
type RelatedClues struct {
	Fingerprint *Fingerprint
	// TagNames join the tags by name, as the vocabulary of both nodes is joined.
	TagNames []string
	Words    []string
	AddedAt  time.Time
}

// RelatedWords are the words the scorer takes from a title and a file name, sorted.
func RelatedWords(title, path string) []string {
	words := make([]string, 0)
	for word := range titleWords(title, path) {
		words = append(words, word)
	}
	slices.Sort(words)
	return words
}

// CluesOf gives the clues of candidate; names maps its tag ids to names, and a tag missing there is left out.
func CluesOf(candidate RelatedCandidate, names map[int64]string) RelatedClues {
	clues := RelatedClues{
		Fingerprint: candidate.Fingerprint,
		TagNames:    make([]string, 0, len(candidate.TagIDs)),
		Words:       RelatedWords(candidate.Title, candidate.Path),
		AddedAt:     candidate.AddedAt,
	}
	for id := range uniqueIDs(candidate.TagIDs) {
		if name, ok := names[id]; ok {
			clues.TagNames = append(clues.TagNames, name)
		}
	}
	slices.Sort(clues.TagNames)
	clues.TagNames = slices.Compact(clues.TagNames)
	return clues
}

// Candidate is the open video as the reference for this node's candidates. ids maps tag names to this node's
// tag ids; a name this node lacks takes a negative id no tag has, so it still counts among the tags on either
// video. The empty path gives no directory points.
func (c RelatedClues) Candidate(ids map[string]int64) RelatedCandidate {
	candidate := RelatedCandidate{
		Fingerprint: c.Fingerprint,
		AddedAt:     c.AddedAt,
		Words:       c.Words,
		TagIDs:      make([]int64, 0, len(c.TagNames)),
	}
	if candidate.Words == nil {
		candidate.Words = []string{}
	}
	var missing int64
	for _, name := range c.TagNames {
		if id, ok := ids[name]; ok {
			candidate.TagIDs = append(candidate.TagIDs, id)
			continue
		}
		missing--
		candidate.TagIDs = append(candidate.TagIDs, missing)
	}
	return candidate
}

// RelatedSide is how far the merged related list has read one node's list. Done means that node has no more
// videos, or its part was skipped because the link could not be reached.
type RelatedSide struct {
	After *RelatedCursor
	Done  bool
}

// MergedRelatedCursor continues the related list of this node and its links with one position per node. Once the
// list has started, a link Links does not hold is done, so a link connected mid-list never joins it.
type MergedRelatedCursor struct {
	Local RelatedSide
	Links map[int64]RelatedSide
}

// Start reports the first page.
func (c MergedRelatedCursor) Start() bool {
	return c.Local == RelatedSide{} && len(c.Links) == 0
}

// Link is the position of the link in slot.
func (c MergedRelatedCursor) Link(slot int64) RelatedSide {
	if side, ok := c.Links[slot]; ok || c.Start() {
		return side
	}
	return RelatedSide{Done: true}
}

const (
	mergedRelatedSeparator = "~"
	relatedSlotSeparator   = ":"
	relatedSideDone        = "-"
)

// String keeps the single-node form once every link's part is over, so a node without links pages as before;
// otherwise it writes the local side and then each link's as slot:side, in slot order.
func (c MergedRelatedCursor) String() string {
	slots := slices.Sorted(maps.Keys(c.Links))
	linksDone := !slices.ContainsFunc(slots, func(slot int64) bool { return !c.Links[slot].Done })
	if linksDone && c.Local.After != nil && !c.Local.Done {
		return c.Local.After.String()
	}
	var text strings.Builder
	text.WriteString(c.Local.text())
	for _, slot := range slots {
		text.WriteString(mergedRelatedSeparator + strconv.FormatInt(slot, 10) + relatedSlotSeparator + c.Links[slot].text())
	}
	return text.String()
}

func (s RelatedSide) text() string {
	switch {
	case s.Done:
		return relatedSideDone
	case s.After == nil:
		return ""
	default:
		return s.After.String()
	}
}

// ParseMergedRelatedCursor reads String's forms; a single-node cursor leaves every link done. The two-side
// local~peer form names no slot and is refused.
func ParseMergedRelatedCursor(text string) (MergedRelatedCursor, error) {
	parts := strings.Split(text, mergedRelatedSeparator)
	if len(parts) == 1 {
		after, err := ParseRelatedCursor(text)
		if err != nil {
			return MergedRelatedCursor{}, err
		}
		return MergedRelatedCursor{Local: RelatedSide{After: &after}}, nil
	}
	local, err := parseRelatedSide(parts[0])
	if err != nil {
		return MergedRelatedCursor{}, err
	}
	cursor := MergedRelatedCursor{Local: local, Links: make(map[int64]RelatedSide, len(parts)-1)}
	done := local.Done
	for _, part := range parts[1:] {
		slotText, sideText, ok := strings.Cut(part, relatedSlotSeparator)
		slot, err := strconv.ParseInt(slotText, 10, 64)
		if _, seen := cursor.Links[slot]; !ok || err != nil || slot < 1 || seen {
			return MergedRelatedCursor{}, ErrInvalidRelatedCursor
		}
		side, err := parseRelatedSide(sideText)
		if err != nil {
			return MergedRelatedCursor{}, err
		}
		cursor.Links[slot], done = side, done && side.Done
	}
	if done {
		return MergedRelatedCursor{}, ErrInvalidRelatedCursor
	}
	return cursor, nil
}

func parseRelatedSide(text string) (RelatedSide, error) {
	switch text {
	case relatedSideDone:
		return RelatedSide{Done: true}, nil
	case "":
		return RelatedSide{}, nil
	}
	after, err := ParseRelatedCursor(text)
	if err != nil {
		return RelatedSide{}, err
	}
	return RelatedSide{After: &after}, nil
}

// RelatedSidePage is one node's next videos after its side of the cursor, in the related order and in that
// node's own ids; Slot is the node's slot, 0 for this node, and More reports videos after them.
type RelatedSidePage struct {
	Slot  int64
	Items []RankedRelated
	More  bool
}

// MergeRelated takes up to limit videos of the pages in the related order, a link's video by its remote id on a
// tie, and returns their ids as the screens address them with the cursor after them, nil at the end. Every side
// the list still holds is passed, a done one empty; a link left out is done from then on.
func MergeRelated(sides []RelatedSidePage, at MergedRelatedCursor, limit int) ([]int64, *MergedRelatedCursor) {
	sides = slices.Clone(sides)
	slices.SortStableFunc(sides, func(a, b RelatedSidePage) int { return cmp.Compare(a.Slot, b.Slot) })
	taken := make([]int, len(sides))
	ids := make([]int64, 0, limit)
	for len(ids) < limit {
		best := -1
		for k, side := range sides {
			if taken[k] < len(side.Items) &&
				(best < 0 || compareRanked(sideItem(side, taken[k]), sideItem(sides[best], taken[best])) < 0) {
				best = k
			}
		}
		if best < 0 {
			break
		}
		ids, taken[best] = append(ids, sideItem(sides[best], taken[best]).VideoID), taken[best]+1
	}
	next := MergedRelatedCursor{Links: map[int64]RelatedSide{}}
	done := true
	for k, side := range sides {
		if side.Slot == 0 {
			next.Local = at.Local.advance(side, taken[k])
			done = done && next.Local.Done
			continue
		}
		next.Links[side.Slot] = at.Link(side.Slot).advance(side, taken[k])
		done = done && next.Links[side.Slot].Done
	}
	if done {
		return ids, nil
	}
	return ids, &next
}

// sideItem is the side's item at index with its id as the screens address it.
func sideItem(side RelatedSidePage, index int) RankedRelated {
	item := side.Items[index]
	if side.Slot != 0 {
		item.VideoID = RemoteID(side.Slot, item.VideoID)
	}
	return item
}

func (s RelatedSide) advance(page RelatedSidePage, taken int) RelatedSide {
	if s.Done {
		return s
	}
	if taken > 0 {
		after := page.Items[taken-1].Cursor()
		s.After = &after
	}
	s.Done = taken == len(page.Items) && !page.More
	return s
}

// MergedRelatedPage asks for one page of the related list over both nodes.
type MergedRelatedPage struct {
	Cursor        MergedRelatedCursor
	Limit         int
	GroupID       int64
	SmartFolderID int64
}

func (p MergedRelatedPage) Context() VideoContext {
	return VideoContext{GroupID: p.GroupID, SmartFolderID: p.SmartFolderID}
}

// MergedRelated is one page of the related list over both nodes. IDs are in the screens' id space, a peer
// video by its remote id; Local holds this node's videos among them. NextID, PrevID and Group are as in
// RelatedVideos and set only on the first page.
type MergedRelated struct {
	IDs        []int64
	Local      []Video
	NextCursor *MergedRelatedCursor
	NextID     int64
	PrevID     int64
	Group      *VideoGroup
}

// ScoredVideo is one video of the page a node answers to its peer's related request.
type ScoredVideo struct {
	Score RelatedScore
	Video Video
}

type ScoredRelated struct {
	Items      []ScoredVideo
	NextCursor *RelatedCursor
}

// RelatedPeer scores one link's videos against the clues of the open video, in the link's own ids. The link
// holding the open video leaves out that video, its bundle and its group itself.
type RelatedPeer interface {
	// Slot is the slot of the link whose videos Related answers.
	Slot() int64
	Related(ctx context.Context, clues RelatedClues, after *RelatedCursor, limit int) (RelatedSidePage, error)
}
