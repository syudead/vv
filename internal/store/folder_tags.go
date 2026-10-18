package store

import "github.com/syudead/vv/internal/domain"

// Derive folder tags by exact name or synonym matches on every read; unidentified content has no tags from either source.

// Pass the tag ID twice, once for manual attachments and once for folder-derived tags.
func videoHasTagCondition(alias, userKey string) string {
	return videoTagSourcesCondition(alias, userKey, func(tagIDColumn string) string { return tagIDColumn + ` = ?` })
}

// Pass the kind ID twice, once for each tag source.
func videoHasTagOfKindCondition(alias, userKey string) string {
	return videoTagSourcesCondition(alias, userKey, func(tagIDColumn string) string {
		return tagIDColumn + ` in (select id from tags where kind_id = ?)`
	})
}

// Use uncorrelated IN sets once per query to avoid repeated per-video scans; CROSS JOIN fixes folder-name lookup order.
func videoTagSourcesCondition(alias, userKey string, tagMatch func(tagIDColumn string) string) string {
	return `(` + alias + `.content_key <> '' and (` + userKey +
		` in (select vt.content_key from video_tags vt where ` + tagMatch("vt.tag_id") + `) ` +
		`or ` + alias + `.id in (select vfn.video_id from tag_names tn cross join video_folder_names vfn on vfn.name = tn.name ` +
		`where ` + tagMatch("tn.tag_id") + `)))`
}

// Return distinct visible representative/tag pairs from both sources; append extra predicates and their arguments twice.
func taggedVideosSQL(extra string) string {
	return `select vt.tag_id as tag_id, v.id as video_id from videos v
		join video_tags vt on vt.content_key = ` + userKeyExpr("v") + `
		where v.content_key <> '' and ` + registeredVideoCondition("v") + ` and ` + shownVideoCondition("v", domain.AudienceOwner) + extra + `
		union
		select tn.tag_id as tag_id, v.id as video_id from video_folder_names vfn
		join tag_names tn on tn.name = vfn.name
		join videos v on v.id = vfn.video_id and v.content_key <> ''
		where ` + registeredVideoCondition("v") + ` and ` + shownVideoCondition("v", domain.AudienceOwner) + extra
}
