package store

import (
	"strings"
	"unicode/utf8"

	"github.com/syudead/vv/internal/domain"
)

// Build predicates for one location; listing.go owns scope and per-video grouping.

// Trigram MATCH needs at least three normalized code points; shorter terms use instr.
const matchMinLength = 3

func termUsesMatch(text string) bool {
	return utf8.RuneCountInString(text) >= matchMinLength
}

// Use MATCH for long terms and literal instr for short terms; guests search locations only to avoid leaking private tags or work information.
func searchExprCondition(expr domain.SearchExpr, alias string, audience domain.Audience) (string, []any) {
	if expr.Empty() {
		return "", nil
	}
	var args []any
	clauses := make([]string, 0, len(expr.Clauses))
	for _, clause := range expr.Clauses {
		terms := make([]string, 0, len(clause.Terms))
		for _, term := range clause.Terms {
			// Normalize phrase newlines to spaces so a term cannot cross the search key's title/path boundary.
			text := strings.ReplaceAll(term.Text, "\n", " ")
			var locationCondition string
			if termUsesMatch(text) {
				locationCondition = alias + `.id in (select rowid from location_search_fts where location_search_fts match ?)`
				args = append(args, quoteMatchPhrase(text))
			} else {
				locationCondition = `instr(` + alias + `.search_key, ?) > 0`
				args = append(args, text)
			}
			condition := `(` + locationCondition + `)`
			if audience.IsOwner() {
				condition = `(` + locationCondition + ` or ` + ownerFactsMatchCondition(alias) + `)`
				// Bind the same term nine times for both user-key sets and the folder-tag source.
				args = append(args, text, text, text, text, text, text, text, text, text)
			}
			if term.Negated {
				condition = `not ` + condition
			}
			terms = append(terms, condition)
		}
		if len(terms) == 1 {
			clauses = append(clauses, terms[0])
		} else {
			clauses = append(clauses, `(`+strings.Join(terms, " or ")+`)`)
		}
	}
	return strings.Join(clauses, " and "), args
}

// Resolve matching owner facts to video sets once, avoiding per-location user-key lookups; exclude unidentified content and never search private facts for guests.
func ownerFactsMatchCondition(alias string) string {
	keys := `select vt.content_key from video_tags vt join tag_names tn on tn.tag_id = vt.tag_id where instr(tn.search_key, ?) > 0 ` +
		`union select wi.content_key from video_work_info wi where instr(wi.code_key, ?) > 0 ` +
		`union select vp.content_key from video_work_people vp ` +
		`join people_names pn on pn.person_id = vp.person_id where instr(pn.search_key, ?) > 0 ` +
		`union select vo.content_key from video_work_organizations vo ` +
		`join organization_names onm on onm.organization_id = vo.organization_id where instr(onm.search_key, ?) > 0`
	return `(` + alias + `.video_id in (` + userKeyVideosSQL(keys) + `) ` +
		`or ` + alias + `.video_id in (select vfn.video_id from tag_names tn ` +
		`cross join tag_names folder_tn on folder_tn.tag_id = tn.tag_id ` +
		`cross join video_folder_names vfn on vfn.name = folder_tn.name ` +
		`join videos v on v.id = vfn.video_id and v.content_key <> '' ` +
		`where instr(tn.search_key, ?) > 0))`
}

// Quote the entire FTS phrase and double embedded quotes so user punctuation cannot become query syntax.
func quoteMatchPhrase(text string) string {
	return `"` + strings.ReplaceAll(text, `"`, `""`) + `"`
}
