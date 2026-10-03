package domain

// FavoriteChange はお気に入りの付け外しの入力である（specs/035-favorites/data-model.md §2・§4）。
// VideoIDs の動画と FolderPaths のフォルダ（絶対パス）のグループを、Favorite にそろえる。
type FavoriteChange struct {
	VideoIDs    []int64
	FolderPaths []string
	Favorite    bool
}

// FavoriteApplied はお気に入りの付け外しを反映した数である（specs/035-favorites/data-model.md §2）。
// Videos は VideoIDs のうち利用者データの鍵を引けた異なる id の数（同じ集まりの id も 1 本ずつ
// 数える）、Folders は今グループのフォルダとして書いた異なるフォルダの数。既に同じ状態のものも数える。
type FavoriteApplied struct {
	Videos  int
	Folders int
}
