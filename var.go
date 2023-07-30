package jlog

type severity int32

const (
	infoLog severity = iota
	warningLog
	errorLog
	fatalLog
	webLog
	numSeverity = 5
)

var severityString = []string{
	infoLog:    "INFO",
	warningLog: "WARNING",
	errorLog:   "ERROR",
	fatalLog:   "FATAL",
	webLog:     "HTTP",
}

var severityName = []string{
	infoLog:    ".info",
	warningLog: ".warn",
	errorLog:   ".error",
	fatalLog:   ".fatal",
	webLog:     ".http",
}

var (
	severityChar = []byte("IWEF")
	split        = byte('-')
	space        = byte(' ')
	leftBracket  = byte('[')
	dotDot       = byte(':')
	rightBracket = byte(']')
	lineBreak    = byte('\n')
)

var maxBufSize = 2048

var gzSuffix = ".gz"
