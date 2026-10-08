package domain

// FolderWatchState はフォルダの監視が今どうなっているかである
// （specs/042-folder-watch-import/contracts/screen-api.md）。
type FolderWatchState string

const (
	// FolderWatchOff は自動の取り込みが切れているか、メディアフォルダが無くて監視するものが無い。
	FolderWatchOff FolderWatchState = "off"
	// FolderWatchStarting は監視を張っている最中である。
	FolderWatchStarting FolderWatchState = "starting"
	// FolderWatchActive はすべてのディレクトリを監視している。
	FolderWatchActive FolderWatchState = "active"
	// FolderWatchLimited は変更を取りこぼしうる問題がある。
	FolderWatchLimited FolderWatchState = "limited"
)

// FolderWatchProblemKind は監視が変更をすべて見られると約束できない理由である。
type FolderWatchProblemKind string

const (
	// FolderWatchProblemLimit は OS の監視の数の上限で、監視を足せなかった。
	FolderWatchProblemLimit FolderWatchProblemKind = "watch_limit"
	// FolderWatchProblemEventsLost は OS が通知を取りこぼした。
	FolderWatchProblemEventsLost FolderWatchProblemKind = "events_lost"
	// FolderWatchProblemFolderUnreachable はメディアフォルダに到達できない。
	FolderWatchProblemFolderUnreachable FolderWatchProblemKind = "folder_unreachable"
	// FolderWatchProblemPermissionDenied はディレクトリを読めない。
	FolderWatchProblemPermissionDenied FolderWatchProblemKind = "permission_denied"
)

// FolderWatchProblem は監視の問題である。Path は問題が 1 つのディレクトリに関わるときだけ入る。
type FolderWatchProblem struct {
	Kind FolderWatchProblemKind
	Path string
}

// AutoImportStatus は自動の取り込みの保存された選択と、監視の今の状態である。
// Problem は State が FolderWatchLimited のときだけ入る。
type AutoImportStatus struct {
	Enabled bool
	State   FolderWatchState
	Problem *FolderWatchProblem
}
