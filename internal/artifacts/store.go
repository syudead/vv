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
	"image"
	"image/jpeg"
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
//	<root>/seek/<p>/<s>/<n>.jpg             シーク用プレビューのシート（<n> は3桁の番号）
//	<root>/seek/<p>/<s>/sprite.json         その配置情報。完成の印を兼ねる
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
	// sheetNameFormat はシーク用プレビューのシートのファイル名である
	// （internal/media の GenerateSeekSprite が書く名前）。
	sheetNameFormat = "%03d.jpg"
	// spriteFileName はシーク用プレビューの配置情報のファイル名である。
	spriteFileName = "sprite.json"
)

// spriteVersion はシーク用プレビューの配置情報の版である。
const spriteVersion = 1

// dirPerm は置き場のディレクトリを作るときの許可属性である。
const dirPerm os.FileMode = 0o755

// manifestVersion はホバープレビューの manifest の版である。
const manifestVersion = 1

// publishLockWait はホバープレビューの公開の錠を待つ間隔である。
const publishLockWait = 100 * time.Millisecond

// errNotConfigured は、置き場の根が設定されていないことを表す。
var errNotConfigured = errors.New("the artifact store is not configured")

// errInvalidKey は、置き場の外を指しうる content key を表す。読み出しでは
// 「無い」と同じに扱えるよう fs.ErrNotExist を包む。
var errInvalidKey = fmt.Errorf("content key cannot be used in the artifact store: %w", fs.ErrNotExist)

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

// publishTarget は公開の前に置き場と content key を確かめ、使えない理由を返す。
func (s *Store) publishTarget(locate func(string) (string, bool), contentKey string) (string, error) {
	if s.root == "" {
		return "", errNotConfigured
	}
	target, ok := locate(contentKey)
	if !ok {
		return "", errInvalidKey
	}
	return target, nil
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
		return "", errNotConfigured
	}
	root := filepath.Join(s.root, temporaryDirName)
	if err := os.MkdirAll(root, dirPerm); err != nil {
		return "", fmt.Errorf("cannot create the temporary directory for generation: %w", err)
	}
	dir, err := os.MkdirTemp(root, pattern)
	if err != nil {
		return "", fmt.Errorf("cannot create the temporary directory for generation: %w", err)
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
		return fmt.Errorf("cannot delete the temporary directories for generation: %w", err)
	}
	return nil
}

// PublishThumbnail はライブラリ用サムネイルを作り直して公開する。write は一時
// 置き場のパスを受けて画像を書く。書かれた画像が空でなければ本来の場所へ
// 改名し、既存の画像を置き換える。
func (s *Store) PublishThumbnail(contentKey string, write func(output string) error) error {
	target, err := s.publishTarget(s.thumbnailPath, contentKey)
	if err != nil {
		return err
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
		return fmt.Errorf("thumbnail was not generated (%s)", contentKey)
	}
	if err := os.MkdirAll(filepath.Dir(target), dirPerm); err != nil {
		return fmt.Errorf("cannot create the thumbnail directory (%s): %w", filepath.Dir(target), err)
	}
	if err := os.Rename(output, target); err != nil {
		return fmt.Errorf("cannot publish the thumbnail: %w", err)
	}
	return nil
}

// PublishSeekThumbnails はシーク用プレビューのスプライトを公開する
// （specs/021-seek-thumbnail-sprite/research.md R-3）。配置情報（sprite.json）の
// ある完成した置き場がすでにあれば write を呼ばずに成功を返す。
//
// write は一時置き場のディレクトリを受け、layout の配置でシート 000.jpg から順に
// 書き、全編の復号から作ったかを返す。書き終えたら、コマの大きさをシート 000.jpg の
// 寸法から読んで、全編の復号から作ったかと合わせて sprite.json を書き、ディレクトリ
// ごと本来の場所へ改名する。
//
// 戻り値は、公開した（または採用した）スプライトが全編の復号から作られたかである。
// 採用したものに記録が無ければ（この記録を持たない旧いもの）SubstitutionUnknown を
// 返す。公開と完了の記録の間で止まった後の再実行や、同じ内容の別の動画でも、代用を
// 取りこぼさない（specs/024-import-progress/research.md R-7）。その直前に、配置情報の無い置き場
// （旧形式の個別 JPEG や、途中で壊れたもの）を消す。同じ内容の公開と削除を
// 直列にする錠は呼び出し側（internal/app）が持つ。
func (s *Store) PublishSeekThumbnails(
	contentKey string, layout domain.SeekSpriteLayout, write func(outputDir string) (fullDecode bool, err error),
) (domain.Substitution, error) {
	unknown := domain.SubstitutionUnknown
	target, err := s.publishTarget(s.seekDir, contentKey)
	if err != nil {
		return unknown, err
	}
	if file, _, err := readSpriteFile(target); err == nil {
		return file.substitution(), nil
	}
	if err := os.MkdirAll(filepath.Dir(target), dirPerm); err != nil {
		return unknown, fmt.Errorf("cannot create the seek thumbnail directory: %w", err)
	}
	temporary, err := s.makeTemporaryDir("seek-*")
	if err != nil {
		return unknown, err
	}
	defer func() { _ = os.RemoveAll(temporary) }()

	fullDecode, err := write(temporary)
	if err != nil {
		return unknown, err
	}
	sprite, err := describeSheets(temporary, layout)
	if err != nil {
		return unknown, fmt.Errorf("seek thumbnails were not generated (%s): %w", contentKey, err)
	}
	file := spriteFileFrom(sprite)
	file.FullDecode = &fullDecode
	data, err := json.Marshal(file)
	if err != nil {
		return unknown, err
	}
	if err := os.WriteFile(filepath.Join(temporary, spriteFileName), append(data, '\n'), 0o644); err != nil {
		return unknown, fmt.Errorf("cannot write the seek thumbnail manifest: %w", err)
	}
	if err := os.RemoveAll(target); err != nil {
		return unknown, fmt.Errorf("cannot delete old seek thumbnails: %w", err)
	}
	// ディレクトリごと改名するので、配置情報があれば完成している。
	if err := os.Rename(temporary, target); err != nil {
		if published, _, readErr := readSpriteFile(target); readErr == nil {
			return published.substitution(), nil
		}
		return unknown, fmt.Errorf("cannot publish the seek thumbnails: %w", err)
	}
	return file.substitution(), nil
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
	target, err := s.publishTarget(func(key string) (string, bool) {
		video, _, ok := s.previewPaths(key)
		return video, ok
	}, contentKey)
	if err != nil {
		return err
	}
	manifest := target + manifestExt
	if err := os.MkdirAll(filepath.Dir(target), dirPerm); err != nil {
		return fmt.Errorf("cannot create the preview directory: %w", err)
	}
	// 2つのファイルの公開は組として不可分にできないので、同じ置き場を使う
	// プロセスはすべて、この1つの錠で公開を直列にする。
	assetLock := flock.New(filepath.Join(s.root, previewDirName, publishLockName))
	locked, err := assetLock.TryLockContext(ctx, publishLockWait)
	if err != nil {
		return fmt.Errorf("cannot acquire the preview generation lock: %w", err)
	}
	if !locked {
		return errors.New("cannot acquire the preview generation lock")
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
		return fmt.Errorf("cannot check the preview: %w", err)
	}
	if info.Size() == 0 {
		return errors.New("preview was not generated: the file is empty")
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
			return fmt.Errorf("cannot check whether the preview can be published: %w", err)
		}
		if !ok {
			return domain.ErrPreviewStale
		}
	}
	if err := os.Rename(tmpVideo, target); err != nil {
		if verifyPreview(target, manifest) != nil {
			return fmt.Errorf("cannot publish the preview: %w", err)
		}
		return nil
	}
	if err := os.Rename(tmpManifest, manifest); err != nil {
		_ = os.Remove(target)
		return fmt.Errorf("cannot publish the preview manifest: %w", err)
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

// PreviewFile は配信のためにホバープレビューの MP4 を開き、manifest に記録した
// 内容の SHA-256 と合わせて返す。PreviewAvailable と同じく、manifest と大きさが
// 一致しなければ fs.ErrNotExist を包んだ誤りを返す。閉じるのは呼び出し側である。
func (s *Store) PreviewFile(contentKey string) (*os.File, string, error) {
	path, manifestPath, ok := s.previewPaths(contentKey)
	if !ok {
		return nil, "", errInvalidKey
	}
	var digest string
	file, err := openRegular(path, func(info os.FileInfo) error {
		manifest, err := checkPreviewComplete(manifestPath, info)
		digest = manifest.SHA256
		return err
	})
	if err != nil {
		return nil, "", err
	}
	return file, digest, nil
}

// SeekSprite は完成したシーク用プレビューの配置情報を読む。置き場か配置情報が
// 無ければ fs.ErrNotExist を包んだ誤りを返し、配置情報の形が違えばそれ以外の
// 誤りを返す。
func (s *Store) SeekSprite(contentKey string) (domain.SeekSprite, error) {
	dir, ok := s.seekDir(contentKey)
	if !ok {
		return domain.SeekSprite{}, errInvalidKey
	}
	return readSeekSprite(dir)
}

// SeekSpriteSheet はシーク用プレビューのシート sheet（0 から）を読む。無ければ
// fs.ErrNotExist を包んだ誤りを返す。枚数との照合は SeekSprite の結果で呼び出し側が
// 行う。
func (s *Store) SeekSpriteSheet(contentKey string, sheet int) ([]byte, error) {
	dir, ok := s.seekDir(contentKey)
	if !ok {
		return nil, errInvalidKey
	}
	// Completed sprites from the previous six-sheet layout remain readable.
	if sheet < 0 || sheet >= 6 {
		return nil, fmt.Errorf("sheet %d: %w", sheet, fs.ErrNotExist)
	}
	return os.ReadFile(filepath.Join(dir, fmt.Sprintf(sheetNameFormat, sheet)))
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
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	_, err = checkPreviewComplete(manifest, info)
	return err == nil
}

// SeekThumbnailsAvailable はシーク用プレビューが完成しているかを返す。完成の印は
// 読める配置情報（sprite.json）で、それの無い置き場（旧形式の個別 JPEG や途中で
// 壊れたもの）は未完成とみなす。置き場は生成の完了時に一時置き場から改名して
// 作られるので、配置情報があればシートも揃っている。
func (s *Store) SeekThumbnailsAvailable(contentKey string) bool {
	_, err := s.SeekSprite(contentKey)
	return err == nil
}

// RemoveContent は内容1つ分の生成物（ライブラリ用サムネイル・シーク用
// プレビュー・ホバープレビューとその manifest）をすべて消す。無いものは無視する。
// 置き場に使えない content key では何も消さない。
func (s *Store) RemoveContent(contentKey string) error {
	var errs []error
	if path, ok := s.thumbnailPath(contentKey); ok {
		if err := removeFile(path); err != nil {
			errs = append(errs, fmt.Errorf("cannot delete the thumbnail: %w", err))
		}
	}
	if dir, ok := s.seekDir(contentKey); ok {
		if err := os.RemoveAll(dir); err != nil {
			errs = append(errs, fmt.Errorf("cannot delete the seek thumbnails: %w", err))
		}
	}
	if path, manifest, ok := s.previewPaths(contentKey); ok {
		for _, p := range []string{path, manifest} {
			if err := removeFile(p); err != nil {
				errs = append(errs, fmt.Errorf("cannot delete the preview: %w", err))
			}
		}
	}
	return errors.Join(errs...)
}

// spriteFile は sprite.json の形である（research.md R-3）。
type spriteFile struct {
	Version     int   `json:"version"`
	IntervalMs  int64 `json:"intervalMs"`
	FrameCount  int   `json:"frameCount"`
	Columns     int   `json:"columns"`
	Rows        int   `json:"rows"`
	FrameWidth  int   `json:"frameWidth"`
	FrameHeight int   `json:"frameHeight"`
	SheetCount  int   `json:"sheetCount"`
	// FullDecode は全編の復号から作ったかである。この記録より前に作られたものには
	// 無い（specs/024-import-progress/research.md R-7）。
	FullDecode *bool `json:"fullDecode,omitempty"`
}

// substitution は、このスプライトが代用（全編の復号）で作られたかを返す。
func (f spriteFile) substitution() domain.Substitution {
	if f.FullDecode == nil {
		return domain.SubstitutionUnknown
	}
	return domain.SubstitutionOf(*f.FullDecode)
}

func spriteFileFrom(sprite domain.SeekSprite) spriteFile {
	return spriteFile{
		Version:     spriteVersion,
		IntervalMs:  sprite.IntervalMs,
		FrameCount:  sprite.FrameCount,
		Columns:     sprite.Columns,
		Rows:        sprite.Rows,
		FrameWidth:  sprite.FrameWidth,
		FrameHeight: sprite.FrameHeight,
		SheetCount:  sprite.SheetCount,
	}
}

// readSeekSprite は置き場 dir の配置情報を読み、形を確かめる。
func readSeekSprite(dir string) (domain.SeekSprite, error) {
	_, sprite, err := readSpriteFile(dir)
	return sprite, err
}

// readSpriteFile は置き場 dir の sprite.json を読み、形を確かめる。
func readSpriteFile(dir string) (spriteFile, domain.SeekSprite, error) {
	data, err := os.ReadFile(filepath.Join(dir, spriteFileName))
	if err != nil {
		return spriteFile{}, domain.SeekSprite{}, err
	}
	var file spriteFile
	if err := json.Unmarshal(data, &file); err != nil {
		return spriteFile{}, domain.SeekSprite{}, fmt.Errorf("cannot read the seek thumbnail manifest: %w", err)
	}
	sprite := domain.SeekSprite{
		SeekSpriteLayout: domain.SeekSpriteLayout{
			IntervalMs: file.IntervalMs,
			FrameCount: file.FrameCount,
			Columns:    file.Columns,
			Rows:       file.Rows,
			SheetCount: file.SheetCount,
		},
		FrameWidth:  file.FrameWidth,
		FrameHeight: file.FrameHeight,
	}
	if file.Version != spriteVersion || !validSeekSprite(sprite) {
		return spriteFile{}, domain.SeekSprite{}, errors.New("the seek thumbnail manifest has an unexpected shape")
	}
	return file, sprite, nil
}

// validSeekSprite は配置情報の値が契約の範囲にあり、互いに食い違わないかを返す
// （contracts/seek-sprite-api.md §2）。
func validSeekSprite(sprite domain.SeekSprite) bool {
	if sprite.IntervalMs < domain.SeekThumbnailInterval.Milliseconds() ||
		sprite.FrameCount < 1 || sprite.FrameCount > 600 ||
		// 掛け算があふれないよう、1 シートのコマ数を上限のコマ数までに
		// 割り算で抑えてから掛ける。
		sprite.Columns < 1 || sprite.Columns > 600 ||
		sprite.Rows < 1 || sprite.Rows > 600/sprite.Columns ||
		sprite.FrameWidth < 2 || sprite.FrameHeight < 2 ||
		sprite.SheetCount < 1 || sprite.SheetCount > 6 {
		return false
	}
	perSheet := sprite.Columns * sprite.Rows
	return sprite.SheetCount == (sprite.FrameCount+perSheet-1)/perSheet
}

// describeSheets は一時置き場 dir に layout の枚数のシートが揃っているかを
// 確かめ、コマの大きさをシート 000.jpg の JPEG の寸法を列数・行数で割って得る。
// ffmpeg の式や解析の値からは計算しない（自動回転と偶数への丸めを再現しない）。
// どのシートも JPEG として読めて 000.jpg と同じ寸法であることも確かめ、表示
// できないシートを含む置き場を完成として公開しない（最後のシートも tile が
// 余りを埋めるので同じ寸法になる）。
func describeSheets(dir string, layout domain.SeekSpriteLayout) (domain.SeekSprite, error) {
	var width, height int
	for sheet := range layout.SheetCount {
		config, err := sheetConfig(filepath.Join(dir, fmt.Sprintf(sheetNameFormat, sheet)))
		if err != nil {
			return domain.SeekSprite{}, fmt.Errorf("sheet %d: %w", sheet, err)
		}
		if sheet == 0 {
			width, height = config.Width, config.Height
		} else if config.Width != width || config.Height != height {
			return domain.SeekSprite{}, fmt.Errorf("sheet %d is %dx%d, but sheet 0 is %dx%d",
				sheet, config.Width, config.Height, width, height)
		}
	}
	if layout.Columns < 1 || layout.Rows < 1 || layout.SheetCount < 1 ||
		width%layout.Columns != 0 || height%layout.Rows != 0 {
		return domain.SeekSprite{}, fmt.Errorf("sheet 0 size %dx%d does not divide into a %dx%d grid",
			width, height, layout.Columns, layout.Rows)
	}
	sprite := domain.SeekSprite{
		SeekSpriteLayout: layout,
		FrameWidth:       width / layout.Columns,
		FrameHeight:      height / layout.Rows,
	}
	if !validSeekSprite(sprite) {
		return domain.SeekSprite{}, errors.New("sheet layout is out of range")
	}
	return sprite, nil
}

// sheetConfig は path が空でない通常ファイルの JPEG であることを確かめ、その
// 寸法を返す。
func sheetConfig(path string) (image.Config, error) {
	info, err := os.Stat(path)
	if err != nil {
		return image.Config{}, err
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return image.Config{}, errors.New("empty")
	}
	file, err := os.Open(path)
	if err != nil {
		return image.Config{}, err
	}
	defer func() { _ = file.Close() }()
	config, err := jpeg.DecodeConfig(file)
	if err != nil {
		return image.Config{}, fmt.Errorf("cannot read as JPEG: %w", err)
	}
	return config, nil
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

// checkPreviewComplete は応答のたびに行う完全性の確認である。MP4 が空でなく、
// manifest が読めて、その大きさが MP4 と一致すれば完成とみなす。確認
// （PreviewAvailable）と配信（PreviewFile）は同じこの規則を使う。
func checkPreviewComplete(manifestPath string, video os.FileInfo) (previewManifest, error) {
	if video.Size() == 0 {
		return previewManifest{}, errors.New("preview is empty")
	}
	manifest, err := readManifest(manifestPath)
	if err != nil {
		return previewManifest{}, err
	}
	if manifest.Size != video.Size() {
		return previewManifest{}, errors.New("preview manifest does not match size")
	}
	return manifest, nil
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
