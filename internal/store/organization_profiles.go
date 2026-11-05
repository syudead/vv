package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// An organization's profile is columns on organizations plus ordered rows in organization_links.

var organizationProfileColumns = []string{"founded_date", "dissolved_date", "location", "bio"}

var organizationProfileCodec = profileCodec{
	columns: organizationProfileColumns,
	scan: func(c *domain.Credit) ([]any, func()) {
		var texts [4]sql.NullString
		return []any{&texts[0], &texts[1], &texts[2], &texts[3]}, func() {
			c.OrganizationProfile = domain.OrganizationProfile{
				FoundedDate: texts[0].String, DissolvedDate: texts[1].String, Location: texts[2].String,
				Bio: texts[3].String,
			}
		}
	},
	links: func(c *domain.Credit) *[]string { return &c.OrganizationProfile.Links },
	write: func(ctx context.Context, tx *sql.Tx, id int64, input domain.CreditInput) error {
		p := input.OrganizationProfile
		args := []any{nullString(p.FoundedDate), nullString(p.DissolvedDate), nullString(p.Location), nullString(p.Bio)}
		return writeCreditProfile(ctx, tx, creditTablesByKind[domain.CreditOrganization], id,
			organizationProfileColumns, args, p.Links)
	},
	set: func(input domain.CreditInput) bool {
		return !input.OrganizationProfile.Equal(domain.OrganizationProfile{})
	},
	differs: func(c domain.Credit, next domain.CreditInput) bool {
		return !c.OrganizationProfile.Equal(next.OrganizationProfile)
	},
	fill: func(target domain.Credit, sources []domain.Credit) (domain.CreditInput, bool) {
		profiles := make([]domain.OrganizationProfile, len(sources))
		for i, source := range sources {
			profiles[i] = source.OrganizationProfile
		}
		filled := domain.FillOrganizationProfile(target.OrganizationProfile, profiles, time.Now())
		return domain.CreditInput{OrganizationProfile: filled}, !filled.Equal(target.OrganizationProfile)
	},
}
