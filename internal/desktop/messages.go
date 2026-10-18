package desktop

import (
	"fmt"
	"strings"
)

// Failure dialogs include the log path except when the data/log directory itself is unwritable.

// WebView2 runtime download URL.
const WebView2URL = "https://developer.microsoft.com/microsoft-edge/webview2/"

func MessageBadArgs(err error) string {
	return fmt.Sprintf("VVMDM cannot start with these arguments.\n\n%v\n\n%s", err, UsageText)
}

func MessageNotExtracted(logFile string) string {
	return withLog("VVMDM must be extracted before it can run.\n\n"+
		"Right-click the zip file, choose \"Extract All\", "+
		"and then run VVMDM.exe in the extracted folder.", logFile)
}

func MessageUnwritable(dir string) string {
	return fmt.Sprintf("VVMDM cannot write to the following folder, so it cannot start.\n\n%s", dir)
}

func MessageNoWebView2(logFile string) string {
	return withLog("VVMDM needs the Microsoft Edge WebView2 Runtime, which was not found on this PC.\n\n"+
		"Install it from "+WebView2URL+" and then run VVMDM again.", logFile)
}

func MessageDatabase(err error, logFile string) string {
	return withLog(fmt.Sprintf("VVMDM could not open its database.\n\n%s", summarize(err)), logFile)
}

func MessagePortInUse(port int, logFile string) string {
	return withLog(fmt.Sprintf("VVMDM could not use port %d. Another program may be using it.\n\n"+
		"To use a different port, start VVMDM.exe with --port <number>, "+
		"for example by adding --port %d to the target of a shortcut.", port, alternativePort(port)), logFile)
}

func MessageFailed(err error, logFile string) string {
	return withLog(fmt.Sprintf("VVMDM stopped because of an error.\n\n%s", summarize(err)), logFile)
}

func withLog(text, logFile string) string {
	return text + "\n\nDetails are in the log:\n" + logFile
}

// Bound dialog text; logs retain the complete error.
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

// Close confirmation defaults to continuing.
const (
	CloseConfirmInstruction = "Import is in progress"
	CloseConfirmContent     = "If you close VVMDM now, the import is interrupted. " +
		"It continues from where it left off the next time you start VVMDM."
	CloseConfirmClose    = "Close"
	CloseConfirmContinue = "Keep running"
	// Fallback question for builds without a Common Controls v6 manifest.
	CloseConfirmFallbackQuestion = "Close VVMDM now?"
)

// Windows displays this reason while waiting for sign-out or shutdown cleanup.
const ShutdownBlockReason = "VVMDM is stopping"

func MessageOtherSession() string {
	return "VVMDM is already running in another sign-in session of this user.\n\n" +
		"Close VVMDM in that session, and then run it here again."
}

func MessageStillRunning() string {
	return "VVMDM is already running, but its window did not appear.\n\n" +
		"Wait a moment, and then run VVMDM again."
}
