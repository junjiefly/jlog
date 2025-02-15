package jlog

import (
	"os"
	"path/filepath"
	"time"
)

var TimeISO8601 = "2006-01-02T15:04:05.000Z0700"
var TimeDefault = "2006-01-02T15:04:05.000"

var loggers []iLog

type config struct {
	logDir        string
	flushInterval int
	logLevel      int64
	fileName      string
	maxSize       int64
	maxBackups    int
	maxAge        int
	compress      bool
	consoleOut    bool
}

type severity int32

const (
	infoLog severity = iota
	warningLog
	errorLog
	fatalLog
	webLog
	numSeverity = 5
)

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

const mb = 1024 * 1024
var maxBufSize = mb

var logCfg config
var program = filepath.Base(os.Args[0])

var timeFormater = timeFormat

func timeFormat(t time.Time) string {
	return t.Local().Format(TimeDefault)
}
