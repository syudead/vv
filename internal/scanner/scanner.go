package scanner

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/syudead/vv/internal/domain"
	"golang.org/x/text/unicode/norm"
)

var ErrNoMediaFolders = errors.New("メディアフォルダが登録されていません")

// mediaExtensions は取り込みの対象にする拡張子である。
//
// 再生できない形式（mkv・avi など）も取り込む。一覧に出したうえで「再生でき
// ない」と示すためで、一覧から消してしまうと利用者は手元に何があるか分から
// なくなる。
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
	ListMediaFolders(ctx context.Context) ([]domain.MediaFolder, error)
	// IndexedVideosByPath は索引に入っているものをパスで引ける形で返す。
	IndexedVideosByPath(ctx context.Context) (map[string]domain.IndexedVideo, error)
	// UpsertVideo は1件を索引に反映する。
	UpsertVideo(ctx context.Context, file domain.VideoFile) (domain.UpsertResult, error)
	// DeleteVideoLocations removes missing filesystem locations and orphan videos.
	DeleteVideoLocations(ctx context.Context, ids []int64) error
}

// Queue は重い処理の積み先である。nil でもよい（積まないだけ）。
type Queue interface {
	EnqueueJob(ctx context.Context, kind domain.JobKind, videoID int64) error
	EnsureJob(ctx context.Context, kind domain.JobKind, videoID int64) error
}

// Reporter は走査の進捗の報告先である。nil でもよい（報告しないだけ）。
type Reporter interface {
	ReportScanProgress(ctx context.Context, result domain.ScanResult) error
}

// Options は走査の組み立てに必要な依存である。
type Options struct {
	Index    Index
	Queue    Queue
	Reporter Reporter
	// Logger は nil なら slog の既定を使う。
	Logger *slog.Logger
}

// Scanner は登録済みメディアフォルダを走査して索引を実際のファイルに合わせる。
type Scanner struct {
	index      Index
	queue      Queue
	reporter   Reporter
	logger     *slog.Logger
	contentKey func(string) (string, error)
}

type scanTarget struct {
	path string
}

// New は走査を組み立てる。
func New(opts Options) *Scanner {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Scanner{
		index:      opts.Index,
		queue:      opts.Queue,
		reporter:   opts.Reporter,
		logger:     logger,
		contentKey: ContentKey,
	}
}

// Scan は走査を1回行い、集計を返す。
//
// 手順は次のとおり。
//
//  1. 索引に入っているものをパスで引ける形で読み出す
//  2. 登録済みの全ルート以下を再帰的に走り、取り込み対象を確定する
//  3. パスが一致する行は、サイズと mtime を比べる。変化が無ければ何もしない
//     （content_key の再計算もしない）
//  4. 対象数を進捗として報告してから、新しい・変わったファイルだけ
//     content_key を計算して反映する。内容が
//     同じでパスが違うものは移動・改名として扱われる（重複を作らない）
//  5. 走査で見つからなかった行を消す
//
// 動画ファイルは読み取りのみで扱う。変更・移動・削除・変換は行わない。
// 個別のファイルの失敗では中止せず、失敗として数えて次へ進む。
func (s *Scanner) Scan(ctx context.Context) (domain.ScanResult, error) {
	folders, err := s.index.ListMediaFolders(ctx)
	if err != nil {
		return domain.ScanResult{}, err
	}
	if len(folders) == 0 {
		return domain.ScanResult{}, ErrNoMediaFolders
	}
	indexed, err := s.index.IndexedVideosByPath(ctx)
	if err != nil {
		return domain.ScanResult{}, err
	}

	var result domain.ScanResult
	// seen は走査で見つけたパス。ここに無い索引の行が「消えたファイル」になる。
	seen := map[string]struct{}{}
	// targets は metadata の比較で変更なしを除いた、実際に取り込むファイル。
	// 全ルートを列挙してから処理し、進捗の分母を先に確定させる。
	targets := []scanTarget{}

	protected := []string{}
	for _, folder := range folders {
		root := folder.Path
		rootInfo, rootErr := os.Lstat(root)
		if rootErr != nil || rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
			protected = append(protected, root)
			result.Failed++
			continue
		}
		walkErr := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			if err != nil {
				// 根が読めない場合は走査そのものの失敗。途中のディレクトリが
				// 読めないだけなら、その範囲を保護して続ける。通常
				// fileではSkipDirを返さない。返すと後続の兄弟まで省略される。
				protected = append(protected, path)
				s.logger.Warn("走査中に読み取れない場所がありました",
					slog.String("path", path), slog.Any("error", err))
				result.Failed++
				if walkErrorIsDirectory(path, root, entry, indexed) {
					return fs.SkipDir
				}
				return nil
			}

			if entry.IsDir() {
				if path != root && isExcludedDir(entry.Name()) {
					return fs.SkipDir
				}
				return nil
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return nil
			}

			if !isMediaFile(entry.Name()) {
				return nil
			}

			seen[path] = struct{}{}
			info, infoErr := entry.Info()
			if infoErr != nil {
				// media file だと判定できた時点で取り込み候補である。metadata を
				// 読めない場合も対象件数と失敗件数に含める。
				result.Total++
				result.Failed++
				s.logger.Warn("取り込み対象の情報を読めないファイルがあります",
					slog.String("path", path), slog.Any("error", infoErr))
				return nil
			}

			if existing, ok := indexed[path]; ok &&
				existing.SizeBytes == info.Size() &&
				existing.MTime.Unix() == info.ModTime().Unix() {
				// 変わっていないファイルは取り込み対象に含めない。欠落した
				// pending jobだけを補い、terminal failureは復活させない。
				if err := s.ensurePendingJobs(ctx, existing); err != nil {
					if ctx.Err() != nil {
						return err
					}
					s.logger.Warn("取り込み済みファイルのjobを確認できませんでした",
						slog.String("path", path), slog.Any("error", err))
					result.Total++
					result.Failed++
				}
				return nil
			}

			targets = append(targets, scanTarget{path: path})
			return nil
		})
		if walkErr != nil {
			if ctx.Err() != nil {
				return result, ctx.Err()
			}
			protected = append(protected, root)
			result.Failed++
			s.logger.Warn("メディアフォルダを最後まで走査できませんでした", slog.String("path", root), slog.Any("error", walkErr))
		}
	}
	result.Total += len(targets)
	// 対象列挙中は total=0 の不確定表示で、ここから確定した 0 / total を示す。
	if err := s.report(ctx, result); err != nil {
		return result, err
	}

	for i, target := range targets {
		if err := s.ingest(ctx, target, &result); err != nil {
			if ctx.Err() != nil {
				return result, err
			}
			// 1件の失敗で全体を止めない。理由は記録に残し、次のファイルへ進む。
			s.logger.Warn("取り込めなかったファイルがあります",
				slog.String("path", target.path), slog.Any("error", err))
			result.Failed++
		} else {
			result.Processed++
		}

		if (i+1)%progressInterval == 0 {
			if err := s.report(ctx, result); err != nil {
				return result, err
			}
		}
	}
	if err := s.report(ctx, result); err != nil {
		return result, err
	}

	removed, err := s.removeMissing(ctx, folders, seen, protected)
	if err != nil {
		return result, err
	}
	result.Removed = removed

	return result, nil
}

func walkErrorIsDirectory(path, root string, entry fs.DirEntry, indexed map[string]domain.IndexedVideo) bool {
	if path == root || entry != nil && entry.IsDir() {
		return true
	}
	for indexedPath := range indexed {
		if indexedPath != path && domain.PathWithinRoot(path, indexedPath) {
			return true
		}
	}
	return false
}

// ingest は1つのファイルを索引に反映する。
func (s *Scanner) ingest(
	ctx context.Context,
	target scanTarget,
	result *domain.ScanResult,
) error {
	info, err := stableTargetInfo(target.path)
	if err != nil {
		return err
	}

	key, err := s.contentKey(target.path)
	if err != nil {
		return err
	}
	after, err := stableTargetInfo(target.path)
	if err != nil {
		return err
	}
	if !os.SameFile(info, after) || info.Size() != after.Size() || !info.ModTime().Equal(after.ModTime()) {
		return fmt.Errorf("取り込み中にファイルが変更されました (%s)", target.path)
	}

	file := domain.VideoFile{
		Path:       target.path,
		Title:      titleOf(target.path),
		ContentKey: key,
		SizeBytes:  info.Size(),
		MTime:      info.ModTime(),
		Container:  domain.ContainerFromPath(target.path),
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

	return s.enqueue(ctx, upserted)
}

func stableTargetInfo(path string) (fs.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("取り込み対象の情報を読めません (%s): %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("取り込み対象が通常ファイルではありません (%s)", path)
	}
	return info, nil
}

func (s *Scanner) ensurePendingJobs(ctx context.Context, video domain.IndexedVideo) error {
	if s.queue == nil {
		return nil
	}
	states := []struct {
		kind    domain.JobKind
		pending bool
	}{
		{kind: domain.JobProbe, pending: video.ProbeState == domain.ProbeStatePending},
		{kind: domain.JobThumbnail, pending: video.ThumbnailState == domain.ThumbnailStatePending},
		{kind: domain.JobPreview, pending: video.ProbeState == domain.ProbeStateDone && video.PreviewState == domain.PreviewStatePending},
	}
	for _, state := range states {
		if state.pending {
			if err := s.queue.EnsureJob(ctx, state.kind, video.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

// enqueue は解析とサムネイルのジョブを積む。
func (s *Scanner) enqueue(ctx context.Context, result domain.UpsertResult) error {
	if s.queue == nil {
		return nil
	}
	kinds := []domain.JobKind{}
	if result.NeedsProbe {
		kinds = append(kinds, domain.JobProbe)
	}
	if result.NeedsThumbnail {
		kinds = append(kinds, domain.JobThumbnail)
	}
	if result.NeedsPreview {
		kinds = append(kinds, domain.JobPreview)
	}
	for _, kind := range kinds {
		if err := s.queue.EnqueueJob(ctx, kind, result.ID); err != nil {
			return err
		}
	}
	return nil
}

// removeMissing は走査で見つからなかった行を消す。
//
// 移動・改名の場合は、新しいパスの取り込みで content_key が一致し、既存の行が
// そちらへ付け替わっている。その結果このパスは索引から消えているので、ここで
// 「消えたファイル」として扱われることはない。
func (s *Scanner) removeMissing(
	ctx context.Context, folders []domain.MediaFolder, seen map[string]struct{}, protected []string,
) (int, error) {
	current, err := s.index.IndexedVideosByPath(ctx)
	if err != nil {
		return 0, err
	}

	var missing []int64
	for path, row := range current {
		if _, ok := seen[path]; ok {
			continue
		}
		managed := false
		for _, folder := range folders {
			if domain.PathWithinRoot(folder.Path, path) {
				managed = true
				break
			}
		}
		if !managed || pathProtected(path, protected) {
			continue
		}
		locationID := row.LocationID
		if locationID == 0 {
			locationID = row.ID
		}
		missing = append(missing, locationID)
	}
	if len(missing) == 0 {
		return 0, nil
	}

	if err := s.index.DeleteVideoLocations(ctx, missing); err != nil {
		return 0, err
	}
	return len(missing), nil
}

func pathProtected(path string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if domain.PathWithinRoot(prefix, path) {
			return true
		}
	}
	return false
}

// report は進捗を報告する。報告の失敗で走査を止めない。
func (s *Scanner) report(ctx context.Context, result domain.ScanResult) error {
	if s.reporter == nil {
		return nil
	}
	if err := s.reporter.ReportScanProgress(ctx, result); err != nil {
		return fmt.Errorf("走査の進捗を記録できません: %w", err)
	}
	return nil
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
// ファイル名をそのまま使う。
func titleOf(path string) string {
	name := filepath.Base(path)
	title := strings.TrimSuffix(name, filepath.Ext(name))
	if title == "" {
		return name
	}
	return norm.NFC.String(title)
}
