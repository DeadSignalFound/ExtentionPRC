package main

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	ansiReset   = "\x1b[0m"
	ansiBold    = "\x1b[1m"
	ansiDim     = "\x1b[2m"
	ansiRed     = "\x1b[31m"
	ansiGreen   = "\x1b[32m"
	ansiYellow  = "\x1b[33m"
	ansiBlue    = "\x1b[34m"
	ansiMagenta = "\x1b[35m"
	ansiCyan    = "\x1b[36m"
	ansiGray    = "\x1b[90m"
	ansiPurple  = "\x1b[38;5;141m"
	ansiOrange  = "\x1b[38;5;208m"
)

var (
	logMu    sync.Mutex
	logOut   io.Writer = os.Stdout
	useColor           = true
	startAt            = time.Now()

	stats struct {
		activities atomic.Uint64
		errors     atomic.Uint64
		clients    atomic.Int64
	}
)

func init() {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		useColor = false
	}
	enableVT()
}

func paint(code, s string) string {
	if !useColor {
		return s
	}
	return code + s + ansiReset
}

func writeLine(sym, tag, col, msg string) {
	logMu.Lock()
	defer logMu.Unlock()
	ts := time.Now().Format("15:04:05.000")
	fmt.Fprintf(logOut, "%s %s %s  %s\n",
		paint(ansiGray, ts),
		paint(col, sym),
		paint(col+ansiBold, padRight(tag, 8)),
		msg,
	)
}

func logInfo(f string, a ...any)   { writeLine("●", "INFO", ansiCyan, fmt.Sprintf(f, a...)) }
func logOK(f string, a ...any)     { writeLine("✓", "OK", ansiGreen, fmt.Sprintf(f, a...)) }
func logWarn(f string, a ...any)   { writeLine("⚠", "WARN", ansiYellow, fmt.Sprintf(f, a...)) }
func logErr(f string, a ...any)    { stats.errors.Add(1); writeLine("✗", "ERROR", ansiRed, fmt.Sprintf(f, a...)) }
func logDiscord(f string, a ...any){ writeLine("◆", "DISCORD", ansiPurple, fmt.Sprintf(f, a...)) }
func logWS(f string, a ...any)     { writeLine("▸", "WS", ansiBlue, fmt.Sprintf(f, a...)) }

func logActivity(a *Activity) {
	stats.activities.Add(1)
	if a == nil {
		writeLine("■", "CLEAR", ansiGray, paint(ansiDim, "presence cleared"))
		return
	}
	verb := activityVerb(a.Type)
	title := firstNonEmpty(a.Details, a.State, a.Name, "(untitled)")
	msg := paint(ansiBold, verb) + " " + paint(ansiCyan, "“"+trunc(title, 72)+"”")
	if a.State != "" && a.State != title {
		msg += " " + paint(ansiDim, "—") + " " + a.State
	}
	if a.Timestamps != nil {
		msg += " " + paint(ansiDim, progress(a.Timestamps))
	}
	if a.Assets != nil && a.Assets.LargeImage != "" {
		msg += " " + paint(ansiDim, "· art "+shortURL(a.Assets.LargeImage))
	}
	writeLine("▶", "ACTIVITY", ansiOrange, msg)
}

func banner(addr string) {
	logMu.Lock()
	defer logMu.Unlock()
	if !useColor {
		fmt.Fprintf(logOut, "discord-rpc-bridge %s  listen=ws://%s/rpc  host=%s/%s  pid=%d  go=%s\n\n",
			version, addr, runtime.GOOS, runtime.GOARCH, os.Getpid(), runtime.Version())
		return
	}
	title := paint(ansiBold+ansiCyan, "discord-rpc-bridge") + paint(ansiDim, " "+version)
	sub := paint(ansiDim, "firefox → discord rich presence")

	top := paint(ansiGray, "╭"+strings.Repeat("─", 49)+"╮")
	mid := paint(ansiGray, "│ ") + pad(title, 47) + paint(ansiGray, " │")
	sub2 := paint(ansiGray, "│ ") + pad(sub, 47) + paint(ansiGray, " │")
	bot := paint(ansiGray, "╰"+strings.Repeat("─", 49)+"╯")

	fmt.Fprintln(logOut)
	fmt.Fprintln(logOut, "  "+top)
	fmt.Fprintln(logOut, "  "+mid)
	fmt.Fprintln(logOut, "  "+sub2)
	fmt.Fprintln(logOut, "  "+bot)
	fmt.Fprintln(logOut)
	row := func(k, v string) {
		fmt.Fprintf(logOut, "  %s %s\n", paint(ansiGray, padRight(k, 8)), v)
	}
	row("listen", paint(ansiBold, "ws://"+addr+"/rpc"))
	row("health", paint(ansiDim, "http://"+addr+"/health"))
	row("host", fmt.Sprintf("%s/%s  %s", runtime.GOOS, runtime.GOARCH, paint(ansiDim, runtime.Version())))
	row("pid", fmt.Sprintf("%d", os.Getpid()))
	fmt.Fprintln(logOut)
	fmt.Fprintln(logOut, "  "+paint(ansiDim, "waiting for firefox extension…"))
	fmt.Fprintln(logOut)
}

func logStats() {
	up := time.Since(startAt).Truncate(time.Second)
	msg := fmt.Sprintf("uptime=%s  clients=%d  activities=%d  errors=%d",
		up, stats.clients.Load(), stats.activities.Load(), stats.errors.Load())
	writeLine("∙", "STATS", ansiGray, paint(ansiDim, msg))
}

func activityVerb(t int) string {
	switch t {
	case 0:
		return "Playing"
	case 1:
		return "Streaming"
	case 2:
		return "Listening to"
	case 3:
		return "Watching"
	case 5:
		return "Competing in"
	}
	return "Activity"
}

func progress(ts *Timestamps) string {
	if ts == nil || ts.Start <= 0 {
		return ""
	}
	nowMs := time.Now().UnixMilli()
	elapsed := (nowMs - ts.Start) / 1000
	if elapsed < 0 {
		elapsed = 0
	}
	if ts.End > ts.Start {
		total := (ts.End - ts.Start) / 1000
		bar := progressBar(elapsed, total, 14)
		return fmt.Sprintf("%s %s/%s", bar, mss(elapsed), mss(total))
	}
	return fmt.Sprintf("[%s]", mss(elapsed))
}

func progressBar(cur, total int64, width int) string {
	if total <= 0 {
		return ""
	}
	if cur > total {
		cur = total
	}
	filled := int(int64(width) * cur / total)
	if filled < 0 {
		filled = 0
	}
	if filled > width {
		filled = width
	}
	return "[" + strings.Repeat("█", filled) + strings.Repeat("░", width-filled) + "]"
}

func mss(sec int64) string {
	if sec < 0 {
		sec = 0
	}
	return fmt.Sprintf("%d:%02d", sec/60, sec%60)
}

func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func padRight(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}

func pad(s string, visibleWidth int) string {
	vis := visibleLen(s)
	if vis >= visibleWidth {
		return s
	}
	return s + strings.Repeat(" ", visibleWidth-vis)
}

func visibleLen(s string) int {
	n, in := 0, false
	for _, r := range s {
		if r == 0x1b {
			in = true
			continue
		}
		if in {
			if r == 'm' {
				in = false
			}
			continue
		}
		n++
	}
	return n
}

func firstNonEmpty(a ...string) string {
	for _, s := range a {
		if s != "" {
			return s
		}
	}
	return ""
}

func shortURL(u string) string {
	if len(u) <= 48 {
		return u
	}
	return u[:32] + "…" + u[len(u)-12:]
}
