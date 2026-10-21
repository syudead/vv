package httpapi

import (
	"errors"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// Candidate routes are owner-only and excluded from accessRoutes. Fingerprint updates use scan events; bundling clears matching candidates in the store.

// Return newest pairs first, with video IDs ascending within each pair.
func (s *server) ListVersionCandidates(w http.ResponseWriter, r *http.Request) {
	if s.versions == nil {
		s.internalError(w, "Version storage is not configured.", nil)
		return
	}
	page, err := s.versions.Candidates(r.Context())
	if err != nil {
		s.internalError(w, "Could not load the possible duplicates.", err)
		return
	}

	videos := make([]domain.Video, 0, len(page.Items)*2)
	for _, item := range page.Items {
		videos = append(videos, item.Videos[0], item.Videos[1])
	}
	judging := false
	if s.versionJudging != nil {
		if judging, err = s.versionJudging.Enabled(r.Context()); err != nil {
			s.internalError(w, "Could not load the duplicate judging settings.", err)
			return
		}
	}
	items := s.versionItems(r, videos)
	// Owners compare the two files' bit rate, frame rate and audio, which only the detail's media information has.
	infos := map[int64]*domain.MediaInfo{}
	if s.mediaInfo != nil {
		ids := make([]int64, 0, len(videos))
		for _, video := range videos {
			ids = append(ids, video.ID)
		}
		if infos, err = s.mediaInfo.MediaInfos(r.Context(), ids); err != nil {
			s.internalError(w, "Could not load the possible duplicates.", err)
			return
		}
		for i := range items {
			items[i].MediaInfo = toAPIMediaInfo(infos[videos[i].ID])
		}
	}
	payload := gen.VersionCandidatePage{
		Items: make([]gen.VersionCandidate, 0, len(page.Items)), Total: page.Total, Judging: judging,
	}
	for i, item := range page.Items {
		payload.Items = append(payload.Items, gen.VersionCandidate{
			Videos:         []gen.Video{items[2*i], items[2*i+1]},
			Distance:       item.Distance,
			Judgement:      toAPICandidateJudgement(item.Judgement),
			Recommendation: recommendRepresentative(item.Videos, infos),
		})
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, payload, s.logger)
}

// recommendRepresentative judges the two files alone; bundling keeps both sides' user data whichever is chosen.
func recommendRepresentative(videos [2]domain.Video, infos map[int64]*domain.MediaInfo) gen.VersionRepresentativeRecommendation {
	chosen, reason := domain.RecommendVersionRepresentative([2]domain.VersionSide{
		{Video: videos[0], Media: infos[videos[0].ID]}, {Video: videos[1], Media: infos[videos[1].ID]},
	})
	return gen.VersionRepresentativeRecommendation{
		VideoId: videos[chosen].ID, Reason: gen.VersionRepresentativeRecommendationReason(reason),
	}
}

// readPairBody writes 400 unless the body names two different videos.
func (s *server) readPairBody(w http.ResponseWriter, r *http.Request) ([2]int64, bool) {
	var body gen.VersionCandidateDismissRequest
	if !s.readJSONBody(w, r, &body) {
		return [2]int64{}, false
	}
	if len(body.VideoIds) != 2 || body.VideoIds[0] == body.VideoIds[1] {
		s.invalidRequest(w, "videoIds must be two different video ids.")
		return [2]int64{}, false
	}
	return [2]int64{body.VideoIds[0], body.VideoIds[1]}, true
}

func (s *server) DismissVersionCandidate(w http.ResponseWriter, r *http.Request) {
	videoIDs, ok := s.readPairBody(w, r)
	if !ok {
		return
	}
	if s.versions == nil {
		s.internalError(w, "Version storage is not configured.", nil)
		return
	}
	err := s.versions.Dismiss(r.Context(), videoIDs)
	if !s.checkVersionsError(w, err, "Could not record the videos as different.") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func toAPICandidateJudgement(judgement domain.VersionCandidateJudgement) gen.VersionCandidateJudgement {
	state := gen.VersionCandidateJudgementStateWaiting
	if judgement.State != "" {
		state = gen.VersionCandidateJudgementState(judgement.State)
	}
	out := gen.VersionCandidateJudgement{
		State: state, LeftVideoId: judgement.LeftVideoID, Probability: judgement.Probability,
		Evidence: toAPIJudgeEvidence(judgement.Evidence),
	}
	if judgement.ActivityID != 0 {
		id := judgement.ActivityID
		out.ActivityId = &id
	}
	if judgement.Error != "" {
		message := judgement.Error
		out.Error = &message
	}
	if !judgement.RetryAt.IsZero() {
		at := judgement.RetryAt
		out.RetryAt = &at
	}
	return out
}

func toAPIJudgeEvidence(evidence *domain.VersionJudgeEvidence) *gen.VersionJudgeEvidence {
	if evidence == nil {
		return nil
	}
	out := &gen.VersionJudgeEvidence{
		Rule: gen.VersionJudgeEvidenceRule(evidence.Rule), Compared: evidence.Compared, Differing: evidence.Differing,
	}
	if evidence.LongestRun > 0 {
		run, start, end := evidence.LongestRun, evidence.RunStartMs, evidence.RunEndMs
		out.LongestRun, out.RunStartMs, out.RunEndMs = &run, &start, &end
	}
	if evidence.VersionMismatch {
		out.VersionMismatch = &evidence.VersionMismatch
	}
	if evidence.Conflict {
		out.Conflict = &evidence.Conflict
	}
	if evidence.ByProperties {
		out.ByProperties = &evidence.ByProperties
	}
	out.Properties = toAPIPropertySignals(evidence.Properties)
	if len(evidence.Frames) > 0 {
		frames := make([]gen.VersionEvidenceFrame, 0, len(evidence.Frames))
		for _, frame := range evidence.Frames {
			frames = append(frames, gen.VersionEvidenceFrame{AMs: frame.AMs, BMs: frame.BMs, Distance: frame.Distance})
		}
		out.Frames = &frames
	}
	return out
}

func toAPIPropertySignals(signals []domain.VersionPropertySignal) *[]gen.VersionPropertySignal {
	if len(signals) == 0 {
		return nil
	}
	out := make([]gen.VersionPropertySignal, 0, len(signals))
	for _, signal := range signals {
		item := gen.VersionPropertySignal{
			Kind: gen.VersionPropertySignalKind(signal.Kind), Same: signal.Same, A: signal.A, B: signal.B,
		}
		if signal.Strong {
			item.Strong = &signal.Strong
		}
		if signal.Counts {
			item.Counts = &signal.Counts
		}
		out = append(out, item)
	}
	return &out
}

func (s *server) RejudgeVersionCandidate(w http.ResponseWriter, r *http.Request) {
	videoIDs, ok := s.readPairBody(w, r)
	if !ok {
		return
	}
	if s.versionReview == nil {
		s.internalError(w, "Duplicate judging is not configured.", nil)
		return
	}
	err := s.versionReview.Rejudge(r.Context(), videoIDs)
	switch {
	case err == nil:
		w.WriteHeader(http.StatusAccepted)
	case errors.Is(err, domain.ErrVersionJudgingOff):
		s.conflictReason(w, gen.ErrorReasonVersionJudgingOff, "Duplicate judging is turned off.")
	case errors.Is(err, domain.ErrVersionNotRejudgeable):
		s.conflictReason(w, gen.ErrorReasonVersionNotRejudgeable, "The pair is waiting or being judged.")
	case errors.Is(err, domain.ErrNotFound):
		s.notFoundReason(w, reasonVideoNotFound, "Video not found.")
	case errors.Is(err, domain.ErrNotVersionCandidate), errors.Is(err, domain.ErrTooFewVersions):
		s.notFound(w, "The pair is not a possible duplicate.")
	default:
		s.internalError(w, "Could not judge the pair again.", err)
	}
}

func (s *server) GetActivityJudgementFrame(w http.ResponseWriter, r *http.Request, id gen.ActivityId, index int) {
	if s.versionReview == nil {
		s.internalError(w, "Duplicate judging is not configured.", nil)
		return
	}
	image, err := s.versionReview.JudgementFrame(r.Context(), id, index)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		s.notFound(w, "The judgement frame was not found.")
	case err != nil:
		s.internalError(w, "Could not load the judgement frame.", err)
	default:
		s.writeImage(w, "image/jpeg", image)
	}
}
