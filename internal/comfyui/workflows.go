package comfyui

import (
	"embed"
	"encoding/json"
	"fmt"
	"slices"
	"sort"

	"github.com/syudead/vv/internal/domain"
)

// Embedded API-format workflows are authoritative; each model has variants with and without
// reference images, and vv:* titles identify application-filled nodes.
//
//go:embed workflows/*.json
var workflowFiles embed.FS

const (
	workflowEdit              = "workflows/thumbnail-edit.json"
	workflowEditReference     = "workflows/thumbnail-edit-reference.json"
	workflowQwenEdit          = "workflows/thumbnail-edit-qwen.json"
	workflowQwenEditReference = "workflows/thumbnail-edit-qwen-reference.json"
)

type preset struct {
	// editReference is the reference-image variant.
	edit          string
	editReference string
	// Connection checks report required files in this order.
	files []requiredFile
	// Only klein supports compatible diffusion-model overrides.
	overridable bool
}

var presets = map[domain.ComfyUIModel]preset{
	domain.ComfyUIModelFlux2Klein: {
		edit: workflowEdit, editReference: workflowEditReference, files: requiredFiles, overridable: true,
	},
	domain.ComfyUIModelQwenImageEdit: {
		edit: workflowQwenEdit, editReference: workflowQwenEditReference, files: qwenRequiredFiles,
	},
}

// Unknown models fall back to the default preset.
func presetFor(model domain.ComfyUIModel) preset {
	if p, ok := presets[model]; ok {
		return p
	}
	return presets[domain.ComfyUIModels[0]]
}

var workflowNames = []string{workflowEdit, workflowEditReference, workflowQwenEdit, workflowQwenEditReference}

type workflowNode struct {
	ClassType string         `json:"class_type"`
	Inputs    map[string]any `json:"inputs"`
	Meta      struct {
		Title string `json:"title"`
	} `json:"_meta"`
}

type workflow map[string]workflowNode

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

var modelLoaders = map[string]string{
	"UNETLoader":          "unet_name",
	"CLIPLoader":          "clip_name",
	"VAELoader":           "vae_name",
	"LoraLoaderModelOnly": "lora_name",
}

var promptInputs = map[string]string{
	"CLIPTextEncode":              "text",
	"TextEncodeQwenImageEditPlus": "prompt",
}

const diffusionLoader = "UNETLoader"

type requiredFile struct {
	name   string
	folder string
	source string
}

// Tests ensure every workflow loader's file appears here.
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

var qwenRequiredFiles = []requiredFile{
	{
		name:   "qwen_image_edit_2511_fp8mixed.safetensors",
		folder: "diffusion_models",
		source: "https://huggingface.co/Comfy-Org/Qwen-Image-Edit_ComfyUI/tree/main/split_files/diffusion_models",
	},
	{
		name:   "qwen_2.5_vl_7b_fp8_scaled.safetensors",
		folder: "text_encoders",
		source: "https://huggingface.co/Comfy-Org/Qwen-Image_ComfyUI/tree/main/split_files/text_encoders",
	},
	{
		name:   "qwen_image_vae.safetensors",
		folder: "vae",
		source: "https://huggingface.co/Comfy-Org/Qwen-Image_ComfyUI/tree/main/split_files/vae",
	},
	{
		name:   "Qwen-Image-Edit-2511-Lightning-4steps-V1.0-bf16.safetensors",
		folder: "loras",
		source: "https://huggingface.co/lightx2v/Qwen-Image-Edit-2511-Lightning",
	},
}

type requirement struct {
	// Sorted by node class name.
	nodes []string
	files []loaderFile
}

type loaderFile struct {
	loader string
	input  string
	file   requiredFile
}

// An overridden model has no known download source; empty leaves base unchanged.
func requiredFilesFor(base []requiredFile, diffusionModel string) []requiredFile {
	files := slices.Clone(base)
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

// Collect both workflow variants and reject loader files missing from the model's requirements;
// callers supply diffusion overrides only for supported models.
func workflowRequirement(model domain.ComfyUIModel, diffusionModel string) (requirement, error) {
	p := presetFor(model)
	required := requiredFilesFor(p.files, diffusionModel)
	nodes := map[string]bool{}
	var files []loaderFile
	for _, name := range []string{p.edit, p.editReference} {
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
	// Preserve Required files table order.
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

type objectInfo map[string]struct {
	Input struct {
		Required map[string]json.RawMessage `json:"required"`
		Optional map[string]json.RawMessage `json:"optional"`
	} `json:"input"`
}

// Accept legacy [choices, settings] and newer ["COMBO", {"options": choices}] input definitions.
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

// Report files before nodes in stable order; when a loader class is absent, report only that class
// because ComfyUI must be upgraded first.
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
