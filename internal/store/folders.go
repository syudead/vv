package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/syudead/vv/internal/domain"
)

// Trim only OS separators, preserving Unix backslashes; on Windows fold ASCII only to match SQLite lower().
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

// Combine instr with a prefix range so SQLite can use the location-path index instead of a full scan.
func folderPrefixCondition(alias, prefix string) (string, []any) {
	expr := folderPathExpr(alias)
	upper := prefix[:len(prefix)-1] + string(prefix[len(prefix)-1]+1)
	return `(` + expr + ` >= ? and ` + expr + ` < ? and instr(` + expr + `, ?) = 1)`, []any{prefix, upper, prefix}
}

func folderPathExpr(alias string) string {
	return folderPathExprFor(alias, runtime.GOOS == "windows")
}

func folderPathExprFor(alias string, windows bool) string {
	if windows {
		return `lower(` + alias + `.path)`
	}
	return alias + `.path`
}

// Use one prefix placeholder per direct-child predicate.
func directChildCondition(alias string) string {
	return directChildConditionFor(alias, runtime.GOOS == "windows")
}

func directChildConditionFor(alias string, windows bool) string {
	rest := `substr(` + folderPathExprFor(alias, windows) + `, length(?) + 1)`
	if windows {
		// Windows accepts both slash styles, so normalize them before checking for another separator.
		return `instr(replace(` + rest + `, char(47), char(92)), char(92)) = 0`
	}
	return `instr(` + rest + `, char(` + strconv.Itoa(int(os.PathSeparator)) + `)) = 0`
}

// FolderLocations exposes only audience-visible representative locations; nonrepresentative-only folders appear empty.
func (s *LibraryStore) FolderLocations(ctx context.Context, audience domain.Audience, dir string) ([]domain.FolderLocation, error) {
	inFolder, args := folderPrefixCondition("l", folderPrefix(dir))
	rows, err := s.db.sql.QueryContext(ctx, `select l.path, l.video_id, videos.content_key, videos.thumbnail_state, videos.preview_state,
			(select ov.thumbnail_position_ms from video_overrides ov
				where ov.content_key = videos.content_key and videos.content_key <> ''),
			(select ov.thumbnail_revision from video_overrides ov
				where ov.content_key = videos.content_key and videos.content_key <> ''),
			(select ti.revision from thumbnail_images ti
				where ti.content_key = videos.content_key and videos.content_key <> ''),
			`+buildColumns+`
		from video_locations l join videos on videos.id = l.video_id
		where `+inFolder+` and `+visibleLocationCondition("l", audience)+
		` and `+shownVideoCondition("videos", audience),
		args...)
	if err != nil {
		return nil, fmt.Errorf("cannot read the folder contents: %w", err)
	}
	defer func() { _ = rows.Close() }()

	locations := []domain.FolderLocation{}
	for rows.Next() {
		var item domain.FolderLocation
		var thumbnail, preview string
		var position, revision, imageRevision sql.NullInt64
		if err := rows.Scan(&item.Path, &item.VideoID, &item.ContentKey, &thumbnail, &preview, &position, &revision,
			&imageRevision, &item.Builds.Thumbnail, &item.Builds.SeekThumbnail, &item.Builds.Preview); err != nil {
			return nil, fmt.Errorf("cannot read the folder contents: %w", err)
		}
		item.ThumbnailState = domain.ThumbnailState(thumbnail)
		item.PreviewState = domain.PreviewState(preview)
		if position.Valid {
			ms := position.Int64
			item.ThumbnailPositionMs = &ms
			item.ThumbnailRevision = revision.Int64
		}
		item.ThumbnailImageRevision = imageRevision.Int64
		locations = append(locations, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the folder contents: %w", err)
	}
	return locations, nil
}

// HasFolderLocations uses the same visibility and descendant scope as FolderLocations.
func (s *LibraryStore) HasFolderLocations(ctx context.Context, audience domain.Audience, dir string) (bool, error) {
	var found int
	inFolder, args := folderPrefixCondition("l", folderPrefix(dir))
	err := s.db.sql.QueryRowContext(ctx, `select exists (select 1 from video_locations l
		join videos on videos.id = l.video_id
		where `+inFolder+` and `+visibleLocationCondition("l", audience)+
		` and `+shownVideoCondition("videos", audience)+`)`,
		args...).Scan(&found)
	if err != nil {
		return false, fmt.Errorf("cannot check whether the folder exists: %w", err)
	}
	return found == 1, nil
}
