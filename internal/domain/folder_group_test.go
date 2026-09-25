package domain

import (
	"reflect"
	"testing"
)

func TestBuildFolderIndexGroups(t *testing.T) {
	roots := []MediaFolder{{ID: 1, Path: "/media"}, {ID: 2, Path: "/other/"}}
	type group struct {
		path string
		name string
		ids  []int64
	}
	cases := []struct {
		name      string
		locations []FolderIndexLocation
		overrides map[string]FolderGroupMode
		want      []group
	}{
		{
			name: "子フォルダを持たず直下に2本以上あるフォルダは深さによらずグループになる",
			locations: []FolderIndexLocation{
				{1, "/media/a/1.mp4"}, {2, "/media/a/2.mp4"},
				{3, "/media/x/y/z/10.mp4"}, {4, "/media/x/y/z/2.mp4"}, {5, "/media/x/y/z/1.mp4"},
			},
			want: []group{
				{"/media/a", "a", []int64{1, 2}},
				// 並びはファイル名の自然順（compareSiblings）。
				{"/media/x/y/z", "z", []int64{5, 4, 3}},
			},
		},
		{
			name:      "登録フォルダ直下の動画は例外が無ければグループにならない",
			locations: []FolderIndexLocation{{1, "/media/1.mp4"}, {2, "/media/2.mp4"}},
		},
		{
			name:      "登録フォルダ直下も直下をまとめるならグループになる",
			locations: []FolderIndexLocation{{1, "/media/1.mp4"}, {2, "/media/2.mp4"}, {3, "/media/sub/3.mp4"}},
			overrides: map[string]FolderGroupMode{"/media": FolderGroupDirect},
			want:      []group{{"/media", "media", []int64{1, 2}}},
		},
		{
			name:      "直下が1本だけのフォルダはグループにならない",
			locations: []FolderIndexLocation{{1, "/media/a/1.mp4"}},
		},
		{
			name:      "直下をまとめるでも1本だけならグループにならない",
			locations: []FolderIndexLocation{{1, "/media/a/1.mp4"}},
			overrides: map[string]FolderGroupMode{"/media/a": FolderGroupDirect},
		},
		{
			name: "子フォルダと動画が混在するフォルダは例外が無ければグループにならず、子はなる",
			locations: []FolderIndexLocation{
				{1, "/media/a/1.mp4"}, {2, "/media/a/2.mp4"},
				{3, "/media/a/b/3.mp4"}, {4, "/media/a/b/4.mp4"},
			},
			want: []group{{"/media/a/b", "b", []int64{3, 4}}},
		},
		{
			name: "直下をまとめると混在するフォルダも直下の動画だけでグループになる",
			locations: []FolderIndexLocation{
				{1, "/media/a/1.mp4"}, {2, "/media/a/2.mp4"},
				{3, "/media/a/b/3.mp4"}, {4, "/media/a/b/4.mp4"},
			},
			overrides: map[string]FolderGroupMode{"/media/a": FolderGroupDirect},
			want: []group{
				{"/media/a", "a", []int64{1, 2}},
				{"/media/a/b", "b", []int64{3, 4}},
			},
		},
		{
			name:      "まとめを解除したフォルダはグループにならない",
			locations: []FolderIndexLocation{{1, "/media/a/1.mp4"}, {2, "/media/a/2.mp4"}},
			overrides: map[string]FolderGroupMode{"/media/a": FolderGroupUngroup},
		},
		{
			name: "同じ内容の複数の所在は代表の所在（パスの最小）のフォルダだけで数え、1本は多くても1つのグループ",
			locations: []FolderIndexLocation{
				{1, "/media/b/1.mp4"}, {1, "/media/a/1.mp4"}, {2, "/media/a/2.mp4"},
				{3, "/media/b/3.mp4"},
			},
			// b の直下を代表にするのは 3 だけなので b はグループにならない。
			want: []group{{"/media/a", "a", []int64{1, 2}}},
		},
		{
			name: "代表でない所在も子フォルダを持つかの判定には数える",
			locations: []FolderIndexLocation{
				{1, "/media/a/1.mp4"}, {2, "/media/a/2.mp4"},
				{3, "/media/a/1/x.mp4"}, {1, "/media/z/c/1.mp4"},
			},
		},
		{
			name:      "例外のパスに一致するフォルダが無くても割り当ては変わらない",
			locations: []FolderIndexLocation{{1, "/media/a/1.mp4"}, {2, "/media/a/2.mp4"}},
			overrides: map[string]FolderGroupMode{"/media/gone": FolderGroupUngroup, "/elsewhere": FolderGroupDirect},
			want:      []group{{"/media/a", "a", []int64{1, 2}}},
		},
		{
			name: "登録フォルダの外の所在は使わない",
			locations: []FolderIndexLocation{
				{1, "/outside/a/1.mp4"}, {2, "/outside/a/2.mp4"},
				{3, "/other/q/3.mp4"}, {4, "/other/q/4.mp4"},
			},
			want: []group{{"/other/q", "q", []int64{3, 4}}},
		},
		{
			name:      "自然順で同順位の名前はバイト順で決着する",
			locations: []FolderIndexLocation{{5, "/media/b/1.mp4"}, {4, "/media/b/01.mp4"}, {6, "/media/b/A.mp4"}, {7, "/media/b/a.mp4"}},
			want:      []group{{"/media/b", "b", []int64{4, 5, 6, 7}}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			index := buildFolderIndexFor(roots, tc.locations, tc.overrides, false, '/')
			got := make([]group, 0, len(index.Groups))
			for _, g := range index.Groups {
				if g.Key != g.Path && g.Key+"/" != g.Path {
					t.Errorf("key %q does not match path %q", g.Key, g.Path)
				}
				if g.TitleKey != NaturalSortKey(g.Name) {
					t.Errorf("title key of %q = %q", g.Name, g.TitleKey)
				}
				got = append(got, group{g.Path, g.Name, g.VideoIDs})
			}
			want := tc.want
			if want == nil {
				want = []group{}
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("groups = %+v, want %+v", got, want)
			}
		})
	}
}

func TestBuildFolderIndexSwitchesOverrides(t *testing.T) {
	roots := []MediaFolder{{ID: 1, Path: "/media"}}
	locations := []FolderIndexLocation{
		{1, "/media/a/1.mp4"}, {2, "/media/a/2.mp4"}, {3, "/media/a/b/3.mp4"},
	}
	count := func(overrides map[string]FolderGroupMode) int {
		return len(buildFolderIndexFor(roots, locations, overrides, false, '/').Groups)
	}
	// 例外なし → 直下をまとめる → まとめを解除 → 例外なし、の順に切り替える。
	steps := []struct {
		mode FolderGroupMode
		want int
	}{{"", 0}, {FolderGroupDirect, 1}, {FolderGroupUngroup, 0}, {"", 0}}
	for _, step := range steps {
		overrides := map[string]FolderGroupMode{}
		if step.mode != "" {
			overrides["/media/a"] = step.mode
		}
		if got := count(overrides); got != step.want {
			t.Errorf("mode %q: groups = %d, want %d", step.mode, got, step.want)
		}
	}
}

func TestBuildFolderIndexWindowsKeys(t *testing.T) {
	roots := []MediaFolder{{ID: 1, Path: `C:\Media`}}
	locations := []FolderIndexLocation{
		{1, `C:\Media\Show\1.mp4`}, {2, `c:\media/show/2.mp4`},
	}
	index := buildFolderIndexFor(roots, locations, nil, true, '\\')
	if len(index.Groups) != 1 {
		t.Fatalf("groups = %+v", index.Groups)
	}
	g := index.Groups[0]
	if g.Key != `c:\media\show` || g.Path != `C:\Media\Show` || g.Name != "Show" || !reflect.DeepEqual(g.VideoIDs, []int64{1, 2}) {
		t.Errorf("group = %+v", g)
	}
	// 例外の鍵も同じ規則で整えたもので突き合わせる。
	index = buildFolderIndexFor(roots, locations, map[string]FolderGroupMode{
		folderKeyFor(`C:/MEDIA/SHOW/`, true, '\\'): FolderGroupUngroup,
	}, true, '\\')
	if len(index.Groups) != 0 {
		t.Errorf("ungrouped: groups = %+v", index.Groups)
	}
}

func TestBuildFolderIndexFolderNames(t *testing.T) {
	roots := []MediaFolder{{ID: 1, Path: "/media"}}
	locations := []FolderIndexLocation{
		{1, "/media/Anime/Show/1.mp4"},
		// 代表でない所在の祖先も入る。重複は除く。
		{1, "/media/Movies/Show/1.mp4"},
		{2, "/media/2.mp4"},
		// 空白だけの段や制御文字の段はタグ名にならないので捨てる。
		{3, "/media/ \t/bad\x01/ Trim /3.mp4"},
	}
	index := buildFolderIndexFor(roots, locations, nil, false, '/')
	want := map[int64][]string{
		1: {"Anime", "Movies", "Show"},
		3: {"Trim"},
	}
	if !reflect.DeepEqual(index.FolderNames, want) {
		t.Errorf("folder names = %#v, want %#v", index.FolderNames, want)
	}
}

func TestFolderKey(t *testing.T) {
	cases := []struct {
		path    string
		windows bool
		sep     rune
		want    string
	}{
		{"/media/a/", false, '/', "/media/a"},
		{"/media/A", false, '/', "/media/A"},
		{`/media/a\b`, false, '/', `/media/a\b`},
		{`C:/Media/A\`, true, '\\', `c:\media\a`},
		{`D:\`, true, '\\', `d:`},
	}
	for _, tc := range cases {
		if got := folderKeyFor(tc.path, tc.windows, tc.sep); got != tc.want {
			t.Errorf("folderKey(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

func TestLocateFolder(t *testing.T) {
	roots := []MediaFolder{{ID: 1, Path: "/media"}, {ID: 2, Path: "/"}}
	cases := []struct {
		path string
		want VideoFolder
		ok   bool
	}{
		{"/media", VideoFolder{RootID: 1, Path: ""}, true},
		{"/media/a", VideoFolder{RootID: 1, Path: "a"}, true},
		{"/media/a/b/c", VideoFolder{RootID: 1, Path: "a/b/c"}, true},
		{"/media/a/b/", VideoFolder{RootID: 1, Path: "a/b"}, true},
		{"/mediax/a", VideoFolder{RootID: 2, Path: "mediax/a"}, true},
	}
	for _, tc := range cases {
		got, ok := locateFolderFor(roots, tc.path, false, '/')
		if ok != tc.ok || got != tc.want {
			t.Errorf("locateFolder(%q) = %+v, %v; want %+v, %v", tc.path, got, ok, tc.want, tc.ok)
		}
	}
	if _, ok := locateFolderFor(roots[:1], "/elsewhere/a", false, '/'); ok {
		t.Error("folder outside every root was located")
	}
	// グループのフォルダから求めた (rootId, path) は、同じフォルダの中の所在から
	// LocateVideoFolder で求めたものと一致する。
	index := buildFolderIndexFor(roots[:1], []FolderIndexLocation{
		{1, "/media/a/b/1.mp4"}, {2, "/media/a/b/2.mp4"},
	}, nil, false, '/')
	folder, _ := locateFolderFor(roots[:1], index.Groups[0].Path, false, '/')
	video, _ := locateVideoFolderFor(roots[:1], "/media/a/b/1.mp4", false, '/')
	if folder != video || folder.Path != "a/b" {
		t.Errorf("group folder = %+v, video folder = %+v", folder, video)
	}

	winRoots := []MediaFolder{{ID: 1, Path: `C:\Media`}}
	got, ok := locateFolderFor(winRoots, `c:\media/A\B`, true, '\\')
	if !ok || got != (VideoFolder{RootID: 1, Path: "A/B"}) {
		t.Errorf("windows locateFolder = %+v, %v", got, ok)
	}
}
