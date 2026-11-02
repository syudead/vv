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
// request, when set, does so with this node's and the peer's data, or answers r and reports false. params turn
// the path parameters after the entity id into the peer's ids.
type remoteOperation struct {
	media   bool
	rewrite remoteRewrite
	body    remoteBody
	request func(s *server, w http.ResponseWriter, r *http.Request) bool
	params  []remoteParam
}

// remoteParam translates the path segment at index.
type remoteParam struct {
	index     int
	translate remotePathParam
}

type remoteRewrite func(body []byte, join domain.VocabularyJoin) (any, error)

// remoteBody returns the body with the own ids of the link in slot, or errMixedNodes when it names an entity of
// this node or of another link.
type remoteBody func(slot int64, body []byte) ([]byte, error)

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

// rewriteRemoteVideo keeps the peer's playback position and favorite, which live on the node holding the video,
// and marks the file not openable, since only the node holding it opens it.
func rewriteRemoteVideo(v *gen.Video, join domain.VocabularyJoin) {
	v.Id = join.Remote(v.Id)
	for _, url := range []*string{v.ThumbnailUrl, v.PreviewUrl, v.SeekThumbnailUrl} {
		rewriteRemoteURL(url, join)
	}
	rewriteRemoteFolder(v.Folder, join)
	if v.Group != nil {
		v.Group.Id = join.Remote(v.Group.Id)
		rewriteRemoteFolder(v.Group.Folder, join)
	}
	if v.SmartFolder != nil {
		v.SmartFolder.Id = join.Remote(v.SmartFolder.Id)
	}
	if v.Versions != nil {
		v.Versions.RepresentativeId = join.Remote(v.Versions.RepresentativeId)
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

func rewriteRemoteURL(url *string, join domain.VocabularyJoin) {
	if url != nil {
		*url = domain.RemoteVideoURL(join.Slot, *url)
	}
}

func rewriteRemoteFolder(folder *gen.VideoFolder, join domain.VocabularyJoin) {
	if folder != nil {
		folder.RootId = join.Remote(folder.RootId)
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
		related.Group.Id = join.Remote(related.Group.Id)
		rewriteRemoteVideos(related.Group.Items, join)
	}
	for _, id := range []*int64{related.NextId, related.PrevId} {
		if id != nil {
			*id = join.Remote(*id)
		}
	}
}

func rewriteRemoteGroupMembers(page *gen.GroupMemberPage, join domain.VocabularyJoin) {
	rewriteRemoteVideos(page.Items, join)
}

func rewriteRemoteVersions(versions *gen.VideoVersions, join domain.VocabularyJoin) {
	rewriteRemoteVideos(versions.Items, join)
	versions.RepresentativeId = join.Remote(versions.RepresentativeId)
}

func rewriteRemoteSprite(sprite *gen.SeekThumbnailSprite, join domain.VocabularyJoin) {
	for i := range sprite.Sheets {
		rewriteRemoteURL(&sprite.Sheets[i], join)
	}
}

// rewriteRemoteLibraryItem keeps the peer's watch values and favorites of a group or smart folder, like a
// video's.
func rewriteRemoteLibraryItem(item *gen.LibraryItem, join domain.VocabularyJoin) {
	if item.Video != nil {
		rewriteRemoteVideo(item.Video, join)
	}
	if item.SmartFolder != nil {
		rewriteRemoteSmartFolderSummary(item.SmartFolder, join)
	}
	if item.Group != nil {
		rewriteRemoteGroup(item.Group, join)
	}
}

func rewriteRemoteGroup(group *gen.LibraryGroup, join domain.VocabularyJoin) {
	group.Id, group.OpenVideoId = join.Remote(group.Id), join.Remote(group.OpenVideoId)
	for i := range group.VideoIds {
		group.VideoIds[i] = join.Remote(group.VideoIds[i])
	}
	rewriteRemotePreviews(group.Previews, join)
	rewriteRemoteFolder(group.Folder, join)
	rewriteRemoteTags(group.Tags, join)
}

func rewriteRemotePreviews(previews []gen.FolderPreview, join domain.VocabularyJoin) {
	for i := range previews {
		preview := &previews[i]
		preview.VideoId = join.Remote(preview.VideoId)
		preview.ThumbnailUrl = domain.RemoteVideoURL(join.Slot, preview.ThumbnailUrl)
		rewriteRemoteURL(preview.PreviewUrl, join)
	}
}

// rewriteRemoteFolderSummary leaves out the grouping, which is edited on the node that holds the folder.
func rewriteRemoteFolderSummary(folder *gen.FolderSummary, join domain.VocabularyJoin) {
	folder.RootId = join.Remote(folder.RootId)
	folder.Grouping = nil
	rewriteRemotePreviews(folder.Previews, join)
}

func rewriteRemoteFolderListing(listing *gen.FolderListing, join domain.VocabularyJoin) {
	rewriteRemoteFolderSummary(&listing.Folder, join)
	for i := range listing.Folders {
		rewriteRemoteFolderSummary(&listing.Folders[i], join)
	}
}

// rewriteRemoteVideoPage drops the peer's item cursors, which only a merged list reads.
func rewriteRemoteVideoPage(page *gen.VideoPage, join domain.VocabularyJoin) {
	rewriteRemoteVideos(page.Items, join)
	page.PeerItems = nil
	if page.MissingTagIds != nil {
		for i, id := range *page.MissingTagIds {
			(*page.MissingTagIds)[i] = join.Remote(id)
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
		detail.Trail[i].Id = join.Remote(detail.Trail[i].Id)
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
		item.Id = join.Remote(item.Id)
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

// rewriteRemotePerson keeps the peer's favorite mark, which PUT /api/favorites sets on the peer for a remote id.
func rewriteRemotePerson(person *gen.Person, join domain.VocabularyJoin) {
	person.Id = join.Person(person.Id, person.Name)
}

func rewriteRemoteOrganizationEntity(organization *gen.Organization, join domain.VocabularyJoin) {
	organization.Id = join.Organization(organization.Id, organization.Name)
	rewriteRemoteOrganization(organization.Parent, join)
}

// rewriteRemoteOrganizeResult marks the peer's undo token so that its undo is forwarded too.
func rewriteRemoteOrganizeResult(result *gen.OrganizeResult, join domain.VocabularyJoin) {
	result.UndoToken = remoteUndoToken(join.Slot, result.UndoToken)
	if result.GroupId != nil {
		*result.GroupId = join.Remote(*result.GroupId)
	}
}
