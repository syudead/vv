package domain

import (
	"errors"
	"reflect"
	"testing"
)

func TestNormalizeVideoEditNamesTheFailingField(t *testing.T) {
	cases := []struct {
		name    string
		edit    VideoEdit
		field   string
		problem WorkInfoProblem
	}{
		{"tag with both ID and name", VideoEdit{TagsAdd: []TagInput{{ID: new(int64(1)), Name: new("a")}}},
			"tags.add[0]", WorkInfoOutOfRange},
		{"repeated removal", VideoEdit{TagsRemove: []int64{3, 3}}, "tags.remove[1]", WorkInfoDuplicate},
		{"tag added and removed", VideoEdit{TagsAdd: []TagInput{{ID: new(int64(3))}}, TagsRemove: []int64{2, 3}},
			"tags.remove[1]", WorkInfoDuplicate},
		{"credit edit", VideoEdit{Credits: []CreditEdit{personEdit(PersonPerformer, nil)}}, "credits[0]", WorkInfoEmpty},
		{"repeated date kind", VideoEdit{Dates: []DateEdit{{Kind: DateRelease}, {Kind: DateRelease, Date: new("2020")}}},
			"dates[1].kind", WorkInfoDuplicate},
		{"invalid date", VideoEdit{Dates: []DateEdit{{Kind: DateRelease, Date: new("2020-13")}}}, "dates[0].date", WorkInfoInvalidDate},
		{"blank date", VideoEdit{Dates: []DateEdit{{Kind: DateRecorded, Date: new(" ")}}}, "dates[0].date", WorkInfoInvalidDate},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var invalid *InvalidWorkInfoError
			_, err := NormalizeVideoEdit(tc.edit)
			if !errors.As(err, &invalid) || invalid.Field != tc.field || invalid.Problem != tc.problem {
				t.Fatalf("err = %v, want problem %d at %s", err, tc.problem, tc.field)
			}
		})
	}
}

func TestNormalizeVideoEditRejectsAnEditWithoutChanges(t *testing.T) {
	edit := VideoEdit{TagsAdd: []TagInput{}, TagsRemove: []int64{}, Credits: []CreditEdit{}, Dates: []DateEdit{}}
	if _, err := NormalizeVideoEdit(edit); !errors.Is(err, ErrNoVideoEdit) {
		t.Fatalf("err = %v, want ErrNoVideoEdit", err)
	}
	if _, err := NormalizeVideoEdit(VideoEdit{Public: new(false)}); err != nil {
		t.Fatalf("visibility only: err = %v, want nil", err)
	}
}

func TestNormalizeVideoEditReportsTheTagNameIndex(t *testing.T) {
	_, err := NormalizeVideoEdit(VideoEdit{TagsAdd: []TagInput{{Name: new("ok")}, {Name: new(" ")}}})
	var at *TagNameAtError
	if !errors.As(err, &at) || at.Index != 1 || !errors.Is(err, ErrInvalidTagName) {
		t.Fatalf("err = %v, want an invalid tag name at index 1", err)
	}
}

func TestVideoEditApplyToWorkInfoSetsAndClearsDates(t *testing.T) {
	edit, err := NormalizeVideoEdit(VideoEdit{Dates: []DateEdit{
		{Kind: DatePublished, Date: new(" 2021-02 ")}, {Kind: DateRelease},
	}})
	if err != nil {
		t.Fatal(err)
	}
	info := WorkInfo{Code: "X", Dates: []WorkDate{{Kind: DateRelease, Date: "2020"}, {Kind: DateRecorded, Date: "2019"}}}
	got, err := edit.ApplyToWorkInfo(info)
	if err != nil {
		t.Fatal(err)
	}
	want := []WorkDate{{Kind: DateRecorded, Date: "2019"}, {Kind: DatePublished, Date: "2021-02"}}
	if !reflect.DeepEqual(got.Dates, want) || got.Code != "X" {
		t.Errorf("got %+v, want dates %+v and the code kept", got, want)
	}
}
