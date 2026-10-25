package httpapi

import (
	"encoding/json"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// remoteOperation is how one operation for a remote id is forwarded. Media has no response timeout, nor has
// an operation that extracts a frame on the peer; rewrite, when set, turns a 200 answer into this node's id
// space, and without it the answer passes through.
type remoteOperation struct {
	media   bool
	rewrite remoteRewrite
}

type remoteRewrite func(body []byte, join domain.VocabularyJoin) (any, error)

// remoteVideoOperations are the operations under /api/videos/{id} offered for the peer's videos. Open,
// progress, favorites, playlists, auto-tagging, probing and activity stay with each node and are absent.
var remoteVideoOperations = map[string]remoteOperation{
	"GET /api/videos/{id}":                        {rewrite: rewriteAs(rewriteRemoteVideo)},
	"GET /api/videos/{id}/group-members":          {rewrite: rewriteAs(rewriteRemoteGroupMembers)},
	"GET /api/videos/{id}/versions":               {rewrite: rewriteAs(rewriteRemoteVersions)},
	"GET /api/videos/{id}/seek-thumbnail":         {rewrite: rewriteAs(rewriteRemoteSprite)},
	"GET /api/videos/{id}/subtitles":              {},
	"GET /api/videos/{id}/transcode-start":        {},
	"PUT /api/videos/{id}/display-name":           {rewrite: rewriteAs(rewriteRemoteVideo)},
	"PUT /api/videos/{id}/thumbnail-position":     {media: true, rewrite: rewriteAs(rewriteRemoteVideo)},
	"GET /api/videos/{id}/stream":                 {media: true},
	"GET /api/videos/{id}/preview":                {media: true},
	"GET /api/videos/{id}/transcode.mp4":          {media: true},
	"GET /api/videos/{id}/subtitles/{file}":       {media: true},
	"GET /api/videos/{id}/thumbnail":              {media: true},
	"GET /api/videos/{id}/seek-thumbnail/{sheet}": {media: true},
	"GET /api/videos/{id}/frame":                  {media: true},
}

// rewriteAs decodes the answer into the operation's generated type, so the type says which ids are videos and
// which are tags.
func rewriteAs[T any](rewrite func(*T, domain.VocabularyJoin)) remoteRewrite {
	return func(body []byte, join domain.VocabularyJoin) (any, error) {
		var value T
		if err := json.Unmarshal(body, &value); err != nil {
			return nil, err
		}
		rewrite(&value, join)
		return value, nil
	}
}

// rewriteRemoteVideo leaves out the playback position and favorite, which stay with each node, and marks the
// file not openable, since only the node holding it opens it.
func rewriteRemoteVideo(v *gen.Video, join domain.VocabularyJoin) {
	v.Id = domain.ToRemoteID(v.Id)
	v.Progress, v.Favorite = nil, nil
	for _, url := range []*string{v.ThumbnailUrl, v.PreviewUrl, v.SeekThumbnailUrl} {
		rewriteRemoteURL(url)
	}
	rewriteRemoteFolder(v.Folder)
	if v.Group != nil {
		v.Group.Id = domain.ToRemoteID(v.Group.Id)
		rewriteRemoteFolder(v.Group.Folder)
	}
	if v.SmartFolder != nil {
		v.SmartFolder.Id = domain.ToRemoteID(v.SmartFolder.Id)
	}
	if v.Versions != nil {
		v.Versions.RepresentativeId = domain.ToRemoteID(v.Versions.RepresentativeId)
	}
	if v.Location != nil {
		v.Location.Openable = false
	}
	for i := range v.Tags {
		tag := &v.Tags[i]
		tag.Id = join.Tag(tag.Id, tag.Name)
		if tag.Kind != nil {
			tag.Kind.Id = join.TagKind(tag.Kind.Id, tag.Kind.Name)
		}
	}
	if v.CardCredits != nil {
		rewriteRemoteCredits(v.CardCredits.Performers, nil, join)
		rewriteRemoteOrganization(v.CardCredits.Organization, join)
	}
	if v.WorkInfo != nil {
		rewriteRemoteCredits(v.WorkInfo.People, v.WorkInfo.Organizations, join)
	}
}

func rewriteRemoteURL(url *string) {
	if url != nil {
		*url = domain.RemoteVideoURL(*url)
	}
}

func rewriteRemoteFolder(folder *gen.VideoFolder) {
	if folder != nil {
		folder.RootId = domain.ToRemoteID(folder.RootId)
	}
}

func rewriteRemoteCredits(people []gen.WorkPerson, organizations []gen.WorkOrganization, join domain.VocabularyJoin) {
	for i := range people {
		person := &people[i].Person
		person.Id = join.Person(person.Id, person.Name)
	}
	for i := range organizations {
		rewriteRemoteOrganization(&organizations[i].Organization, join)
	}
}

func rewriteRemoteOrganization(organization *gen.CreditRef, join domain.VocabularyJoin) {
	if organization != nil {
		organization.Id = join.Organization(organization.Id, organization.Name)
	}
}

func rewriteRemoteVideos(videos []gen.Video, join domain.VocabularyJoin) {
	for i := range videos {
		rewriteRemoteVideo(&videos[i], join)
	}
}

func rewriteRemoteRelated(related *gen.RelatedVideos, join domain.VocabularyJoin) {
	rewriteRemoteVideos(related.Items, join)
	if related.Group != nil {
		related.Group.Id = domain.ToRemoteID(related.Group.Id)
		rewriteRemoteVideos(related.Group.Items, join)
	}
	for _, id := range []*int64{related.NextId, related.PrevId} {
		if id != nil {
			*id = domain.ToRemoteID(*id)
		}
	}
}

func rewriteRemoteGroupMembers(page *gen.GroupMemberPage, join domain.VocabularyJoin) {
	rewriteRemoteVideos(page.Items, join)
}

func rewriteRemoteVersions(versions *gen.VideoVersions, join domain.VocabularyJoin) {
	rewriteRemoteVideos(versions.Items, join)
	versions.RepresentativeId = domain.ToRemoteID(versions.RepresentativeId)
}

func rewriteRemoteSprite(sprite *gen.SeekThumbnailSprite, _ domain.VocabularyJoin) {
	for i := range sprite.Sheets {
		rewriteRemoteURL(&sprite.Sheets[i])
	}
}
