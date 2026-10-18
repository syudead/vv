package comfyui

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func TestWorkflowsHaveTheTitlesVVFills(t *testing.T) {
	titles := func(prompt string, reference bool) map[string]string {
		m := map[string]string{
			"vv:frame":  "LoadImage",
			"vv:prompt": prompt,
			"vv:mask":   "LoadImageMask",
			"vv:output": "SaveImage",
		}
		if reference {
			m["vv:reference"] = "LoadImage"
		}
		return m
	}
	cases := map[string]map[string]string{
		workflowEdit:              titles("CLIPTextEncode", false),
		workflowEditReference:     titles("CLIPTextEncode", true),
		workflowQwenEdit:          titles("TextEncodeQwenImageEditPlus", false),
		workflowQwenEditReference: titles("TextEncodeQwenImageEditPlus", true),
	}
	for _, name := range workflowNames {
		w, err := loadWorkflow(name)
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]string{}
		for id, node := range w {
			if !strings.HasPrefix(node.Meta.Title, "vv:") {
				continue
			}
			if _, dup := got[node.Meta.Title]; dup {
				t.Errorf("%s: %s appears twice (node %s)", name, node.Meta.Title, id)
			}
			got[node.Meta.Title] = node.ClassType
		}
		if want := cases[name]; !reflect.DeepEqual(got, want) {
			t.Errorf("%s: vv:* titles = %v, want %v", name, got, want)
		}
	}
}

func TestWorkflowLoadersNameTheRequiredFiles(t *testing.T) {
	required, err := workflowRequirement(domain.ComfyUIModelFlux2Klein, "")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range required.files {
		names = append(names, f.file.name)
	}
	want := []string{"flux-2-klein-4b-fp8.safetensors", "qwen_3_4b.safetensors", "flux2-vae.safetensors"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("files = %v, want %v", names, want)
	}
	for _, node := range []string{"Flux2Scheduler", "GetImageSize", "LoadImage", "SaveImage"} {
		if !contains(required.nodes, node) {
			t.Errorf("nodes lack %s: %v", node, required.nodes)
		}
	}
}

func TestWorkflowRequirementWithDiffusionModel(t *testing.T) {
	required, err := workflowRequirement(domain.ComfyUIModelFlux2Klein, "klein-4b-finetune.safetensors")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range required.files {
		names = append(names, f.file.name)
	}
	want := []string{"klein-4b-finetune.safetensors", "qwen_3_4b.safetensors", "flux2-vae.safetensors"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("files = %v, want %v", names, want)
	}
	if got := required.files[0].file; got.folder != "diffusion_models" || got.source != "" {
		t.Errorf("replaced file = %+v", got)
	}
	if requiredFiles[0].name != "flux-2-klein-4b-fp8.safetensors" {
		t.Errorf("requiredFiles was modified: %+v", requiredFiles[0])
	}
}

func TestEveryModelHasAPresetWhoseLoadersNameItsFiles(t *testing.T) {
	for _, model := range domain.ComfyUIModels {
		p, ok := presets[model]
		if !ok {
			t.Fatalf("%s has no preset", model)
		}
		required, err := workflowRequirement(model, "")
		if err != nil {
			t.Fatalf("%s: %v", model, err)
		}
		if len(required.files) != len(p.files) {
			t.Errorf("%s: the workflows load %d files, the table has %d", model, len(required.files), len(p.files))
		}
		if !contains(workflowNames, p.edit) || !contains(workflowNames, p.editReference) {
			t.Errorf("%s: workflows %s and %s are not all in workflowNames", model, p.edit, p.editReference)
		}
	}
	required, err := workflowRequirement(domain.ComfyUIModelQwenImageEdit, "")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range required.files {
		names = append(names, f.file.name)
	}
	want := []string{
		"qwen_image_edit_2511_fp8mixed.safetensors", "qwen_2.5_vl_7b_fp8_scaled.safetensors",
		"qwen_image_vae.safetensors", "Qwen-Image-Edit-2511-Lightning-4steps-V1.0-bf16.safetensors",
	}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("Qwen files = %v, want %v", names, want)
	}
	if !contains(required.nodes, "TextEncodeQwenImageEditPlus") || !contains(required.nodes, "LoraLoaderModelOnly") {
		t.Errorf("Qwen nodes = %v", required.nodes)
	}
}

func TestCheckUsesTheSavedModel(t *testing.T) {
	var files []string
	for _, f := range qwenRequiredFiles {
		files = append(files, f.name)
	}
	required, err := workflowRequirement(domain.ComfyUIModelQwenImageEdit, "")
	if err != nil {
		t.Fatal(err)
	}
	info := map[string]any{}
	for _, node := range required.nodes {
		inputs := map[string]any{}
		if input, ok := modelLoaders[node]; ok {
			inputs[input] = []any{files, map[string]any{}}
		}
		info[node] = map[string]any{"input": map[string]any{"required": inputs}}
	}
	data, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	url := fakeProvider(t, "/system_stats", systemStatsReply, "", data, nil)
	qwen := fixedSettings{settings: domain.ComfyUISettings{
		Provider: domain.ComfyUISelfHosted, URL: url, Model: domain.ComfyUIModelQwenImageEdit,
	}}
	if err := NewWithOptions(qwen, Options{DiffusionModel: "klein-4b-finetune.safetensors"}).Check(context.Background()); err != nil {
		t.Fatalf("Check with Qwen files = %v", err)
	}
	var missing *domain.ComfyUIMissingError
	if err := New(selfHosted(url)).Check(context.Background()); !errors.As(err, &missing) {
		t.Fatalf("Check with the default model = %v, want klein's files missing", err)
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// combo selects the newer COMBO schema; drop omits node classes.
func objectInfoFor(t *testing.T, files []string, combo bool, drop ...string) []byte {
	t.Helper()
	required, err := workflowRequirement(domain.ComfyUIModelFlux2Klein, "")
	if err != nil {
		t.Fatal(err)
	}
	info := map[string]any{}
	for _, node := range required.nodes {
		if contains(drop, node) {
			continue
		}
		inputs := map[string]any{}
		if input, ok := modelLoaders[node]; ok {
			options := append([]string{"other.safetensors"}, files...)
			if combo {
				inputs[input] = []any{"COMBO", map[string]any{"options": options}}
			} else {
				inputs[input] = []any{options, map[string]any{}}
			}
		}
		info[node] = map[string]any{"input": map[string]any{"required": inputs}}
	}
	data, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func fullObjectInfo(t *testing.T) []byte {
	t.Helper()
	var files []string
	for _, f := range requiredFiles {
		files = append(files, f.name)
	}
	return objectInfoFor(t, files, false)
}

func fakeProvider(t *testing.T, path, reply, prefix string, objectInfo []byte, gotKey *string) string {
	t.Helper()
	server := serveRoutes(t, map[string]http.HandlerFunc{
		path: func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(reply)) },
		prefix + "/object_info": func(w http.ResponseWriter, r *http.Request) {
			if gotKey != nil {
				*gotKey = r.Header.Get("X-API-Key")
			}
			_, _ = w.Write(objectInfo)
		},
	})
	return server.URL
}

const systemStatsReply = `{"system":{"os":"posix","python_version":"3.12.3"},"devices":[]}`

func TestCheckAvailableWithComboOptions(t *testing.T) {
	var files []string
	for _, f := range requiredFiles {
		files = append(files, f.name)
	}
	url := fakeProvider(t, "/system_stats", systemStatsReply, "", objectInfoFor(t, files, true), nil)
	if err := New(selfHosted(url)).Check(context.Background()); err != nil {
		t.Fatalf("Check = %v", err)
	}
}

func TestCheckReportsMissingNodesAndFiles(t *testing.T) {
	info := objectInfoFor(t, []string{"qwen_3_4b.safetensors"}, false, "Flux2Scheduler", "GetImageSize")
	// Files follow table order, then node classes follow name order.
	want := []domain.ComfyUIMissingItem{
		{
			Kind: domain.ComfyUIMissingModel, Name: "flux-2-klein-4b-fp8.safetensors", Folder: "diffusion_models",
			Source: "https://huggingface.co/black-forest-labs/FLUX.2-klein-4b-fp8",
		},
		{
			Kind: domain.ComfyUIMissingModel, Name: "flux2-vae.safetensors", Folder: "vae",
			Source: "https://huggingface.co/Comfy-Org/flux2-dev/tree/main/split_files/vae",
		},
		{Kind: domain.ComfyUIMissingNode, Name: "Flux2Scheduler"},
		{Kind: domain.ComfyUIMissingNode, Name: "GetImageSize"},
	}
	var cloudKey string
	selfURL := fakeProvider(t, "/system_stats", systemStatsReply, "", info, nil)
	cloudURL := fakeProvider(t, "/api/jobs", `{"jobs":[]}`, "/api", info, &cloudKey)
	clients := map[string]*Client{
		"self-hosted": New(selfHosted(selfURL)),
		"cloud":       NewWithOptions(cloud("secret"), Options{CloudBaseURL: cloudURL + "/api"}),
	}
	for name, client := range clients {
		err := client.Check(context.Background())
		var missing *domain.ComfyUIMissingError
		if !errors.As(err, &missing) {
			t.Fatalf("%s: err = %v, want a missing error", name, err)
		}
		if !reflect.DeepEqual(missing.Items, want) {
			t.Errorf("%s: items = %+v", name, missing.Items)
		}
		if !strings.Contains(err.Error(), "missing files") || !strings.Contains(err.Error(), "update ComfyUI") {
			t.Errorf("%s: message = %q", name, err)
		}
	}
	if cloudKey != "secret" {
		t.Errorf("Comfy Cloud /object_info key = %q", cloudKey)
	}
}

func TestCheckReportsMissingLoaderAsNodeOnly(t *testing.T) {
	var files []string
	for _, f := range requiredFiles {
		files = append(files, f.name)
	}
	url := fakeProvider(t, "/system_stats", systemStatsReply, "", objectInfoFor(t, files, false, "UNETLoader"), nil)
	var missing *domain.ComfyUIMissingError
	err := New(selfHosted(url)).Check(context.Background())
	if !errors.As(err, &missing) {
		t.Fatalf("err = %v", err)
	}
	want := []domain.ComfyUIMissingItem{{Kind: domain.ComfyUIMissingNode, Name: "UNETLoader"}}
	if !reflect.DeepEqual(missing.Items, want) {
		t.Fatalf("items = %+v", missing.Items)
	}
}

func TestCheckRejectsObjectInfoThatIsNotJSON(t *testing.T) {
	url := fakeProvider(t, "/system_stats", systemStatsReply, "", []byte("<html>secret</html>"), nil)
	err := New(selfHosted(url)).Check(context.Background())
	if err == nil || !strings.Contains(err.Error(), "is not ComfyUI (/object_info is not JSON)") ||
		strings.Contains(err.Error(), "secret") {
		t.Fatalf("err = %v", err)
	}
}
