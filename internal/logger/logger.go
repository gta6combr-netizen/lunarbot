package logger

import (
	"fmt"
	"sync"
	"time"
)

const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
	colorGray   = "\033[90m"
)

var (
	logMu      sync.Mutex
	recentLogs []string
	maxLogs    = 200
)

func appendLog(line string) {
	logMu.Lock()
	defer logMu.Unlock()
	recentLogs = append(recentLogs, line)
	if len(recentLogs) > maxLogs {
		recentLogs = recentLogs[len(recentLogs)-maxLogs:]
	}
}

func Recent(n int) []string {
	logMu.Lock()
	defer logMu.Unlock()
	if n <= 0 || n > len(recentLogs) {
		n = len(recentLogs)
	}
	out := make([]string, n)
	copy(out, recentLogs[len(recentLogs)-n:])
	return out
}

func ts() string {
	return time.Now().Format("15:04:05")
}

func Cyan(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Printf("%s[%s]%s %s%s%s\n", colorGray, ts(), colorReset, colorCyan, msg, colorReset)
	appendLog(fmt.Sprintf("[%s] %s", ts(), msg))
}

func Green(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Printf("%s[%s]%s %s%s%s\n", colorGray, ts(), colorReset, colorGreen, msg, colorReset)
	appendLog(fmt.Sprintf("[%s] %s", ts(), msg))
}

func Yellow(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Printf("%s[%s]%s %s%s%s\n", colorGray, ts(), colorReset, colorYellow, msg, colorReset)
	appendLog(fmt.Sprintf("[%s] %s", ts(), msg))
}

func Red(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Printf("%s[%s]%s %s%s%s\n", colorGray, ts(), colorReset, colorRed, msg, colorReset)
	appendLog(fmt.Sprintf("[%s] ERR: %s", ts(), msg))
}

func Command(user, group, cmd string, isGroup bool) {
	scope := "PV"
	if isGroup {
		if group != "" {
			scope = group
		} else {
			scope = "Grupo"
		}
	}
	msg := fmt.Sprintf("[CMD] %s | %s | %s", cmd, user, scope)
	fmt.Printf("%s[%s]%s %s%s%s\n", colorGray, ts(), colorReset, colorCyan, msg, colorReset)
	appendLog(fmt.Sprintf("[%s] %s", ts(), msg))
}
