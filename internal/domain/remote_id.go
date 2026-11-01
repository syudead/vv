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

// LinkSlot is the slot the remote ids carry while every remote id reaches the one link, whatever slot the
// store gave it; the migration gives the existing link slot 1, so its ids keep their offset.
const LinkSlot int64 = 1

// RemoteID is the id under which this node addresses the entity with id on the link of slot.
func RemoteID(slot, id int64) int64 { return id + slot<<slotBits }

// RemoteSlot is the slot of the link whose entity id names, or 0 for this node's own entity.
func RemoteSlot(id int64) int64 { return id >> slotBits }

// IsRemoteID reports whether id names an entity of a link.
func IsRemoteID(id int64) bool { return id >= slotSize }

// FromRemoteID is the link's own id of the entity this node addresses as remoteID.
func FromRemoteID(remoteID int64) int64 { return remoteID & (slotSize - 1) }

// VocabularyJoin holds this node's tags, tag kinds, people and organizations by name. A peer entity whose
// name this node holds takes the local id; any other takes its remote id in LinkSlot. The zero value joins
// nothing.
type VocabularyJoin struct {
	Tags          map[string]int64
	TagKinds      map[string]int64
	People        map[string]int64
	Organizations map[string]int64
}

func (j VocabularyJoin) Tag(peerID int64, name string) int64 { return joinByName(j.Tags, peerID, name) }

func (j VocabularyJoin) TagKind(peerID int64, name string) int64 {
	return joinByName(j.TagKinds, peerID, name)
}

func (j VocabularyJoin) Person(peerID int64, name string) int64 {
	return joinByName(j.People, peerID, name)
}

func (j VocabularyJoin) Organization(peerID int64, name string) int64 {
	return joinByName(j.Organizations, peerID, name)
}

// joinByName never joins an empty name, which a provisional person has.
func joinByName(local map[string]int64, peerID int64, name string) int64 {
	if id, ok := local[name]; ok && name != "" {
		return id
	}
	return RemoteID(LinkSlot, peerID)
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

// IDNodes reports whether the positive ids name entities of this node, of the peer, or of both; a group never
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

// SplitRemoteIDs separates ids into this node's and the peer's own ids, keeping their order.
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
