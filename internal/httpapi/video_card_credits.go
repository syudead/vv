package httpapi

import (
	"context"
	"log/slog"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// videoExtras holds the per-page reads list items share; a nil map means the read was skipped or failed.
type videoExtras struct {
	progress map[string]domain.Progress
	tags     map[string][]domain.VideoTag
	credits  map[string]domain.WorkInfo
}

func (s *server) extrasFor(ctx context.Context, videos []domain.Video) videoExtras {
	return videoExtras{
		progress: s.progressFor(ctx, videos),
		tags:     s.tagsFor(ctx, videos),
		credits:  s.creditsFor(ctx, videos),
	}
}

// item converts one listed video; callers still apply forAudience.
func (e videoExtras) item(view domain.VideoView) gen.Video {
	key := view.Video.UserKey
	item := withTags(withProgress(toAPIVideo(view), e.progress, key), e.tags, key)
	if info, ok := e.credits[key]; ok {
		item.CardCredits = toAPICardCredits(info)
	}
	return item
}

// creditsFor skips guests and tolerates read failures so the list remains available.
func (s *server) creditsFor(ctx context.Context, videos []domain.Video) map[string]domain.WorkInfo {
	if s.workInfo == nil || len(videos) == 0 || !audienceFrom(ctx).IsOwner() {
		return nil
	}
	keys := make([]string, 0, len(videos))
	for _, video := range videos {
		if video.UserKey != "" {
			keys = append(keys, video.UserKey)
		}
	}
	credits, err := s.workInfo.ByUserKeys(ctx, keys)
	if err != nil {
		s.logger.Warn("could not read card credits", slog.Any("error", err))
		return nil
	}
	return credits
}

// toAPICardCredits returns nil when the card has nothing to show.
func toAPICardCredits(info domain.WorkInfo) *gen.VideoCardCredits {
	out := gen.VideoCardCredits{Code: optionalString(info.Code), Performers: []gen.WorkPerson{}}
	for _, person := range info.People {
		if person.Role == domain.PersonPerformer {
			out.Performers = append(out.Performers, toAPIWorkPerson(person))
		}
	}
	if organization, ok := cardOrganization(info.Organizations); ok {
		ref := toAPIOrganizationRef(organization)
		out.Organization = &ref
	}
	if out.Code == nil && len(out.Performers) == 0 && out.Organization == nil {
		return nil
	}
	return &out
}

func cardOrganization(organizations []domain.WorkOrganization) (domain.CreditRef, bool) {
	for _, role := range []domain.OrganizationRole{domain.OrganizationLabel, domain.OrganizationMaker} {
		for _, organization := range organizations {
			if organization.Role == role {
				return organization.Organization, true
			}
		}
	}
	return domain.CreditRef{}, false
}
