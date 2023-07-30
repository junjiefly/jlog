package jlog

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
	"unsafe"
)

type iLog struct {
	severity
	size int
	sync.RWMutex
	file *os.File
}

func newLogger(s severity) iLog {
	return iLog{severity: s}
}

func (log *iLog) create() {
	path := filepath.Join(logDir, processName+severityName[log.severity])
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0666)
	var size int64 = 0
	if err != nil {
		os.Exit(2)
	} else {
		file.WriteString("open log file at " + time.Now().Local().Format("2006-01-02 15:04:05.000")[:23] + "\n")
		fi, _ := file.Stat()
		if fi != nil {
			size = fi.Size()
		}
	}
	log.file = file
	log.size = int(size)
}

var timeNow = time.Now

func (log *iLog) rotate() {
	if log.size >= logSize*mb {
		log.file.Close()
		newName := filepath.Join(logDir, processName+severityName[log.severity]+".temp")
		oldName := filepath.Join(logDir, processName+severityName[log.severity])
		_ = os.Rename(oldName, newName)

		var size int64 = 0
		file, err := os.OpenFile(oldName, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0666)
		if err != nil {
			os.Exit(2)
		} else {
			fi, _ := file.Stat()
			if fi != nil {
				size = fi.Size()
			}
		}
		log.size = int(size)
		log.file = file
		rotateChan <- log.severity
	}
}

func (log *iLog) header() *buffer {
	_, file, line, ok := runtime.Caller(3)
	if !ok {
		file = "???"
		line = 1
	} else {
		slash := strings.LastIndex(file, "/")
		if slash >= 0 {
			file = file[slash+1:]
		}
	}
	if line < 0 {
		line = 0
	}
	return log.formatHeader(file, line)
}

const digits = "0123456789"

func someDigits(fb *buffer, d int) {
	buf := fb.getBuf()
	pos := fb.getReadOffset()
	buf = buf[pos:]
	j := len(buf)
	for {
		j--
		buf[j] = digits[d%10]
		d /= 10
		if d == 0 {
			break
		}
	}
	ret := copy(buf, buf[j:])
	_ = fb.resize(ret+pos, 0)
	return
}

func str2bytes(s string) []byte {
	x := (*[2]uintptr)(unsafe.Pointer(&s))
	h := [3]uintptr{x[0], x[1], x[1]}
	return *(*[]byte)(unsafe.Pointer(&h))
}

func bytes2str(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

func (log *iLog) formatHeader(file string, line int) *buffer {
	now := timeNow()
	if line < 0 {
		line = 0 // not a real line number, but acceptable to someDigits
	}
	fb := newBuffer()
	fb.Write(str2bytes(now.Local().Format("2006-01-02 15:04:05.000")[:23]))
	fb.writeByte(space)
	fb.writeByte(leftBracket)
	fb.writeByte(severityChar[log.severity])
	fb.writeByte(space)
	fb.Write(str2bytes(file))
	fb.writeByte(dotDot)
	someDigits(fb, line)
	fb.writeByte(rightBracket)
	fb.writeByte(space)
	return fb
}

func (log *iLog) output(buf []byte) {
	log.Lock()
	if log.file == nil {
		log.create()
	}
	ret, _ := log.file.Write(buf)
	log.size += ret
	log.rotate()
	log.Unlock()
}

func (log *iLog) println(args ...interface{}) {
	fb := log.header()
	fmt.Fprintln(fb, args...)
	log.output(fb.bytes())
	freeBuffer(fb)
}

func (log *iLog) printf(format string, args ...interface{}) {
	fb := log.header()
	fmt.Fprintf(fb, format, args...)
	log.output(fb.bytes())
	freeBuffer(fb)
}

func (log *iLog) flush() {
	if log == nil {
		return
	}
	log.Lock()
	if log.file != nil {
		log.file.Sync()
	}
	log.Unlock()
}

func (log *iLog) close() {
	log.Lock()
	if log.file != nil {
		log.file.Close()
	}
	log.Unlock()
}

var loggers []iLog

type decision bool

func V(level int64) decision {
	if level > logLevel {
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

// stacks is a wrapper for runtime.Stack that attempts to recover the data for all goroutines.
func stacks(all bool) []byte {
	// We don't know how big the traces are, so grow a few times if they don't fit. Start large, though.
	n := 10000
	if all {
		n = 100000
	}
	var trace []byte
	for i := 0; i < 5; i++ {
		trace = make([]byte, n)
		nbytes := runtime.Stack(trace, all)
		if nbytes < len(trace) {
			return trace[:nbytes]
		}
		n *= 2
	}
	return trace
}

func Fatalln(args ...interface{}) {
	loggers[fatalLog].println(args...)
	trace := stacks(true)
	loggers[fatalLog].file.Write(trace)
	loggers[fatalLog].flush()
	Shutdown()
	os.Exit(255)
}

func Fatalf(format string, args ...interface{}) {
	loggers[fatalLog].printf(format, args...)
	trace := stacks(true)
	loggers[fatalLog].file.Write(trace)
	loggers[fatalLog].flush()
	Shutdown()
	os.Exit(255)
}
