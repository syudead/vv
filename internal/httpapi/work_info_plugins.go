package httpapi

import (
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// 作品情報プラグインの追加・承認・取り消し・削除と設定の値の書き込みの経路
// （specs/049-work-info-plugins/contracts/screen-api.md）。どれも所有者だけで、accessRoutes に載せない。
// 検査・規則・保存は internal/app と internal/store が持ち、ここは要求の解釈と契約の形への変換だけを持つ。
// 応答の型は秘密の設定の値を持たない（domain.WorkInfoPluginEntry が値を落としてある）。

// workInfoPluginUploadMaxBody はアップロードの要求の本文全体の上限である。module 部分の上限に、部分の
// 区切りと見出しのぶんの 1 MiB を足す。
const workInfoPluginUploadMaxBody = domain.WorkInfoPluginModuleMaxBytes + 1<<20

// WorkInfoPlugins は作品情報プラグインの登録である。internal/app の *WorkInfoPlugins が満たす。
type WorkInfoPlugins interface {
	// ListWorkInfoPlugins はすべてのプラグインを、名前、次にファイル名の順で返す。
	ListWorkInfoPlugins(ctx context.Context) ([]domain.WorkInfoPluginEntry, error)
	// UploadWorkInfoPlugin はモジュールを検査して保存し、保存したプラグインと、新しい行かを返す。
	// 上限を超えれば domain.ErrWorkInfoPluginTooLarge を返す。
	UploadWorkInfoPlugin(ctx context.Context, fileName string, module []byte) (domain.WorkInfoPluginEntry, bool, error)
	// ApproveWorkInfoPlugin はダイジェスト digest のモジュールを承認する。domain.ErrNotFound・
	// domain.ErrWorkInfoPluginChanged・domain.ErrWorkInfoPluginUnusable を返しうる。
	ApproveWorkInfoPlugin(ctx context.Context, id int64, digest string) (domain.WorkInfoPluginEntry, error)
	// RevokeWorkInfoPluginApproval は承認を取り消す。無ければ domain.ErrNotFound を返す。
	RevokeWorkInfoPluginApproval(ctx context.Context, id int64) (domain.WorkInfoPluginEntry, error)
	// SetWorkInfoPluginSettings は設定の値を書く。domain.ErrNotFound・domain.ErrWorkInfoPluginUnusable・
	// domain.ErrWorkInfoPluginSettingInvalid を返しうる。
	SetWorkInfoPluginSettings(ctx context.Context, id int64, values map[string]*string) (domain.WorkInfoPluginEntry, error)
	// DeleteWorkInfoPlugin はプラグインとその設定の値を消す。無ければ domain.ErrNotFound を返す。
	DeleteWorkInfoPlugin(ctx context.Context, id int64) error
}

// workInfoPluginsConfigured は登録がつながっているかを確かめ、無ければ 500 を書いて偽を返す。
func (s *server) workInfoPluginsConfigured(w http.ResponseWriter) bool {
	if s.workInfoPlugins == nil {
		s.internalError(w, "Work-info plugins are not configured.", nil)
		return false
	}
	return true
}

// ListWorkInfoPlugins はプラグインの一覧を返す（GET /api/work-info-plugins）。
func (s *server) ListWorkInfoPlugins(w http.ResponseWriter, r *http.Request) {
	if !s.workInfoPluginsConfigured(w) {
		return
	}
	entries, err := s.workInfoPlugins.ListWorkInfoPlugins(r.Context())
	if err != nil {
		s.internalError(w, "Could not load the work-info plugins.", err)
		return
	}
	plugins := make([]gen.WorkInfoPlugin, 0, len(entries))
	for _, entry := range entries {
		plugins = append(plugins, toWorkInfoPlugin(entry))
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.WorkInfoPluginList{Plugins: plugins}, s.logger)
}

// UploadWorkInfoPlugin は multipart の module を追加するか、同じ名前のプラグインと差し替える
// （POST /api/work-info-plugins）。
func (s *server) UploadWorkInfoPlugin(w http.ResponseWriter, r *http.Request) {
	if !s.workInfoPluginsConfigured(w) {
		return
	}
	fileName, module, ok := s.readWorkInfoPluginUpload(w, r)
	if !ok {
		return
	}
	entry, created, err := s.workInfoPlugins.UploadWorkInfoPlugin(r.Context(), fileName, module)
	if err != nil {
		s.workInfoPluginError(w, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	s.writeWorkInfoPlugin(w, status, entry)
}

// readWorkInfoPluginUpload はアップロードの multipart/form-data から module を 1 つ読み、そのファイル名と
// 中身を返す。module が無いかほかの部分を送れば 400、2 つ送れば readMultipart が 400、module か本文全体が
// 上限を越えれば 413 plugin_too_large である。
func (s *server) readWorkInfoPluginUpload(w http.ResponseWriter, r *http.Request) (string, []byte, bool) {
	var (
		fileName string
		module   []byte
	)
	tooLarge := func() { s.workInfoPluginError(w, domain.ErrWorkInfoPluginTooLarge) }
	ok := s.readMultipart(w, r, workInfoPluginUploadMaxBody, tooLarge, func(name string, part *multipart.Part) bool {
		if name != "module" {
			s.invalidRequest(w, "Only module may be sent.")
			return false
		}
		data, err := readWorkInfoPluginPart(part)
		switch {
		case errors.Is(err, domain.ErrWorkInfoPluginTooLarge):
			tooLarge()
			return false
		case err != nil:
			s.invalidRequest(w, "The request body could not be read.")
			return false
		}
		fileName, module = part.FileName(), data
		return true
	})
	if !ok {
		return "", nil, false
	}
	if module == nil {
		s.invalidRequest(w, "module is required.")
		return "", nil, false
	}
	return fileName, module, true
}

// readWorkInfoPluginPart は module 部分を上限まで読む。部分か本文全体が上限を越えれば
// domain.ErrWorkInfoPluginTooLarge を返す。空の部分も nil でない空の値として返す。
func readWorkInfoPluginPart(part io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(part, domain.WorkInfoPluginModuleMaxBytes+1))
	var bodyTooLarge *http.MaxBytesError
	switch {
	case errors.As(err, &bodyTooLarge), err == nil && len(data) > domain.WorkInfoPluginModuleMaxBytes:
		return nil, domain.ErrWorkInfoPluginTooLarge
	case err != nil:
		return nil, err
	case data == nil:
		return []byte{}, nil
	}
	return data, nil
}

// ApproveWorkInfoPlugin は所有者が見たモジュールを承認する（PUT /api/work-info-plugins/{id}/approval）。
func (s *server) ApproveWorkInfoPlugin(w http.ResponseWriter, r *http.Request, id gen.WorkInfoPluginId) {
	var body gen.ApproveWorkInfoPluginRequest
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if body.Digest == "" {
		s.invalidRequest(w, "digest is required.")
		return
	}
	if !s.workInfoPluginsConfigured(w) {
		return
	}
	entry, err := s.workInfoPlugins.ApproveWorkInfoPlugin(r.Context(), id, body.Digest)
	if err != nil {
		s.workInfoPluginError(w, err)
		return
	}
	s.writeWorkInfoPlugin(w, http.StatusOK, entry)
}

// RevokeWorkInfoPluginApproval は承認を取り消す（DELETE /api/work-info-plugins/{id}/approval）。
func (s *server) RevokeWorkInfoPluginApproval(w http.ResponseWriter, r *http.Request, id gen.WorkInfoPluginId) {
	if !s.workInfoPluginsConfigured(w) {
		return
	}
	entry, err := s.workInfoPlugins.RevokeWorkInfoPluginApproval(r.Context(), id)
	if err != nil {
		s.workInfoPluginError(w, err)
		return
	}
	s.writeWorkInfoPlugin(w, http.StatusOK, entry)
}

// SetWorkInfoPluginSettings は設定の値を書く（PUT /api/work-info-plugins/{id}/settings）。
func (s *server) SetWorkInfoPluginSettings(w http.ResponseWriter, r *http.Request, id gen.WorkInfoPluginId) {
	var body gen.SetWorkInfoPluginSettingsRequest
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if body.Values == nil {
		s.invalidRequest(w, "values is required.")
		return
	}
	if !s.workInfoPluginsConfigured(w) {
		return
	}
	entry, err := s.workInfoPlugins.SetWorkInfoPluginSettings(r.Context(), id, body.Values)
	if err != nil {
		s.workInfoPluginError(w, err)
		return
	}
	s.writeWorkInfoPlugin(w, http.StatusOK, entry)
}

// DeleteWorkInfoPlugin はプラグインを消す（DELETE /api/work-info-plugins/{id}）。
func (s *server) DeleteWorkInfoPlugin(w http.ResponseWriter, r *http.Request, id gen.WorkInfoPluginId) {
	if !s.workInfoPluginsConfigured(w) {
		return
	}
	if err := s.workInfoPlugins.DeleteWorkInfoPlugin(r.Context(), id); err != nil {
		s.workInfoPluginError(w, err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) writeWorkInfoPlugin(w http.ResponseWriter, status int, entry domain.WorkInfoPluginEntry) {
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, status, toWorkInfoPlugin(entry), s.logger)
}

// workInfoPluginError はプラグインの誤りを契約の応答にする（contracts/screen-api.md）。
func (s *server) workInfoPluginError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrWorkInfoPluginTooLarge):
		limit := domain.WorkInfoPluginModuleMaxBytes
		s.writeErrorBody(w, http.StatusRequestEntityTooLarge, gen.Error{
			Code: codePluginTooLarge, Limit: &limit, Message: "The plugin must be at most 16 MiB.",
		})
	case errors.Is(err, domain.ErrNotFound):
		s.notFound(w, "The work-info plugin was not found.")
	case errors.Is(err, domain.ErrWorkInfoPluginChanged):
		s.writeError(w, http.StatusConflict, codePluginChanged, "The plugin was replaced. Review it again before approving.")
	case errors.Is(err, domain.ErrWorkInfoPluginUnusable):
		s.writeError(w, http.StatusConflict, codePluginUnusable, "The plugin has a problem and cannot be used.")
	case errors.Is(err, domain.ErrWorkInfoPluginSettingInvalid):
		s.invalidRequest(w, "Each key must be a setting the plugin declares, with a value of at most 2000 characters.")
	default:
		s.internalError(w, "Could not change the work-info plugin.", err)
	}
}

// toWorkInfoPlugin は画面に見せるプラグインを契約の形にする。秘密の設定は値を持たない。
func toWorkInfoPlugin(entry domain.WorkInfoPluginEntry) gen.WorkInfoPlugin {
	operations := make([]gen.WorkInfoOperation, 0, len(entry.Operations))
	for _, op := range entry.Operations {
		operations = append(operations, gen.WorkInfoOperation(op))
	}
	settings := make([]gen.WorkInfoPluginSetting, 0, len(entry.Settings))
	for _, setting := range entry.Settings {
		item := gen.WorkInfoPluginSetting{
			Key: setting.Key, Label: setting.Label, Secret: setting.Secret, Required: setting.Required, Set: setting.Set,
		}
		if !setting.Secret && setting.Set {
			item.Value = &setting.Value
		}
		settings = append(settings, item)
	}
	plugin := gen.WorkInfoPlugin{
		Id:          entry.ID,
		Name:        optionalString(entry.Name),
		FileName:    entry.FileName,
		Version:     optionalString(entry.Version),
		Description: optionalString(entry.Description),
		Operations:  operations,
		Digest:      entry.Digest,
		State:       gen.WorkInfoPluginState(entry.State),
		Settings:    settings,
		// 取り決めの版 1 のプラグインはどれもインターネットのサイトに届く（要件 2）。
		Network:   true,
		UpdatedAt: entry.UpdatedAt.Unix(),
	}
	if entry.Problem != "" {
		problem := gen.WorkInfoPluginProblem(entry.Problem)
		plugin.Problem = &problem
	}
	return plugin
}
