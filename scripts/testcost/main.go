// testcost は、ブランチが足したテストを、そのレベル（logic・store・http・media・
// component・page・e2e）と所要時間とともに Markdown の表で出す。task test-cost が呼ぶ。
//
// テストはパッケージやファイルの中で順に流れ、CI の時間は最も遅い組で決まる。
// 新しいテストが一つ上のレベルで書かれると、その差がそのまま CI の時間に乗る
// （docs/design-docs/go-testing.md・web-testing.md）。この表は、上のレベルの
// テストごとに「一つ下では落ちない理由」を PR に書くための一覧で、失敗にはしない。
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	base := flag.String("base", "origin/main", "the ref the branch is compared with (through its merge base)")
	measure := flag.Bool("measure", false, "run the added Go and Vitest tests and report their durations")
	flag.Parse()

	tests, err := collect(*base)
	if err != nil {
		fmt.Fprintf(os.Stderr, "testcost: %v\n", err)
		os.Exit(2)
	}
	if *measure {
		if err := measureAll(tests); err != nil {
			fmt.Fprintf(os.Stderr, "testcost: %v\n", err)
			os.Exit(2)
		}
	}
	fmt.Print(render(tests, *measure))
}

// collect は base との差分から足されたテストを集め、レベルを付ける。
func collect(base string) ([]*addedTest, error) {
	added, err := addedLines(base)
	if err != nil {
		return nil, err
	}
	var tests []*addedTest
	for _, file := range sortedKeys(added) {
		found, err := testsInFile(file, added[file])
		if err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		tests = append(tests, found...)
	}
	return tests, nil
}

// testsInFile は 1 つのファイルで足されたテストを、ファイルの種類ごとに読む。
func testsInFile(file string, lines []string) ([]*addedTest, error) {
	switch kindOf(file) {
	case goTestFile:
		return goTests(file, lines)
	case webUnitFile:
		return webTests(file, lines)
	case e2eFile:
		return e2eTests(file, lines), nil
	default:
		return nil, nil
	}
}
