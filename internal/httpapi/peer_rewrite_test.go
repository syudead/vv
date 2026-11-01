package httpapi

import (
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// The two-node tests cover the fields a scanned video carries; this covers the references they lack.
func TestRewriteRemoteVideoMapsEveryReference(t *testing.T) {
	r := domain.ToRemoteID
	video := gen.Video{
		Id:          5,
		Group:       &gen.VideoGroupRef{Id: 6, Folder: &gen.VideoFolder{RootId: 2}},
		SmartFolder: &gen.VideoSmartFolderRef{Id: 7},
		Versions:    &gen.VideoVersionsRef{RepresentativeId: 8},
		Location:    &gen.VideoLocation{Openable: true},
		Tags:        []gen.VideoTag{{Id: 10, Name: "海", Kind: &gen.TagKindRef{Id: 11, Name: "場所"}}},
		CardCredits: &gen.VideoCardCredits{
			Performers:   []gen.WorkPerson{{Person: gen.CreditRef{Id: 20, Name: "山田"}}},
			Organization: &gen.CreditRef{Id: 30, Name: "スタジオ"},
		},
		WorkInfo: &gen.WorkInfo{
			People:        []gen.WorkPerson{{Person: gen.CreditRef{Id: 21, Name: "佐藤"}}},
			Organizations: []gen.WorkOrganization{{Organization: gen.CreditRef{Id: 31, Name: "レーベル"}}},
		},
	}
	join := domain.VocabularyJoin{Tags: map[string]int64{"海": 3}, People: map[string]int64{"山田": 4}}

	rewriteRemoteVideo(&video, join)

	for _, tc := range []struct {
		label     string
		got, want int64
	}{
		{"group", video.Group.Id, r(6)},
		{"group.folder.rootId", video.Group.Folder.RootId, r(2)},
		{"smartFolder", video.SmartFolder.Id, r(7)},
		{"versions.representativeId", video.Versions.RepresentativeId, r(8)},
		{"同じ名前のタグ", video.Tags[0].Id, 3},
		{"手元に無いタグの種別", video.Tags[0].Kind.Id, r(11)},
		{"同じ名前の出演者", video.CardCredits.Performers[0].Person.Id, 4},
		{"カードの組織", video.CardCredits.Organization.Id, r(30)},
		{"作品情報の人物", video.WorkInfo.People[0].Person.Id, r(21)},
		{"作品情報の組織", video.WorkInfo.Organizations[0].Organization.Id, r(31)},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %d, want %d", tc.label, tc.got, tc.want)
		}
	}
	if video.Location.Openable {
		t.Error("リモートの動画が開ける")
	}
}

// A same-named person takes the local id, and each appearance keeps the peer's hasCrop, whose crop is forwarded.
func TestRewriteRemoteFaceAppearancesJoinsPeopleAndKeepsCrops(t *testing.T) {
	list := gen.VideoFaceAppearanceList{Analysed: true, Items: []gen.VideoFaceAppearance{
		{AppearanceId: 7, Person: gen.CreditRef{Id: 20, Name: "山田"}, HasCrop: true,
			Spans: []gen.FaceAppearanceSpan{{Start: 5, End: 8}}},
		{AppearanceId: 8, Person: gen.CreditRef{Id: 21, Provisional: true}},
	}}

	rewriteRemoteFaceAppearances(&list, domain.VocabularyJoin{People: map[string]int64{"山田": 4}})

	r := domain.ToRemoteID
	for i, want := range []struct {
		appearance, person int64
		crop               bool
	}{{r(7), 4, true}, {r(8), r(21), false}} {
		item := list.Items[i]
		if item.AppearanceId != want.appearance || item.Person.Id != want.person || item.HasCrop != want.crop {
			t.Errorf("items[%d] = %+v, want appearance %d, person %d, hasCrop %t", i, item, want.appearance, want.person, want.crop)
		}
	}
	if spans := list.Items[0].Spans; len(spans) != 1 || spans[0] != (gen.FaceAppearanceSpan{Start: 5, End: 8}) {
		t.Errorf("spans = %+v, want the peer's", spans)
	}
}
