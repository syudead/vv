package domain

import "path/filepath"

// ScanActivityKind は取り込み中の今の処理が何をしているかである。値は
// api/openapi.yaml の ScanActivityKind に対応する
// （specs/024-import-progress/contracts/scan-api.md §2）。
type ScanActivityKind string

const (
	// ActivityRegistering は走査がファイルを索引へ登録している。
	ActivityRegistering ScanActivityKind = "registering"
	// ActivityProbe は動画の情報を読んでいる。
	ActivityProbe ScanActivityKind = "probe"
	// ActivityThumbnail は代表サムネイルを作っている。
	ActivityThumbnail ScanActivityKind = "thumbnail"
	// ActivitySeekThumbnail はシーク用サムネイルを作っている。
	ActivitySeekThumbnail ScanActivityKind = "seekThumbnail"
	// ActivityPreview は一覧用プレビューを作っている。
	ActivityPreview ScanActivityKind = "preview"
	// ActivityFingerprint は別バージョンを探すための映像の指紋を作っている。
	ActivityFingerprint ScanActivityKind = "fingerprint"
)

// ActivityKindOf は仕事の種類を今の処理の種類にする。知らない種類なら false を返す。
func ActivityKindOf(kind JobKind) (ScanActivityKind, bool) {
	switch kind {
	case JobProbe:
		return ActivityProbe, true
	case JobThumbnail:
		return ActivityThumbnail, true
	case JobSeekThumbnail:
		return ActivitySeekThumbnail, true
	case JobPreview:
		return ActivityPreview, true
	case JobFingerprint:
		return ActivityFingerprint, true
	}
	return "", false
}

// ScanActivity は取り込み中の今の処理である。保存せず、internal/app がメモリに持つ
// （specs/024-import-progress/research.md R-8）。Kind が空なら、何も動いていない。
type ScanActivity struct {
	Kind ScanActivityKind
	// VideoID は登録された動画の id。未登録のファイルは 0 である。
	VideoID int64
	// Path はファイルの絶対パスである。
	Path string
	// Folder はファイルの置かれたフォルダで、Located のときだけ意味を持つ。
	// internal/app が読み出しの時点で登録フォルダから求める。
	Folder VideoFolder
	// RootName は Folder を含む登録フォルダの表示名である。
	RootName string
	Located  bool
}

// Active は何かが動いているかを返す。
func (a ScanActivity) Active() bool {
	return a.Kind != ""
}

// FileName はファイル名を返す。
func (a ScanActivity) FileName() string {
	return filepath.Base(a.Path)
}

// Locate は登録フォルダから、ファイルの置かれたフォルダを求めて返す。どの登録
// フォルダにも含まれなければ、フォルダを持たないまま返す。
func (a ScanActivity) Locate(roots []MediaFolder) ScanActivity {
	folder, ok := LocateVideoFolder(roots, a.Path)
	if !ok {
		return a
	}
	a.Folder, a.Located = folder, true
	for _, root := range roots {
		if root.ID == folder.RootID {
			a.RootName = FolderName(root.Path, "")
			break
		}
	}
	return a
}
