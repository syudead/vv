package domain

type RelatedVideos struct {
	Items      []Video
	NextCursor *RelatedCursor
	// NextID is supplied only on the first page: next group member, otherwise top related video, or zero.
	NextID int64
	// PrevID applies only to group context on the first page; otherwise the UI uses navigation history.
	PrevID int64
	// Group is supplied only on the first page and must be visible to the audience.
	Group *VideoGroup
}

type RelatedPage struct {
	After         *RelatedCursor
	Limit         int
	GroupID       int64
	SmartFolderID int64
}

func (p RelatedPage) Context() VideoContext {
	return VideoContext{GroupID: p.GroupID, SmartFolderID: p.SmartFolderID}
}

// VideoContext prefers an explicit group over smart-folder context; if the selected context lacks the video, use the lowest-ID direct group (docs/design-docs/catalog-organization.md).
type VideoContext struct {
	GroupID       int64
	SmartFolderID int64
}

// GroupMemberWindow bounds response size independently of group size, centering the current video; remaining members are paginated.
const GroupMemberWindow = 100

const MaxGroupMemberPage = 200

// Limit zero reads only position/count; Around centers the video and ignores Offset.
type GroupWindow struct {
	Offset int
	Limit  int
	Around bool
}

// Start converts a one-based member position to a zero-based window offset.
func (w GroupWindow) Start(total, position int) int {
	if w.Limit <= 0 {
		return 0
	}
	start := w.Offset
	if w.Around {
		start = position - 1 - w.Limit/2
		start = min(start, total-w.Limit)
	}
	return max(0, min(start, total))
}

// VideoGroup members are visible playback-order videos; SmartFolder switches ID, Name, and order to the folder context.
type VideoGroup struct {
	ID          int64
	SmartFolder bool
	// Folder is nil for manual groups and automatic folders outside registered roots.
	Folder    *VideoFolder
	Name      string
	MemberIDs []int64
	// Offset is zero-based within MemberIDs.
	Offset  int
	Members []Video
}

func (g VideoGroup) Total() int {
	return len(g.MemberIDs)
}

// Position is one-based, or zero when absent.
func (g VideoGroup) Position(id int64) int {
	for i, member := range g.MemberIDs {
		if member == id {
			return i + 1
		}
	}
	return 0
}

// Neighbors returns zero at either end of the group.
func (g VideoGroup) Neighbors(id int64) (prev, next int64) {
	position := g.Position(id)
	if position == 0 {
		return 0, 0
	}
	if position > 1 {
		prev = g.MemberIDs[position-2]
	}
	if position < len(g.MemberIDs) {
		next = g.MemberIDs[position]
	}
	return prev, next
}
