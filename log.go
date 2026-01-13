package jlog

import (
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

func init() {
	loggers = []iLog{
		infoLog:    newLogger(infoLog),
		warningLog: newLogger(warningLog),
		errorLog:   newLogger(errorLog),
		fatalLog:   newLogger(fatalLog),
		httpLog:    newLogger(httpLog),
	}
}

func InitWithDefaultConfig() error {
	logCfg = Config{
		LogDir:        "log",
		FlushInterval: 5,
		FileName:      program,
		LogLevel:      0,
		MaxSize:       100,
		MaxBackups:    10,
		MaxAge:        0,
		Compress:      true,
		Stdout:    false,
		LocalWrite:    true,
	}
	return nil
}


func InitWithConfig(cfg Config)  {
	logCfg = Config{
		LogDir:        cfg.LogDir,
		FlushInterval: cfg.FlushInterval,
		FileName:      cfg.FileName,
		LogLevel:      cfg.LogLevel,
		MaxSize:       cfg.MaxSize,
		MaxBackups:    cfg.MaxBackups,
		MaxAge:        cfg.MaxAge,
		Compress:      cfg.Compress,
		Stdout:    cfg.Stdout,
		LocalWrite:    cfg.LocalWrite,
	}
	go func() {
		flushThread()
	}()
	return
}

func V(level int64) decision {
	if level > logCfg.LogLevel {
		return false
	}
	return true
}

func (d decision) Infoln(args ...interface{}) {
	if d {
		loggers[infoLog].println(args...)
	}
}

func (d decision) Infof(format string, args ...interface{}) {
	if d {
		loggers[infoLog].printf(format, args...)
	}
}
func (d decision) Warningln(args ...interface{}) {
	if d {
		loggers[warningLog].println(args...)
	}
}

func (d decision) Warningf(format string, args ...interface{}) {
	if d {
		loggers[warningLog].printf(format, args...)
	}
}

func (d decision) Errorln(args ...interface{}) {
	if d {
		loggers[errorLog].println(args...)
	}
}

func (d decision) Errorf(format string, args ...interface{}) {
	if d {
		loggers[errorLog].printf(format, args...)
	}
}

func newLogEntry(loglevel severity) *entry {
	e := newEntry()
	e.s = loglevel
	e.buf = newBuffer()
	e.buf.writeByte('{')
	e = e.time(time.Now())
	if loglevel != httpLog {
		e = e.level(severityType[loglevel])
	}
	return e
}

func newLogEntryWithTime(loglevel severity, t time.Time) *entry {
	e := newEntry()
	e.s = loglevel
	e.buf = newBuffer()
	e.buf.writeByte('{')
	e = e.time(t)
	if loglevel != httpLog {
		e = e.level(severityType[loglevel])
	}
	return e
}

func (d decision) Warnings() *entry {
	if d {
		return newLogEntry(warningLog)
	}
	return nil
}

func (d decision) Infos() *entry {
	if d {
		return newLogEntry(infoLog)
	}
	return nil
}

func Infos() *entry {
	return newLogEntry(infoLog)
}

func (d decision) Errors() *entry {
	if d {
		return newLogEntry(errorLog)
	}
	return nil
}

func Fatalln(args ...interface{}) {
	loggers[fatalLog].println(args...)
	trace := stacks(true)
	loggers[fatalLog].write(trace)
	loggers[fatalLog].flush()
	Shutdown()
	os.Exit(255)
}

func Fatalf(format string, args ...interface{}) {
	loggers[fatalLog].printf(format, args...)
	trace := stacks(true)
	loggers[fatalLog].write(trace)
	loggers[fatalLog].flush()
	Shutdown()
	os.Exit(255)
}

func Http(r *http.Request, reqId, host string, startTime int64, retCode int, spitTime string, size int64, errMsg error) {
	remoteAddr := r.RemoteAddr
	if idx := strings.LastIndex(remoteAddr, ":"); idx >= 0 {
		remoteAddr = remoteAddr[:idx]
	}
	now := timeNow()
	cost := (now.UnixNano() - startTime) / 1e6
	fb := newBuffer()

	fb.writeTime(now)
	fb.writeByte(space)

	fb.Write(str2bytes(remoteAddr))
	fb.writeByte(space)

	fb.Write(str2bytes(host))
	fb.writeByte(space)

	fb.Write(str2bytes(reqId))
	fb.writeByte(space)

	fb.Write(str2bytes(r.Method))
	fb.writeByte(space)

	someDigits(fb, retCode)
	fb.writeByte(space)

	someDigits(fb, int(size))
	fb.writeByte(space)

	someDigits(fb, int(cost))
	fb.Write(str2bytes("ms"))
	fb.writeByte(space)

	fb.Write(str2bytes(r.RequestURI))
	fb.writeByte(space)

	if spitTime == "" {
		fb.writeByte(split)
	} else {
		fb.Write(str2bytes(spitTime))
	}
	fb.writeByte(space)

	if errMsg == nil {
		fb.writeByte(split)
	} else {
		fb.Write(str2bytes(errMsg.Error()))
	}
	fb.writeByte(space)

	fb.Write(str2bytes(r.Proto))
	fb.writeByte(space)

	fb.Write(str2bytes(r.UserAgent()))
	fb.writeByte(space)

	fb.writeByte(lineBreak)
	loggers[httpLog].output(fb.bytes())
	freeBuffer(fb)
}

func Https(r *http.Request, reqId, host string, startTime int64, retCode int, spitTime string, size int64, errMsg error) {
	remoteAddr := r.RemoteAddr
	if idx := strings.LastIndex(remoteAddr, ":"); idx >= 0 {
		remoteAddr = remoteAddr[:idx]
	}
	now := timeNow()
	cost := (now.UnixNano() - startTime) / 1e6
	e := newLogEntryWithTime(httpLog, now)

	e = e.Str("remote", remoteAddr).Str("host", host).ReqId(reqId).Str("method", r.Method).Int("status", retCode)
	e = e.Int64("size", size).Int64("cost", cost).Str("uri", r.RequestURI)
	e = e.Str("split", spitTime).Str("proto", r.Proto).Str("ua", r.UserAgent())
	if errMsg == nil {
		e.Msg("")
	} else {
		e.Msg(errMsg.Error())
	}
}

func TimeFormat(format string) {
	timeFormat(format)
}

func SetLogLevel(level int) {
	old := logCfg.LogLevel
	if level < 0 {
		logCfg.LogLevel = 0
	} else if level > 4 {
		logCfg.LogLevel = 4
	} else {
		logCfg.LogLevel = int64(level)
	}
	V(logCfg.LogLevel).Infoln("log level changes from", old, "to", logCfg.LogLevel)
}

func SetOutout(writers ...io.Writer) {
	logCfg.Writers = writers
}

func Flush() {
	for k := range loggers {
		loggers[k].flush()
	}
}

func Shutdown() {
	V(logCfg.LogLevel).Infoln("shutting down ...")
	for k := range loggers {
		loggers[k].flush()
		loggers[k].close()
	}
}
