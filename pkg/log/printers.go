package log

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

var colorMap = map[uint32]string{
	LogError:     ColorRed,
	LogWarn:      ColorYellow,
	LogEvent:     ColorCyan,
	LogInfo:      ColorReset,
	LogHighlight: ColorGreen,
	LogPrint:     ColorGray,
	LogDebug:     ColorGray,
}

var levelMap = map[uint32]string{
	LogError:     "ERROR",
	LogWarn:      "WARN",
	LogEvent:     "EVENT",
	LogInfo:      "INFO",
	LogHighlight: "HILI",
	LogPrint:     "PRINT",
	LogDebug:     "DEBUG",
}

func ColorPrinterClassic(level uint32, component, msg string) {
	color, ok := colorMap[level]
	if !ok {
		color = ColorRed
	}
	now := time.Now() // get this early.
	fmt.Fprintf(os.Stderr, "%s | %s%s%s", now.Format(DefaultTimeFormat), color, msg, ColorReset)
	if !strings.HasSuffix(msg, "\n") {
		fmt.Fprintf(os.Stderr, "\n")
	}
}

func ColorPrinter(out io.Writer, level uint32, component, msg string) {
	color, ok := colorMap[level]
	if !ok {
		color = ColorRed
	}
	now := time.Now() // get this early.
	fmt.Fprintf(out, "%s | %s%-7.7s%s | %s%s%s", now.Format(DefaultTimeFormat), color, component, ColorReset, color, msg, ColorReset)
	if !strings.HasSuffix(msg, "\n") {
		fmt.Fprintf(os.Stderr, "\n")
	}

}

func BasicPrinter(out io.Writer, level uint32, component, msg string) {
	levelString, ok := levelMap[level]
	if !ok {
		levelString = "----"
	}
	now := time.Now() // get this early.
	fmt.Fprintf(out, "%s | %-5.5s | %-5.5s | %s", now.Format(DefaultTimeFormat), levelString, component, msg)
	if !strings.HasSuffix(msg, "\n") {
		fmt.Fprintf(out, "\n")
	}
}

func ColorPrintf(out io.Writer, color string, s string, v ...interface{}) {
	fmt.Fprintf(out, "%s%s%s", color, fmt.Sprintf(s, v...), ColorReset)
}
