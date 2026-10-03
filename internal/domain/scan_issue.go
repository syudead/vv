package domain

import (
	"cmp"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
)

// ScanIssueKind は直近の取り込みで起きた、利用者に知らせる出来事の種類である。
// 値は scan_issues.kind と api/openapi.yaml の ScanIssueKind に対応する
// （specs/024-import-progress/data-model.md §3）。
type ScanIssueKind string

const (
	// IssueUnreadable は走査がファイルの情報や内容を読めなかった。
	IssueUnreadable ScanIssueKind = "unreadable"
	// IssueChangedDuringImport は登録の途中でファイルが変わった。
	IssueChangedDuringImport ScanIssueKind = "changed_during_import"
	// IssueRegisterFailed は索引への書き込みや仕事の積み込みに失敗した。
	IssueRegisterFailed ScanIssueKind = "register_failed"
	// IssueProbeFailed は解析の仕事がやり直しの上限まで失敗した。
	IssueProbeFailed ScanIssueKind = "probe_failed"
	// IssueThumbnailFailed は代表サムネイルの仕事が上限まで失敗した。
	IssueThumbnailFailed ScanIssueKind = "thumbnail_failed"
	// IssueSeekThumbnailFailed はシーク用サムネイルの仕事が上限まで失敗した。
	IssueSeekThumbnailFailed ScanIssueKind = "seek_thumbnail_failed"
	// IssuePreviewFailed は一覧用プレビューの仕事が上限まで失敗した。
	IssuePreviewFailed ScanIssueKind = "preview_failed"
	// IssueFingerprintFailed は映像の指紋の仕事が上限まで失敗した。
	IssueFingerprintFailed ScanIssueKind = "fingerprint_failed"
	// IssueThumbnailFirstFrame は代表サムネイルを先頭のコマで作った（代用）。
	IssueThumbnailFirstFrame ScanIssueKind = "thumbnail_first_frame"
	// IssueSeekThumbnailFullDecode はシーク用サムネイルを全編から作り直した（代用）。
	IssueSeekThumbnailFullDecode ScanIssueKind = "seek_thumbnail_full_decode"
)

// scanIssueKinds は重い順に並べた種類である。まとめた件の kinds はこの順に並ぶ。
var scanIssueKinds = []ScanIssueKind{
	IssueUnreadable, IssueChangedDuringImport, IssueRegisterFailed,
	IssueProbeFailed, IssueThumbnailFailed, IssueSeekThumbnailFailed, IssuePreviewFailed,
	IssueFingerprintFailed, IssueThumbnailFirstFrame, IssueSeekThumbnailFullDecode,
}

// ScanIssueKinds は既知の種類を重い順に返す。
func ScanIssueKinds() []ScanIssueKind {
	return slices.Clone(scanIssueKinds)
}

// Valid は既知の種類かを返す。
func (k ScanIssueKind) Valid() bool {
	return slices.Contains(scanIssueKinds, k)
}

// Severity は種類の重さである。
func (k ScanIssueKind) Severity() ScanIssueSeverity {
	switch k {
	case IssueThumbnailFirstFrame, IssueSeekThumbnailFullDecode:
		return IssueSubstituted
	default:
		return IssueFailed
	}
}

// FromScan は走査がファイルを登録できなかった種類かを返す。この種類を持つ件は、
// 対象の動画の集合に入っていなければ「登録できなかったファイル」として分母と
// 済みの本数に足す（specs/024-import-progress/research.md R-5）。
func (k ScanIssueKind) FromScan() bool {
	return k == IssueUnreadable || k == IssueChangedDuringImport || k == IssueRegisterFailed
}

// FailedIssueKind は仕事の種類がやり直しの上限まで失敗したときの問題の種類を返す。
func FailedIssueKind(kind JobKind) (ScanIssueKind, bool) {
	switch kind {
	case JobProbe:
		return IssueProbeFailed, true
	case JobThumbnail:
		return IssueThumbnailFailed, true
	case JobSeekThumbnail:
		return IssueSeekThumbnailFailed, true
	case JobPreview:
		return IssuePreviewFailed, true
	case JobFingerprint:
		return IssueFingerprintFailed, true
	}
	return "", false
}

// SubstitutedIssueKind は仕事の種類が代用して成功したときの問題の種類を返す。
// 代用のある段階は、代表サムネイルとシーク用サムネイルだけである。
func SubstitutedIssueKind(kind JobKind) (ScanIssueKind, bool) {
	switch kind {
	case JobThumbnail:
		return IssueThumbnailFirstFrame, true
	case JobSeekThumbnail:
		return IssueSeekThumbnailFullDecode, true
	}
	return "", false
}

// Substitution は、生成の段階が成功したときに代用したかである。internal/media の
// 生成の関数が返す値から internal/app が決め、成功を書く保存側へ渡す
// （specs/024-import-progress/research.md R-7）。
type Substitution int

const (
	// SubstitutionUnknown は、生成せずに既存の生成物を採用したなど、代用したかが
	// 分からないことを表す。保存側は代用の問題の行を変えない。
	SubstitutionUnknown Substitution = iota
	// SubstitutionNone は代用せずに作ったことを表す。保存側は代用の行を消す。
	SubstitutionNone
	// SubstitutionUsed は代用して作ったことを表す。保存側は代用の行を入れる。
	SubstitutionUsed
)

// SubstitutionOf は生成の関数が返した「代用したか」を Substitution にする。
func SubstitutionOf(substituted bool) Substitution {
	if substituted {
		return SubstitutionUsed
	}
	return SubstitutionNone
}

// ScanIssueSeverity はまとめた件の重さである。値は api/openapi.yaml の
// ScanIssue.severity に対応する。
type ScanIssueSeverity string

const (
	// IssueFailed は失敗の種類を1つ以上含む。取り込みを partial にする。
	IssueFailed ScanIssueSeverity = "failed"
	// IssueSubstituted は代用の種類だけを含む。
	IssueSubstituted ScanIssueSeverity = "substituted"
)

// ScanFileIssue は走査が1つのファイルで出会った失敗である。internal/scanner が
// 報告し、internal/app が直近の取り込みの問題として記録する。
type ScanFileIssue struct {
	// VideoID は走査が知っている既存の動画の id。知らなければ 0 である。
	VideoID int64
	// Path はファイルの絶対パスである。
	Path string
	Kind ScanIssueKind
}

// ScanIssueRecord は保存された問題の行を、動画（未登録ならファイルのパス）ごとに
// まとめたものである。保存側が返し、ScanIssues が表示の形にする。
type ScanIssueRecord struct {
	// VideoID は登録された動画の id。未登録のファイルは 0 である。
	VideoID int64
	// Path は表示の所在の絶対パスである。動画なら今の代表の所在（登録フォルダの
	// 中に所在が無ければ空）、未登録なら出来事の時点のパス。
	Path  string
	Kinds []ScanIssueKind
	// InImport はその動画が直近の取り込みの対象の集合に入っているかである。
	InImport bool
}

// ScanIssue は直近の取り込みの問題の、まとめた1件である
// （specs/024-import-progress/contracts/scan-api.md §3）。
type ScanIssue struct {
	// VideoID は登録された動画の id。未登録のファイルは 0 である。
	VideoID  int64
	Severity ScanIssueSeverity
	// Kinds は1件以上の種類で、重い順に並ぶ。
	Kinds    []ScanIssueKind
	FileName string
	Folder   VideoFolder
	// RootName は所在を含む登録フォルダの表示名である。
	RootName string
	// Path は表示の所在の絶対パスである。並びの最後の決め手と、未登録の件の識別に使う。
	Path string
	// Unregistered は、対象の集合に入らないまま走査が登録できなかったファイルかである。
	Unregistered bool
}

// ScanIssueCounts は直近の取り込みの問題の本数である。
type ScanIssueCounts struct {
	Failed      int
	Substituted int
	// Unregistered は登録できなかったファイルの件数である（分母と済みの本数に足す）。
	Unregistered int
}

// ScanIssues は保存された行を表示の形にし、並べて返す。所在がどの登録フォルダにも
// 含まれない件は除く（contracts/scan-api.md §3）。並びは、失敗を先に、同じ重さの
// 中はファイル名、次にフォルダ（登録フォルダの表示名、相対パス）の順である。
func ScanIssues(roots []MediaFolder, records []ScanIssueRecord) []ScanIssue {
	out := make([]ScanIssue, 0, len(records))
	for _, record := range records {
		if record.Path == "" || len(record.Kinds) == 0 {
			continue
		}
		folder, ok := LocateVideoFolder(roots, record.Path)
		if !ok {
			continue
		}
		issue := ScanIssue{
			VideoID:  record.VideoID,
			Severity: IssueSubstituted,
			Kinds:    sortedIssueKinds(record.Kinds),
			FileName: filepath.Base(record.Path),
			Folder:   folder,
			Path:     record.Path,
		}
		for _, root := range roots {
			if root.ID == folder.RootID {
				issue.RootName = FolderName(root.Path, "")
				break
			}
		}
		for _, kind := range issue.Kinds {
			if kind.Severity() == IssueFailed {
				issue.Severity = IssueFailed
			}
			if kind.FromScan() && !record.InImport {
				issue.Unregistered = true
			}
		}
		out = append(out, issue)
	}
	slices.SortFunc(out, func(a, b ScanIssue) int {
		return compareIssueKeys(a.sortKey(), b.sortKey())
	})
	return out
}

func sortedIssueKinds(kinds []ScanIssueKind) []ScanIssueKind {
	out := make([]ScanIssueKind, 0, len(kinds))
	for _, kind := range scanIssueKinds {
		if slices.Contains(kinds, kind) {
			out = append(out, kind)
		}
	}
	return out
}

// CountScanIssues はまとめた件を重さごとに数える。
func CountScanIssues(issues []ScanIssue) ScanIssueCounts {
	var counts ScanIssueCounts
	for _, issue := range issues {
		if issue.Severity == IssueFailed {
			counts.Failed++
		} else {
			counts.Substituted++
		}
		if issue.Unregistered {
			counts.Unregistered++
		}
	}
	return counts
}

// scanIssueKey は並びとカーソルに使う値である。どの2件も異なる値を持つ。
type scanIssueKey struct {
	Severity int    `json:"s"`
	FileName string `json:"f"`
	RootName string `json:"r"`
	Folder   string `json:"p"`
	Location string `json:"l"`
	Video    int64  `json:"v"`
}

func (i ScanIssue) sortKey() scanIssueKey {
	severity := 0
	if i.Severity != IssueFailed {
		severity = 1
	}
	return scanIssueKey{
		Severity: severity, FileName: i.FileName, RootName: i.RootName,
		Folder: i.Folder.Path, Location: i.Path, Video: i.VideoID,
	}
}

func compareIssueKeys(a, b scanIssueKey) int {
	return cmp.Or(
		cmp.Compare(a.Severity, b.Severity),
		cmp.Compare(a.FileName, b.FileName),
		cmp.Compare(a.RootName, b.RootName),
		cmp.Compare(a.Folder, b.Folder),
		cmp.Compare(a.Location, b.Location),
		cmp.Compare(a.Video, b.Video),
	)
}

// ScanIssuePage は問題の一覧の1ページである。
type ScanIssuePage struct {
	ScanID int64
	Items  []ScanIssue
	// NextCursor は続きがあるときだけ入る。
	NextCursor string
}

// PageScanIssues は並べた一覧から、cursor の次の件を limit 件まで返す。カーソルは
// 最後に返した件の並びの値なので、ページのあいだに行が増えたり消えたりしても、
// 同じ件を2度返すことはない。解釈できないカーソルは ErrInvalidCursor である。
func PageScanIssues(issues []ScanIssue, cursor string, limit int) ([]ScanIssue, string, error) {
	start := 0
	if cursor != "" {
		after, err := decodeIssueCursor(cursor)
		if err != nil {
			return nil, "", err
		}
		start, _ = slices.BinarySearchFunc(issues, after, func(issue ScanIssue, key scanIssueKey) int {
			return compareIssueKeys(issue.sortKey(), key)
		})
		for start < len(issues) && compareIssueKeys(issues[start].sortKey(), after) <= 0 {
			start++
		}
	}
	end := min(len(issues), start+max(1, limit))
	page := issues[start:end]
	if end >= len(issues) {
		return page, "", nil
	}
	next, err := encodeIssueCursor(page[len(page)-1].sortKey())
	if err != nil {
		return nil, "", err
	}
	return page, next, nil
}

func encodeIssueCursor(key scanIssueKey) (string, error) {
	body, err := json.Marshal(key)
	if err != nil {
		return "", fmt.Errorf("cannot encode the issue cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(body), nil
}

func decodeIssueCursor(cursor string) (scanIssueKey, error) {
	body, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return scanIssueKey{}, fmt.Errorf("%w: %s", ErrInvalidCursor, strconv.Quote(cursor))
	}
	var key scanIssueKey
	if err := json.Unmarshal(body, &key); err != nil || (key.Severity != 0 && key.Severity != 1) {
		return scanIssueKey{}, fmt.Errorf("%w: %s", ErrInvalidCursor, strconv.Quote(cursor))
	}
	return key, nil
}
