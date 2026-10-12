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

// 埋め込んだ各ワークフローは、アダプタが埋める vv:* のノードを決まった型で 1 つずつ持つ
// （contracts/workflow.md の Workflows、research.md R-2）。参考画像の vv:reference は
// 参考画像ありの方だけが持つ。
func TestWorkflowsHaveTheTitlesVVFills(t *testing.T) {
	base := map[string]string{
		"vv:frame":  "LoadImage",
		"vv:prompt": "CLIPTextEncode",
		"vv:mask":   "LoadImageMask",
		"vv:output": "SaveImage",
	}
	withReference := map[string]string{"vv:reference": "LoadImage"}
	for k, v := range base {
		withReference[k] = v
	}
	cases := map[string]map[string]string{workflowEdit: base, workflowEditReference: withReference}
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

// ローダーが名指すファイルはどれも Required files の表にあり、表のファイルはどれも使われる。
func TestWorkflowLoadersNameTheRequiredFiles(t *testing.T) {
	required, err := workflowRequirement("")
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

// 拡散モデルを差し替えると、確かめるファイルはそのファイルに代わり、入手先は空になる。
func TestWorkflowRequirementWithDiffusionModel(t *testing.T) {
	required, err := workflowRequirement("klein-4b-finetune.safetensors")
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

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// objectInfoFor はワークフローのノードの型すべてを持つ /object_info を作る。ローダーの入力は
// files を選択肢に持つ。combo が真なら新しい ComfyUI の ["COMBO", {"options": …}] の形にする。
// drop に挙げた型は省く。
func objectInfoFor(t *testing.T, files []string, combo bool, drop ...string) []byte {
	t.Helper()
	required, err := workflowRequirement("")
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

// fullObjectInfo はワークフローに要るノードとファイルをすべて持つ /object_info である。
func fullObjectInfo(t *testing.T) []byte {
	t.Helper()
	var files []string
	for _, f := range requiredFiles {
		files = append(files, f.name)
	}
	return objectInfoFor(t, files, false)
}

// fakeProvider は接続の経路 path と /object_info（prefix の下）に答える偽の提供元を立てる。
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

// 新しい ComfyUI の COMBO の形でも、すべてのノードとファイルがあれば使える。
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

// ローダーの選択肢に無いファイルごとにフォルダと入手先を持つ model の項目を、無い型ごとに node の
// 項目を返す。自前の ComfyUI でも Comfy Cloud でも同じ確かめを行う。
func TestCheckReportsMissingNodesAndFiles(t *testing.T) {
	info := objectInfoFor(t, []string{"qwen_3_4b.safetensors"}, false, "Flux2Scheduler", "GetImageSize")
	// モデルファイルが表の順に先、ノードの型が名前の順に後に並ぶ。
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

// ローダーの型そのものが無ければ型だけを挙げ、そのファイルは挙げない。
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

// /object_info が JSON でなければ ComfyUI でないと伝え、本文は写さない。
func TestCheckRejectsObjectInfoThatIsNotJSON(t *testing.T) {
	url := fakeProvider(t, "/system_stats", systemStatsReply, "", []byte("<html>secret</html>"), nil)
	err := New(selfHosted(url)).Check(context.Background())
	if err == nil || !strings.Contains(err.Error(), "is not ComfyUI (/object_info is not JSON)") ||
		strings.Contains(err.Error(), "secret") {
		t.Fatalf("err = %v", err)
	}
}
