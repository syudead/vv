package domain

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestValidateGroupName(t *testing.T) {
	for name, c := range map[string]struct {
		input   string
		want    string
		problem GroupNameProblem
	}{
		"前後の空白を削る":    {input: "  Season 1　", want: "Season 1"},
		"100 文字は通る":   {input: strings.Repeat("あ", 100), want: strings.Repeat("あ", 100)},
		"空白だけは空":      {input: " \t ", problem: GroupNameEmpty},
		"空":           {input: "", problem: GroupNameEmpty},
		"101 文字は長すぎる": {input: " " + strings.Repeat("a", 101) + " ", problem: GroupNameTooLong},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := ValidateGroupName(c.input)
			if c.problem == 0 {
				if err != nil || got != c.want {
					t.Fatalf("ValidateGroupName(%q) = %q, %v; want %q", c.input, got, err, c.want)
				}
				return
			}
			var invalid *InvalidGroupNameError
			if !errors.As(err, &invalid) || invalid.Problem != c.problem || !errors.Is(err, ErrInvalidGroupName) {
				t.Fatalf("ValidateGroupName(%q) error = %v; want problem %d", c.input, err, c.problem)
			}
		})
	}
}

// 同期の表の各行。
func TestPlanFolderGroupSync(t *testing.T) {
	long := strings.Repeat("長", 120)
	for name, c := range map[string]struct {
		folders   []FolderSyncFolder
		groups    []AutoGroup
		overrides map[string]FolderGroupMode
		want      FolderGroupSync
	}{
		"規則がグループにする新しいフォルダにグループを作る": {
			folders: []FolderSyncFolder{
				{Key: "/m/b", Name: "b", Grouped: true, ContentKeys: []string{"b1", "b2"}},
				{Key: "/m/a", Name: "a", Grouped: true, ContentKeys: []string{"a1", "a2"}},
				{Key: "/m/one", Name: "one", ContentKeys: []string{"o1"}},
			},
			want: FolderGroupSync{Create: []FolderGroupCreate{
				{OriginFolder: "/m/a", Name: "a", ContentKeys: []string{"a1", "a2"}},
				{OriginFolder: "/m/b", Name: "b", ContentKeys: []string{"b1", "b2"}},
			}},
		},
		"フォルダ名は 100 文字で切る": {
			folders: []FolderSyncFolder{{Key: "/m/l", Name: long, Grouped: true, ContentKeys: []string{"x", "y"}}},
			want: FolderGroupSync{Create: []FolderGroupCreate{
				{OriginFolder: "/m/l", Name: strings.Repeat("長", 100), ContentKeys: []string{"x", "y"}},
			}},
		},
		"ungroup のフォルダには作らない": {
			folders:   []FolderSyncFolder{{Key: "/m/a", Name: "a", Grouped: true, ContentKeys: []string{"a1", "a2"}}},
			overrides: map[string]FolderGroupMode{"/m/a": FolderGroupUngroup},
		},
		"グループのあるフォルダには作らず、新しい直下の動画を自然順で末尾に足す": {
			folders: []FolderSyncFolder{{Key: "/m/a", Name: "a", Grouped: true, ContentKeys: []string{"a1", "a2", "a3", "a4"}}},
			groups: []AutoGroup{{ID: 7, OriginFolder: "/m/a", Items: []AutoGroupItem{
				{ContentKey: "a2", FromFolder: true}, {ContentKey: "a1", FromFolder: true},
			}}},
			want: FolderGroupSync{Change: []FolderGroupChange{{GroupID: 7, Append: []string{"a3", "a4"}}}},
		},
		"除外と手で足した項目はそのまま": {
			folders: []FolderSyncFolder{{Key: "/m/a", Name: "a", Grouped: true, ContentKeys: []string{"a1", "a2"}}},
			groups: []AutoGroup{{ID: 7, OriginFolder: "/m/a", Items: []AutoGroupItem{
				{ContentKey: "a1", FromFolder: true}, {ContentKey: "other", FromFolder: false},
			}, Exclusions: []string{"a2"}}},
		},
		"直下でなくなったフォルダ由来の項目を外し、手で足した項目は残す": {
			folders: []FolderSyncFolder{{Key: "/m/a", Name: "a", Grouped: true, ContentKeys: []string{"a1", "a3"}}},
			groups: []AutoGroup{{ID: 7, OriginFolder: "/m/a", Items: []AutoGroupItem{
				{ContentKey: "a1", FromFolder: true}, {ContentKey: "a2", FromFolder: true}, {ContentKey: "elsewhere", FromFolder: false},
			}}},
			want: FolderGroupSync{Change: []FolderGroupChange{{GroupID: 7, Remove: []string{"a2"}, Append: []string{"a3"}}}},
		},
		"規則がもうグループにしないフォルダもグループは残り、足し引きは続く": {
			folders: []FolderSyncFolder{{Key: "/m/a", Name: "a", ContentKeys: []string{"a1"}}},
			groups: []AutoGroup{{ID: 7, OriginFolder: "/m/a", Items: []AutoGroupItem{
				{ContentKey: "a1", FromFolder: true}, {ContentKey: "a2", FromFolder: true},
			}}},
			want: FolderGroupSync{Change: []FolderGroupChange{{GroupID: 7, Remove: []string{"a2"}}}},
		},
		"フォルダが無くなるとフォルダ由来の項目をすべて外す": {
			groups: []AutoGroup{
				{ID: 9, OriginFolder: "/m/gone", Items: []AutoGroupItem{{ContentKey: "g1", FromFolder: true}, {ContentKey: "hand"}}},
				{ID: 3, OriginFolder: "/m/gone2", Items: []AutoGroupItem{{ContentKey: "h1", FromFolder: true}}},
				{ID: 5, OriginFolder: "/m/empty"},
			},
			want: FolderGroupSync{Change: []FolderGroupChange{
				{GroupID: 3, Remove: []string{"h1"}},
				{GroupID: 9, Remove: []string{"g1"}},
			}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			got := PlanFolderGroupSync(c.folders, c.groups, c.overrides)
			want := c.want
			if want.Create == nil {
				want.Create = []FolderGroupCreate{}
			}
			if want.Change == nil {
				want.Change = []FolderGroupChange{}
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("PlanFolderGroupSync = %+v\nwant %+v", got, want)
			}
		})
	}
}

// 索引の Folders は、規則がグループにしないフォルダも含め、所在のどれかが直下にある動画を
// ファイル名の自然順で持つ。
func TestBuildFolderIndexFolders(t *testing.T) {
	roots := []MediaFolder{{ID: 1, Path: "/m"}}
	locations := []FolderIndexLocation{
		{VideoID: 1, Path: "/m/Show/10.mp4"},
		{VideoID: 2, Path: "/m/Show/2.mp4"},
		{VideoID: 3, Path: "/m/One/a.mp4"},
		// 動画 2 は別のフォルダにも所在を持つ。
		{VideoID: 2, Path: "/m/Zed/b.mp4"},
		{VideoID: 4, Path: "/m/loose.mp4"},
	}
	index := buildFolderIndexFor(roots, locations, nil, false, '/')
	want := []FolderVideos{
		{Key: "/m", Name: "m", VideoIDs: []int64{4}},
		{Key: "/m/One", Name: "One", VideoIDs: []int64{3}},
		{Key: "/m/Show", Name: "Show", Grouped: true, VideoIDs: []int64{2, 1}},
		{Key: "/m/Zed", Name: "Zed", VideoIDs: []int64{2}},
	}
	if !reflect.DeepEqual(index.Folders, want) {
		t.Fatalf("Folders = %+v\nwant %+v", index.Folders, want)
	}
}
