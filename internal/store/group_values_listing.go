package store

import (
	"strconv"

	"github.com/syudead/vv/internal/domain"
)

func storedGroupListSQL(roots string, audience domain.Audience) groupListSQL {
	a := strconv.Itoa(audienceValue(audience))
	return groupListSQL{
		members: `gm as (select group_id, video_id, position from group_members
			where audience = ` + a + ` and group_id in (` + roots + `))`,
		total: `(select video_count from group_values where group_id = gm.group_id and audience = ` + a + `)`,
		carded: `select gm.video_id from gm left join whole on whole.group_id = gm.group_id
			group by gm.video_id having count(whole.group_id) = count(*)`,
		values: `mv as (select v.* from group_values v join whole on whole.group_id = v.group_id where v.audience = ` + a + `)`,
		items: `select g.id, g.id * 2 + 1, null, null,
			mv.added_at, mv.mtime, mv.created_at, g.title_key, mv.duration_ms, mv.size_bytes, mv.played_at, ` + groupFavoritedAtExpr("g") + `
			from mv join groups g on g.id = mv.group_id`,
	}
}

func storedWholeGroupsSQL(spec listSpec, roots string) string {
	return `select group_id from group_values where audience = ` + strconv.Itoa(audienceValue(spec.scope.audience)) +
		` and video_count > 0 and group_id in (` + roots + `)`
}
