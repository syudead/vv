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

// Plugin management is owner-only and excluded from accessRoutes. Domain entries already omit secret setting values.

// Allow 1 MiB above the module limit for multipart headers and boundaries.
const workInfoPluginUploadMaxBody = domain.WorkInfoPluginModuleMaxBytes + 1<<20

type WorkInfoPlugins interface {
	// ListWorkInfoPlugins sorts by name, then filename.
	ListWorkInfoPlugins(ctx context.Context) ([]domain.WorkInfoPluginEntry, error)
	// UploadWorkInfoPlugin validates and stores the module, returning whether the row is new; oversized modules return ErrWorkInfoPluginTooLarge.
	UploadWorkInfoPlugin(ctx context.Context, fileName string, module []byte) (domain.WorkInfoPluginEntry, bool, error)
	// ApproveWorkInfoPlugin approves the specified digest; it may return ErrNotFound, ErrWorkInfoPluginChanged, or ErrWorkInfoPluginUnusable.
	ApproveWorkInfoPlugin(ctx context.Context, id int64, digest string) (domain.WorkInfoPluginEntry, error)
	// RevokeWorkInfoPluginApproval returns ErrNotFound when absent.
	RevokeWorkInfoPluginApproval(ctx context.Context, id int64) (domain.WorkInfoPluginEntry, error)
	// SetWorkInfoPluginSettings may return ErrNotFound, ErrWorkInfoPluginUnusable, or ErrWorkInfoPluginSettingInvalid.
	SetWorkInfoPluginSettings(ctx context.Context, id int64, values map[string]*string) (domain.WorkInfoPluginEntry, error)
	// DeleteWorkInfoPlugin removes settings too; an absent plugin returns ErrNotFound, the bundled one
	// ErrWorkInfoPluginBuiltin.
	DeleteWorkInfoPlugin(ctx context.Context, id int64) error
	// SetLinkOrder returns the list, or ErrLinkOrderChanged when order does not name each plugin that handles links once.
	SetLinkOrder(ctx context.Context, order []int64) ([]domain.WorkInfoPluginEntry, error)
	// WorkInfoPluginLastRun returns redacted stderr and whether it was retained; an absent plugin returns ErrNotFound.
	WorkInfoPluginLastRun(ctx context.Context, id int64) (domain.WorkInfoPluginRun, bool, error)
	// SearchWorkInfo requires an approved plugin; workInfoRunError maps failures to the API contract.
	SearchWorkInfo(ctx context.Context, id int64, query string) ([]domain.WorkInfoCandidate, error)
	// SearchPeople requires an approved person plugin and fails like SearchWorkInfo.
	SearchPeople(ctx context.Context, id int64, query string) ([]domain.PersonCandidate, error)
	// FetchWorkInfo caches fetched work information for the video without modifying it; the plugin must be approved.
	FetchWorkInfo(ctx context.Context, videoID, pluginID int64, id string) (domain.WorkInfoFetchResult, error)
	// WorkInfoFillQueries lists one video per user-data key with its title and search words.
	WorkInfoFillQueries(ctx context.Context, videoIDs []int64) ([]domain.WorkInfoFillQuery, error)
	// FetchWorkInfoByLink fetches like FetchWorkInfo by a link; ErrWorkInfoLinkUnsupported for a link outside the
	// plugin's domains.
	FetchWorkInfoByLink(ctx context.Context, videoID, pluginID int64, link string) (domain.WorkInfoFetchResult, error)
	// StartLinkFetch queues the fill of a virtual video just created from link and returns at once.
	StartLinkFetch(video domain.Video, link string)
	// ApplyWorkInfoFetch applies a selection and may return ErrWorkInfoApplyInvalid, ErrNotFound, or
	// ErrWorkInfoFetchStale.
	ApplyWorkInfoFetch(ctx context.Context, videoID int64, fetchID string, selection domain.WorkInfoSelection, coverByPage bool) (domain.WorkInfoApplied, error)
	// WorkInfoFetchCover returns ErrNotFound when the result has no cover at index.
	WorkInfoFetchCover(videoID int64, fetchID string, index int) (domain.WorkInfoCover, error)
	// WorkInfoFetchPersonImage returns ErrNotFound for a missing result and ErrPersonImageNotFound when the person at
	// index has no kept image.
	WorkInfoFetchPersonImage(videoID int64, fetchID string, index int) (domain.WorkInfoCover, error)
	// FetchPersonWithPlugin may return ErrPersonNotFound, ErrPersonExternalIDMissing, a PersonLinkAmbiguousError,
	// ErrPersonLinkUnsupported, ErrWorkInfoRunInvalid, or the errors workInfoRunError maps.
	FetchPersonWithPlugin(ctx context.Context, personID, pluginID int64, key domain.PersonFetchKey) (domain.PersonPluginFetchResult, error)
	// PersonPluginFetchImage returns ErrNotFound for a missing result and ErrPersonImageNotFound for an index outside
	// its images.
	PersonPluginFetchImage(personID int64, fetchID string, index int) (domain.WorkInfoCover, error)
	// ApplyPersonPluginFetch may return ErrWorkInfoApplyInvalid, ErrPersonNotFound, or ErrWorkInfoFetchStale; a negative
	// imageIndex names no image.
	ApplyPersonPluginFetch(ctx context.Context, personID int64, fetchID string, selection domain.PersonPluginSelection, imageIndex int) (domain.PersonPluginApplied, error)
}

func (s *server) workInfoPluginsConfigured(w http.ResponseWriter) bool {
	if s.workInfoPlugins == nil {
		s.internalError(w, "Work-info plugins are not configured.", nil)
		return false
	}
	return true
}

func (s *server) ListWorkInfoPlugins(w http.ResponseWriter, r *http.Request) {
	if !s.workInfoPluginsConfigured(w) {
		return
	}
	entries, err := s.workInfoPlugins.ListWorkInfoPlugins(r.Context())
	if err != nil {
		s.internalError(w, "Could not load the work-info plugins.", err)
		return
	}
	s.writeWorkInfoPluginList(w, entries)
}

func (s *server) SetWorkInfoPluginLinkOrder(w http.ResponseWriter, r *http.Request) {
	var body gen.WorkInfoPluginLinkOrder
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if body.PluginIds == nil {
		s.invalidRequest(w, "pluginIds is required.")
		return
	}
	if !s.workInfoPluginsConfigured(w) {
		return
	}
	entries, err := s.workInfoPlugins.SetLinkOrder(r.Context(), body.PluginIds)
	if err != nil {
		s.workInfoPluginError(w, err)
		return
	}
	s.writeWorkInfoPluginList(w, entries)
}

func (s *server) writeWorkInfoPluginList(w http.ResponseWriter, entries []domain.WorkInfoPluginEntry) {
	plugins := make([]gen.WorkInfoPlugin, 0, len(entries))
	for _, entry := range entries {
		plugins = append(plugins, toWorkInfoPlugin(entry))
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.WorkInfoPluginList{Plugins: plugins}, s.logger)
}

// UploadWorkInfoPlugin adds a multipart module or replaces the plugin with the same name.
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

// Require exactly one module part. Missing, extra, or duplicate parts return 400; exceeding module or body limits returns 413 plugin_too_large.
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

// readWorkInfoPluginPart enforces module and body limits and returns a non-nil slice even for an empty part.
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

// GetWorkInfoPluginLastRun returns {} when stderr was not retained.
func (s *server) GetWorkInfoPluginLastRun(w http.ResponseWriter, r *http.Request, id gen.WorkInfoPluginId) {
	if !s.workInfoPluginsConfigured(w) {
		return
	}
	run, ok, err := s.workInfoPlugins.WorkInfoPluginLastRun(r.Context(), id)
	if err != nil {
		s.workInfoPluginError(w, err)
		return
	}
	var body gen.WorkInfoPluginLastRun
	if ok {
		body.Run = &gen.WorkInfoPluginRun{
			RanAt: run.RanAt.Unix(), Operation: gen.WorkInfoOperation(run.Operation),
			Stderr: run.Stderr, Truncated: run.Truncated,
		}
		if run.Failure != "" {
			failure := gen.WorkInfoPluginRunFailure(run.Failure)
			body.Run.Failure = &failure
		}
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, body, s.logger)
}

func (s *server) writeWorkInfoPlugin(w http.ResponseWriter, status int, entry domain.WorkInfoPluginEntry) {
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, status, toWorkInfoPlugin(entry), s.logger)
}

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
	case errors.Is(err, domain.ErrWorkInfoPluginKindMismatch):
		s.writePluginKindMismatch(w)
	case errors.Is(err, domain.ErrWorkInfoPluginBuiltin):
		s.writeError(w, http.StatusConflict, codePluginBuiltin, "The plugin is bundled with VVMDM and cannot be replaced or deleted.")
	case errors.Is(err, domain.ErrLinkOrderChanged):
		s.writeError(w, http.StatusConflict, codeLinkOrderChanged, "The plugins that read links changed. Reload the list.")
	default:
		s.internalError(w, "Could not change the work-info plugin.", err)
	}
}

// Secret settings never include values in API responses.
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
		item.Default = optionalString(setting.Default)
		if setting.Options != nil {
			item.Options = &setting.Options
		}
		item.StaleValue = optionalString(setting.StaleValue)
		settings = append(settings, item)
	}
	plugin := gen.WorkInfoPlugin{
		Id:          entry.ID,
		Kind:        gen.WorkInfoPluginKind(entry.Kind),
		Sources:     entry.Sources,
		Domains:     entry.Domains,
		Name:        optionalString(entry.Name),
		FileName:    entry.FileName,
		Version:     optionalString(entry.Version),
		Description: optionalString(entry.Description),
		Operations:  operations,
		Digest:      entry.Digest,
		State:       gen.WorkInfoPluginState(entry.State),
		Settings:    settings,
		// All version 1 plugins can access internet sites.
		Network:   true,
		UpdatedAt: entry.UpdatedAt.Unix(),
		Builtin:   entry.Builtin,
	}
	if entry.LinkRank > 0 {
		rank := entry.LinkRank
		plugin.LinkRank = &rank
	}
	if entry.Problem != "" {
		problem := gen.WorkInfoPluginProblem(entry.Problem)
		plugin.Problem = &problem
	}
	if !entry.LastRunAt.IsZero() {
		lastRunAt := entry.LastRunAt.Unix()
		plugin.LastRunAt = &lastRunAt
	}
	return plugin
}
