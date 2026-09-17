package scanner

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/syudead/vv/internal/domain"
	"golang.org/x/text/unicode/norm"
)

// mediaExtensions は取り込みの対象にする拡張子である（R-107）。
//
// 再生できない形式（mkv・avi など）も取り込む。一覧に出したうえで「再生でき
// ない」と示すためで、一覧から消してしまうと利用者は手元に何があるか分から
// なくなる（FR-003）。
var mediaExtensions = map[string]struct{}{
	".mp4": {}, ".m4v": {}, ".webm": {}, ".mkv": {}, ".mov": {}, ".avi": {},
	".wmv": {}, ".flv": {}, ".ts": {}, ".mpg": {}, ".mpeg": {},
}

// excludedNames は名前が一致したら丸ごと飛ばすものである。NAS が作る管理用の
// ディレクトリと、ファイルシステムの復旧領域を対象にする。
var excludedNames = map[string]struct{}{
	"@eaDir":     {}, // Synology のサムネイル置き場
	"#recycle":   {}, // Synology のごみ箱
	"lost+found": {}, // fsck の置き場
}

// partialExtensions は書き込み途中のファイルが持つ拡張子である。
// 取り込むと、途中の内容で content_key を計算してしまう。
var partialExtensions = map[string]struct{}{
	".part": {}, ".crdownload": {}, ".tmp": {},
}

// progressInterval は進捗を報告する間隔（件数）である。1件ごとに書くと
// 走査中の書き込みが増えすぎ、一覧の応答に影響する。
const progressInterval = 20

// Index は走査結果の保存先である。scanner は保存の手段を知らない。
type Index interface {
	// IndexedVideosByPath は索引に入っているものをパスで引ける形で返す。
	IndexedVideosByPath(ctx context.Context) (map[string]domain.IndexedVideo, error)
	// UpsertVideo は1件を索引に反映する。
	UpsertVideo(ctx context.Context, file domain.VideoFile) (domain.UpsertResult, error)
	// DeleteVideos は指定した行を消す。
	DeleteVideos(ctx context.Context, ids []int64) error
	// ContentKeys は参照されている内容の識別子を集合で返す。
	ContentKeys(ctx context.Context) (map[string]struct{}, error)
}

// Queue は重い処理の積み先である。nil でもよい（積まないだけ）。
type Queue interface {
	EnqueueJob(ctx context.Context, kind domain.JobKind, videoID int64) error
}

// Reporter は走査の進捗の報告先である。nil でもよい（報告しないだけ）。
type Reporter interface {
	ReportScanProgress(ctx context.Context, result domain.ScanResult) error
}

// Options は走査の組み立てに必要な依存である。
type Options struct {
	// MediaDir は走査する根。この配下だけを対象にする。
	MediaDir string
	Index    Index
	Queue    Queue
	Reporter Reporter
	// ThumbnailsDir は孤児サムネイルの掃除先。空なら掃除しない。
	ThumbnailsDir string
	// Logger は nil なら slog の既定を使う。
	Logger *slog.Logger
}

// Scanner は MDM_MEDIA_DIR を走査して索引を実際のファイルに合わせる。
type Scanner struct {
	mediaDir      string
	index         Index
	queue         Queue
	reporter      Reporter
	thumbnailsDir string
	logger        *slog.Logger
	contentKey    func(string) (string, error)
}

// New は走査を組み立てる。
func New(opts Options) *Scanner {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Scanner{
		mediaDir:      opts.MediaDir,
		index:         opts.Index,
		queue:         opts.Queue,
		reporter:      opts.Reporter,
		thumbnailsDir: opts.ThumbnailsDir,
		logger:        logger,
		contentKey:    ContentKey,
	}
}

// Scan は走査を1回行い、集計を返す（R-107）。
//
// 手順は次のとおり。
//
//  1. 索引に入っているものをパスで引ける形で読み出す
//  2. MediaDir 以下を再帰的に走り、対象のファイルを列挙する
//  3. パスが一致する行は、サイズと mtime を比べる。変化が無ければ何もしない
//     （content_key の再計算もしない）
//  4. 新しい・変わったファイルだけ content_key を計算して反映する。内容が
//     同じでパスが違うものは移動・改名として扱われる（重複を作らない）
//  5. 走査で見つからなかった行を消す
//  6. 参照されなくなったサムネイルを掃除する
//
// 動画ファイルは読み取りのみで扱う。変更・移動・削除・変換は行わない（FR-009）。
// 個別のファイルの失敗では中止せず、失敗として数えて次へ進む（FR-008）。
func (s *Scanner) Scan(ctx context.Context) (domain.ScanResult, error) {
	indexed, err := s.index.IndexedVideosByPath(ctx)
	if err != nil {
		return domain.ScanResult{}, err
	}

	var result domain.ScanResult
	// seen は走査で見つけたパス。ここに無い索引の行が「消えたファイル」になる。
	seen := map[string]struct{}{}

	walkErr := filepath.WalkDir(s.mediaDir, func(path string, entry fs.DirEntry, err error) error {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if err != nil {
			// 根が読めない場合は走査そのものの失敗。途中のディレクトリが
			// 読めないだけなら、そこを飛ばして続ける（FR-008）。
			if path == s.mediaDir {
				return err
			}
			s.logger.Warn("走査中に読み取れない場所がありました",
				slog.String("path", path), slog.Any("error", err))
			result.Failed++
			return fs.SkipDir
		}

		if entry.IsDir() {
			if path != s.mediaDir && isExcludedDir(entry.Name()) {
				return fs.SkipDir
			}
			return nil
		}

		if !isMediaFile(entry.Name()) {
			return nil
		}

		normalized := norm.NFC.String(path)
		seen[normalized] = struct{}{}
		result.Total++

		if err := s.ingest(ctx, path, normalized, entry, indexed, &result); err != nil {
			if ctx.Err() != nil {
				return err
			}
			// 1件の失敗で全体を止めない。理由は記録に残し、次のファイルへ進む。
			s.logger.Warn("取り込めなかったファイルがあります",
				slog.String("path", path), slog.Any("error", err))
			result.Failed++
		}

		if result.Total%progressInterval == 0 {
			s.report(ctx, result)
		}
		return nil
	})
	if walkErr != nil {
		return result, fmt.Errorf("%s を走査できません: %w", s.mediaDir, walkErr)
	}

	removed, err := s.removeMissing(ctx, indexed, seen)
	if err != nil {
		return result, err
	}
	result.Removed = removed

	s.report(ctx, result)
	s.cleanOrphanThumbnails(ctx)

	return result, nil
}

// ingest は1つのファイルを索引に反映する。
func (s *Scanner) ingest(
	ctx context.Context,
	path, normalized string,
	entry fs.DirEntry,
	indexed map[string]domain.IndexedVideo,
	result *domain.ScanResult,
) error {
	info, err := entry.Info()
	if err != nil {
		return err
	}

	// 変わっていないファイルは何もしない。ここで content_key を計算しないので、
	// 2 回目以降の走査はディレクトリ走査と比較だけで終わる（R-107）。
	if existing, ok := indexed[normalized]; ok &&
		existing.SizeBytes == info.Size() &&
		existing.MTime.Unix() == info.ModTime().Unix() {
		return nil
	}

	key, err := s.contentKey(path)
	if err != nil {
		return err
	}

	file := domain.VideoFile{
		Path:       normalized,
		Title:      titleOf(normalized),
		ContentKey: key,
		SizeBytes:  info.Size(),
		MTime:      info.ModTime(),
		Container:  domain.ContainerFromPath(normalized),
	}

	upserted, err := s.index.UpsertVideo(ctx, file)
	if err != nil {
		return err
	}

	switch upserted.Outcome {
	case domain.OutcomeAdded:
		result.Added++
	case domain.OutcomeUpdated:
		result.Updated++
	case domain.OutcomeMoved:
		result.Moved++
	case domain.OutcomeUnchanged:
		return nil
	}

	// 内容が変わった（または新しい）ものだけ、重い処理を積む。移動・改名では
	// 内容が同じなので、解析もサムネイルも作り直さない（FR-025）。
	if upserted.Outcome == domain.OutcomeMoved {
		return nil
	}
	return s.enqueue(ctx, upserted.ID)
}

// enqueue は解析とサムネイルのジョブを積む。
func (s *Scanner) enqueue(ctx context.Context, videoID int64) error {
	if s.queue == nil {
		return nil
	}
	for _, kind := range []domain.JobKind{domain.JobProbe, domain.JobThumbnail} {
		if err := s.queue.EnqueueJob(ctx, kind, videoID); err != nil {
			return err
		}
	}
	return nil
}

// removeMissing は走査で見つからなかった行を消す（FR-005）。
//
// 移動・改名の場合は、新しいパスの取り込みで content_key が一致し、既存の行が
// そちらへ付け替わっている。その結果このパスは索引から消えているので、ここで
// 「消えたファイル」として扱われることはない。
func (s *Scanner) removeMissing(
	ctx context.Context, indexed map[string]domain.IndexedVideo, seen map[string]struct{},
) (int, error) {
	current, err := s.index.IndexedVideosByPath(ctx)
	if err != nil {
		return 0, err
	}

	var missing []int64
	for path, row := range current {
		if _, ok := seen[path]; !ok {
			missing = append(missing, row.ID)
		}
	}
	if len(missing) == 0 {
		return 0, nil
	}

	if err := s.index.DeleteVideos(ctx, missing); err != nil {
		return 0, err
	}
	return len(missing), nil
}

// cleanOrphanThumbnails は、どの content_key からも参照されなくなった画像を
// 消す（data-model.md 2 節）。
//
// 動画がライブラリから消えても画像はファイルとして残るため、掃除しないと
// MDM_DATA_DIR が増え続ける。掃除の失敗で走査全体を失敗にはしない。次回の
// 走査でやり直せる後始末だからである。
func (s *Scanner) cleanOrphanThumbnails(ctx context.Context) {
	if s.thumbnailsDir == "" {
		return
	}

	keys, err := s.index.ContentKeys(ctx)
	if err != nil {
		s.logger.Warn("サムネイルの掃除を省きました", slog.Any("error", err))
		return
	}

	referenced := map[string]struct{}{}
	for key := range keys {
		referenced[thumbnailFileName(key)] = struct{}{}
	}

	removed := 0
	err = filepath.WalkDir(s.thumbnailsDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err //nolint:nilerr // ディレクトリは対象外、誤りはそのまま上げる
		}
		name := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		if _, ok := referenced[name]; ok {
			return nil
		}
		if err := os.Remove(path); err != nil {
			s.logger.Warn("サムネイルを消せませんでした",
				slog.String("path", path), slog.Any("error", err))
			return nil
		}
		removed++
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		s.logger.Warn("サムネイルの掃除に失敗しました", slog.Any("error", err))
	}
	if removed > 0 {
		s.logger.Info("参照されないサムネイルを削除しました", slog.Int("removed", removed))
	}
}

// report は進捗を報告する。報告の失敗で走査を止めない。
func (s *Scanner) report(ctx context.Context, result domain.ScanResult) {
	if s.reporter == nil {
		return
	}
	if err := s.reporter.ReportScanProgress(ctx, result); err != nil {
		s.logger.Warn("走査の進捗を記録できませんでした", slog.Any("error", err))
	}
}

// isMediaFile は取り込みの対象かどうかを返す。
func isMediaFile(name string) bool {
	// 名前が "." で始まるものは隠しファイルとして飛ばす。
	if strings.HasPrefix(name, ".") {
		return false
	}

	ext := strings.ToLower(filepath.Ext(name))

	// 書き込み途中のファイルは、途中の内容で content_key を計算してしまう。
	if _, ok := partialExtensions[ext]; ok {
		return false
	}

	_, ok := mediaExtensions[ext]
	return ok
}

// isExcludedDir は丸ごと飛ばすディレクトリかどうかを返す。
func isExcludedDir(name string) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	_, ok := excludedNames[name]
	return ok
}

// titleOf は表示名を決める。拡張子を除いたファイル名で、空になる場合は
// ファイル名をそのまま使う（data-model.md）。
func titleOf(path string) string {
	name := filepath.Base(path)
	title := strings.TrimSuffix(name, filepath.Ext(name))
	if title == "" {
		return name
	}
	return norm.NFC.String(title)
}

// thumbnailFileName は content_key からサムネイルのファイル名（拡張子を除く）を
// 決める。internal/media の ThumbnailPath と同じ規則である。
//
// 掃除のためだけに internal/media へ依存するより、規則を1行で写す方が
// 依存の向き（ARCHITECTURE.md）を保てる。規則が変わったときに両方を直す
// 必要があるので、双方のコメントで対応を示しておく。
func thumbnailFileName(contentKey string) string {
	return strings.NewReplacer(":", "_", "/", "_", `\`, "_").Replace(contentKey)
}
