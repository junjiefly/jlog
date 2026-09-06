package jlog

import (
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
	"time"
)

var TimeISO8601 = "2006-01-02T15:04:05.000Z0700"
var TimeRFC3339 = time.RFC3339
var TimeRFC3339Nano = time.RFC3339Nano

// Level is the severity of a record. Records below the configured threshold
// are dropped; FatalLevel records are always written.
type Level int32

const (
	DebugLevel Level = iota
	InfoLevel
	WarnLevel
	ErrorLevel
	FatalLevel
)

var levelName = []string{
	DebugLevel: "debug",
	InfoLevel:  "info",
	WarnLevel:  "warn",
	ErrorLevel: "error",
	FatalLevel: "fatal",
}

var levelChar = []byte("DIWEF")

// logKind identifies a built-in log destination, not the record level.
type logKind int32

const (
	appLog logKind = iota
)

var logKindFileSuffix = []string{
	appLog: ".log",
}

var loggers = []iLog{
	appLog: {kind: appLog},
}

type Config struct {
	LogDir        string
	FlushInterval int
	Level         Level
	FileName      string
	MaxSize       int64
	MaxBackups    int
	MaxAge        int
	Compress      bool
	Stdout        bool
	LocalWrite    bool

	Writers []io.Writer
}

var (
	split        = byte('-')
	space        = byte(' ')
	leftBracket  = byte('[')
	dotDot       = byte(':')
	rightBracket = byte(']')
	lineBreak    = byte('\n')
)

const mb = 1024 * 1024
const maxConfiguredSize int64 = (1<<63 - 1) / mb
const maxInt = int64(^uint(0) >> 1)
const minInt64 = int64(-1) << 63

var maxBufSize = mb

var logCfg Config

// cfgMu guards logCfg; currentLevel is the hot-path threshold.
var cfgMu sync.RWMutex

// lifecycleMu serializes Init and Shutdown so a concurrent Shutdown cannot
// return while a just-started flush goroutine remains active.
var lifecycleMu sync.Mutex

// shuttingDown is checked while a log lock is held, so Shutdown cannot close
// a log and have another goroutine immediately recreate it.
var shuttingDown int32

var currentLevel int32 = int32(InfoLevel)

// firstError keeps the public API compatible with older Go releases.
func firstError(err, next error) error {
	if err != nil {
		return err
	}
	return next
}

var program = filepath.Base(os.Args[0])

const defaultTimeFormat = "2006-01-02T15:04:05.000"

var customTimeFormatValue atomic.Value

func init() {
	customTimeFormatValue.Store(defaultTimeFormat)
}

func timeFormat(format string) {
	if format == "" {
		format = defaultTimeFormat
	}
	customTimeFormatValue.Store(format)
}

func currentTimeFormat() string {
	if format, ok := customTimeFormatValue.Load().(string); ok && format != "" {
		return format
	}
	return defaultTimeFormat
}

func normalizeConfig(cfg Config) Config {
	logDir := cfg.LogDir
	if logDir == "" {
		logDir = "log"
	}
	fileName := cfg.FileName
	if fileName == "" {
		fileName = program
	}
	flushInterval := cfg.FlushInterval
	if flushInterval <= 0 {
		flushInterval = 30
	}
	maxIntervalSeconds := maxInt / int64(time.Second)
	if int64(flushInterval) > maxIntervalSeconds {
		flushInterval = int(maxIntervalSeconds)
	}
	maxSize := cfg.MaxSize
	if maxSize <= 0 {
		maxSize = 100
	}
	if maxSize > maxConfiguredSize {
		maxSize = maxConfiguredSize
	}
	maxBackups := cfg.MaxBackups
	if maxBackups < 0 {
		maxBackups = 0
	}
	maxAge := cfg.MaxAge
	if maxAge < 0 {
		maxAge = 0
	}
	maxAgeDays := maxInt / int64(24*time.Hour)
	if int64(maxAge) > maxAgeDays {
		maxAge = int(maxAgeDays)
	}

	writers := make([]io.Writer, 0, len(cfg.Writers))
	for _, writer := range cfg.Writers {
		if validWriter(writer) {
			writers = append(writers, writer)
		}
	}

	return Config{
		LogDir:        logDir,
		FlushInterval: flushInterval,
		Level:         clampLevel(cfg.Level),
		FileName:      fileName,
		MaxSize:       maxSize,
		MaxBackups:    maxBackups,
		MaxAge:        maxAge,
		Compress:      cfg.Compress,
		Stdout:        cfg.Stdout,
		LocalWrite:    cfg.LocalWrite,
		Writers:       writers,
	}
}

func validWriter(writer io.Writer) bool {
	if writer == nil {
		return false
	}
	value := reflect.ValueOf(writer)
	switch value.Kind() {
	case reflect.Ptr, reflect.Func, reflect.Map, reflect.Chan, reflect.Slice, reflect.Interface:
		return !value.IsNil()
	default:
		return true
	}
}
