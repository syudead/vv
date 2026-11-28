package foldcheck_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/syudead/vv/internal/domain"
)

// The SPA's tests read the same files, so a rule changed on one side fails the other side's tests.

func readSharedFixture(t *testing.T, name string, into any) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, into); err != nil {
		t.Fatal(err)
	}
}

type nameRuleFile struct {
	MaxLength int            `json:"maxLength"`
	Cases     []nameRuleCase `json:"cases"`
}

// nameRuleCase has Want when the name is accepted and Problem (empty, controlCharacters or tooLong) otherwise.
type nameRuleCase struct {
	Note    string `json:"note"`
	Input   string `json:"input"`
	Want    string `json:"want"`
	Problem string `json:"problem"`
}

// checkNameRule runs normalize, which reports the fixture's problem name or "" with the normalized name.
func checkNameRule(t *testing.T, fixture string, maxLength int, normalize func(string) (string, string)) {
	t.Helper()
	var file nameRuleFile
	readSharedFixture(t, fixture, &file)
	if file.MaxLength != maxLength {
		t.Errorf("%s maxLength = %d, want %d", fixture, file.MaxLength, maxLength)
	}
	if len(file.Cases) == 0 {
		t.Fatal("no cases")
	}
	for _, c := range file.Cases {
		got, problem := normalize(c.Input)
		if problem != c.Problem || got != c.Want {
			t.Errorf("%s: normalize(%q) = %q, problem %q; want %q, problem %q", c.Note, c.Input, got, problem, c.Want, c.Problem)
		}
	}
}

func TestTagNameSharedCases(t *testing.T) {
	problems := map[domain.TagNameProblem]string{
		domain.TagNameEmpty:             "empty",
		domain.TagNameControlCharacters: "controlCharacters",
		domain.TagNameTooLong:           "tooLong",
	}
	checkNameRule(t, "tag_name.json", domain.TagNameMaxLength, func(input string) (string, string) {
		name, err := domain.NormalizeTagName(input)
		var invalid *domain.InvalidTagNameError
		if errors.As(err, &invalid) {
			return name, problems[invalid.Problem]
		}
		return name, errorText(err)
	})
}

func TestCreditNameSharedCases(t *testing.T) {
	problems := map[domain.WorkInfoProblem]string{
		domain.WorkInfoEmpty:             "empty",
		domain.WorkInfoControlCharacters: "controlCharacters",
		domain.WorkInfoTooLong:           "tooLong",
	}
	checkNameRule(t, "credit_name.json", domain.CreditNameMaxLength, func(input string) (string, string) {
		name, err := domain.NormalizeCreditName(input)
		var invalid *domain.InvalidWorkInfoError
		if errors.As(err, &invalid) {
			return name, problems[invalid.Problem]
		}
		return name, errorText(err)
	})
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func TestRemoteIDSharedCases(t *testing.T) {
	var file struct {
		Offset int64 `json:"offset"`
		Cases  []struct {
			Note   string `json:"note"`
			ID     int64  `json:"id"`
			Remote bool   `json:"remote"`
			Slot   int64  `json:"slot"`
		} `json:"cases"`
	}
	readSharedFixture(t, "remote_id.json", &file)
	if got := domain.RemoteID(1, 0); got != file.Offset {
		t.Errorf("RemoteID(1, 0) = %d, want offset %d", got, file.Offset)
	}
	if len(file.Cases) == 0 {
		t.Fatal("no cases")
	}
	for _, c := range file.Cases {
		if got := domain.IsRemoteID(c.ID); got != c.Remote {
			t.Errorf("%s: IsRemoteID(%d) = %v, want %v", c.Note, c.ID, got, c.Remote)
		}
		if got := domain.RemoteSlot(c.ID); got != c.Slot {
			t.Errorf("%s: RemoteSlot(%d) = %d, want %d", c.Note, c.ID, got, c.Slot)
		}
	}
}

func TestWatchStateSharedCases(t *testing.T) {
	var file struct {
		Cases []struct {
			Note     string `json:"note"`
			Progress *struct {
				PositionMs int64 `json:"positionMs"`
				Completed  bool  `json:"completed"`
			} `json:"progress"`
			Want domain.WatchState `json:"want"`
		} `json:"cases"`
	}
	readSharedFixture(t, "watch_state.json", &file)
	if len(file.Cases) == 0 {
		t.Fatal("no cases")
	}
	for _, c := range file.Cases {
		var progress *domain.Progress
		if c.Progress != nil {
			progress = &domain.Progress{PositionMs: c.Progress.PositionMs, Completed: c.Progress.Completed}
		}
		if got := domain.ClassifyWatch(progress); got != c.Want {
			t.Errorf("%s: ClassifyWatch = %q, want %q", c.Note, got, c.Want)
		}
	}
}

// The SPA has no generated constant for the batch limit, so the fixture ties it to the schema and the domain.
func TestSharedLimitsMatchSchema(t *testing.T) {
	var limits struct {
		MaxTagBatch int `json:"maxTagBatch"`
	}
	readSharedFixture(t, "limits.json", &limits)
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "api", "openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Components struct {
			Schemas struct {
				TagBatchRequest struct {
					Properties struct {
						IDs struct {
							MaxItems int `yaml:"maxItems"`
						} `yaml:"ids"`
					} `yaml:"properties"`
				} `yaml:"TagBatchRequest"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(data, &spec); err != nil {
		t.Fatal(err)
	}
	schema := spec.Components.Schemas.TagBatchRequest.Properties.IDs.MaxItems
	if limits.MaxTagBatch != domain.MaxTagBatch || schema != domain.MaxTagBatch {
		t.Errorf("maxTagBatch fixture %d, TagBatchRequest.ids.maxItems %d, domain.MaxTagBatch %d", limits.MaxTagBatch, schema, domain.MaxTagBatch)
	}
}
