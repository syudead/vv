package domain

import (
	"context"
	"slices"
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
// videos, or its part was skipped because the peer could not be reached.
type RelatedSide struct {
	After *RelatedCursor
	Done  bool
}

// MergedRelatedCursor continues the related list of both nodes with one position per node.
type MergedRelatedCursor struct {
	Local, Peer RelatedSide
}

// Start reports the first page.
func (c MergedRelatedCursor) Start() bool {
	return c == MergedRelatedCursor{}
}

const (
	mergedRelatedSeparator = "~"
	relatedSideDone        = "-"
)

// String keeps the single-node form while the peer's part is over, so an unpaired node pages as before.
func (c MergedRelatedCursor) String() string {
	if c.Peer.Done && c.Local.After != nil && !c.Local.Done {
		return c.Local.After.String()
	}
	return c.Local.text() + mergedRelatedSeparator + c.Peer.text()
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

// ParseMergedRelatedCursor reads String's forms; a single-node cursor skips the peer.
func ParseMergedRelatedCursor(text string) (MergedRelatedCursor, error) {
	local, peer, merged := strings.Cut(text, mergedRelatedSeparator)
	if !merged {
		after, err := ParseRelatedCursor(text)
		if err != nil {
			return MergedRelatedCursor{}, err
		}
		return MergedRelatedCursor{Local: RelatedSide{After: &after}, Peer: RelatedSide{Done: true}}, nil
	}
	var cursor MergedRelatedCursor
	var err error
	if cursor.Local, err = parseRelatedSide(local); err != nil {
		return MergedRelatedCursor{}, err
	}
	if cursor.Peer, err = parseRelatedSide(peer); err != nil {
		return MergedRelatedCursor{}, err
	}
	if cursor.Local.Done && cursor.Peer.Done {
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
// node's own ids; More reports videos after them.
type RelatedSidePage struct {
	Items []RankedRelated
	More  bool
}

// MergeRelated takes up to limit videos of both pages in the related order, a peer video by its remote id on a
// tie, and returns their ids as the screens address them with the cursor after them, nil at the end.
func MergeRelated(local, peer RelatedSidePage, at MergedRelatedCursor, limit int) ([]int64, *MergedRelatedCursor) {
	ids := make([]int64, 0, min(limit, len(local.Items)+len(peer.Items)))
	i, j := 0, 0
	for len(ids) < limit && (i < len(local.Items) || j < len(peer.Items)) {
		if j == len(peer.Items) || i < len(local.Items) && compareRanked(local.Items[i], asRemote(peer.Items[j])) <= 0 {
			ids = append(ids, local.Items[i].VideoID)
			i++
			continue
		}
		ids = append(ids, ToRemoteID(peer.Items[j].VideoID))
		j++
	}
	next := MergedRelatedCursor{Local: at.Local.advance(local, i), Peer: at.Peer.advance(peer, j)}
	if next.Local.Done && next.Peer.Done {
		return ids, nil
	}
	return ids, &next
}

func asRemote(item RankedRelated) RankedRelated {
	item.VideoID = ToRemoteID(item.VideoID)
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

// RelatedPeer scores the peer's videos against the clues of the open video, in the peer's own ids.
type RelatedPeer interface {
	Related(ctx context.Context, clues RelatedClues, exclude []int64, after *RelatedCursor, limit int) (RelatedSidePage, error)
}
