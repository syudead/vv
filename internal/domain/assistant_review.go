package domain

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// AssistantReviewKind is what a review run tidies up.
type AssistantReviewKind string

const (
	AssistantReviewTags   AssistantReviewKind = "tags"
	AssistantReviewPeople AssistantReviewKind = "people"
	AssistantReviewWorks  AssistantReviewKind = "works"
)

// Valid is true for the three kinds an organize outcome names.
func (k AssistantReviewKind) Valid() bool {
	return k == AssistantReviewTags || k == AssistantReviewPeople || k == AssistantReviewWorks
}

// AssistantReviewStatus is a run's state; a run moves from running to one of the others once.
type AssistantReviewStatus string

const (
	AssistantReviewRunning     AssistantReviewStatus = "running"
	AssistantReviewDone        AssistantReviewStatus = "done"
	AssistantReviewFailed      AssistantReviewStatus = "failed"
	AssistantReviewInterrupted AssistantReviewStatus = "interrupted"
)

// AssistantReviewRun is one organize request's search for candidate sets and their judging.
type AssistantReviewRun struct {
	ID      int64
	Kind    AssistantReviewKind
	Request string
	Status  AssistantReviewStatus
	// Total sets found, Judged so far, and Listed at or above AssistantReviewThreshold.
	Total, Judged, Listed int
	Error                 string
	StartedAt, FinishedAt time.Time
}

// AssistantReviewThreshold is the lowest probability a set is listed with; a set below it is dropped.
const AssistantReviewThreshold = 0.5

// AssistantReviewTagLimit bounds the tags searched to the most used ones.
const AssistantReviewTagLimit = 2000

// AssistantReviewFactTitles is how many titles of each member the classifier reads.
const AssistantReviewFactTitles = 3

// assistantReviewWorkFacts bounds the videos of a works set the classifier reads.
const assistantReviewWorkFacts = 10

// AssistantReviewCandidateState is what the owner did with a listed set.
type AssistantReviewCandidateState string

const (
	AssistantCandidateListed   AssistantReviewCandidateState = "listed"
	AssistantCandidateAdopted  AssistantReviewCandidateState = "adopted"
	AssistantCandidateRejected AssistantReviewCandidateState = "rejected"
)

// ErrAssistantCandidateNotListed refuses adopting or rejecting a set that is no longer listed.
var ErrAssistantCandidateNotListed = errors.New("the review candidate is not listed")

// AssistantReviewMember is one member of a set: a tag or person by id, or a video by user key. The JSON form is
// what the candidate row stores; Count, Aliases, Titles and Folder are read for judging and listing only.
type AssistantReviewMember struct {
	Kind AssistantLabelKind `json:"kind"`
	ID   int64              `json:"id,omitempty"`
	Key  string             `json:"key,omitempty"`
	Name string             `json:"name"`
	// Count is the videos a tag is on or a person is credited on.
	Count   int      `json:"-"`
	Aliases []string `json:"-"`
	Titles  []string `json:"-"`
	Folder  string   `json:"-"`
}

// AssistantReviewSet is a group of members a finder proposes as one entity or one work.
type AssistantReviewSet struct {
	Members []AssistantReviewMember
}

// SetKey is the members' ids, or user keys for videos, sorted and joined: the dismissal key.
func (s AssistantReviewSet) SetKey() string {
	parts := make([]string, len(s.Members))
	for i, m := range s.Members {
		parts[i] = m.Key
		if m.Kind != AssistantLabelVideo {
			parts[i] = strconv.FormatInt(m.ID, 10)
		}
	}
	slices.Sort(parts)
	return strings.Join(parts, ",")
}

// AssistantReviewQuestionKey is the key of the one question a set is judged by.
const AssistantReviewQuestionKey = "same"

var assistantReviewQuestions = map[AssistantReviewKind]string{
	AssistantReviewTags: "Do tag_a and tag_b, read with their synonyms and the titles of videos carrying them, " +
		"name one concept?",
	AssistantReviewPeople: "Are person_a and person_b, read with their aliases and the titles of their works, " +
		"the same person?",
	AssistantReviewWorks: "Are these videos, read from their titles and folders, parts of one work?",
}

// ClassifierRequest asks the one yes/no question of the kind from the members' facts only.
func (s AssistantReviewSet) ClassifierRequest(kind AssistantReviewKind) ClassifierRequest {
	facts := map[string]string{}
	if kind == AssistantReviewWorks {
		for i, m := range s.Members[:min(len(s.Members), assistantReviewWorkFacts)] {
			n := strconv.Itoa(i + 1)
			facts["title_"+n] = m.Name
			if m.Folder != "" {
				facts["folder_"+n] = m.Folder
			}
		}
	} else {
		prefix, other := "tag", "synonyms"
		if kind == AssistantReviewPeople {
			prefix, other = "person", "aliases"
		}
		for i, m := range s.Members[:min(len(s.Members), 2)] {
			side := string(rune('a' + i))
			facts[prefix+"_"+side] = m.Name
			if len(m.Aliases) > 0 {
				facts[other+"_"+side] = strings.Join(m.Aliases, "; ")
			}
			if len(m.Titles) > 0 {
				facts["titles_"+side] = strings.Join(m.Titles, "; ")
			}
		}
	}
	return ClassifierRequest{
		Input:     ClassifierInput{Facts: facts},
		Questions: []ClassifierQuestion{{Key: AssistantReviewQuestionKey, Text: assistantReviewQuestions[kind]}},
	}
}

// AssistantNamesNear reports whether two tag names fold to the same key, or are one or two edits apart: one for
// names of 4 or 5 characters, two when the longer has 6 or more. Shorter names must fold equal.
func AssistantNamesNear(a, b string) bool {
	a, b = FoldForMatch(a), FoldForMatch(b)
	if a == "" || b == "" {
		return false
	}
	if a == b {
		return true
	}
	ra, rb := []rune(a), []rune(b)
	shorter, longer := min(len(ra), len(rb)), max(len(ra), len(rb))
	if shorter < 4 {
		return false
	}
	allowed := 1
	if longer >= 6 {
		allowed = 2
	}
	return longer-shorter <= allowed && editDistance(ra, rb, allowed) <= allowed
}

// editDistance is the Levenshtein distance, cut off at limit+1.
func editDistance(a, b []rune, limit int) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		best := cur[0]
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
			best = min(best, cur[j])
		}
		if best > limit {
			return limit + 1
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

// AssistantNamedEntity is a tag or person with its folded-comparable names, for the tag pair finder.
type AssistantNamedEntity struct {
	ID int64
	// Names holds the display name first and then its synonyms or aliases.
	Names []string
}

// AssistantNearPairs returns the index pairs of entities with an equal folded name, or with names
// AssistantNamesNear accepts, in input order.
func AssistantNearPairs(entities []AssistantNamedEntity) [][2]int {
	type folded struct {
		owner int
		runes int
		key   string
	}
	var names []folded
	for i, e := range entities {
		for _, name := range e.Names {
			if key := FoldForMatch(name); key != "" {
				names = append(names, folded{owner: i, runes: utf8.RuneCountInString(key), key: key})
			}
		}
	}
	// Sorting by length lets the scan stop once the lengths differ by more than two.
	slices.SortStableFunc(names, func(a, b folded) int { return cmp.Compare(a.runes, b.runes) })
	seen := map[[2]int]bool{}
	for i, a := range names {
		for _, b := range names[i+1:] {
			if b.runes-a.runes > 2 {
				break
			}
			if a.owner == b.owner || !AssistantNamesNear(a.key, b.key) {
				continue
			}
			seen[[2]int{min(a.owner, b.owner), max(a.owner, b.owner)}] = true
		}
	}
	pairs := make([][2]int, 0, len(seen))
	for pair := range seen {
		pairs = append(pairs, pair)
	}
	slices.SortFunc(pairs, func(a, b [2]int) int {
		return cmp.Or(cmp.Compare(a[0], b[0]), cmp.Compare(a[1], b[1]))
	})
	return pairs
}

// assistantWorkSuffix matches a trailing number with an optional part marker and the separators before it.
var assistantWorkSuffix = regexp.MustCompile(
	`(?i)[\s\-_.,#:()\[\]]*(?:(?:part|pt|vol|disc|cd|ep|no)\.?[\s\-_.#]*)?\d+[\s\-_.)\]]*$`)

// AssistantWorkTitle strips a trailing number or part marker from a title: "title part 2" and "title 1" are both
// "title". A title that is nothing but the number is kept whole.
func AssistantWorkTitle(title string) string {
	title = strings.TrimRightFunc(title, unicode.IsSpace)
	loc := assistantWorkSuffix.FindStringIndex(title)
	if loc == nil || loc[0] == 0 {
		return title
	}
	return strings.TrimRightFunc(title[:loc[0]], unicode.IsSpace)
}

// AssistantWorkKey is the folded work title two parts of one work share.
func AssistantWorkKey(title string) string {
	return FoldForMatch(AssistantWorkTitle(FoldForMatch(title)))
}

// AssistantWorkVideo is an ungrouped library video the works finder reads.
type AssistantWorkVideo struct {
	Key    string
	Title  string
	Folder string
	// CodeKey is the folded work code; empty when the video has none.
	CodeKey string
}

// AssistantWorkSets groups videos sharing a work code, or whose folded titles differ only in a trailing number or
// part marker; overlapping groups join into one set. Copies of one title without a code are not a set.
func AssistantWorkSets(videos []AssistantWorkVideo) []AssistantReviewSet {
	parent := make([]int, len(videos))
	for i := range parent {
		parent[i] = i
	}
	find := func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	union := func(a, b int) { parent[find(b)] = find(a) }
	firstByCode, firstByTitle := map[string]int{}, map[string]int{}
	titlesByKey := map[string]map[string]bool{}
	for i, v := range videos {
		if v.CodeKey != "" {
			if first, ok := firstByCode[v.CodeKey]; ok {
				union(first, i)
			} else {
				firstByCode[v.CodeKey] = i
			}
		}
		key := AssistantWorkKey(v.Title)
		if key == "" {
			continue
		}
		if titlesByKey[key] == nil {
			titlesByKey[key] = map[string]bool{}
		}
		titlesByKey[key][FoldForMatch(v.Title)] = true
	}
	for i, v := range videos {
		key := AssistantWorkKey(v.Title)
		if key == "" || len(titlesByKey[key]) < 2 {
			continue
		}
		if first, ok := firstByTitle[key]; ok {
			union(first, i)
		} else {
			firstByTitle[key] = i
		}
	}
	byRoot := map[int][]AssistantReviewMember{}
	var roots []int
	for i, v := range videos {
		root := find(i)
		if _, ok := byRoot[root]; !ok {
			roots = append(roots, root)
		}
		byRoot[root] = append(byRoot[root], AssistantReviewMember{
			Kind: AssistantLabelVideo, Key: v.Key, Name: v.Title, Folder: v.Folder,
		})
	}
	var sets []AssistantReviewSet
	for _, root := range roots {
		if members := byRoot[root]; len(members) >= 2 {
			sets = append(sets, AssistantReviewSet{Members: members})
		}
	}
	return sets
}

// AssistantReviewDestination picks the member a tags or people set merges into: the one with the larger count,
// and the older id on a tie.
func AssistantReviewDestination(members []AssistantReviewMember) int {
	best := 0
	for i, m := range members {
		if m.Count > members[best].Count || (m.Count == members[best].Count && m.ID < members[best].ID) {
			best = i
		}
	}
	return best
}

// assistantGroupNameMax is the longest group name createGroup takes.
const assistantGroupNameMax = 100

// AssistantReviewPlan is the one operation adopting a set runs: mergeTag or mergePeople into the destination, or
// createGroup of the videos named by their common title. The members carry their current names and counts.
func AssistantReviewPlan(kind AssistantReviewKind, members []AssistantReviewMember) (AssistantPlan, error) {
	switch kind {
	case AssistantReviewTags, AssistantReviewPeople:
		return assistantMergePlan(kind, members), nil
	case AssistantReviewWorks:
		return assistantGroupPlan(members), nil
	default:
		return AssistantPlan{}, fmt.Errorf("unknown review kind %q", kind)
	}
}

func assistantMergePlan(kind AssistantReviewKind, members []AssistantReviewMember) AssistantPlan {
	actionID, target, label := "mergeTag", AssistantTargetTag, AssistantLabelTag
	if kind == AssistantReviewPeople {
		actionID, target, label = "mergePeople", AssistantTargetPerson, AssistantLabelPerson
	}
	dest := AssistantReviewDestination(members)
	var sources []AssistantLabel
	var sourceIDs []int64
	for i, m := range members {
		if i != dest {
			sources = append(sources, AssistantLabel{Kind: label, ID: m.ID, Name: m.Name})
			sourceIDs = append(sourceIDs, m.ID)
		}
	}
	return AssistantPlan{
		ActionID: actionID,
		Targets: AssistantPlanTargets{Kind: target, Labels: []AssistantLabel{
			{Kind: label, ID: members[dest].ID, Name: members[dest].Name},
		}},
		Change: []AssistantChange{{Argument: AssistantArgNames, Labels: sources}},
		Call: AssistantPlanCall{
			Path: map[string]string{"id": strconv.FormatInt(members[dest].ID, 10)},
			Body: map[string]any{"sourceIds": sourceIDs},
		},
	}
}

func assistantGroupPlan(members []AssistantReviewMember) AssistantPlan {
	name := AssistantWorkTitle(members[0].Name)
	if name == "" {
		name = members[0].Name
	}
	if runes := []rune(name); len(runes) > assistantGroupNameMax {
		name = strings.TrimRightFunc(string(runes[:assistantGroupNameMax]), unicode.IsSpace)
	}
	plan := AssistantPlan{
		ActionID: "createGroup",
		Targets:  AssistantPlanTargets{Kind: AssistantTargetVideos},
		Call:     AssistantPlanCall{Body: map[string]any{"name": name}, VideoField: "videoIds"},
	}
	for _, m := range members {
		plan.Targets.VideoKeys = append(plan.Targets.VideoKeys, m.Key)
		if len(plan.Targets.Labels) < assistantCardVideoLabels {
			plan.Targets.Labels = append(plan.Targets.Labels, AssistantLabel{Kind: AssistantLabelVideo, Key: m.Key, Name: m.Name})
		}
	}
	// A string always encodes.
	value, _ := json.Marshal(name)
	plan.Change = []AssistantChange{{Argument: AssistantArgName, Value: value}}
	return plan
}

// assistantCardVideoLabels is how many video labels a plan keeps for the card and the stale check.
const assistantCardVideoLabels = 10

// AssistantReviewCandidate is one listed set of a run. Its members carry their current names and counts when read
// for the list or an adoption; Gone is true when a member no longer exists.
type AssistantReviewCandidate struct {
	ID          int64
	RunID       int64
	Members     []AssistantReviewMember
	SetKey      string
	Probability float64
	State       AssistantReviewCandidateState
	ProposalID  int64
	Gone        bool
}

// AssistantReviewPage is a run and one page of its candidates.
type AssistantReviewPage struct {
	Run        AssistantReviewRun
	Items      []AssistantReviewCandidate
	NextCursor string
}
