# jlog
    * jlog is a log library developed to adapt to high concurrency scenarios. 
    * Pursuing extreme performance and concise configuration. 
    * There is no need to worry about disk space, as it will automatically packs and compresses logs. 
    * You only need to import the package to run directly immediately.
    Try it now!

# Benchmark
```go
    zap/log.go
    httpFileWriteSyncer := zapcore.AddSync(&lumberjack.Logger{
    Filename:   cfg.logDir + "/" + cfg.fileName + ".http",
    MaxSize:    cfg.maxSize * 1024 * 1024,
    MaxBackups: cfg.maxBackups,
    MaxAge:     cfg.maxAge,
    Compress:   cfg.compress,
    })
    
    httpAsyncWriter := &zapcore.BufferedWriteSyncer{
    WS:            zapcore.NewMultiWriteSyncer(httpFileWriteSyncer),
    Size:          256 * 1024,     
    FlushInterval: 30 * time.Second, 
    }

    var httpFileCore zapcore.Core
    httpFileCore = zapcore.NewCore(encoder, zapcore.NewMultiWriteSyncer(httpAsyncWriter), infoPriority)
    httpLog = zap.New(zapcore.NewTee([]zapcore.Core{httpFileCore}...), zap.AddCaller(), zap.AddCallerSkip(1))

    log_test.go
	
    package main
    
    import (
        "bufio"
        "fmt"
        "github.com/golang/glog"
        "github.com/junjiefly/jlog"
        "github.com/rs/zerolog"
        zlog "github.com/rs/zerolog/log"
        "go.uber.org/zap"
        "zap/log"
        "os"
        "testing"
        "time"
    )
    
    var msg string
    var zeroLog_unformat_sync zerolog.Logger
    var zeroLog_unformat_async zerolog.Logger
    
    var zeroLog_json_sync zerolog.Logger
    var zeroLog_json_async zerolog.Logger
    var now = time.Now()
    var tempBuf []byte
    
    func TestMain(m *testing.M) {
        log.InitLog()
        fmt.Println("main")
        bufSize := 512
        var buf = make([]byte, bufSize)
        tempBuf = make([]byte, bufSize)
        for i := 0; i < bufSize; i++ {
            buf[i] = 'a'
            tempBuf[i] = 'a'
        }
    
        msg = string(buf)
    
        zero_unformat_sync_fileWriter, _ := os.Create("zero_unformat_sync.log")
        //defer zero_unformat_sync_fileWriter.Close()
        zero_unformat_sync_fileWriter_consoleWriter := zerolog.ConsoleWriter{
            Out:        zero_unformat_sync_fileWriter,
            TimeFormat: "2006-01-02 15:04:05",
            NoColor:    true, // 禁用颜色输出（可选）
        }
        zeroLog_unformat_sync = zlog.Output(zero_unformat_sync_fileWriter_consoleWriter)
        zero_unformat_async_fileWriter, _ := os.Create("zero_unformat_async.log")
        //defer zero_unformat_sync_fileWriter.Close()
        w := bufio.NewWriterSize(zero_unformat_async_fileWriter, 256*1024)
        zero_unformat_async_fileWriter_consoleWriter := zerolog.ConsoleWriter{
            Out:        w,
            TimeFormat: "2006-01-02 15:04:05",
            NoColor:    true, // 禁用颜色输出（可选）
        }
        zeroLog_unformat_async = zlog.Output(zero_unformat_async_fileWriter_consoleWriter)
    
        zero_json_sync_fileWriter, _ := os.Create("zero_json_sync.log")
        //defer zero_unformat_sync_fileWriter.Close()
        zeroLog_json_sync = zlog.Output(zero_json_sync_fileWriter)
    
        zero_json_async_fileWriter, _ := os.Create("zero_json_async.log")
        //defer zero_unformat_sync_fileWriter.Close()
        zero_json_async_w := bufio.NewWriterSize(zero_json_async_fileWriter, 256*1024)
        zeroLog_json_async = zlog.Output(zero_json_async_w)
        m.Run()
    }
    
    func BenchmarkJlog(b *testing.B) {
        for i := 0; i < b.N; i++ {
            jlog.V(0).Infoln(msg, i, b.N)
        }
    }
    
    func BenchmarkGlog(b *testing.B) {
        for i := 0; i < b.N; i++ {
            glog.V(0).Infoln(msg, i, b.N)
        }
    }
    
    func BenchmarkZapSugar(b *testing.B) {
        for i := 0; i < b.N; i++ {
            log.Info(msg, i, b.N)
        }
    }
    
    func BenchmarkZeroLogUnformatAsyncWrite(b *testing.B) {
        zlog.Logger = zeroLog_unformat_async
        for i := 0; i < b.N; i++ {
            zlog.Info().Msgf("%s %d", msg, i)
        }
    }
    
    func BenchmarkJlogJson(b *testing.B) {
        for i := 0; i < b.N; i++ {
            jlog.V(0).Infos().Uint("idx", 65).Uint64("sum", 6555).Uint32("idx", 6555).Uint16("sum", 10).Uint8("idx", 10).Int("IDX:", 10).Int64("idx:", 100).Int32("idx", 100).Int16("idx", 10).Int8("idx", 3).Bytes("sum", tempBuf).Float64("floast64", 0.464545).Float32("float32", 0.1545).Bool("bool", true).Time("ttt", now).Str("mstr", msg).Msg(msg)
        }
    }
    
    func BenchmarkZapJsonAsyncWrite(b *testing.B) {
        for i := 0; i < b.N; i++ {
            log.HttpInfo(msg, zap.Uint("idx", 65), zap.Uint64("sum", 6555), zap.Uint32("idx", 6555), zap.Uint16("sum", 10), zap.Uint8("idx", 10), zap.Int("IDX:", 10), zap.Int64("idx:", 100), zap.Int32("idx", 100), zap.Int16("idx", 10), zap.Int8("idx", 3), zap.Binary("sum", tempBuf), zap.Float64("floast64", 0.464545), zap.Float32("float32", 0.1545), zap.Bool("bool", true), zap.Time("ttt", now), zap.String("string", msg))
        }
    }
	
    func BenchmarkZeroLogJsonAsyncWrite(b *testing.B) {
        zlog.Logger = zeroLog_json_async
        for i := 0; i < b.N; i++ {
            zlog.Info().Uint("idx", 65).Uint64("sum", 6555).Uint32("idx", 6555).Uint16("sum", 10).Uint8("idx", 10).Int("IDX:", 10).Int64("idx:", 100).Int32("idx", 100).Int16("idx", 10).Int8("idx", 3).Bytes("sum", tempBuf).Float64("floast64", 0.464545).Float32("float32", 0.1545).Bool("bool", true).Time("ttt", now).Str("mstr", msg).Msg(msg)
        }
    }
	
    test result:

    goos: linux
    goarch: amd64
    pkg: jlog
    cpu: Intel Xeon E312xx (Sandy Bridge)
    BenchmarkGlog-2                        	  233276	      4775 ns/op	     344 B/op	       7 allocs/op
    BenchmarkJlog-2                        	  280686	      4041 ns/op	     301 B/op	       4 allocs/op
    BenchmarkJlogJson-2                    	  317008	      3773 ns/op	       0 B/op	       0 allocs/op
    BenchmarkZapSugar-2                    	  119136	      9196 ns/op	     937 B/op	      10 allocs/op
    BenchmarkZeroLogUnformatAsyncWrite-2   	   36118	     33062 ns/op	    5706 B/op	      43 allocs/op
    BenchmarkZapJsonAsyncWrite-2           	   65380	     18758 ns/op	    2788 B/op	      10 allocs/op
    BenchmarkZeroLogJsonAsyncWrite-2       	  227290	      5388 ns/op	       0 B/op	       0 allocs/op
    PASS
    ok  	jlog	9.036s	
    ps: test does not contain rotate or compress costs.   
```

# Example

```go
    package main

    import (
        "flag"
        "github.com/junjiefly/jlog"
    )
    
    func main() {
        flag.Parse()
        defer jlog.Flush() //or defer jlog.Shutdown()  # use Flush or Shutdown before process exits
        jlog.V(0).Infoln("hello world!")
    }
	
    [root@dev100 log]# go build main.go
    [root@dev100 log]# ./main -logDir=./log
    [root@dev100 log]# cat jlog.info 
    Log file opened at: 2024/02/14 16:39:33
    Running on machine: dev100
    2024-02-14 16:39:33.026 [I main.go:12] hello world!
```
  
