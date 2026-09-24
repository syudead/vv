package domain

import "errors"

// メディアファイルとディレクトリの確かめで返す誤り。どのパスが存在するかを
// 漏らさないよう、呼び出し側はこれらを区別せずに応答してよい。
var (
	// ErrMediaFileUnavailable は、所在が登録フォルダの内側の通常ファイルを
	// 指していないか、開けないことを表す。
	ErrMediaFileUnavailable = errors.New("メディアファイルを開けません")
	// ErrMediaFileOutsideRoot は、所在は登録フォルダの内側にあるが、symlink を
	// 辿った先が外にあることを表す。ErrMediaFileUnavailable でもある。
	ErrMediaFileOutsideRoot = errors.New("メディアファイルのリンク先が登録フォルダの外です")

	// ErrInvalidDirectoryPath は、ディレクトリ選択に絶対パスでない値が渡されたことを表す。
	ErrInvalidDirectoryPath = errors.New("絶対pathを指定してください")
	// ErrDirectoryNotFound は、ディレクトリが無い、ディレクトリでない、または
	// symlink を経由していることを表す。
	ErrDirectoryNotFound = errors.New("ディレクトリが見つかりません")
	// ErrDirectoryUnavailable は、ディレクトリを読み取れないことを表す。
	ErrDirectoryUnavailable = errors.New("ディレクトリを読み取れません")
)

// DirectoryEntry はディレクトリ選択に並べる子ディレクトリの1件である。
type DirectoryEntry struct {
	Name string
	Path string
}

// DirectoryListing はディレクトリ選択の1画面分である。根の一覧では
// CurrentPath と ParentPath が空で、最上位のディレクトリでは ParentPath が空である。
type DirectoryListing struct {
	CurrentPath string
	ParentPath  string
	Directories []DirectoryEntry
}
