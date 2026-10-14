package main

import (
	"fmt"
	"sort"
	"strings"
)

// level はテストのレベルである。値が大きいほど 1 件あたりが重い。
type level int

const (
	levelLogic level = iota
	levelComponent
	levelStore
	levelHTTP
	levelPage
	levelMedia
	levelE2E
)

func (l level) String() string {
	return [...]string{"logic", "component", "store", "http", "page", "media", "e2e"}[l]
}

// needsReason は、そのレベルのテストに「一つ下では落ちない理由」が要るかを返す。
func (l level) needsReason() bool {
	return l >= levelStore
}

// addedTest はブランチが足した 1 件のテストである。
type addedTest struct {
	File    string
	Name    string
	Package string // Go のテストだけ。go test に渡すディレクトリ。
	Level   level
	Seconds float64 // -measure で測れたときだけ 0 より大きい。
	Missing bool    // -measure で結果に名前が見つからなかった。
}

// render はテストを重いレベルから並べた Markdown の表にする。
func render(tests []*addedTest, measured bool) string {
	if len(tests) == 0 {
		return "No tests added.\n"
	}
	sorted := append([]*addedTest(nil), tests...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Level > sorted[j].Level })

	var b strings.Builder
	b.WriteString("| Level | Test | File | Seconds |\n| --- | --- | --- | --- |\n")
	for _, t := range sorted {
		fmt.Fprintf(&b, "| %s | %s | `%s` | %s |\n", t.Level, escape(t.Name), t.File, seconds(t, measured))
	}
	b.WriteString("\n")
	b.WriteString(summary(sorted, measured))
	return b.String()
}

// summary はレベルごとの件数と合計の秒数、理由が要る件数を書く。
func summary(tests []*addedTest, measured bool) string {
	count := map[level]int{}
	total := map[level]float64{}
	reasons := 0
	for _, t := range tests {
		count[t.Level]++
		total[t.Level] += t.Seconds
		if t.Level.needsReason() {
			reasons++
		}
	}
	var parts []string
	for l := levelE2E; l >= levelLogic; l-- {
		if count[l] == 0 {
			continue
		}
		part := fmt.Sprintf("%s %d", l, count[l])
		if measured && l != levelE2E {
			part += fmt.Sprintf(" (%.2f s)", total[l])
		}
		parts = append(parts, part)
	}
	line := "Added: " + strings.Join(parts, ", ") + ".\n"
	if reasons > 0 {
		line += fmt.Sprintf("%d test(s) at store, http, page, media or e2e level: state in the pull request why the level below would not fail (docs/design-docs/go-testing.md, docs/design-docs/web-testing.md).\n", reasons)
	}
	return line
}

func seconds(t *addedTest, measured bool) string {
	switch {
	case !measured || t.Level == levelE2E:
		return "-"
	case t.Missing:
		return "not found"
	default:
		return fmt.Sprintf("%.2f", t.Seconds)
	}
}

func escape(s string) string {
	return strings.ReplaceAll(s, "|", "\\|")
}
