package jlog

import (
	"flag"
	"os"
	"time"
)

var flushInterval int64
var logLevel int64
var logDir string
var processName string
var logSize int = 10
var logCount int64 = 10
var rotateChan chan severity
var compress bool
var rotateFlag bool
var Flag *flag.FlagSet

func checkDir() {
	_, err := os.Stat(logDir)
	if err != nil {
		if os.IsNotExist(err) {
			err = os.MkdirAll(logDir, os.ModePerm)
			if err == nil {
				return
			}
		}
	}
	return
}

func init() {
	Flag = flag.NewFlagSet("jlog", flag.ExitOnError)
	Flag.Int64Var(&flushInterval, "flushInterval", 5, "log flush interval[second]")
	Flag.StringVar(&logDir, "logDir", "log", "log dir")
	Flag.Int64Var(&logLevel, "logLevel", 0, "log level[0~4]")
	Flag.IntVar(&logSize, "logSize", 10, "log size[MB]")
	Flag.Int64Var(&logCount, "logCount", 10, "log count")
	Flag.StringVar(&processName, "processName", "jlog", "processName")
	Flag.BoolVar(&compress, "compress", false, "compress logs")
	checkDir()
	loggers = []iLog{
		infoLog:    newLogger(infoLog),
		warningLog: newLogger(warningLog),
		errorLog:   newLogger(errorLog),
		fatalLog:   newLogger(fatalLog),
		webLog:     newLogger(webLog),
	}
	rotateChan = make(chan severity, 100)
	go func() {
		flushThread()
	}()
}

func PrintDefaults() {
	Flag.PrintDefaults()
}

func flushThread() {
	duration := time.Duration(flushInterval) * time.Second
	c := time.NewTicker(duration)
	defer c.Stop()
	for {
		c.Reset(duration)
		select {
		case <-c.C:
			Flush()
		case s := <-rotateChan:
			rotateFlag = true
			rotateLog(s)
			rotateFlag = false
		}
	}
}

func Flush() {
	for k := range loggers {
		loggers[k].flush()
	}
}

func Shutdown() {
	for rotateFlag {
		time.Sleep(time.Millisecond * 10)
	}
	V(logLevel).Infoln("shutting down ...")
	for k := range loggers {
		loggers[k].flush()
		loggers[k].close()
	}
}


func SetLogLevel(level int) {
	old := logLevel
	if level < 0 {
		logLevel = 0
	} else if level > 4 {
		logLevel = 4
	} else {
		logLevel = int64(level)
	}
	V(logLevel).Infoln("log level changes from", old, "to", logLevel)
}
