package scanner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/syudead/vv/internal/domain"
	"golang.org/x/text/unicode/norm"
)

var ErrNoMediaFolders = errors.New("no media folders are configured")

// errNotDirectory はメディアフォルダ・親ディレクトリがディレクトリでない（シンボリック
// リンクを含む）ことを表す。
var errNotDirectory = errors.New("not a directory")

// errNotRegular は、対象のファイルが通常のファイルでなくなったことを表す。
var errNotRegular = errors.New("not a regular file")

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

// Index は走査結果の保存先である。scanner は保存の手段を知らない。
type Index interface {
	ListMediaFolders(ctx context.Context) ([]domain.MediaFolder, error)
	// IndexedVideosByPath は索引に入っているものをパスで引ける形で返す。
	IndexedVideosByPath(ctx context.Context) (map[string]domain.IndexedVideo, error)
	// UpsertVideo は1件を索引に反映する。
	UpsertVideo(ctx context.Context, file domain.VideoFile) (domain.UpsertResult, error)
	// DeleteVideoLocations removes missing filesystem locations and orphan videos.
	DeleteVideoLocations(ctx context.Context, ids []int64) error
	// UpdateLocationCreatedAt は所在のファイルの作成日時だけを書き直す。ゼロ値は取れなかった
	// （null に戻す）。変わっていないファイルの作成日時が索引と違うときに呼ぶ
	// （specs/033-video-dates/data-model.md §5）。
	UpdateLocationCreatedAt(ctx context.Context, locationID int64, createdAt time.Time) error
}

// Queue は重い処理の積み先である。nil でもよい（積まないだけ）。
type Queue interface {
	EnqueueJob(ctx context.Context, kind domain.JobKind, videoID int64) error
	EnsureJob(ctx context.Context, kind domain.JobKind, videoID int64) error
}

// Reporter は走査の進捗・今のファイル・ファイルごとの失敗の報告先である。nil でも
// よい（報告しないだけ）。失敗は直近の取り込みの問題として記録される
// （specs/024-import-progress/data-model.md §3）。scanner は記録の手段を知らない。
type Reporter interface {
	// ReportScanProgress は進みを報告する。登録の対象を数え終えたときと、
	// 1ファイルを終えるごとに呼ばれる。
	ReportScanProgress(ctx context.Context, result domain.ScanResult) error
	ReportScanIssue(ctx context.Context, issue domain.ScanFileIssue) error
	// ReportScanFile は登録を始めるファイルを知らせる。videoID は走査が知っている
	// 既存の動画（知らなければ 0）である。path が空なら、ファイルの登録をすべて
	// 終えたことを表す（specs/024-import-progress/research.md R-8）。
	ReportScanFile(path string, videoID int64)
}

// fileError は1つのファイルを取り込めなかった理由である。kind は問題の種類、
// videoID は走査が知っている既存の動画（知らなければ 0）である。
type fileError struct {
	kind    domain.ScanIssueKind
	videoID int64
	err     error
}

func (e *fileError) Error() string { return e.err.Error() }
func (e *fileError) Unwrap() error { return e.err }

// failFile は err を kind の失敗として包む。
func failFile(kind domain.ScanIssueKind, videoID int64, err error) error {
	return &fileError{kind: kind, videoID: videoID, err: err}
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
	walkDir    func(string, fs.WalkDirFunc) error
	lstat      func(string) (fs.FileInfo, error)
	// createdAt はファイルの作成日時を読む。試験で差し替える。
	createdAt func(string, fs.FileInfo) (time.Time, bool)
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
		walkDir:    filepath.WalkDir,
		lstat:      os.Lstat,
		createdAt:  fileCreatedAt,
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

	for _, folder := range folders {
		root := folder.Path
		rootInfo, rootErr := os.Lstat(root)
		if rootErr != nil {
			return domain.ScanResult{}, domain.NewScanFailure(domain.ScanErrorMediaFolderUnreadable, root,
				fmt.Errorf("could not read the media folder (%s): %w", root, rootErr))
		}
		if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
			return domain.ScanResult{}, domain.NewScanFailure(domain.ScanErrorMediaFolderNotDirectory, root,
				fmt.Errorf("the media folder is not a directory (%s)", root))
		}
		walkErr := s.walkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			if err != nil {
				s.logger.Warn("could not read a location during the scan",
					slog.String("path", path), slog.Any("error", err))
				// メディアフォルダそのものが読めなければ、途中の場所ではなくフォルダの失敗である。
				code := domain.ScanErrorLocationUnreadable
				if path == root {
					code = domain.ScanErrorMediaFolderUnreadable
				}
				return domain.NewScanFailure(code, path, fmt.Errorf("could not read a location (%s): %w", path, err))
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
				s.logger.Warn("could not read the file information of a scan target",
					slog.String("path", path), slog.Any("error", infoErr))
				return s.reportIssue(ctx, domain.ScanFileIssue{
					VideoID: indexed[path].ID, Path: path, Kind: domain.IssueUnreadable,
				})
			}

			if existing, ok := indexed[path]; ok &&
				existing.SizeBytes == info.Size() &&
				existing.MTime.Unix() == info.ModTime().Unix() {
				// 変わっていないファイルは取り込み対象に含めない。作成日時が索引と
				// 違えば所在の列だけを直し、欠落した pending jobだけを補い、terminal
				// failureは復活させない。
				createdErr := s.refreshCreatedAt(ctx, path, info, existing)
				jobsErr := s.ensurePendingJobs(ctx, existing)
				if err := errors.Join(createdErr, jobsErr); err != nil {
					if ctx.Err() != nil {
						return err
					}
					s.logger.Warn("could not update an indexed file",
						slog.String("path", path), slog.Any("error", err))
					result.Total++
					result.Failed++
					return s.reportIssue(ctx, domain.ScanFileIssue{
						VideoID: existing.ID, Path: path, Kind: domain.IssueRegisterFailed,
					})
				}
				return nil
			}

			targets = append(targets, scanTarget{path: path})
			return nil
		})
		if walkErr != nil {
			if ctx.Err() != nil {
				return domain.ScanResult{}, ctx.Err()
			}
			return domain.ScanResult{}, fmt.Errorf("could not finish scanning the media folder (%s): %w", root, walkErr)
		}
	}
	result.Total += len(targets)
	// 対象列挙中は total=0 の不確定表示で、ここから確定した 0 / total を示す。
	if err := s.report(ctx, result); err != nil {
		return result, err
	}

	for _, target := range targets {
		s.reportFile(target.path, indexed[target.path].ID)
		if err := s.ingest(ctx, target, &result); err != nil {
			if ctx.Err() != nil {
				return result, err
			}
			// 1件の失敗で全体を止めない。理由は取り込みの問題として報告し、次のファイルへ進む。
			s.logger.Warn("could not ingest a file",
				slog.String("path", target.path), slog.Any("error", err))
			result.Failed++
			issue := domain.ScanFileIssue{
				VideoID: indexed[target.path].ID, Path: target.path, Kind: domain.IssueRegisterFailed,
			}
			var failed *fileError
			if errors.As(err, &failed) {
				issue.Kind = failed.kind
				if failed.videoID != 0 {
					issue.VideoID = failed.videoID
				}
			}
			if err := s.reportIssue(ctx, issue); err != nil {
				return result, err
			}
		} else {
			result.Processed++
		}

		// 1ファイルごとに進みを知らせる。画面への送信は接続ごとにまとまるので、
		// 遅い接続に古い値は積もらない（research.md R-8）。
		if err := s.report(ctx, result); err != nil {
			return result, err
		}
	}
	s.reportFile("", 0)

	checkedDirs := map[string]struct{}{}
	for _, folder := range folders {
		if err := ensureReadableDirectory(folder.Path); err != nil {
			code := domain.ScanErrorMediaFolderUnreadable
			if errors.Is(err, errNotDirectory) {
				code = domain.ScanErrorMediaFolderNotDirectory
			}
			return result, domain.NewScanFailure(code, folder.Path,
				fmt.Errorf("could not check the media folder after the scan (%s): %w", folder.Path, err))
		}
		checkedDirs[folder.Path] = struct{}{}
	}

	// 列挙後に消えた個別ファイルを、発見済みという理由だけで索引へ残さない。
	// 親ディレクトリごと消えた場合は列挙結果を信頼できないため、削除せず
	// 走査全体を失敗させる。
	for path := range seen {
		info, err := os.Lstat(path)
		if errors.Is(err, fs.ErrNotExist) {
			parent := filepath.Dir(path)
			if _, ok := checkedDirs[parent]; !ok {
				if parentErr := ensureReadableDirectory(parent); parentErr != nil {
					return result, domain.NewScanFailure(domain.ScanErrorLocationUnreadable, parent,
						fmt.Errorf("could not check the parent directory after the scan (%s): %w", parent, parentErr))
				}
				checkedDirs[parent] = struct{}{}
			}
			delete(seen, path)
			continue
		}
		if err != nil {
			s.logger.Warn("could not check a file after the scan",
				slog.String("path", path), slog.Any("error", err))
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			delete(seen, path)
		}
	}

	removed, err := s.removeMissing(ctx, folders, seen)
	if err != nil {
		return result, err
	}
	result.Removed = removed

	return result, nil
}

func ensureReadableDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errNotDirectory
	}
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	_, readErr := dir.Readdirnames(1)
	closeErr := dir.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return readErr
	}
	return closeErr
}

// ingest は1つのファイルを索引に反映する。
func (s *Scanner) ingest(
	ctx context.Context,
	target scanTarget,
	result *domain.ScanResult,
) error {
	info, err := s.stableTargetInfo(target.path)
	if err != nil {
		return failFile(domain.IssueUnreadable, 0, err)
	}

	key, err := s.contentKey(target.path)
	if err != nil {
		return failFile(domain.IssueUnreadable, 0, err)
	}
	after, err := s.stableTargetInfo(target.path)
	if err != nil {
		// 消えたか通常のファイルでなくなったときだけ、途中で変わったと言える。
		// それ以外の読めなさ（I/O の失敗や権限）は、読めなかったとして報告する。
		kind := domain.IssueUnreadable
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, errNotRegular) {
			kind = domain.IssueChangedDuringImport
		}
		return failFile(kind, 0, err)
	}
	if !os.SameFile(info, after) || info.Size() != after.Size() || !info.ModTime().Equal(after.ModTime()) {
		return failFile(domain.IssueChangedDuringImport, 0,
			fmt.Errorf("the file changed while it was being ingested (%s)", target.path))
	}

	file := domain.VideoFile{
		Path:       target.path,
		Title:      titleOf(target.path),
		ContentKey: key,
		SizeBytes:  info.Size(),
		MTime:      info.ModTime(),
		Container:  domain.ContainerFromPath(target.path),
	}
	// 読めなかった作成日時はゼロ値のまま登録する（失敗にしない）。
	if createdAt, ok := s.createdAt(target.path, info); ok {
		file.FileCreatedAt = createdAt
	}

	upserted, err := s.index.UpsertVideo(ctx, file)
	if err != nil {
		return failFile(domain.IssueRegisterFailed, 0, err)
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

	if err := s.enqueue(ctx, upserted); err != nil {
		return failFile(domain.IssueRegisterFailed, upserted.ID, err)
	}
	return nil
}

func (s *Scanner) stableTargetInfo(path string) (fs.FileInfo, error) {
	info, err := s.lstat(path)
	if err != nil {
		return nil, fmt.Errorf("could not read the file information (%s): %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w (%s)", errNotRegular, path)
	}
	return info, nil
}

// refreshCreatedAt は変わっていないファイルの作成日時を読み、索引の値と秒で違うときだけ
// 所在の列を書き直す。読めなかったときはゼロ値として比べるので、索引に値があれば null に
// 戻る（specs/033-video-dates/research.md R-6）。中身の識別子も job も触らない。
func (s *Scanner) refreshCreatedAt(
	ctx context.Context, path string, info fs.FileInfo, existing domain.IndexedVideo,
) error {
	createdAt, ok := s.createdAt(path, info)
	if !ok {
		createdAt = time.Time{}
	}
	if sameCreatedAt(createdAt, existing.FileCreatedAt) {
		return nil
	}
	locationID := existing.LocationID
	if locationID == 0 {
		locationID = existing.ID
	}
	return s.index.UpdateLocationCreatedAt(ctx, locationID, createdAt)
}

// sameCreatedAt は作成日時を、索引が持つ秒で比べる。ゼロ値どうしは同じである。
func sameCreatedAt(a, b time.Time) bool {
	if a.IsZero() || b.IsZero() {
		return a.IsZero() == b.IsZero()
	}
	return a.Unix() == b.Unix()
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
		// failed は積み直さない。やり直しは読み取りのやり直し（RetryProbe）が受け持つ。
		{kind: domain.JobSeekThumbnail, pending: video.SeekThumbnailState == domain.SeekThumbnailPending},
		{kind: domain.JobPreview, pending: video.ProbeState == domain.ProbeStateDone && video.PreviewState == domain.PreviewStatePending},
		// 指紋は状態の列を持たないので、上限まで失敗した分もここで積み直す
		// （specs/030-video-versions/data-model.md §6）。
		{kind: domain.JobFingerprint, pending: video.FingerprintMissing},
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

// enqueue は解析・サムネイル・シーク用サムネイル・プレビューのうち要るジョブを積む。
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
	if result.NeedsSeekThumbnail {
		kinds = append(kinds, domain.JobSeekThumbnail)
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
	ctx context.Context, folders []domain.MediaFolder, seen map[string]struct{},
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
		if !managed {
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

// report は進捗を報告する。進捗と処理結果の整合性を守るため、報告の失敗時は走査を止める。
func (s *Scanner) report(ctx context.Context, result domain.ScanResult) error {
	if s.reporter == nil {
		return nil
	}
	if err := s.reporter.ReportScanProgress(ctx, result); err != nil {
		return fmt.Errorf("could not record the scan progress: %w", err)
	}
	return nil
}

// reportFile は登録を始めるファイルを知らせる。path が空なら登録を終えたことを表す。
func (s *Scanner) reportFile(path string, videoID int64) {
	if s.reporter != nil {
		s.reporter.ReportScanFile(path, videoID)
	}
}

// reportIssue はファイルの失敗を報告する。報告の失敗時は、進捗と同じく走査を止める。
// 記録できなかった失敗は、利用者から見えなくなるためである。
func (s *Scanner) reportIssue(ctx context.Context, issue domain.ScanFileIssue) error {
	if s.reporter == nil {
		return nil
	}
	if err := s.reporter.ReportScanIssue(ctx, issue); err != nil {
		return fmt.Errorf("could not record a file the scan could not import (%s): %w", issue.Path, err)
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
