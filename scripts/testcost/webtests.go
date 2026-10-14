package main

import (
	"os"
	"regexp"
)

var (
	// webTestDecl は足された行のうち、Vitest と Playwright のテストの宣言に一致する。
	// it.each(...)("名前") や test.skip("名前") も含める。
	webTestDecl = regexp.MustCompile("^\\s*(?:it|test)(?:\\.\\w+)*(?:\\(.*?\\))?\\(\\s*([\"'`])(.+?)[\"'`]")
	// pageImport は *Page のモジュール（ページの部品か web/src/testing のページ用の補助）を
	// 読み込む行に一致する。
	pageImport = regexp.MustCompile(`from\s+["'][^"']*[A-Za-z]Page["']`)
	// componentRender は部品を描画する呼び出しに一致する。renderHook は含めない。
	componentRender = regexp.MustCompile(`\brender\(`)
)

// webTests は 1 つの Vitest のファイルで足されたテストを、ファイルが何を描画するかで
// レベルを付けて返す。
func webTests(file string, lines []string) ([]*addedTest, error) {
	names := webTestNames(lines)
	if len(names) == 0 {
		return nil, nil
	}
	body, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	lvl := webLevel(string(body))
	tests := make([]*addedTest, 0, len(names))
	for _, name := range names {
		tests = append(tests, &addedTest{File: file, Name: name, Level: lvl})
	}
	return tests, nil
}

// e2eTests は Playwright のファイルで足されたテストを返す。
func e2eTests(file string, lines []string) []*addedTest {
	var tests []*addedTest
	for _, name := range webTestNames(lines) {
		tests = append(tests, &addedTest{File: file, Name: name, Level: levelE2E})
	}
	return tests
}

func webTestNames(lines []string) []string {
	var names []string
	for _, line := range lines {
		if m := webTestDecl.FindStringSubmatch(line); m != nil {
			names = append(names, m[2])
		}
	}
	return names
}

// webLevel はファイルの中身からレベルを決める（docs/design-docs/web-testing.md）。
func webLevel(body string) level {
	switch {
	case pageImport.MatchString(body):
		return levelPage
	case componentRender.MatchString(body):
		return levelComponent
	default:
		return levelLogic
	}
}
