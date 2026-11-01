package domain

import (
	"strconv"
	"strings"
)

// slotBits is the width of a link's own ids in the screen API: a remote id is the link's id plus its slot
// times 2^40, and local ids stay below 2^40.
const slotBits = 40

const slotSize int64 = 1 << slotBits

// MaxRemoteSlot keeps every remote id below 2^53, JavaScript's largest safe integer.
const MaxRemoteSlot int64 = 1<<(53-slotBits) - 1

// RemoteID is the id under which this node addresses the entity with id on the link of slot.
func RemoteID(slot, id int64) int64 { return id + slot<<slotBits }

// RemoteSlot is the slot of the link whose entity id names, or 0 for this node's own entity.
func RemoteSlot(id int64) int64 { return id >> slotBits }

// IsRemoteID reports whether id names an entity of a link.
func IsRemoteID(id int64) bool { return id >= slotSize }

// FromRemoteID is the link's own id of the entity this node addresses as remoteID.
func FromRemoteID(remoteID int64) int64 { return remoteID & (slotSize - 1) }

// VocabularyJoin holds this node's tags, tag kinds, people and organizations by name, and the slot of the link
// whose answer it rewrites. A link's entity whose name this node holds takes the local id; any other takes its
// remote id in Slot.
type VocabularyJoin struct {
	Slot          int64
	Tags          map[string]int64
	TagKinds      map[string]int64
	People        map[string]int64
	Organizations map[string]int64
}

func (j VocabularyJoin) Tag(peerID int64, name string) int64 { return j.byName(j.Tags, peerID, name) }

func (j VocabularyJoin) TagKind(peerID int64, name string) int64 {
	return j.byName(j.TagKinds, peerID, name)
}

func (j VocabularyJoin) Person(peerID int64, name string) int64 {
	return j.byName(j.People, peerID, name)
}

func (j VocabularyJoin) Organization(peerID int64, name string) int64 {
	return j.byName(j.Organizations, peerID, name)
}

// Remote is the remote id of the link's entity id, for an entity that is never joined by name.
func (j VocabularyJoin) Remote(id int64) int64 { return RemoteID(j.Slot, id) }

// InSlot is j for rewriting the answer of the link in slot.
func (j VocabularyJoin) InSlot(slot int64) VocabularyJoin {
	j.Slot = slot
	return j
}

// byName never joins an empty name, which a provisional person has.
func (j VocabularyJoin) byName(local map[string]int64, peerID int64, name string) int64 {
	if id, ok := local[name]; ok && name != "" {
		return id
	}
	return RemoteID(j.Slot, peerID)
}

const videoPathPrefix = "/api/videos/"

// RemoteVideoURL rewrites the video id of the link's /api/videos/{id}... URL, such as a thumbnail URL, into
// the remote form of slot; any other URL is returned unchanged.
func RemoteVideoURL(slot int64, raw string) string {
	rest, ok := strings.CutPrefix(raw, videoPathPrefix)
	if !ok {
		return raw
	}
	end := strings.IndexAny(rest, "/?#")
	if end < 0 {
		end = len(rest)
	}
	id, err := strconv.ParseInt(rest[:end], 10, 64)
	if err != nil || id <= 0 || IsRemoteID(id) {
		return raw
	}
	return videoPathPrefix + strconv.FormatInt(RemoteID(slot, id), 10) + rest[end:]
}

// IDNodes reports whether the positive ids name entities of this node, of a link, or of both; a group never
// holds both nodes' videos, and a nonpositive id names neither.
func IDNodes(ids ...int64) (local, remote bool) {
	for _, id := range ids {
		switch {
		case IsRemoteID(id):
			remote = true
		case id > 0:
			local = true
		}
	}
	return local, remote
}

// IDSlot is the one slot of the remote ids among ids, 0 without one; ok is false when they name two links.
func IDSlot(ids ...int64) (slot int64, ok bool) {
	for _, id := range ids {
		if !IsRemoteID(id) {
			continue
		}
		if next := RemoteSlot(id); slot == 0 {
			slot = next
		} else if next != slot {
			return 0, false
		}
	}
	return slot, true
}

// SplitRemoteIDs separates ids into this node's and the links' own ids, keeping their order; the caller checks
// with IDSlot that the remote ones name one link.
func SplitRemoteIDs(ids []int64) (local, peer []int64) {
	for _, id := range ids {
		if IsRemoteID(id) {
			peer = append(peer, FromRemoteID(id))
		} else {
			local = append(local, id)
		}
	}
	return local, peer
}

// ToPeerIDs turns the remote ids among ids into the peer's own ids in place; other ids stay for the peer to refuse.
func ToPeerIDs(ids []int64) {
	for i, id := range ids {
		if IsRemoteID(id) {
			ids[i] = FromRemoteID(id)
		}
	}
}
