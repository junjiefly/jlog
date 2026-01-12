package jlog

import (
	"flag"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

var Flags = flag.NewFlagSet("jlog", flag.ContinueOnError)

func init() {
	Flags.StringVar(&logCfg.logDir, "logDir", "log", "log dir path")
	Flags.IntVar(&logCfg.flushInterval, "logFlushInterval", 5, "log flush interval[second]")
	Flags.StringVar(&logCfg.fileName, "logName", program, "log file name")
	Flags.Int64Var(&logCfg.logLevel, "logLevel", 0, "default log level")
	Flags.Int64Var(&logCfg.maxSize, "logSize", 100, "max log file size[mb]")
	Flags.IntVar(&logCfg.maxBackups, "logBackups", 10, "maximum number of backup log files")
	Flags.IntVar(&logCfg.maxAge, "logAge", 0, "maximum number of days to retain old log files")
	Flags.BoolVar(&logCfg.compress, "logCompress", true, "if the rotated log files should be compressed")
	Flags.BoolVar(&logCfg.consoleOut, "logConsole", false, "if write log to console")
	Flags.BoolVar(&logCfg.localWrite, "logLocalWrite", true, "if write local log files")
	loggers = []iLog{
		infoLog:    newLogger(infoLog),
		warningLog: newLogger(warningLog),
		errorLog:   newLogger(errorLog),
		fatalLog:   newLogger(fatalLog),
		httpLog:    newLogger(httpLog),
	}
	go func() {
		flushThread()
	}()
}

func V(level int64) decision {
	if level > logCfg.logLevel {
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
	old := logCfg.logLevel
	if level < 0 {
		logCfg.logLevel = 0
	} else if level > 4 {
		logCfg.logLevel = 4
	} else {
		logCfg.logLevel = int64(level)
	}
	V(logCfg.logLevel).Infoln("log level changes from", old, "to", logCfg.logLevel)
}

func SetOutout(writers ...io.Writer) {
	logCfg.writers = writers
}

func Flush() {
	for k := range loggers {
		loggers[k].flush()
	}
}

func Shutdown() {
	V(logCfg.logLevel).Infoln("shutting down ...")
	for k := range loggers {
		loggers[k].flush()
		loggers[k].close()
	}
}
