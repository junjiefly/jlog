package jlog

import (
	"net"
	"net/http"
	"os"
	"sync/atomic"
	"time"
)

func InitWithDefaultConfig() error {
	lifecycleMu.Lock()
	defer lifecycleMu.Unlock()
	atomic.StoreInt32(&shuttingDown, 1)

	cfg := normalizeConfig(Config{
		LogDir:        "log",
		FlushInterval: 5,
		FileName:      program,
		Level:         InfoLevel,
		MaxSize:       100,
		MaxBackups:    10,
		MaxAge:        0,
		Compress:      true,
		Stdout:        false,
		LocalWrite:    true,
	})
	cfgMu.Lock()
	logCfg = cfg
	cfgMu.Unlock()
	atomic.StoreInt32(&currentLevel, int32(cfg.Level))
	closeErr := closeLoggers()
	restartFlushThread(cfg.FlushInterval)
	atomic.StoreInt32(&shuttingDown, 0)
	return closeErr
}

func InitWithConfig(cfg Config) error {
	lifecycleMu.Lock()
	defer lifecycleMu.Unlock()
	atomic.StoreInt32(&shuttingDown, 1)

	cfg = normalizeConfig(cfg)
	cfgMu.Lock()
	logCfg = cfg
	cfgMu.Unlock()
	atomic.StoreInt32(&currentLevel, int32(cfg.Level))
	err := closeLoggers()
	restartFlushThread(cfg.FlushInterval)
	atomic.StoreInt32(&shuttingDown, 0)
	return err
}

// SetLevel sets the minimum record level that is written.
func SetLevel(level Level) {
	level = clampLevel(level)
	atomic.StoreInt32(&currentLevel, int32(level))
}

func clampLevel(level Level) Level {
	if level < DebugLevel {
		return DebugLevel
	}
	if level > ErrorLevel {
		return ErrorLevel
	}
	return level
}

func enabled(l Level) bool {
	return int32(l) >= atomic.LoadInt32(&currentLevel)
}

// closeLoggers forces lazily created logs to use the configuration that was
// just installed. Existing handles are flushed before they are dropped.
func closeLoggers() error {
	customLogMu.Lock()
	defer customLogMu.Unlock()

	var firstErr error
	for k := range loggers {
		firstErr = firstError(firstErr, loggers[k].close())
	}
	for _, customLog := range customLogs {
		firstErr = firstError(firstErr, customLog.logger.close())
	}
	return firstErr
}

func Debugln(args ...interface{}) {
	if enabled(DebugLevel) {
		loggers[appLog].println(DebugLevel, args...)
	}
}

func Debugf(format string, args ...interface{}) {
	if enabled(DebugLevel) {
		loggers[appLog].printf(DebugLevel, format, args...)
	}
}

func Infoln(args ...interface{}) {
	if enabled(InfoLevel) {
		loggers[appLog].println(InfoLevel, args...)
	}
}

func Infof(format string, args ...interface{}) {
	if enabled(InfoLevel) {
		loggers[appLog].printf(InfoLevel, format, args...)
	}
}

func Warnln(args ...interface{}) {
	if enabled(WarnLevel) {
		loggers[appLog].println(WarnLevel, args...)
	}
}

func Warnf(format string, args ...interface{}) {
	if enabled(WarnLevel) {
		loggers[appLog].printf(WarnLevel, format, args...)
	}
}

func Errorln(args ...interface{}) {
	if enabled(ErrorLevel) {
		loggers[appLog].println(ErrorLevel, args...)
	}
}

func Errorf(format string, args ...interface{}) {
	if enabled(ErrorLevel) {
		loggers[appLog].printf(ErrorLevel, format, args...)
	}
}

func Debug() *entry {
	if !enabled(DebugLevel) {
		return nil
	}
	return newLogEntry(DebugLevel)
}

func Info() *entry {
	if !enabled(InfoLevel) {
		return nil
	}
	return newLogEntry(InfoLevel)
}

func Warn() *entry {
	if !enabled(WarnLevel) {
		return nil
	}
	return newLogEntry(WarnLevel)
}

func Error() *entry {
	if !enabled(ErrorLevel) {
		return nil
	}
	return newLogEntry(ErrorLevel)
}

func newLogEntry(l Level) *entry {
	e := newEntry()
	e.logger = &loggers[appLog]
	e.buf = newBuffer()
	e.buf.writeByte('{')
	e = e.time(time.Now())
	e = e.level(levelName[l])
	e.baseReserved = true
	return e
}

func Fatalln(args ...interface{}) {
	loggers[appLog].fatalPrintln(args...)
	trace := stacks(true)
	loggers[appLog].outputLine(trace)
	loggers[appLog].flush()
	Shutdown()
	os.Exit(255)
}

func Fatalf(format string, args ...interface{}) {
	loggers[appLog].fatalPrintf(format, args...)
	trace := stacks(true)
	loggers[appLog].outputLine(trace)
	loggers[appLog].flush()
	Shutdown()
	os.Exit(255)
}

func Http(r *http.Request, reqId, host string, startTime int64, retCode int, spitTime string, size int64, errMsg error) {
	if r == nil {
		return
	}
	remoteAddr := remoteHost(r)
	now := timeNow()
	cost := (now.UnixNano() - startTime) / 1e6
	fb := newBuffer()

	fb.writeTime(now)
	fb.writeByte(space)

	writeTextField(fb, remoteAddr)
	fb.writeByte(space)

	writeTextField(fb, host)
	fb.writeByte(space)

	writeTextField(fb, reqId)
	fb.writeByte(space)

	writeTextField(fb, r.Method)
	fb.writeByte(space)

	someDigits(fb, int64(retCode))
	fb.writeByte(space)

	someDigits(fb, size)
	fb.writeByte(space)

	someDigits(fb, cost)
	fb.Write(str2bytes("ms"))
	fb.writeByte(space)

	writeTextField(fb, r.RequestURI)
	fb.writeByte(space)

	if spitTime == "" {
		fb.writeByte(split)
	} else {
		writeTextField(fb, spitTime)
	}
	fb.writeByte(space)

	if errMsg == nil {
		fb.writeByte(split)
	} else {
		writeTextField(fb, errMsg.Error())
	}
	fb.writeByte(space)

	writeTextField(fb, r.Proto)
	fb.writeByte(space)

	writeTextField(fb, r.UserAgent())
	fb.writeByte(space)

	fb.writeByte(lineBreak)
	httpLog.logger.output(fb.bytes())
	freeBuffer(fb)
}

func Https(r *http.Request, reqId, host string, startTime int64, retCode int, spitTime string, size int64, errMsg error) {
	if r == nil {
		return
	}
	remoteAddr := remoteHost(r)
	now := timeNow()
	cost := (now.UnixNano() - startTime) / 1e6
	e := httpLog.Event()

	e = e.Str("remote", remoteAddr).Str("host", host).ReqId(reqId).Str("method", r.Method).Int("status", retCode)
	e = e.Int64("size", size).Int64("cost", cost).Str("uri", r.RequestURI)
	e = e.Str("split", spitTime).Str("proto", r.Proto).Str("ua", r.UserAgent())
	if errMsg == nil {
		e.Msg("")
	} else {
		e.Msg(errMsg.Error())
	}
}

func remoteHost(r *http.Request) string {
	if r == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func TimeFormat(format string) {
	timeFormat(format)
}

func Flush() error {
	customLogMu.RLock()
	logs := make([]*CustomLog, len(customLogs))
	copy(logs, customLogs)
	customLogMu.RUnlock()

	var firstErr error
	for k := range loggers {
		firstErr = firstError(firstErr, loggers[k].flush())
	}
	for _, customLog := range logs {
		firstErr = firstError(firstErr, customLog.logger.flush())
	}
	return firstErr
}

func Shutdown() error {
	lifecycleMu.Lock()
	defer lifecycleMu.Unlock()
	atomic.StoreInt32(&shuttingDown, 1)

	stopFlushThread()
	customLogMu.RLock()
	logs := make([]*CustomLog, len(customLogs))
	copy(logs, customLogs)
	customLogMu.RUnlock()

	var firstErr error
	for k := range loggers {
		firstErr = firstError(firstErr, loggers[k].flush())
		firstErr = firstError(firstErr, loggers[k].close())
	}
	for _, customLog := range logs {
		firstErr = firstError(firstErr, customLog.logger.flush())
		firstErr = firstError(firstErr, customLog.logger.close())
	}
	return firstErr
}
