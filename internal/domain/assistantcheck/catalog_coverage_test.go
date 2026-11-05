// Package assistantcheck_test checks the assistant's classification against the tool registry generated from
// api/openapi.yaml; it lives outside domain so the file read does not violate depguard.
package assistantcheck_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// registryOperation is the part of internal/httpapi/gen/assistant_tools.gen.json the checks read.
type registryOperation struct {
	ID       string `json:"operationId"`
	Method   string `json:"method"`
	Path     string `json:"path"`
	Owner    bool   `json:"owner"`
	Response string `json:"response"`
	Secret   bool   `json:"secret"`
	IDs      []struct {
		In    string `json:"in"`
		Field string `json:"field"`
		Kind  string `json:"kind"`
	} `json:"ids"`
}

func loadRegistry(t *testing.T) []registryOperation {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "httpapi", "gen", "assistant_tools.gen.json"))
	if err != nil {
		t.Fatalf("生成した道具の一覧を読めない: %v", err)
	}
	var file struct {
		Operations []registryOperation `json:"operations"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatalf("生成した道具の一覧を解釈できない: %v", err)
	}
	if len(file.Operations) < 200 {
		t.Fatalf("生成した道具の一覧に操作が %d 件しかない", len(file.Operations))
	}
	return file.Operations
}

// registryProblems reports every way the registry and the classification disagree; an empty list means every
// owner operation is a read, a write or an exclusion, exactly once, and the reads and writes are safe to offer.
func registryProblems(operations []registryOperation) []string {
	var problems []string
	known := map[string]bool{}
	for _, op := range operations {
		known[op.ID] = true
		if !op.Owner {
			continue
		}
		class, err := domain.ClassifyAssistantOperation(op.ID, op.Method, op.Path)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s (%s %s): %v", op.ID, op.Method, op.Path, err))
			continue
		}
		problems = append(problems, toolProblems(op, class)...)
	}
	for _, operation := range domain.AssistantListedOperations() {
		if !known[operation] {
			problems = append(problems, operation+" is listed but is not in api/openapi.yaml")
		}
	}
	return problems
}

func toolProblems(op registryOperation, class domain.AssistantClassification) []string {
	var problems []string
	if class.Kind == domain.AssistantToolRead && op.Secret {
		problems = append(problems, op.ID+" answers a credential but is a read tool")
	}
	if class.Kind == domain.AssistantToolRead && op.Response == "media" {
		problems = append(problems, op.ID+" answers a media stream but is a read tool")
	}
	if class.Kind != domain.AssistantToolWrite || !class.Row.Schema {
		return problems
	}
	for _, id := range op.IDs {
		if !domain.AssistantLabelsEntity(id.Kind) {
			problems = append(problems, fmt.Sprintf("%s: the %s id %s has the kind %q, which a card cannot label",
				op.ID, id.In, id.Field, id.Kind))
		}
	}
	return problems
}

func TestAssistantClassifiesEveryOwnerOperation(t *testing.T) {
	for _, problem := range registryProblems(loadRegistry(t)) {
		t.Error(problem)
	}
}

// Each copy stands for a spec change the check must catch before the assistant gains or loses a tool unnoticed.
func TestAssistantClassificationCheckCatchesSpecChanges(t *testing.T) {
	cases := []struct {
		name   string
		change func([]registryOperation) []registryOperation
		want   string
	}{
		{"どの一覧にもない書き込み", func(ops []registryOperation) []registryOperation {
			return append(ops, registryOperation{ID: "frobnicateVideo", Method: "POST", Path: "/api/frobnicate", Owner: true})
		}, "frobnicateVideo"},
		{"書き込みの行と除外の両方にある GET", func(ops []registryOperation) []registryOperation {
			return setOperation(ops, "renameTag", func(op *registryOperation) { op.Method = "GET" })
		}, "renameTag"},
		{"資格情報を返す読み取り", func(ops []registryOperation) []registryOperation {
			return setOperation(ops, "getOllamaSettings", func(op *registryOperation) { op.Secret = true })
		}, "getOllamaSettings answers a credential"},
		{"注釈を外した targetId", func(ops []registryOperation) []registryOperation {
			return setOperation(ops, "moveFaceAppearances", func(op *registryOperation) {
				for i := range op.IDs {
					if op.IDs[i].Field == "/targetId" {
						op.IDs[i].Kind = ""
					}
				}
			})
		}, "/targetId"},
		{"スペックから消えた操作", func(ops []registryOperation) []registryOperation {
			return slices.DeleteFunc(ops, func(op registryOperation) bool { return op.ID == "undoActivity" })
		}, "undoActivity is listed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			problems := registryProblems(tc.change(loadRegistry(t)))
			if !slices.ContainsFunc(problems, func(p string) bool { return strings.Contains(p, tc.want) }) {
				t.Errorf("問題 %v に %q が無い", problems, tc.want)
			}
		})
	}
}

func setOperation(ops []registryOperation, id string, change func(*registryOperation)) []registryOperation {
	for i := range ops {
		if ops[i].ID == id {
			change(&ops[i])
		}
	}
	return ops
}

func TestAssistantExcludesTheProxySettingsForTheirCredentials(t *testing.T) {
	for _, op := range loadRegistry(t) {
		if op.ID != "getPluginProxySettings" {
			continue
		}
		class, err := domain.ClassifyAssistantOperation(op.ID, op.Method, op.Path)
		if !op.Secret || err != nil || class.Kind != domain.AssistantToolExcluded {
			t.Fatalf("secret %v, %+v, %v: want 資格情報の印と対象外", op.Secret, class, err)
		}
		if class.Exclusion.Reason == "" || class.Exclusion.Page != "/settings/network" {
			t.Errorf("除外 = %+v, want 理由とネットワーク設定のページ", class.Exclusion)
		}
		return
	}
	t.Fatal("getPluginProxySettings が一覧に無い")
}

func TestAssistantRegistryLabelsTheIDsOfAFaceMove(t *testing.T) {
	for _, op := range loadRegistry(t) {
		if op.ID != "moveFaceAppearances" {
			continue
		}
		got := map[string]string{}
		for _, id := range op.IDs {
			got[id.In+" "+id.Field] = id.Kind
		}
		want := map[string]string{"path id": "person", "body /targetId": "person", "body /appearanceIds": "faceAppearance"}
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("id の種類 = %v, want %v", got, want)
		}
		return
	}
	t.Fatal("moveFaceAppearances が一覧に無い")
}
