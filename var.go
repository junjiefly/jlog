package jlog

import (
	"io"
	"os"
	"path/filepath"
	"time"
)

var TimeISO8601 = "2006-01-02T15:04:05.000Z0700"
var TimeRFC3339 = time.RFC3339
var TimeRFC3339Nano = time.RFC3339Nano

var loggers []iLog

type Config struct {
	LogDir        string
	FlushInterval int
	LogLevel int64
	FileName   string
	MaxSize    int64
	MaxBackups int
	MaxAge     int
	Compress   bool
	Stdout     bool
	LocalWrite bool

	Writers []io.Writer
}

type severity int32

const (
	infoLog severity = iota
	warningLog
	errorLog
	fatalLog
	httpLog
	numSeverity = 5
)

var severityName = []string{
	infoLog:    ".info",
	warningLog: ".warn",
	errorLog:   ".error",
	fatalLog:   ".fatal",
	httpLog:     ".http",
}

var severityType = []string{
	infoLog:    "info",
	warningLog: "warn",
	errorLog:   "error",
	fatalLog:   "fatal",
	httpLog:    "http",
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

var logCfg Config
var program = filepath.Base(os.Args[0])

var timeFormater = timeFormatDefault

func timeFormat(format string) {
	timeFormater = func(buf []byte, t time.Time) []byte {
		return t.AppendFormat(buf, format)
	}
}

func timeFormatDefault(buf []byte, t time.Time) []byte {
	buf = t.AppendFormat(buf, time.RFC3339Nano)
	return buf[:len(buf)-12]
}
