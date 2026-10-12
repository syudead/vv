package comfyui

import (
	"embed"
	"encoding/json"
	"fmt"
	"slices"
	"sort"

	"github.com/syudead/vv/internal/domain"
)

// workflowFiles は vv が用意する 2 つのワークフローである。ComfyUI の API 形式の JSON で、
// ここに埋め込んだファイルが正本である（specs/045-comfyui-thumbnails/contracts/workflow.md、
// research.md R-2）。vv が埋めるノードは _meta.title の vv:* で見つける。
//
//go:embed workflows/*.json
var workflowFiles embed.FS

const (
	// workflowEdit は参考画像が無いときのワークフローである。
	workflowEdit = "workflows/thumbnail-edit.json"
	// workflowEditReference は参考画像があるときのワークフローである。
	workflowEditReference = "workflows/thumbnail-edit-reference.json"
)

// workflowNames は埋め込んだワークフローの全部である。必要なファイルの確かめはこのすべてを見る。
var workflowNames = []string{workflowEdit, workflowEditReference}

// workflowNode はワークフローの 1 つのノードである。
type workflowNode struct {
	ClassType string         `json:"class_type"`
	Inputs    map[string]any `json:"inputs"`
	Meta      struct {
		Title string `json:"title"`
	} `json:"_meta"`
}

// workflow はノードの id からノードへの対応である。
type workflow map[string]workflowNode

// loadWorkflow は埋め込んだワークフロー name を読む。
func loadWorkflow(name string) (workflow, error) {
	data, err := workflowFiles.ReadFile(name)
	if err != nil {
		return nil, fmt.Errorf("cannot read the bundled workflow %s: %w", name, err)
	}
	var w workflow
	if err := json.Unmarshal(data, &w); err != nil {
		return nil, fmt.Errorf("the bundled workflow %s is not valid JSON: %w", name, err)
	}
	return w, nil
}

// modelLoaders はモデルファイルを読むローダーノードの型と、ファイル名を受ける入力の名前である。
var modelLoaders = map[string]string{
	"UNETLoader": "unet_name",
	"CLIPLoader": "clip_name",
	"VAELoader":  "vae_name",
}

// diffusionLoader は拡散モデルを読むローダーの型である。Options.DiffusionModel はこの型の
// ローダーが読むファイルを差し替える。
const diffusionLoader = "UNETLoader"

// requiredFile はワークフローが読むモデルファイルの 1 つと、その置き場所と入手先である
// （contracts/workflow.md の Required files、research.md R-12）。
type requiredFile struct {
	name   string
	folder string
	source string
}

// requiredFiles はワークフローのローダーが名指すモデルファイルの全部である。ワークフローの
// ローダーが名指すファイルはどれもここに載っている（workflows_test.go が確かめる）。
var requiredFiles = []requiredFile{
	{
		name:   "flux-2-klein-4b-fp8.safetensors",
		folder: "diffusion_models",
		source: "https://huggingface.co/black-forest-labs/FLUX.2-klein-4b-fp8",
	},
	{
		name:   "qwen_3_4b.safetensors",
		folder: "text_encoders",
		source: "https://huggingface.co/Comfy-Org/z_image_turbo/tree/main/split_files/text_encoders",
	},
	{
		name:   "flux2-vae.safetensors",
		folder: "vae",
		source: "https://huggingface.co/Comfy-Org/flux2-dev/tree/main/split_files/vae",
	},
}

// requirement はワークフローが使うノードの型と、ローダーが名指すファイルである。
type requirement struct {
	// nodes はすべてのノードの型で、名前の順に並ぶ。
	nodes []string
	// files はローダーの型ごとに、そのローダーの入力が名指すファイルの名前である。
	files []loaderFile
}

// loaderFile はローダーの型と入力の名前、その入力が名指すファイルである。
type loaderFile struct {
	loader string
	input  string
	file   requiredFile
}

// requiredFilesFor は requiredFiles の拡散モデルを diffusionModel に代えた一覧を返す。
// diffusionModel が空なら requiredFiles のままである。差し替えたファイルの入手先は分からない。
func requiredFilesFor(diffusionModel string) []requiredFile {
	files := slices.Clone(requiredFiles)
	if diffusionModel == "" {
		return files
	}
	for i, f := range files {
		if f.folder == "diffusion_models" {
			files[i] = requiredFile{name: diffusionModel, folder: f.folder}
		}
	}
	return files
}

// workflowRequirement は埋め込んだワークフローの全部が使うノードの型とファイルを集める。
// diffusionModel が空でなければ、拡散モデルのローダーはそのファイルを読むものとして扱う。
// ローダーが requiredFiles に無いファイルを名指していれば誤りを返す。
func workflowRequirement(diffusionModel string) (requirement, error) {
	required := requiredFilesFor(diffusionModel)
	nodes := map[string]bool{}
	var files []loaderFile
	for _, name := range workflowNames {
		w, err := loadWorkflow(name)
		if err != nil {
			return requirement{}, err
		}
		for _, node := range w {
			nodes[node.ClassType] = true
			input, ok := modelLoaders[node.ClassType]
			if !ok {
				continue
			}
			value, _ := node.Inputs[input].(string)
			if diffusionModel != "" && node.ClassType == diffusionLoader {
				value = diffusionModel
			}
			index := slices.IndexFunc(required, func(f requiredFile) bool { return f.name == value })
			if index < 0 {
				return requirement{}, fmt.Errorf("the bundled workflow %s loads %q, which is not a required file", name, value)
			}
			entry := loaderFile{loader: node.ClassType, input: input, file: required[index]}
			if !slices.Contains(files, entry) {
				files = append(files, entry)
			}
		}
	}
	sorted := make([]string, 0, len(nodes))
	for node := range nodes {
		sorted = append(sorted, node)
	}
	sort.Strings(sorted)
	// ファイルは requiredFiles（Required files の表）の順に並べる。
	ordered := make([]loaderFile, 0, len(files))
	for _, f := range required {
		for _, entry := range files {
			if entry.file.name == f.name {
				ordered = append(ordered, entry)
			}
		}
	}
	return requirement{nodes: sorted, files: ordered}, nil
}

// objectInfo は GET /object_info の応答のうち、ノードの型ごとの入力の定義である。
type objectInfo map[string]struct {
	Input struct {
		Required map[string]json.RawMessage `json:"required"`
		Optional map[string]json.RawMessage `json:"optional"`
	} `json:"input"`
}

// options は型 class の入力 input の選択肢を返す。入力の定義は [選択肢の配列, 設定] か、
// 新しい ComfyUI の ["COMBO", {"options": 選択肢}] である。どちらでもなければ空を返す。
func (o objectInfo) options(class, input string) []string {
	node, ok := o[class]
	if !ok {
		return nil
	}
	spec, ok := node.Input.Required[input]
	if !ok {
		spec = node.Input.Optional[input]
	}
	var parts []json.RawMessage
	if json.Unmarshal(spec, &parts) != nil || len(parts) == 0 {
		return nil
	}
	var list []string
	if json.Unmarshal(parts[0], &list) == nil {
		return list
	}
	var kind string
	if json.Unmarshal(parts[0], &kind) == nil && kind == "COMBO" && len(parts) > 1 {
		var combo struct {
			Options []string `json:"options"`
		}
		if json.Unmarshal(parts[1], &combo) == nil {
			return combo.Options
		}
	}
	return nil
}

// missing はローダーの選択肢に無いファイルを Required files の表の順に、続けて info に無い
// ノードの型を名前の順に返す（設定画面はモデルファイルを先に並べる、ui-design.md）。ローダーの
// 型が無ければ、その型だけを挙げ、ファイルは挙げない（ComfyUI を新しくするのが先である）。
func (r requirement) missing(info objectInfo) []domain.ComfyUIMissingItem {
	var items []domain.ComfyUIMissingItem
	for _, f := range r.files {
		if _, ok := info[f.loader]; !ok {
			continue
		}
		if slices.Contains(info.options(f.loader, f.input), f.file.name) {
			continue
		}
		items = append(items, domain.ComfyUIMissingItem{
			Kind: domain.ComfyUIMissingModel, Name: f.file.name, Folder: f.file.folder, Source: f.file.source,
		})
	}
	for _, node := range r.nodes {
		if _, ok := info[node]; !ok {
			items = append(items, domain.ComfyUIMissingItem{Kind: domain.ComfyUIMissingNode, Name: node})
		}
	}
	return items
}
