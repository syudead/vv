package domain

// FavoriteChange はお気に入りの付け外しの入力である。
// VideoIDs の動画、GroupIDs のグループ、SmartFolderIDs のスマートフォルダを、Favorite にそろえる
type FavoriteChange struct {
	VideoIDs       []int64
	GroupIDs       []int64
	SmartFolderIDs []int64
	Favorite       bool
}

// FavoriteApplied はお気に入りの付け外しを反映した数である。
// Videos は VideoIDs のうち利用者データの鍵を引けた異なる id の数（同じ集まりの id も 1 本ずつ
// 数える）、Groups は GroupIDs のうち今ある異なるグループの数、SmartFolders は SmartFolderIDs の
// うち今ある異なるスマートフォルダの数。既に同じ状態のものも数える。
type FavoriteApplied struct {
	Videos       int
	Groups       int
	SmartFolders int
}
