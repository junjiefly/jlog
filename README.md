# jlog
    * jlog is a log library developed to adapt to high concurrency scenarios. 
    * Pursuing extreme performance and concise configuration. 
    * There is no need to worry about disk space, as it will automatically packs and compresses logs. 
    * You only need to import the package to run directly immediately.
    Try it now!

#Bench
```go
    import (
        "fmt"
        "github.com/golang/glog"
        "github.com/junjiefly/jlog"
        "jlog/log"  //assemble zap here
        "testing"
    )
    var count int64 = 10000
    var msg string
    func TestMain(m *testing.M) {
        log.InitLog()
        bufSize := 512
        var buf = make([]byte, bufSize)
        for i := 0; i < bufSize; i++ {
           buf[i] = 'a'
        }
        msg = string(buf)
        m.Run()
    }
    
    func BenchmarkJlog(b *testing.B) {
        for i := 0; i < b.N; i++ {
            jlog.V(0).Infoln(msg, count)
        }
    }
    
    func BenchmarkZap(b *testing.B) {
        for i := 0; i < b.N; i++ {
            log.HttpInfo(msg, count)
        }
    }
    
    func BenchmarkGlog(b *testing.B) {
        for i := 0; i < b.N; i++ {
            glog.V(0).Infoln(msg, count)
        }
    }
    
    func BenchmarkZapSugar(b *testing.B) {
        for i := 0; i < b.N; i++ {
            log.Info(msg, count)
        }
    }

    goos: linux
    goarch: amd64
    pkg: jlog
    cpu: Intel Xeon E312xx (Sandy Bridge)
    BenchmarkJlog-2       	  250899	      4427 ns/op	     341 B/op	       6 allocs/op
    BenchmarkZap-2        	  145455	      7992 ns/op	     384 B/op	       7 allocs/op
    BenchmarkGlog-2       	  253213	      4562 ns/op	     304 B/op	       7 allocs/op
    BenchmarkZapSugar-2   	  126445	      8909 ns/op	     921 B/op	       9 allocs/op
    PASS
    ok  	jlog	4.875s

    ps: test does contains rotate or compress costs.   
```

#Example

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
  
