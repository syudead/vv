package app

import (
	"bytes"
	"context"
	"encoding/json"
	"image"

	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// imageRunner answers images by reference; a reference it does not hold is a 404 from the operation.
type imageRunner struct {
	*fakeToolRunner
	images map[string][]byte
}

func (r imageRunner) Image(_ context.Context, reference string) (domain.AssistantImage, error) {
	if data, ok := r.images[reference]; ok {
		return domain.AssistantImage{Bytes: data, ContentType: "image/png"}, nil
	}
	return domain.AssistantImage{}, &domain.AssistantCallError{Status: 404, Code: "not_found"}
}

// fakeWire records Clef requests and answers one JSON answer.
type fakeWire struct {
	requests []domain.ClefRequest
	answer   string
}

func (w *fakeWire) SystemOne(_ context.Context, req domain.ClefRequest) (json.RawMessage, error) {
	w.requests = append(w.requests, req)
	return json.RawMessage(w.answer), nil
}

type fakeModel struct{ vision bool }

func (m fakeModel) AssistantReadiness(context.Context) (domain.AssistantStatus, error) {
	return domain.AssistantStatus{Vision: m.vision}, nil
}

// runImages runs one turn with the fixture's chat and the images the runner holds.
func (f *loopFixture) runImages(t *testing.T, images map[string][]byte) domain.AssistantTurn {
	t.Helper()
	f.a.runTurn(context.Background(), imageRunner{fakeToolRunner: f.runner, images: images}, domain.AssistantTurn{
		ID: 1, Status: domain.AssistantTurnRunning, Request: "猫が映ってる動画を探して", StartedAt: f.now,
	})
	if f.turns.finished == nil {
		t.Fatal("the turn was not finished")
	}
	return *f.turns.finished
}

const (
	frameA = "getVideoFrame?id=7&positionMs=1000"
	frameB = "getVideoFrame?id=7&positionMs=9000"
)

func TestClefSendsTheModelsRequestWithTheImagesBytes(t *testing.T) {
	args := `{"state":{"title":"Clip 7"},"images":["` + frameA + `","` + frameB + `"],` +
		`"questions":{"cat":{"type":"noul","instructions":"Is a cat visible?"}}}`
	f := newLoopFixture(t, calls(toolCall("clef", args)), text("猫が映っています。"))
	wire := &fakeWire{answer: `{"answers":{"cat":{"noul":0.94,"type":"noul"}}}`}
	f.a.clef = wire
	images := map[string][]byte{frameA: []byte("frame-a"), frameB: []byte("frame-b")}

	turn := f.runImages(t, images)

	if len(wire.requests) != 1 {
		t.Fatalf("wire calls = %d, want 1", len(wire.requests))
	}
	want := domain.ClefRequest{
		State:     map[string]string{"title": "Clip 7"},
		Images:    [][]byte{[]byte("frame-a"), []byte("frame-b")},
		Questions: map[string]domain.ClefQuestion{"cat": {Type: "noul", Instructions: "Is a cat visible?"}},
	}
	if !reflect.DeepEqual(wire.requests[0], want) {
		t.Errorf("wire request = %+v, want %+v", wire.requests[0], want)
	}
	messages := f.chat.requests[1].Messages
	if result := messages[len(messages)-1]; result.Role != domain.ChatRoleTool || result.Content != wire.answer {
		t.Errorf("tool result = %+v, want Clef's answer", result)
	}
	if !turn.Judged || turn.Status != domain.AssistantTurnDone {
		t.Errorf("turn = %+v, want a done turn marked as judged", turn)
	}
	if p := turn.Progress; len(p) != 1 || p[0].Tool != "clef" || p[0].Images != 2 {
		t.Errorf("progress = %+v, want clef with 2 images", p)
	}
}

func TestClefSendsNothingWhenAReferenceCannotBeRead(t *testing.T) {
	args := `{"images":["` + frameA + `","getVideoFrame?id=99&positionMs=0"],"questions":{"cat":{"type":"noul","instructions":"Cat?"}}}`
	f := newLoopFixture(t, calls(toolCall("clef", args)), text("確かめられませんでした。"))
	wire := &fakeWire{}
	f.a.clef = wire

	turn := f.runImages(t, map[string][]byte{frameA: []byte("frame-a")})

	if len(wire.requests) != 0 {
		t.Errorf("wire calls = %d, want none", len(wire.requests))
	}
	messages := f.chat.requests[1].Messages
	result := messages[len(messages)-1]
	if !strings.Contains(result.Content, `"error"`) || !strings.Contains(result.Content, "id=99") {
		t.Errorf("tool result = %s, want an error naming the reference", result.Content)
	}
	if turn.Judged || turn.Progress[0].Error == "" {
		t.Errorf("turn = %+v, want a failed step and no judged mark", turn)
	}
}

func TestViewImagesAttachesTheImagesForAVisionModelOnly(t *testing.T) {
	args := `{"images":["` + frameA + `","` + frameB + `"]}`
	images := map[string][]byte{frameA: pngOf(t, 2048, 1024), frameB: pngOf(t, 320, 180)}

	f := newLoopFixture(t, calls(toolCall("view_images", args)), text("猫が映っています。"))
	f.a.model = fakeModel{vision: true}
	turn := f.runImages(t, images)

	if !slices.Contains(f.chat.toolNames(0), "view_images") {
		t.Fatalf("tools = %v, want view_images for a vision model", f.chat.toolNames(0))
	}
	messages := f.chat.requests[1].Messages
	result, attached := messages[len(messages)-2], messages[len(messages)-1]
	if result.Role != domain.ChatRoleTool || result.Content != `{"attached":2}` {
		t.Errorf("tool result = %+v, want attached: 2", result)
	}
	if attached.Role != domain.ChatRoleUser || len(attached.Images) != 2 || !strings.Contains(attached.Content, frameB) {
		t.Fatalf("last message = %+v, want a user message with both images", attached)
	}
	large, _, err := image.DecodeConfig(bytes.NewReader(attached.Images[0]))
	if err != nil || large.Width != 1024 || large.Height != 512 {
		t.Errorf("first image = %dx%d (%v), want it downscaled to 1024x512", large.Width, large.Height, err)
	}
	if !bytes.Equal(attached.Images[1], images[frameB]) {
		t.Error("the small image was changed")
	}
	if p := turn.Progress; len(p) != 1 || p[0].Images != 2 {
		t.Errorf("progress = %+v, want view_images with 2 images", p)
	}

	f = newLoopFixture(t, calls(toolCall("view_images", args)), text("見られません。"))
	f.a.model = fakeModel{vision: false}
	f.runImages(t, images)
	if slices.Contains(f.chat.toolNames(0), "view_images") {
		t.Errorf("tools = %v, want no view_images without vision", f.chat.toolNames(0))
	}
	if last := f.chat.requests[1].Messages; len(last[len(last)-1].Images) != 0 {
		t.Error("images were attached for a model without vision")
	}
}

// Some models write nested arguments as JSON strings; clef reads them as the values they hold.
func TestClefReadsArgumentsWrittenAsJSONStrings(t *testing.T) {
	args := `{"images":"[\"` + frameA + `\"]","questions":"{\"cat\":{\"type\":\"noul\",\"instructions\":\"Cat?\"}}","state":"{}"}`
	f := newLoopFixture(t, calls(toolCall("clef", args)), text("猫が映っています。"))
	wire := &fakeWire{answer: `{"answers":{}}`}
	f.a.clef = wire

	f.runImages(t, map[string][]byte{frameA: []byte("frame-a")})

	if len(wire.requests) != 1 || len(wire.requests[0].Images) != 1 || wire.requests[0].Questions["cat"].Instructions != "Cat?" {
		t.Fatalf("wire requests = %+v, want the decoded image and question", wire.requests)
	}
}
