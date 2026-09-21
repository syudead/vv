package main

import (
	"errors"
	"strings"
	"testing"
)

// stubRun は外部コマンドの代わりに決まった応答を返す。実際の PATH に依存すると
// 検査そのものの正しさを確かめられないため。
func stubRun(responses map[string]string, failures map[string]bool) firstLineFunc {
	return func(name string, args ...string) (string, error) {
		if failures[name] {
			return "", errors.New("実行できません")
		}
		return responses[name], nil
	}
}

func TestInspectDetectsUnusableTool(t *testing.T) {
	missing := inspect(tool{name: "go", required: true}, stubRun(nil, map[string]bool{"go": true}))
	if missing.ok {
		t.Error("起動できないコマンドを ok と判定した")
	}

	silent := inspect(tool{name: "go", required: true}, stubRun(map[string]string{"go": ""}, nil))
	if silent.ok {
		t.Error("版を何も返さないコマンドを ok と判定した")
	}

	healthy := inspect(tool{name: "go", required: true}, stubRun(map[string]string{"go": "go version go1.26.0"}, nil))
	if !healthy.ok || healthy.detail != "go version go1.26.0" {
		t.Errorf("版を読めたのに ok=%v detail=%q", healthy.ok, healthy.detail)
	}
}

func TestReportFailsOnlyForRequiredTools(t *testing.T) {
	_, code := report([]result{
		{tool: tool{name: "docker", required: false}, detail: "見つかりません"},
		{tool: tool{name: "go", required: true}, ok: true, detail: "go version go1.26.0"},
	})
	if code != 0 {
		t.Errorf("任意のコマンドが欠けただけで終了コード %d を返した", code)
	}

	out, code := report([]result{{tool: tool{name: "go", required: true}, detail: "見つかりません"}})
	if code != 1 {
		t.Errorf("必須のコマンドが欠けたのに終了コード %d を返した", code)
	}
	if !strings.Contains(out, "[ERR ] go") {
		t.Errorf("どのコマンドが欠けたか出力から分からない:\n%s", out)
	}
}

func TestInspectAllReportsDockerDaemon(t *testing.T) {
	list := []tool{{name: "docker", versionArgs: []string{"--version"}}}

	reachable := inspectAll(list, stubRun(map[string]string{"docker": "Docker version 29.3.1"}, nil))
	if !strings.Contains(reachable[0].detail, "daemon Docker version 29.3.1") {
		t.Errorf("デーモンの版を出していない: %q", reachable[0].detail)
	}

	stopped := inspectAll(list, func(name string, args ...string) (string, error) {
		if len(args) > 0 && args[0] == "info" {
			return "", errors.New("daemon 停止中")
		}
		return "Docker version 29.3.1", nil
	})
	if !strings.Contains(stopped[0].detail, "デーモンへ到達できません") {
		t.Errorf("デーモン停止を伝えていない: %q", stopped[0].detail)
	}
}
