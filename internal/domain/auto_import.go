package domain

type FolderWatchState string

const (
	// FolderWatchOff also covers having no registered media folders.
	FolderWatchOff      FolderWatchState = "off"
	FolderWatchStarting FolderWatchState = "starting"
	FolderWatchActive   FolderWatchState = "active"
	// FolderWatchLimited means changes may be missed.
	FolderWatchLimited FolderWatchState = "limited"
)

type FolderWatchProblemKind string

const (
	FolderWatchProblemLimit             FolderWatchProblemKind = "watch_limit"
	FolderWatchProblemEventsLost        FolderWatchProblemKind = "events_lost"
	FolderWatchProblemFolderUnreachable FolderWatchProblemKind = "folder_unreachable"
	FolderWatchProblemPermissionDenied  FolderWatchProblemKind = "permission_denied"
)

// Path is set only for problems affecting a specific directory.
type FolderWatchProblem struct {
	Kind FolderWatchProblemKind
	Path string
}

// Problem is set only in FolderWatchLimited state.
type AutoImportStatus struct {
	Enabled bool
	State   FolderWatchState
	Problem *FolderWatchProblem
}
