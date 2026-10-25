package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// remoteOperation is how one operation for a remote id is forwarded. Media has no response timeout, nor has
// an operation that extracts a frame on the peer; rewrite, when set, turns a 200 answer into this node's id
// space, and without it the answer passes through. body, when set, turns the request body into the peer's ids;
// request, when set, does so with this node's and the peer's data, or answers r and reports false.
type remoteOperation struct {
	media   bool
	rewrite remoteRewrite
	body    remoteBody
	request func(s *server, w http.ResponseWriter, r *http.Request) bool
}

type remoteRewrite func(body []byte, join domain.VocabularyJoin) (any, error)

// remoteBody returns the body with the peer's own ids, or errMixedNodes when it names one of this node's.
type remoteBody func(body []byte) ([]byte, error)

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

// remoteVocabularyOperations are the reads and single edits offered for the peer's tags, people and
// organizations.
var remoteVocabularyOperations = map[string]remoteOperation{
	"PATCH /api/tags/{id}":        {rewrite: rewriteAs(rewriteRemoteTag)},
	"GET /api/people/{id}":        {rewrite: rewriteAs(rewriteRemotePerson)},
	"PATCH /api/people/{id}":      {rewrite: rewriteAs(rewriteRemotePerson)},
	"GET /api/people/{id}/image":  {},
	"GET /api/organizations/{id}": {rewrite: rewriteAs(rewriteRemoteOrganizationEntity)},
	"PATCH /api/organizations/{id}": {
		rewrite: rewriteAs(rewriteRemoteOrganizationEntity), request: (*server).remoteOrganizationPatch,
	},
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
	rewriteRemoteTags(v.Tags, join)
	if v.CardCredits != nil {
		rewriteRemoteCredits(v.CardCredits.Performers, nil, join)
		rewriteRemoteOrganization(v.CardCredits.Organization, join)
	}
	if v.WorkInfo != nil {
		rewriteRemoteCredits(v.WorkInfo.People, v.WorkInfo.Organizations, join)
	}
}

func rewriteRemoteTags(tags []gen.VideoTag, join domain.VocabularyJoin) {
	for i := range tags {
		tag := &tags[i]
		tag.Id = join.Tag(tag.Id, tag.Name)
		rewriteRemoteKindRef(tag.Kind, join)
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

// rewriteRemoteLibraryItem leaves out the watch values and favorites of a group or smart folder, which stay
// with each node like a video's.
func rewriteRemoteLibraryItem(item *gen.LibraryItem, join domain.VocabularyJoin) {
	if item.Video != nil {
		rewriteRemoteVideo(item.Video, join)
	}
	if folder := item.SmartFolder; folder != nil {
		folder.Id, folder.Favorite = domain.ToRemoteID(folder.Id), nil
		if folder.ParentId != nil {
			*folder.ParentId = domain.ToRemoteID(*folder.ParentId)
		}
	}
	if item.Group != nil {
		rewriteRemoteGroup(item.Group, join)
	}
}

func rewriteRemoteGroup(group *gen.LibraryGroup, join domain.VocabularyJoin) {
	group.Id, group.OpenVideoId = domain.ToRemoteID(group.Id), domain.ToRemoteID(group.OpenVideoId)
	group.Favorite, group.WatchState, group.WatchedCount, group.LastPlayedAt = nil, nil, nil, nil
	for i := range group.VideoIds {
		group.VideoIds[i] = domain.ToRemoteID(group.VideoIds[i])
	}
	rewriteRemotePreviews(group.Previews)
	rewriteRemoteFolder(group.Folder)
	rewriteRemoteTags(group.Tags, join)
}

func rewriteRemotePreviews(previews []gen.FolderPreview) {
	for i := range previews {
		preview := &previews[i]
		preview.VideoId = domain.ToRemoteID(preview.VideoId)
		preview.ThumbnailUrl = domain.RemoteVideoURL(preview.ThumbnailUrl)
		rewriteRemoteURL(preview.PreviewUrl)
	}
}

// rewriteRemoteFolderSummary leaves out the grouping, which is edited on the node that holds the folder.
func rewriteRemoteFolderSummary(folder *gen.FolderSummary) {
	folder.RootId = domain.ToRemoteID(folder.RootId)
	folder.Grouping = nil
	rewriteRemotePreviews(folder.Previews)
}

func rewriteRemoteFolderListing(listing *gen.FolderListing, _ domain.VocabularyJoin) {
	rewriteRemoteFolderSummary(&listing.Folder)
	for i := range listing.Folders {
		rewriteRemoteFolderSummary(&listing.Folders[i])
	}
}

// rewriteRemoteVideoPage drops the peer's item cursors, which only a merged list reads.
func rewriteRemoteVideoPage(page *gen.VideoPage, join domain.VocabularyJoin) {
	rewriteRemoteVideos(page.Items, join)
	page.PeerItems = nil
	if page.MissingTagIds != nil {
		for i, id := range *page.MissingTagIds {
			(*page.MissingTagIds)[i] = domain.ToRemoteID(id)
		}
	}
}

func rewriteRemoteTag(tag *gen.Tag, join domain.VocabularyJoin) {
	tag.Id = join.Tag(tag.Id, tag.Name)
	rewriteRemoteKindRef(tag.Kind, join)
}

func rewriteRemoteKindRef(kind *gen.TagKindRef, join domain.VocabularyJoin) {
	if kind != nil {
		kind.Id = join.TagKind(kind.Id, kind.Name)
	}
}

func rewriteRemoteGroupList(page *gen.GroupListPage, join domain.VocabularyJoin) {
	for i := range page.Groups {
		rewriteRemoteGroup(&page.Groups[i], join)
	}
}

func rewriteRemoteGroupDetail(detail *gen.GroupDetail, join domain.VocabularyJoin) {
	rewriteRemoteGroup(&detail.Group, join)
	for i := range detail.Trail {
		detail.Trail[i].Id = domain.ToRemoteID(detail.Trail[i].Id)
	}
	rewriteRemoteGroupItems(detail.Items, join)
}

func rewriteRemoteGroupItemPage(page *gen.GroupItemPage, join domain.VocabularyJoin) {
	rewriteRemoteGroupItems(page.Items, join)
}

// rewriteRemoteGroupItems gives item ids the offset too, since moves and removals address items by them.
func rewriteRemoteGroupItems(items []gen.GroupItem, join domain.VocabularyJoin) {
	for i := range items {
		item := &items[i]
		item.Id = domain.ToRemoteID(item.Id)
		if item.Video != nil {
			rewriteRemoteVideo(item.Video, join)
		}
		if item.Group != nil {
			rewriteRemoteGroup(item.Group, join)
		}
	}
}

func rewriteRemoteGroupTag(result *gen.GroupTagResult, join domain.VocabularyJoin) {
	result.Tag.Id = join.Tag(result.Tag.Id, result.Tag.Name)
	rewriteRemoteKindRef(result.Tag.Kind, join)
}

func rewriteRemoteTagKind(kind *gen.TagKind, join domain.VocabularyJoin) {
	kind.Id = join.TagKind(kind.Id, kind.Name)
}

// rewriteRemotePerson leaves out the favorite mark, which stays with each node.
func rewriteRemotePerson(person *gen.Person, join domain.VocabularyJoin) {
	person.Id = join.Person(person.Id, person.Name)
	person.Favorite = false
}

func rewriteRemoteOrganizationEntity(organization *gen.Organization, join domain.VocabularyJoin) {
	organization.Id = join.Organization(organization.Id, organization.Name)
	rewriteRemoteOrganization(organization.Parent, join)
}

// rewriteRemoteOrganizeResult marks the peer's undo token so that its undo is forwarded too.
func rewriteRemoteOrganizeResult(result *gen.OrganizeResult, _ domain.VocabularyJoin) {
	result.UndoToken = remoteUndoPrefix + result.UndoToken
	if result.GroupId != nil {
		*result.GroupId = domain.ToRemoteID(*result.GroupId)
	}
}
