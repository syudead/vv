// Package artifacts は内容ごとの生成物（ライブラリ用サムネイル・シーク用
// プレビュー・ホバープレビュー）の置き場を受け持つ。
//
// content key から生成物のパスを決める規則、生成途中の一時置き場からの公開、
// 存在と完全性の確認、配信のための読み出し、削除、起動時の一時置き場の掃除を
// ここだけが持つ。生成（ffmpeg）は internal/media、いつ作っていつ消すかの判断は
// internal/app が持ち、どちらもこのパッケージを import せずに、自分が宣言した
// interface 越しに使う。
//
// 依存してよいのは internal/domain だけで、他の internal/* は参照しない。
// 依存の向きは ARCHITECTURE.md を参照。
package artifacts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofrs/flock"

	"github.com/syudead/vv/internal/domain"
)

// 置き場の並べ方。<root> は Store に渡した根（MDM_DATA_DIR/thumbnails）、<s> は
// content key をファイル名に使える形にしたもの、<p> は <s> の先頭2文字である。
//
//	<root>/<p>/<s>.jpg                      ライブラリ用サムネイル
//	<root>/seek/<p>/<s>/<n>.jpg             シーク用プレビュー（<n> は6桁の番号）
//	<root>/preview/<p>/<s>.mp4              ホバープレビュー
//	<root>/preview/<p>/<s>.mp4.sha256       その manifest
//	<root>/preview/.publish.lock            ホバープレビューの公開の錠
//	<root>/.tmp/                            生成途中の一時置き場
//
// この並べ方は既存の MDM_DATA_DIR の生成物と互換でなければならない。変えると、
// 生成済みのファイルが配信されなくなり、削除からも漏れる。
const (
	seekDirName      = "seek"
	previewDirName   = "preview"
	temporaryDirName = ".tmp"
	publishLockName  = ".publish.lock"
	thumbnailExt     = ".jpg"
	previewExt       = ".mp4"
	manifestExt      = ".sha256"
	// frameNameFormat はシーク用プレビューの1フレームのファイル名である。ffmpeg の
	// 連番の出力にもそのまま渡す。
	frameNameFormat = "%06d.jpg"
)

// dirPerm は置き場のディレクトリを作るときの許可属性である。
const dirPerm os.FileMode = 0o755

// manifestVersion はホバープレビューの manifest の版である。
const manifestVersion = 1

// publishLockWait はホバープレビューの公開の錠を待つ間隔である。
const publishLockWait = 100 * time.Millisecond

// errInvalidKey は、置き場の外を指しうる content key を表す。読み出しでは
// 「無い」と同じに扱えるよう fs.ErrNotExist を包む。
var errInvalidKey = fmt.Errorf("生成物の置き場に使えない内容の識別子です: %w", fs.ErrNotExist)

// Store は1つの根の下にある生成物の置き場である。
type Store struct {
	root string
}

// New は root を根とする置き場を返す。root が空なら、どの生成物も無いものとして
// 扱い、公開は失敗する。
func New(root string) *Store {
	return &Store{root: root}
}

// fileName は content key をファイル名に使える形にする。content key は
// "<16進>:<サイズ>" なので、区切りの ":" を置き換える。":" は Windows 共有や
// 一部のファイルシステムで扱えない。パスの区切りも置き換える。
//
// 置き換えた結果が空、または "." で始まるものは使えない。"."・".." や、その
// 先頭2文字の振り分けは置き場の外を指し、".tmp" などは置き場自身の名前と
// 重なるからである。実際の content key はこれに当たらない。
func fileName(contentKey string) (string, bool) {
	name := strings.NewReplacer(":", "_", "/", "_", `\`, "_").Replace(contentKey)
	if name == "" || strings.HasPrefix(name, ".") {
		return "", false
	}
	return name, true
}

// locate は dir（根からの相対）の下で、content key の先頭2文字のディレクトリに
// 振り分けた名前を返す。2文字のディレクトリに分けるのは、1ディレクトリに数万の
// ファイルを置かないためである。
func (s *Store) locate(dir, contentKey, ext string) (string, bool) {
	if s.root == "" {
		return "", false
	}
	name, ok := fileName(contentKey)
	if !ok {
		return "", false
	}
	prefix := name
	if len(prefix) > 2 {
		prefix = prefix[:2]
	}
	return filepath.Join(s.root, dir, prefix, name+ext), true
}

func (s *Store) thumbnailPath(contentKey string) (string, bool) {
	return s.locate("", contentKey, thumbnailExt)
}

func (s *Store) seekDir(contentKey string) (string, bool) {
	return s.locate(seekDirName, contentKey, "")
}

func (s *Store) previewPaths(contentKey string) (video, manifest string, ok bool) {
	video, ok = s.locate(previewDirName, contentKey, previewExt)
	return video, video + manifestExt, ok
}

// makeTemporaryDir は生成途中の成果物を置くディレクトリを作る。一時置き場は根の
// 直下に1か所だけ置き、確定するときに同じファイルシステムの中で本来の場所へ
// 改名する。そのため、生成途中の成果物は確認にも配信にも見えない。
func (s *Store) makeTemporaryDir(pattern string) (string, error) {
	if s.root == "" {
		return "", errors.New("生成物の置き場が設定されていません")
	}
	root := filepath.Join(s.root, temporaryDirName)
	if err := os.MkdirAll(root, dirPerm); err != nil {
		return "", fmt.Errorf("生成途中の一時領域を作れません: %w", err)
	}
	dir, err := os.MkdirTemp(root, pattern)
	if err != nil {
		return "", fmt.Errorf("生成途中の一時領域を作れません: %w", err)
	}
	return dir, nil
}

// RemoveTemporary は生成途中の成果物の置き場を丸ごと消す。起動時、ワーカーを
// 動かす前に呼ぶ。生成の途中でプロセスが止まると後片付けが走らず、ここに残る。
// 一時領域を1か所にまとめてあるので、置き場全体を読まずに済む。
func (s *Store) RemoveTemporary() error {
	if s.root == "" {
		return nil
	}
	if err := os.RemoveAll(filepath.Join(s.root, temporaryDirName)); err != nil {
		return fmt.Errorf("生成途中の一時領域を削除できません: %w", err)
	}
	return nil
}

// PublishThumbnail はライブラリ用サムネイルを作り直して公開する。write は一時
// 置き場のパスを受けて画像を書く。書かれた画像が空でなければ本来の場所へ
// 改名し、既存の画像を置き換える。
func (s *Store) PublishThumbnail(contentKey string, write func(output string) error) error {
	target, ok := s.thumbnailPath(contentKey)
	if !ok {
		return errInvalidKey
	}
	temporary, err := s.makeTemporaryDir("thumbnail-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(temporary) }()

	output := filepath.Join(temporary, "thumbnail"+thumbnailExt)
	if err := write(output); err != nil {
		return err
	}
	if info, err := os.Stat(output); err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		return fmt.Errorf("サムネイルが生成されませんでした (%s)", contentKey)
	}
	if err := os.MkdirAll(filepath.Dir(target), dirPerm); err != nil {
		return fmt.Errorf("サムネイルの置き場所を作れません (%s): %w", filepath.Dir(target), err)
	}
	if err := os.Rename(output, target); err != nil {
		return fmt.Errorf("サムネイルを確定できません: %w", err)
	}
	return nil
}

// PublishSeekThumbnails はシーク用プレビューを公開する。完成したものがすでに
// あれば write を呼ばずに成功を返す。write は一時置き場の中の連番のファイル名の
// 型（ffmpeg の出力にそのまま渡せる形）を受けてフレームを書く。
func (s *Store) PublishSeekThumbnails(contentKey string, write func(outputPattern string) error) error {
	target, ok := s.seekDir(contentKey)
	if !ok {
		return errInvalidKey
	}
	if isDir(target) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(target), dirPerm); err != nil {
		return fmt.Errorf("シークサムネイルの置き場所を作れません: %w", err)
	}
	temporary, err := s.makeTemporaryDir("seek-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(temporary) }()

	if err := write(filepath.Join(temporary, frameNameFormat)); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(temporary, fmt.Sprintf(frameNameFormat, 0))); err != nil {
		return fmt.Errorf("シークサムネイルが生成されませんでした (%s): %w", contentKey, err)
	}
	// ディレクトリごと改名するので、置き場があれば完成している。
	if err := os.Rename(temporary, target); err != nil {
		if isDir(target) {
			return nil
		}
		return fmt.Errorf("シークサムネイルを確定できません: %w", err)
	}
	return nil
}

// PublishPreview はホバープレビューの MP4 と manifest を公開する。
//
// 完全性（manifest の大きさと SHA-256）を確かめられるものがすでにあれば、write を
// 呼ばずに成功を返す。無ければ残っている片方を消し、write に一時置き場のパスを
// 渡して MP4 を書かせ、manifest を作る。公開の直前に current を呼び、false なら
// 公開せずに domain.ErrPreviewStale を返す。
func (s *Store) PublishPreview(
	ctx context.Context, contentKey string,
	write func(output string) error,
	current func(context.Context) (bool, error),
) error {
	target, manifest, ok := s.previewPaths(contentKey)
	if !ok {
		return errInvalidKey
	}
	if err := os.MkdirAll(filepath.Dir(target), dirPerm); err != nil {
		return fmt.Errorf("プレビューの置き場所を作れません: %w", err)
	}
	// 2つのファイルの公開は組として不可分にできないので、同じ置き場を使う
	// プロセスはすべて、この1つの錠で公開を直列にする。
	assetLock := flock.New(filepath.Join(s.root, previewDirName, publishLockName))
	locked, err := assetLock.TryLockContext(ctx, publishLockWait)
	if err != nil {
		return fmt.Errorf("プレビューの生成ロックを取得できません: %w", err)
	}
	if !locked {
		return errors.New("プレビューの生成ロックを取得できません")
	}
	defer func() { _ = assetLock.Unlock() }()

	if verifyPreview(target, manifest) == nil {
		return nil
	}
	_ = os.Remove(target)
	_ = os.Remove(manifest)

	temporary, err := s.makeTemporaryDir("preview-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(temporary) }()

	tmpVideo := filepath.Join(temporary, "preview"+previewExt)
	if err := write(tmpVideo); err != nil {
		return err
	}
	info, err := os.Stat(tmpVideo)
	if err != nil {
		return fmt.Errorf("プレビューを確認できません: %w", err)
	}
	if info.Size() == 0 {
		return errors.New("プレビューが生成されませんでした: ファイルが空です")
	}
	digest, err := fileSHA256(tmpVideo)
	if err != nil {
		return err
	}
	data, err := json.Marshal(previewManifest{Version: manifestVersion, Size: info.Size(), SHA256: digest})
	if err != nil {
		return err
	}
	tmpManifest := tmpVideo + manifestExt
	if err := os.WriteFile(tmpManifest, append(data, '\n'), 0o644); err != nil {
		return err
	}
	if current != nil {
		ok, err := current(ctx)
		if err != nil {
			return fmt.Errorf("プレビューの公開条件を確認できません: %w", err)
		}
		if !ok {
			return domain.ErrPreviewStale
		}
	}
	if err := os.Rename(tmpVideo, target); err != nil {
		if verifyPreview(target, manifest) != nil {
			return fmt.Errorf("プレビューを確定できません: %w", err)
		}
		return nil
	}
	if err := os.Rename(tmpManifest, manifest); err != nil {
		_ = os.Remove(target)
		return fmt.Errorf("プレビューのmanifestを確定できません: %w", err)
	}
	return nil
}

// ThumbnailFile は配信のためにライブラリ用サムネイルを開く。無ければ
// fs.ErrNotExist を包んだ誤りを返す。閉じるのは呼び出し側である。
func (s *Store) ThumbnailFile(contentKey string) (*os.File, error) {
	path, ok := s.thumbnailPath(contentKey)
	if !ok {
		return nil, errInvalidKey
	}
	return openRegular(path, nil)
}

// PreviewFile は配信のためにホバープレビューの MP4 を開く。PreviewAvailable と
// 同じく、manifest と大きさが一致しなければ fs.ErrNotExist を包んだ誤りを返す。
// 閉じるのは呼び出し側である。
func (s *Store) PreviewFile(contentKey string) (*os.File, error) {
	path, manifest, ok := s.previewPaths(contentKey)
	if !ok {
		return nil, errInvalidKey
	}
	return openRegular(path, func(info os.FileInfo) error {
		return checkManifestSize(manifest, info)
	})
}

// SeekThumbnail は再生位置 positionMs を含むシーク用プレビューの1フレームを
// 読む。無ければ fs.ErrNotExist を包んだ誤りを返す。
func (s *Store) SeekThumbnail(contentKey string, positionMs int64) ([]byte, error) {
	dir, ok := s.seekDir(contentKey)
	if !ok {
		return nil, errInvalidKey
	}
	index := max(int64(0), positionMs/domain.SeekThumbnailInterval.Milliseconds())
	return os.ReadFile(filepath.Join(dir, fmt.Sprintf(frameNameFormat, index)))
}

// PreviewAvailable はホバープレビューを配信できるかを返す。
//
// MP4 が空でない通常ファイルで、manifest が読めてその大きさと一致すれば完成と
// みなす。SHA-256 の照合は一覧の応答のたびに全部のプレビューを読み切ることに
// なるので、ここでは行わず、生成のときに既存のファイルを採用するか決める
// PublishPreview だけで行う。
func (s *Store) PreviewAvailable(contentKey string) bool {
	path, manifest, ok := s.previewPaths(contentKey)
	if !ok {
		return false
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		return false
	}
	return checkManifestSize(manifest, info) == nil
}

// SeekThumbnailsAvailable はシーク用プレビューの置き場があるかを返す。置き場は
// 生成の完了時に一時置き場から改名して作られるので、あれば完成している。
func (s *Store) SeekThumbnailsAvailable(contentKey string) bool {
	dir, ok := s.seekDir(contentKey)
	return ok && isDir(dir)
}

// RemoveContent は内容1つ分の生成物（ライブラリ用サムネイル・シーク用
// プレビュー・ホバープレビューとその manifest）をすべて消す。無いものは無視する。
// 置き場に使えない content key では何も消さない。
func (s *Store) RemoveContent(contentKey string) error {
	var errs []error
	if path, ok := s.thumbnailPath(contentKey); ok {
		if err := removeFile(path); err != nil {
			errs = append(errs, fmt.Errorf("サムネイルを削除できません: %w", err))
		}
	}
	if dir, ok := s.seekDir(contentKey); ok {
		if err := os.RemoveAll(dir); err != nil {
			errs = append(errs, fmt.Errorf("シークサムネイルを削除できません: %w", err))
		}
	}
	if path, manifest, ok := s.previewPaths(contentKey); ok {
		for _, p := range []string{path, manifest} {
			if err := removeFile(p); err != nil {
				errs = append(errs, fmt.Errorf("プレビューを削除できません: %w", err))
			}
		}
	}
	return errors.Join(errs...)
}

// previewManifest はホバープレビューの完全性を確かめるための記録である。
type previewManifest struct {
	Version int    `json:"version"`
	Size    int64  `json:"size"`
	SHA256  string `json:"sha256"`
}

// readManifest は manifest を読み、形を確かめる。
func readManifest(path string) (previewManifest, error) {
	var manifest previewManifest
	data, err := os.ReadFile(path)
	if err != nil {
		return manifest, err
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return manifest, err
	}
	if manifest.Version != manifestVersion || len(manifest.SHA256) != sha256.Size*2 {
		return manifest, errors.New("preview manifest has an unknown format")
	}
	return manifest, nil
}

// checkManifestSize は manifest が読めて、その大きさが MP4 と一致するかを
// 確かめる。
func checkManifestSize(manifestPath string, video os.FileInfo) error {
	manifest, err := readManifest(manifestPath)
	if err != nil {
		return err
	}
	if manifest.Size != video.Size() {
		return errors.New("preview manifest does not match size")
	}
	return nil
}

// verifyPreview は manifest と MP4 の中身全体を照合する。
func verifyPreview(path, manifestPath string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return errors.New("preview is not a regular file")
	}
	manifest, err := readManifest(manifestPath)
	if err != nil {
		return err
	}
	if manifest.Size != info.Size() {
		return errors.New("preview manifest does not match size")
	}
	actual, err := fileSHA256(path)
	if err != nil {
		return err
	}
	if !strings.EqualFold(manifest.SHA256, actual) {
		return errors.New("preview manifest does not match digest")
	}
	return nil
}

// openRegular は通常ファイルを開く。check が誤りを返したら閉じて、
// fs.ErrNotExist を包んだ誤りを返す。
func openRegular(path string, check func(os.FileInfo) error) (*os.File, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err == nil && !info.Mode().IsRegular() {
		err = errors.New("not a regular file")
	}
	if err == nil && check != nil {
		err = check(info)
	}
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("%s: %w: %w", path, fs.ErrNotExist, err)
	}
	return file, nil
}

func removeFile(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
