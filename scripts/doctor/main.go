// doctor は手元の開発環境に必要な外部コマンドが揃っているかを調べる。
// task doctor の実体。
package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// tool は1つの外部コマンドに対する検査項目である。
type tool struct {
	name string
	// versionArgs は版を表示させる引数。存在確認だけでは PATH は通っているが
	// 実行できない状態を見逃すので、実際に起動して1行目を読む。
	versionArgs []string
	hint        string
	required    bool
}

// tools は task が実際に呼ぶコマンドと対応する。task up しか使わない利用者は
// ここに並ぶものを1つも必要としない（README の「必ず動く道」）。
var tools = []tool{
	{name: "git", versionArgs: []string{"--version"}, hint: "Git を導入して PATH へ通すこと。", required: true},
	{name: "jq", versionArgs: []string{"--version"}, hint: "API 応答の JSON 確認に使う。mise で導入できる: mise install。", required: false},
	{name: "go", versionArgs: []string{"version"}, hint: "mise で導入する: mise install", required: true},
	{name: "node", versionArgs: []string{"--version"}, hint: "mise で導入する: mise install", required: true},
	{name: "npm", versionArgs: []string{"--version"}, hint: "mise で導入する: mise install", required: true},
	{name: "ffmpeg", versionArgs: []string{"-version"}, hint: "ffmpeg を導入して ffmpeg を PATH へ通すこと。", required: true},
	{name: "ffprobe", versionArgs: []string{"-version"}, hint: "ffmpeg を導入して ffprobe を PATH へ通すこと。", required: true},
	{name: "bash", versionArgs: []string{"--version"}, hint: "task migrations-check が使う。Windows では Git for Windows などで導入する。", required: true},
	{name: "task", versionArgs: []string{"--version"}, hint: "mise で導入する: mise install", required: true},
	{name: "mise", versionArgs: []string{"--version"}, hint: "mise.toml が固定する版を使うなら導入する。", required: false},
	{name: "docker", versionArgs: []string{"--version"}, hint: "task up / task down に必要。", required: false},
}

// result は検査の結果である。detail には版か失敗の理由が入る。
type result struct {
	tool
	ok     bool
	detail string
}

// firstLineFunc は外部コマンドの1行目を返す。テストから差し替える。
type firstLineFunc func(name string, args ...string) (string, error)

func firstLine(name string, args ...string) (string, error) {
	output, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return "", err
	}
	line, _, _ := strings.Cut(strings.TrimSpace(string(output)), "\n")
	return strings.TrimSpace(line), nil
}

func inspect(t tool, run firstLineFunc) result {
	version, err := run(t.name, t.versionArgs...)
	if err != nil {
		return result{tool: t, detail: "見つからないか実行できません"}
	}
	if version == "" {
		return result{tool: t, detail: "版の確認が何も返しませんでした"}
	}
	return result{tool: t, ok: true, detail: version}
}

// inspectAll は docker だけ追加でデーモンの到達性も見る。docker が入っていても
// デーモンが止まっていれば task up は動かないので、版だけでは足りない。
func inspectAll(list []tool, run firstLineFunc) []result {
	results := make([]result, 0, len(list))
	for _, t := range list {
		r := inspect(t, run)
		if t.name == "docker" && r.ok {
			if server, err := run("docker", "info", "--format", "{{.ServerVersion}}"); err == nil && server != "" {
				r.detail += "; daemon " + server
			} else {
				r.detail += "; デーモンへ到達できません"
			}
		}
		results = append(results, r)
	}
	return results
}

// report は表示する文章と終了コードを返す。必須が欠けたときだけ 1 になる。
func report(results []result) (string, int) {
	var out strings.Builder
	out.WriteString("Local development environment\n\n")

	missing := []string{}
	for _, r := range results {
		marker := "OK"
		kind := "optional"
		if r.required {
			kind = "required"
		}
		if !r.ok {
			marker = "WARN"
			if r.required {
				marker = "ERR"
				missing = append(missing, r.name)
			}
		}
		fmt.Fprintf(&out, "[%-4s] %-8s (%s) %s\n", marker, r.name, kind, r.detail)
		if !r.ok {
			fmt.Fprintf(&out, "      %s\n", r.hint)
		}
	}

	out.WriteString("\n")
	if len(missing) > 0 {
		fmt.Fprintf(&out, "必要な外部コマンドが足りません: %s\n", strings.Join(missing, ", "))
		return out.String(), 1
	}
	out.WriteString("必要な外部コマンドは揃っています。\n")
	return out.String(), 0
}

func main() {
	text, code := report(inspectAll(tools, firstLine))
	fmt.Print(text)
	os.Exit(code)
}
