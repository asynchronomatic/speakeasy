package log

import (
	"fmt"
	"os"
	"time"
)

const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorCyan   = "\033[36m"
	ColorGray   = "\033[2;37m"
)

const (
	LogError = 1 << iota
	LogWarn
	LogEvent
	LogInfo
	LogHighlight
	LogPrint
	LogDebug

	LogNormal = LogError | LogWarn | LogEvent | LogInfo | LogHighlight | LogPrint
	LogAll    = LogError | LogWarn | LogEvent | LogInfo | LogHighlight | LogPrint | LogDebug
)

type Log struct {
	fmtTime   string
	level     uint32
	component string
	printer   func(level uint32, component string, msg string)
}

func (l *Log) Printf(s string, v ...interface{}) {
	l.emit(LogPrint, s, v...)
}

func (l *Log) Panicf(s string, v ...interface{}) {
	panic(fmt.Sprintf(s, v...))
}

func (l *Log) Fatalf(s string, v ...interface{}) {
	l.emit(LogError, s, v...)
	os.Exit(1)
}

func (l *Log) Infof(s string, v ...interface{}) {
	l.emit(LogInfo, s, v...)
}

func (l *Log) Eventf(s string, v ...interface{}) {
	l.emit(LogEvent, s, v...)
}

func (l *Log) Warnf(s string, v ...interface{}) {
	l.emit(LogWarn, s, v...)
}

func (l *Log) Debugf(s string, v ...interface{}) {
	l.emit(LogDebug, s, v...)
}

func (l *Log) Highlightf(s string, v ...interface{}) {
	l.emit(LogHighlight, s, v...)
}

func (l *Log) Errorf(s string, v ...interface{}) {
	l.emit(LogError, s, v...)
}

func (l *Log) emit(level uint32, s string, v ...interface{}) {
	if l.level&level == level {
		l.printer(level, l.component, fmt.Sprintf(s, v...))
	}
}

func (l *Log) ColorPrint(color string, s string) {
	now := time.Now() // get this early.
	fmt.Fprintf(os.Stdout, "%s | %4s | %s%s%s", now.Format(l.fmtTime), color, l.component, s, ColorReset)
}

func (l *Log) GetLevel() uint32 {
	return l.level
}

func (l *Log) SetLevel(level uint32) {
	l.Infof("set log level:%v", level)
	l.level = level
}

func (l *Log) SetTimeFormat(fmt string) {
	l.fmtTime = fmt
}

func (l *Log) SetPrinter(printer func(level uint32, component string, msg string)) {
	l.printer = printer
}

func (l *Log) SetName(name string) {
	l.component = name
}

func (l *Log) WithName(name string) *Log {
	return &Log{
		component: name,
		level:     l.level,
		fmtTime:   l.fmtTime,
		printer:   l.printer,
	}
}

func New(name string) *Log {
	return &Log{
		component: name,
		level:     LogNormal,
		fmtTime:   Default.fmtTime,
		printer:   Default.printer,
	}
}
