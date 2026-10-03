package desktop

import (
	"fmt"
	"strings"
)

// 起動の失敗のダイアログの文（specs/037-windows-app/contracts/windows-app.md §2）。
// 文言は英語で、どの文にもログの場所を添える（ログを書けない 2 番目を除く）。

// WebView2URL は WebView2 ランタイムの入手先である。
const WebView2URL = "https://developer.microsoft.com/microsoft-edge/webview2/"

// MessageBadArgs は使えない引数のダイアログの文である。
func MessageBadArgs(err error) string {
	return fmt.Sprintf("VVMDM cannot start with these arguments.\n\n%v\n\n%s", err, UsageText)
}

// MessageNotExtracted は zip から直接実行したときのダイアログの文である（§2 の 1 番目）。
func MessageNotExtracted(logFile string) string {
	return withLog("VVMDM must be extracted before it can run.\n\n"+
		"Right-click the zip file, choose \"Extract All\", "+
		"and then run VVMDM.exe in the extracted folder.", logFile)
}

// MessageUnwritable はデータの置き場に書けないときのダイアログの文である（§2 の 2 番目）。
func MessageUnwritable(dir string) string {
	return fmt.Sprintf("VVMDM cannot write to the following folder, so it cannot start.\n\n%s", dir)
}

// MessageNoWebView2 は WebView2 ランタイムが無いときのダイアログの文である（§2 の 3 番目）。
func MessageNoWebView2(logFile string) string {
	return withLog("VVMDM needs the Microsoft Edge WebView2 Runtime, which was not found on this PC.\n\n"+
		"Install it from "+WebView2URL+" and then run VVMDM again.", logFile)
}

// MessageDatabase はデータベースを開けないときのダイアログの文である（§2 の 4 番目）。
func MessageDatabase(err error, logFile string) string {
	return withLog(fmt.Sprintf("VVMDM could not open its database.\n\n%s", summarize(err)), logFile)
}

// MessagePortInUse は待ち受けられないときのダイアログの文である（§2 の 5 番目）。
func MessagePortInUse(port int, logFile string) string {
	return withLog(fmt.Sprintf("VVMDM could not use port %d. Another program may be using it.\n\n"+
		"To use a different port, start VVMDM.exe with --port <number>, "+
		"for example by adding --port %d to the target of a shortcut.", port, alternativePort(port)), logFile)
}

// MessageFailed は上のどれにも当たらない失敗のダイアログの文である。
func MessageFailed(err error, logFile string) string {
	return withLog(fmt.Sprintf("VVMDM stopped because of an error.\n\n%s", summarize(err)), logFile)
}

func withLog(text, logFile string) string {
	return text + "\n\nDetails are in the log:\n" + logFile
}

// summarizeLimit はダイアログに載せる誤りの文の長さの上限である。全文はログにある。
const summarizeLimit = 600

func summarize(err error) string {
	text := strings.TrimSpace(err.Error())
	if runes := []rune(text); len(runes) > summarizeLimit {
		return string(runes[:summarizeLimit]) + "…"
	}
	return text
}

func alternativePort(port int) int {
	if port == 65535 {
		return DefaultPort
	}
	return port + 1
}
