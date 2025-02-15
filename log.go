package jlog

import (
	"flag"
	"net/http"
	"os"
	"strings"
	"time"
)

func init() {
	flag.StringVar(&logCfg.logDir, "logDir", "./log", "log dir path")
	flag.IntVar(&logCfg.flushInterval, "logFlushInterval", 30, "log flush interval[second]")
	flag.StringVar(&logCfg.fileName, "logName", program, "log file name")
	flag.Int64Var(&logCfg.logLevel, "logLevel", 0, "default log level")
	flag.Int64Var(&logCfg.maxSize, "logSize", 1024, "max log file size[mb]")
	flag.IntVar(&logCfg.maxBackups, "logBackups", 10, "maximum number of backup log files")
	flag.IntVar(&logCfg.maxAge, "logAge", 0, "maximum number of days to retain old log files")
	flag.BoolVar(&logCfg.compress, "logCompress", true, "if the rotated log files should be compressed")
	flag.BoolVar(&logCfg.consoleOut, "logConsole", false, "if output log to console")
	checkDir()
	loggers = []iLog{
		infoLog:    newLogger(infoLog),
		warningLog: newLogger(warningLog),
		errorLog:   newLogger(errorLog),
		fatalLog:   newLogger(fatalLog),
		webLog:     newLogger(webLog),
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

	fb.Write(str2bytes(now.Local().Format("2006-01-02 15:04:05.000")[:23]))
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
	loggers[webLog].output(fb.bytes())
	freeBuffer(fb)
}

func TimeFormat(format string){
	timeFormater = func(t time.Time) string {
		return t.Local().Format(format)
	}
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
