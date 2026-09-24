package domain

import (
	"fmt"
	"path/filepath"
)

// NormalizeMediaFolderPath はメディアフォルダとして受け取ったパスを、整えた
// 絶対パスにする。相対パスと空文字は ErrInvalidMediaFolder で断る。
//
// Unicode の綴りはファイルシステムにあるとおりに残す。正規化の形を区別する
// ファイルシステムでは、綴りを書き換えると別の項目を指しうる。
//
// ファイルシステムには触れない。パスが実在するディレクトリかどうかは、
// ファイルシステムを扱うアダプタが確かめる。
func NormalizeMediaFolderPath(path string) (string, error) {
	if path == "" || !filepath.IsAbs(path) {
		return "", ErrInvalidMediaFolder
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalidMediaFolder, err)
	}
	return filepath.Clean(absolute), nil
}

// CheckMediaFolderMutation は、メディアフォルダの追加・置き換え・削除を今
// 行ってよいかを返す。走査は開始時点のフォルダの一覧を使って進むので、
// 走査中（scanRunning）の変更は ErrScanRunning で断る。
func CheckMediaFolderMutation(scanRunning bool) error {
	if scanRunning {
		return ErrScanRunning
	}
	return nil
}

// CheckMediaFolderPlacement は、path にメディアフォルダを置いてよいかを返す。
// folders は登録済みのフォルダで、id は置き換える対象の識別子（追加なら 0）
// である。置き換える対象自身とは比べない。
//
// 走査中の変更は ErrScanRunning で断る（CheckMediaFolderMutation）。
// 登録済みのほかのフォルダと同じパス・内側・外側のどれかに当たれば
// ErrFolderConflict で断る。フォルダが入れ子になると、同じファイルが2つの
// 登録から見え、所在の持ち主が決まらない。包含は PathWithinRoot と同じ規則
// （OS の区切りと大文字小文字の扱い）で判定する。
func CheckMediaFolderPlacement(scanRunning bool, folders []MediaFolder, id int64, path string) error {
	if err := CheckMediaFolderMutation(scanRunning); err != nil {
		return err
	}
	for _, folder := range folders {
		if folder.ID == id {
			continue
		}
		if PathWithinRoot(folder.Path, path) || PathWithinRoot(path, folder.Path) {
			return ErrFolderConflict
		}
	}
	return nil
}
