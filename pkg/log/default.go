package log

var DefaultTimeFormat = "2006-01-02 15:04:05"

var Default = &Log{
	component: "ALL",
	fmtTime:   DefaultTimeFormat,
	level:     LogNormal,
	printer:   ColorPrinter,
}

func Printf(s string, v ...interface{}) {
	Default.Printf(s, v...)
}

func Panicf(s string, v ...interface{}) {
	Default.Panicf(s, v...)
}

func Fatalf(s string, v ...interface{}) {
	Default.Fatalf(s, v...)
}

func Infof(s string, v ...interface{}) {
	Default.Infof(s, v...)
}

func Eventf(s string, v ...interface{}) {
	Default.Eventf(s, v...)
}

func Warnf(s string, v ...interface{}) {
	Default.Warnf(s, v...)
}

func Debugf(s string, v ...interface{}) {
	Default.Debugf(s, v...)
}

func Highlightf(s string, v ...interface{}) {
	Default.Highlightf(s, v...)
}

func Errorf(s string, v ...interface{}) {
	Default.Errorf(s, v...)
}

func WithName(name string) *Log {
	return Default.WithName(name)
}
