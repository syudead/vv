package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// measureAll は足された Go と Vitest のテストだけを流し、所要時間を書き込む。
// Playwright のテストは Go のサーバーとブラウザが要るので測らない。
func measureAll(tests []*addedTest) error {
	if err := measureGo(tests); err != nil {
		return err
	}
	return measureWeb(tests)
}

// measureGo はパッケージごとに、足されたテストだけを -count=1 で流す。
func measureGo(tests []*addedTest) error {
	byPackage := map[string][]*addedTest{}
	for _, t := range tests {
		if t.Package != "" {
			byPackage[t.Package] = append(byPackage[t.Package], t)
		}
	}
	for pkg, group := range byPackage {
		names := make([]string, 0, len(group))
		for _, t := range group {
			names = append(names, regexp.QuoteMeta(t.Name))
		}
		cmd := exec.Command("go", "test", "-count=1", "-json", "-run", "^("+strings.Join(names, "|")+")$", "./"+filepath.ToSlash(pkg))
		cmd.Stderr = os.Stderr
		// テストの失敗は所要時間の計測を止めない。結果に名前が無ければ not found になる。
		out, _ := cmd.Output()
		applyDurations(group, goDurations(out))
	}
	return nil
}

// goDurations は go test -json の出力から、トップレベルのテストの秒数を返す。
func goDurations(out []byte) map[string]float64 {
	durations := map[string]float64{}
	scanner := bufio.NewScanner(bytes.NewReader(out))
	scanner.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	for scanner.Scan() {
		var event struct {
			Action  string
			Test    string
			Elapsed float64
		}
		if json.Unmarshal(scanner.Bytes(), &event) != nil {
			continue
		}
		if (event.Action == "pass" || event.Action == "fail") && event.Test != "" && !strings.Contains(event.Test, "/") {
			durations[event.Test] = event.Elapsed
		}
	}
	return durations
}

// measureWeb は足されたテストを持つ Vitest のファイルを流す。ファイルの中の他の
// テストも流れるが、秒数は足されたテストの分だけを拾う。
func measureWeb(tests []*addedTest) error {
	files := map[string]bool{}
	var group []*addedTest
	for _, t := range tests {
		if t.Package == "" && t.Level != levelE2E {
			files[strings.TrimPrefix(t.File, "web/")] = true
			group = append(group, t)
		}
	}
	if len(group) == 0 {
		return nil
	}
	report, err := os.CreateTemp("", "testcost-*.json")
	if err != nil {
		return err
	}
	_ = report.Close()
	defer func() { _ = os.Remove(report.Name()) }()

	args := []string{"run", "test:unit", "--", "--reporter=json", "--outputFile=" + report.Name()}
	for file := range files {
		args = append(args, file)
	}
	// vitest の設定（別名の解決など）は web/ から起動したときだけ読まれる。
	cmd := exec.Command("npm", args...)
	cmd.Dir = "web"
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	// 失敗したテストも秒数は出るので、終了コードでは止めない。
	_ = cmd.Run()
	body, err := os.ReadFile(report.Name())
	if err != nil {
		return fmt.Errorf("vitest wrote no report: %w", err)
	}
	durations, err := vitestDurations(body)
	if err != nil {
		return err
	}
	applyDurations(group, durations)
	return nil
}

// vitestDurations は Vitest の JSON 出力から、テストの名前（describe を除いた題）ごとの
// 秒数を返す。同じ題が複数あれば合計する。
func vitestDurations(body []byte) (map[string]float64, error) {
	var report struct {
		TestResults []struct {
			Name             string
			Message          string
			AssertionResults []struct {
				Title    string
				Duration float64
			}
		}
	}
	if err := json.Unmarshal(body, &report); err != nil {
		return nil, fmt.Errorf("cannot read the vitest report: %w", err)
	}
	durations := map[string]float64{}
	for _, file := range report.TestResults {
		if file.Message != "" {
			fmt.Fprintf(os.Stderr, "testcost: %s did not run: %s\n", file.Name, file.Message)
		}
		for _, result := range file.AssertionResults {
			durations[result.Title] += result.Duration / 1000
		}
	}
	return durations, nil
}

func applyDurations(tests []*addedTest, durations map[string]float64) {
	for _, t := range tests {
		d, ok := durations[t.Name]
		t.Seconds = d
		t.Missing = !ok
	}
}
