package store

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/syudead/vv/internal/domain"
)

// folderPrefix はフォルダ配下の所在の接頭辞（フォルダのパス + 区切り）を返す。
// `/` のように区切りで終わるフォルダには区切りを重ねない。落とすのはその OS の
// 区切りだけで、Unix では `\` はファイル名の一部なので残す（`A\` という名前の
// フォルダを `A` と取り違えない）。
//
// Windows では大文字小文字を区別しない。比べる相手の SQL の lower() は ASCII の
// 英字しか小文字にしないので、ここも ASCII だけを小文字にそろえる
// （strings.ToLower だと `Ä` が `ä` になり、SQL 側の `Ä` と一致しなくなる）。
func folderPrefix(dir string) string {
	return folderPrefixFor(dir, runtime.GOOS == "windows")
}

func folderPrefixFor(dir string, windows bool) string {
	separator := string(os.PathSeparator)
	cutset := separator
	if windows {
		separator = `\`
		cutset = `/\`
	}
	prefix := strings.TrimRight(dir, cutset) + separator
	if windows {
		prefix = domain.LowerASCII(prefix)
	}
	return prefix
}

// folderPathExpr は所在のパスを接頭辞と比べられる形にする。Windows では
// 大文字小文字を区別しない（registeredLocationCondition と同じ扱い）。
func folderPathExpr(alias string) string {
	return folderPathExprFor(alias, runtime.GOOS == "windows")
}

func folderPathExprFor(alias string, windows bool) string {
	if windows {
		return `lower(` + alias + `.path)`
	}
	return alias + `.path`
}

// directChildCondition は、接頭辞の後ろに区切りが無い（直下にある）ことを表す。
// 接頭辞の `?` は1つだけにする。呼び出し側は接頭辞を条件ごとに1回ずつ渡す。
func directChildCondition(alias string) string {
	return directChildConditionFor(alias, runtime.GOOS == "windows")
}

func directChildConditionFor(alias string, windows bool) string {
	rest := `substr(` + folderPathExprFor(alias, windows) + `, length(?) + 1)`
	if windows {
		// Windows では `/` も区切りなので、`\` にそろえてから1回で調べる。
		return `instr(replace(` + rest + `, char(47), char(92)), char(92)) = 0`
	}
	return `instr(` + rest + `, char(` + strconv.Itoa(int(os.PathSeparator)) + `)) = 0`
}

// FolderLocations はフォルダ配下（深さを問わない）の、登録フォルダの下にある
// 所在をすべて返す。集計は internal/domain の SummarizeFolder が行う。
func (s *LibraryStore) FolderLocations(ctx context.Context, dir string) ([]domain.FolderLocation, error) {
	//nolint:gosec // 組み立てるのは定型の条件句だけで、値はすべて引数で渡す。
	rows, err := s.db.sql.QueryContext(ctx, `select l.path, l.video_id, videos.content_key, videos.thumbnail_state, videos.preview_state
		from video_locations l join videos on videos.id = l.video_id
		where instr(`+folderPathExpr("l")+`, ?) = 1 and `+registeredLocationCondition("l"),
		folderPrefix(dir))
	if err != nil {
		return nil, fmt.Errorf("フォルダの中身を読み出せません: %w", err)
	}
	defer func() { _ = rows.Close() }()

	locations := []domain.FolderLocation{}
	for rows.Next() {
		var item domain.FolderLocation
		var thumbnail, preview string
		if err := rows.Scan(&item.Path, &item.VideoID, &item.ContentKey, &thumbnail, &preview); err != nil {
			return nil, fmt.Errorf("フォルダの中身を読み出せません: %w", err)
		}
		item.ThumbnailState = domain.ThumbnailState(thumbnail)
		item.PreviewState = domain.PreviewState(preview)
		locations = append(locations, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("フォルダの中身を読み出せません: %w", err)
	}
	return locations, nil
}

// HasFolderLocations はフォルダ配下（深さを問わない）に、登録フォルダの下に
// ある所在が1件でもあるかを返す。
func (s *LibraryStore) HasFolderLocations(ctx context.Context, dir string) (bool, error) {
	var found int
	//nolint:gosec // 組み立てるのは定型の条件句だけで、値はすべて引数で渡す。
	err := s.db.sql.QueryRowContext(ctx, `select exists (select 1 from video_locations l
		where instr(`+folderPathExpr("l")+`, ?) = 1 and `+registeredLocationCondition("l")+`)`,
		folderPrefix(dir)).Scan(&found)
	if err != nil {
		return false, fmt.Errorf("フォルダの有無を確かめられません: %w", err)
	}
	return found == 1, nil
}
