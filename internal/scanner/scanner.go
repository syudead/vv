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
	"slices"
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
	".wmv": {}, ".flv": {}, ".ts": {}, ".m2ts": {}, ".mts": {}, ".mpg": {}, ".mpeg": {},
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

// Queue は重い処理の積み先である。nil でもよい（積まないだけ）。decider は取り込みを始めたもので、
// 積んだ仕事の import の記録に残る（specs/055-activity/research.md R-5）。1 本の動画に要る仕事は 1 回の
// 呼び出しでまとめて積み、1 件の記録にする（R-3）。
type Queue interface {
	EnqueueJobs(ctx context.Context, videoID int64, decider domain.ActivityDecider, kinds ...domain.JobKind) error
	EnsureJobs(ctx context.Context, videoID int64, decider domain.ActivityDecider, kinds ...domain.JobKind) error
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

// scanPass は1回の走査が集めるものである。
type scanPass struct {
	indexed map[string]domain.IndexedVideo
	result  domain.ScanResult
	// seen は走査で見つけたパス。ここに無い索引の行が「消えたファイル」になる。
	seen map[string]struct{}
	// targets は metadata の比較で変更なしを除いた、実際に取り込むファイル。
	// 全ルートを列挙してから処理し、進捗の分母を先に確定させる。
	targets []scanTarget
	// decider は走査を始めたもので、積む仕事の記録に渡す。
	decider domain.ActivityDecider
	// scoped は範囲を絞った走査（監視の走査）であることを表す。取り込みの途中で変わった・
	// 消えたファイルを失敗にせず、変更の知らせで始まる次の走査に任せる。
	scoped bool
	// vanished は監視の走査が歩いている途中で消えたディレクトリである。その下の所在は
	// 今回は消さず、消えた知らせで始まる次の監視の走査に任せる（research.md R-3）。
	vanished []domain.DirtyDirectory
}

// Scan は全体の走査を1回行い、集計を返す。
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
//
// decider は走査を始めたもの（手動の走査は所有者、起動時の再開は規則）で、積む仕事の記録に渡す。
func (s *Scanner) Scan(ctx context.Context, decider domain.ActivityDecider) (domain.ScanResult, error) {
	return s.scan(ctx, nil, false, decider)
}

// ScanScoped は dirs のディレクトリだけを読む走査を1回行い、集計を返す
// （specs/042-folder-watch-import/research.md R-2・R-3）。
//
// Scan と手順は同じだが、歩くのは dirs だけで、消すのは「パスが dirs の読む範囲に
// あり、今回の走査で見つからなかった」所在だけである。範囲の外は、索引にあっても
// 見ていないので何も変えない。追加をすべて終えてから削除するので、dirs の間で動かした
// 動画は同じ動画のまま所在が付け替わる。
//
// 走査の context が domain.ErrScanSuperseded を理由に取り消されたら、次のファイルの
// 前で止め、何も消さずに domain.ErrScanSuperseded を返す。
//
// 監視の走査は規則が始めたものとして仕事を積む（specs/055-activity/research.md R-5）。
func (s *Scanner) ScanScoped(ctx context.Context, dirs []domain.DirtyDirectory) (domain.ScanResult, error) {
	return s.scan(ctx, domain.NormalizeDirtyDirectories(dirs), true, domain.ActivityByRule)
}

func (s *Scanner) scan(
	ctx context.Context, dirs []domain.DirtyDirectory, scoped bool, decider domain.ActivityDecider,
) (domain.ScanResult, error) {
	result, err := s.scanOnce(ctx, dirs, scoped, decider)
	if err != nil && ctx.Err() != nil && errors.Is(context.Cause(ctx), domain.ErrScanSuperseded) {
		return result, domain.ErrScanSuperseded
	}
	return result, err
}

func (s *Scanner) scanOnce(
	ctx context.Context, dirs []domain.DirtyDirectory, scoped bool, decider domain.ActivityDecider,
) (domain.ScanResult, error) {
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

	pass := &scanPass{
		indexed: indexed, seen: map[string]struct{}{}, targets: []scanTarget{}, scoped: scoped, decider: decider,
	}
	// scanned は今回の走査が読むメディアフォルダ。全体の走査は全部、範囲を絞った走査は
	// 読むディレクトリを持つものだけである（ほかのフォルダが読めなくても止めない）。
	scanned := folders
	if scoped {
		scanned = nil
		for _, folder := range folders {
			if slices.ContainsFunc(dirs, func(d domain.DirtyDirectory) bool {
				return domain.PathWithinRoot(folder.Path, d.Path)
			}) {
				scanned = append(scanned, folder)
			}
		}
	}

	// collected は実際に歩いたディレクトリ。削除の範囲はここから決める。メディアフォルダの
	// 外にある項目や、メディアフォルダの祖先の項目は歩かないので、範囲に入れない。
	var collected []domain.DirtyDirectory
	for _, folder := range scanned {
		root := folder.Path
		if err := checkMediaFolder(root); err != nil {
			return domain.ScanResult{}, err
		}
		if !scoped {
			err = s.walkDir(root, func(path string, entry fs.DirEntry, err error) error {
				return s.visit(ctx, pass, root, root, true, path, entry, err)
			})
			if err != nil {
				if ctx.Err() != nil {
					return domain.ScanResult{}, ctx.Err()
				}
				return domain.ScanResult{}, fmt.Errorf("could not finish scanning the media folder (%s): %w", root, err)
			}
			continue
		}
		for _, dir := range dirs {
			if !domain.PathWithinRoot(root, dir.Path) {
				continue
			}
			collected = append(collected, dir)
			if err := s.collectDirectory(ctx, pass, root, dir); err != nil {
				if ctx.Err() != nil {
					return domain.ScanResult{}, ctx.Err()
				}
				return domain.ScanResult{}, err
			}
		}
	}
	result := pass.result
	seen := pass.seen
	result.Total += len(pass.targets)
	// 対象列挙中は total=0 の不確定表示で、ここから確定した 0 / total を示す。
	if err := s.report(ctx, result); err != nil {
		return result, err
	}
	if err := s.ingestTargets(ctx, pass, &result); err != nil {
		return result, err
	}
	if err := s.dropVanished(pass, scanned); err != nil {
		return result, err
	}

	// 取って代わられた走査は、ここまでに足したものを残し、何も消さない。
	if err := ctx.Err(); err != nil {
		return result, err
	}
	inScope := func(string) bool { return true }
	if scoped {
		inScope = func(path string) bool {
			return domain.DirtyDirectoriesContain(collected, path) &&
				!domain.DirtyDirectoriesContain(pass.vanished, path)
		}
	}
	removed, err := s.removeMissing(ctx, folders, seen, inScope)
	if err != nil {
		return result, err
	}
	result.Removed = removed

	return result, nil
}

// ingestTargets は pass の取り込み対象を1つずつ索引に反映し、result を進める。
func (s *Scanner) ingestTargets(ctx context.Context, pass *scanPass, result *domain.ScanResult) error {
	for _, target := range pass.targets {
		if err := ctx.Err(); err != nil {
			return err
		}
		videoID := pass.indexed[target.path].ID
		s.reportFile(target.path, videoID)
		if err := s.ingest(ctx, target, result, pass.decider); err != nil {
			if ctx.Err() != nil {
				return err
			}
			if err := s.recordIngestFailure(ctx, pass, result, target.path, videoID, err); err != nil {
				return err
			}
		} else {
			result.Processed++
		}

		// 1ファイルごとに進みを知らせる。画面への送信は接続ごとにまとまるので、
		// 遅い接続に古い値は積もらない（research.md R-8）。
		if err := s.report(ctx, *result); err != nil {
			return err
		}
	}
	s.reportFile("", 0)
	return nil
}

// recordIngestFailure は1件の取り込みの失敗を数え、取り込みの問題として報告する。1件の
// 失敗で全体は止めない。
//
// 監視の走査では、途中で変わった・消えたファイルは失敗にせず対象から外す。その変更は
// 変更の知らせになって、ディレクトリを次の監視の走査へ回すからである
// （specs/042-folder-watch-import/research.md R-3）。
func (s *Scanner) recordIngestFailure(
	ctx context.Context, pass *scanPass, result *domain.ScanResult, path string, videoID int64, err error,
) error {
	issue := domain.ScanFileIssue{VideoID: videoID, Path: path, Kind: domain.IssueRegisterFailed}
	var failed *fileError
	if errors.As(err, &failed) {
		issue.Kind = failed.kind
		if failed.videoID != 0 {
			issue.VideoID = failed.videoID
		}
	}
	if pass.scoped && issue.Kind == domain.IssueChangedDuringImport {
		s.logger.Info("a file changed during a watch scan; the next watch scan reads it again",
			slog.String("path", path), slog.Any("error", err))
		result.Total--
		return nil
	}
	s.logger.Warn("could not ingest a file", slog.String("path", path), slog.Any("error", err))
	result.Failed++
	return s.reportIssue(ctx, issue)
}

// dropVanished は、列挙のあとに消えたファイルを pass.seen から外す。発見済みという理由
// だけで索引へ残さないためである。
//
// 親ディレクトリごと消えた場合は列挙結果を信頼できないため、全体の走査は失敗にする。
// 監視の走査では、消えたディレクトリは移動か削除の知らせで次の監視の走査に回るので、
// その所在を今は残し（消さず）、走査は続ける。移動先の追加を先に行ってから消すためである
// （research.md R-3）。
func (s *Scanner) dropVanished(pass *scanPass, scanned []domain.MediaFolder) error {
	checkedDirs := map[string]struct{}{}
	for _, folder := range scanned {
		if err := ensureReadableDirectory(folder.Path); err != nil {
			code := domain.ScanErrorMediaFolderUnreadable
			if errors.Is(err, errNotDirectory) {
				code = domain.ScanErrorMediaFolderNotDirectory
			}
			return domain.NewScanFailure(code, folder.Path,
				fmt.Errorf("could not check the media folder after the scan (%s): %w", folder.Path, err))
		}
		checkedDirs[folder.Path] = struct{}{}
	}
	keptDirs := map[string]struct{}{}
	for path := range pass.seen {
		info, err := os.Lstat(path)
		if errors.Is(err, fs.ErrNotExist) {
			keep, err := s.checkVanishedParent(pass.scoped, filepath.Dir(path), checkedDirs, keptDirs)
			if err != nil {
				return err
			}
			if !keep {
				delete(pass.seen, path)
			}
			continue
		}
		if err != nil {
			s.logger.Warn("could not check a file after the scan",
				slog.String("path", path), slog.Any("error", err))
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			delete(pass.seen, path)
		}
	}
	return nil
}

// checkVanishedParent は消えたファイルの親 parent を確かめる。親が読めればファイルだけが
// 消えたので keep は偽である。監視の走査で親ごと消えていれば、所在を残すので keep は真である。
// 確かめた結果は checkedDirs と keptDirs に覚える。
func (s *Scanner) checkVanishedParent(
	scoped bool, parent string, checkedDirs, keptDirs map[string]struct{},
) (keep bool, err error) {
	if _, ok := keptDirs[parent]; ok {
		return true, nil
	}
	if _, ok := checkedDirs[parent]; ok {
		return false, nil
	}
	parentErr := ensureReadableDirectory(parent)
	if parentErr == nil {
		checkedDirs[parent] = struct{}{}
		return false, nil
	}
	if scoped && (errors.Is(parentErr, fs.ErrNotExist) || errors.Is(parentErr, errNotDirectory)) {
		s.logger.Info("a directory vanished during a watch scan; the next watch scan reads it again",
			slog.String("path", parent), slog.Any("error", parentErr))
		keptDirs[parent] = struct{}{}
		return true, nil
	}
	return false, domain.NewScanFailure(domain.ScanErrorLocationUnreadable, parent,
		fmt.Errorf("could not check the parent directory after the scan (%s): %w", parent, parentErr))
}

// checkMediaFolder はメディアフォルダ root が読めるディレクトリかを確かめる。
func checkMediaFolder(root string) error {
	rootInfo, rootErr := os.Lstat(root)
	if rootErr != nil {
		return domain.NewScanFailure(domain.ScanErrorMediaFolderUnreadable, root,
			fmt.Errorf("could not read the media folder (%s): %w", root, rootErr))
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return domain.NewScanFailure(domain.ScanErrorMediaFolderNotDirectory, root,
			fmt.Errorf("the media folder is not a directory (%s)", root))
	}
	return nil
}

// collectDirectory はメディアフォルダ root の中の読み直すディレクトリ dir を歩く。
// 消えたディレクトリ（と、シンボリックリンクやファイルに変わったもの）は空として扱う。
// 除外するディレクトリの中は読まない。
func (s *Scanner) collectDirectory(ctx context.Context, pass *scanPass, root string, dir domain.DirtyDirectory) error {
	if rel, err := filepath.Rel(root, dir.Path); err == nil && rel != "." {
		for _, name := range strings.Split(rel, string(filepath.Separator)) {
			if isExcludedDir(name) {
				return nil
			}
		}
	}
	info, err := s.lstat(dir.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return domain.NewScanFailure(domain.ScanErrorLocationUnreadable, dir.Path,
			fmt.Errorf("could not read a location (%s): %w", dir.Path, err))
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil
	}
	err = s.walkDir(dir.Path, func(path string, entry fs.DirEntry, err error) error {
		if err != nil && path == dir.Path && errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return s.visit(ctx, pass, root, dir.Path, dir.Recursive, path, entry, err)
	})
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var failure *domain.ScanFailure
		if errors.As(err, &failure) {
			return err
		}
		return fmt.Errorf("could not finish scanning the directory (%s): %w", dir.Path, err)
	}
	return nil
}

// visit は走査の1つの入口（ファイルかディレクトリ）を見て、取り込み対象を pass に集める。
// walkRoot は歩き始めた場所で、除外の判定の対象にしない。recursive が偽なら、その下の
// ディレクトリには入らない。
func (s *Scanner) visit(
	ctx context.Context, pass *scanPass, folderRoot, walkRoot string, recursive bool,
	path string, entry fs.DirEntry, err error,
) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if err != nil {
		return s.visitError(pass, folderRoot, path, err)
	}

	if entry.IsDir() {
		if path != walkRoot && !recursive {
			return fs.SkipDir
		}
		if path != walkRoot && isExcludedDir(entry.Name()) {
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

	pass.seen[path] = struct{}{}
	info, infoErr := entry.Info()
	if infoErr != nil && pass.scoped && errors.Is(infoErr, fs.ErrNotExist) {
		// 列挙のあとに消えた。取り込みの対象にはせず、消えた知らせが次の監視の走査に回す。
		// seen には残すので、親ごと消えたかは dropVanished が確かめる（research.md R-3）。
		return nil
	}
	if infoErr == nil && !info.Mode().IsRegular() {
		// 列挙の時点で通常のファイルでない（名前付きパイプなど）。途中で変わったのではなく、
		// 動画として読めないファイルである。
		infoErr = fmt.Errorf("%w (%s)", errNotRegular, path)
	}
	if infoErr != nil {
		// media file だと判定できた時点で取り込み候補である。metadata を
		// 読めない場合も対象件数と失敗件数に含める。
		pass.result.Total++
		pass.result.Failed++
		s.logger.Warn("could not read the file information of a scan target",
			slog.String("path", path), slog.Any("error", infoErr))
		return s.reportIssue(ctx, domain.ScanFileIssue{
			VideoID: pass.indexed[path].ID, Path: path, Kind: domain.IssueUnreadable,
		})
	}

	if existing, ok := pass.indexed[path]; ok &&
		existing.SizeBytes == info.Size() &&
		existing.MTime.Unix() == info.ModTime().Unix() {
		// 変わっていないファイルは取り込み対象に含めない。作成日時が索引と
		// 違えば所在の列だけを直し、欠落した pending jobだけを補い、terminal
		// failureは復活させない。
		createdErr := s.refreshCreatedAt(ctx, path, info, existing)
		jobsErr := s.ensurePendingJobs(ctx, existing, pass.decider)
		if err := errors.Join(createdErr, jobsErr); err != nil {
			if ctx.Err() != nil {
				return err
			}
			s.logger.Warn("could not update an indexed file",
				slog.String("path", path), slog.Any("error", err))
			pass.result.Total++
			pass.result.Failed++
			return s.reportIssue(ctx, domain.ScanFileIssue{
				VideoID: existing.ID, Path: path, Kind: domain.IssueRegisterFailed,
			})
		}
		return nil
	}

	pass.targets = append(pass.targets, scanTarget{path: path})
	return nil
}

// visitError は歩いている途中で読めなかった場所 path の失敗を返す。監視の走査で、途中で
// 消えたディレクトリは失敗にせず pass.vanished に覚え、その下を歩かない。
func (s *Scanner) visitError(pass *scanPass, folderRoot, path string, err error) error {
	if pass.scoped && path != folderRoot && errors.Is(err, fs.ErrNotExist) {
		s.logger.Info("a directory vanished during a watch scan; the next watch scan reads it again",
			slog.String("path", path), slog.Any("error", err))
		pass.vanished = append(pass.vanished, domain.DirtyDirectory{Path: path, Recursive: true})
		return nil
	}
	s.logger.Warn("could not read a location during the scan",
		slog.String("path", path), slog.Any("error", err))
	// メディアフォルダそのものが読めなければ、途中の場所ではなくフォルダの失敗である。
	code := domain.ScanErrorLocationUnreadable
	if path == folderRoot {
		code = domain.ScanErrorMediaFolderUnreadable
	}
	return domain.NewScanFailure(code, path, fmt.Errorf("could not read a location (%s): %w", path, err))
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
	decider domain.ActivityDecider,
) error {
	info, err := s.stableTargetInfo(target.path)
	if err != nil {
		return failFile(unreadableKind(err), 0, err)
	}

	key, err := s.contentKey(target.path)
	if err != nil {
		return failFile(unreadableKind(err), 0, err)
	}
	after, err := s.stableTargetInfo(target.path)
	if err != nil {
		return failFile(unreadableKind(err), 0, err)
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

	if err := s.enqueue(ctx, upserted, decider); err != nil {
		return failFile(domain.IssueRegisterFailed, upserted.ID, err)
	}
	return nil
}

// unreadableKind は対象のファイルを読めなかった理由 err を問題の種類にする。列挙のあとで
// 消えたか通常のファイルでなくなったときだけ、途中で変わったと言える。それ以外の読めなさ
// （I/O の失敗や権限）は、読めなかったとして報告する。
func unreadableKind(err error) domain.ScanIssueKind {
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, errNotRegular) {
		return domain.IssueChangedDuringImport
	}
	return domain.IssueUnreadable
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

func (s *Scanner) ensurePendingJobs(ctx context.Context, video domain.IndexedVideo, decider domain.ActivityDecider) error {
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
	var kinds []domain.JobKind
	for _, state := range states {
		if state.pending {
			kinds = append(kinds, state.kind)
		}
	}
	if len(kinds) == 0 {
		return nil
	}
	return s.queue.EnsureJobs(ctx, video.ID, decider, kinds...)
}

// enqueue は解析・サムネイル・シーク用サムネイル・プレビューのうち要るジョブを積む。
func (s *Scanner) enqueue(ctx context.Context, result domain.UpsertResult, decider domain.ActivityDecider) error {
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
	if len(kinds) == 0 {
		return nil
	}
	return s.queue.EnqueueJobs(ctx, result.ID, decider, kinds...)
}

// removeMissing は走査で見つからなかった行を消す。
//
// 移動・改名の場合は、新しいパスの取り込みで content_key が一致し、既存の行が
// そちらへ付け替わっている。その結果このパスは索引から消えているので、ここで
// 「消えたファイル」として扱われることはない。
//
// inScope が偽を返すパスは、今回の走査が見ていないので消さない。全体の走査は全部を
// 見ているので常に真である。
func (s *Scanner) removeMissing(
	ctx context.Context, folders []domain.MediaFolder, seen map[string]struct{}, inScope func(string) bool,
) (int, error) {
	current, err := s.index.IndexedVideosByPath(ctx)
	if err != nil {
		return 0, err
	}

	var missing []int64
	for path, row := range current {
		if _, ok := seen[path]; ok || !inScope(path) {
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
