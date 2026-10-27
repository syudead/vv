package domain

import (
	"strconv"
	"strings"
)

// PeerIDOffset is added to the peer's ids in the screen API; local ids stay below it, and 2^40 stays below
// JavaScript's safe integer.
const PeerIDOffset int64 = 1 << 40

// ToRemoteID is the id under which this node addresses the peer's entity with peerID.
func ToRemoteID(peerID int64) int64 { return peerID + PeerIDOffset }

// IsRemoteID reports whether id names an entity of the peer.
func IsRemoteID(id int64) bool { return id >= PeerIDOffset }

// FromRemoteID is the peer's own id of the entity this node addresses as remoteID.
func FromRemoteID(remoteID int64) int64 { return remoteID - PeerIDOffset }

// VocabularyJoin holds this node's tags, tag kinds, people and organizations by name. A peer entity whose
// name this node holds takes the local id; any other takes the offset. The zero value joins nothing.
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
	return ToRemoteID(peerID)
}

const videoPathPrefix = "/api/videos/"

// RemoteVideoURL rewrites the video id of the peer's /api/videos/{id}... URL, such as a thumbnail URL, into
// the remote form; any other URL is returned unchanged.
func RemoteVideoURL(raw string) string {
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
	return videoPathPrefix + strconv.FormatInt(ToRemoteID(id), 10) + rest[end:]
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
