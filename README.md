# jlog

jlog is a high-performance Go logging library focused on low allocation, low
overhead, and concise configuration. It writes application logs and HTTP access
logs to two separate files and automatically rotates and compresses them.

## Features

- Text APIs: `Debugf`, `Infof`, `Warnf`, `Errorf`, plus `Debugln`, `Infoln`,
  `Warnln`, and `Errorln`.
- Structured JSON APIs: `Debug`, `Info`, `Warn`, and `Error`.
- Separate log files: application records go to `<LogDir>/<FileName>.log`; HTTP
  records from `Http` and `Https` go to `<LogDir>/<FileName>.http`.
- Custom structured logs with dedicated file names and fixed fields.
- Level thresholds: `DebugLevel < InfoLevel < WarnLevel < ErrorLevel`.
- Fatal records are always written by `Fatalln` and `Fatalf`.
- Automatic rotation and compression.
- Multiple output destinations and stdout support.
- Configurable time format.
- External writers receive log records only; local-file open metadata is not
  forwarded to them.

## Installation

```bash
go get github.com/junjiefly/jlog
```

## Quick Start

```go
package main

import (
    "github.com/junjiefly/jlog"
)

func main() {
    if err := jlog.InitWithDefaultConfig(); err != nil {
        panic(err)
    }
    defer func() { _ = jlog.Flush() }()

    jlog.Infof("hello %s", "world")
    jlog.Info().
        Str("key", "value").
        Int("count", 1).
        Msg("structured log")
}
```

The default configuration writes to `log/<program name>.log` and
`log/<program name>.http`, uses `InfoLevel`, and starts a background flush
goroutine with a 5 second interval.

## Configuration

Use `InitWithConfig` for explicit settings:

```go
if err := jlog.InitWithConfig(jlog.Config{
    LogDir:        "./log",
    FileName:      "myapp",
    FlushInterval: 5,
    MaxSize:       100, // MB
    MaxBackups:    10,
    MaxAge:        30, // days
    Compress:      true,
    Stdout:        false,
    LocalWrite:    true,
    Level:         jlog.InfoLevel,
}); err != nil {
    panic(err)
}
defer func() { _ = jlog.Shutdown() }()
```

`InitWithConfig` can be called again to replace the configuration. It reuses the
existing flush goroutine when the flush interval is unchanged and restarts it
when the interval changes.

`Shutdown` stops the flush goroutine and releases both built-in logs. Non-fatal logging
calls are ignored after `Shutdown` until `InitWithDefaultConfig` or
`InitWithConfig` is called again; `Fatalln` and `Fatalf` still write.

Zero-valued configuration fields use these defaults: `LogDir: "log"`,
`FileName: <program name>`, `FlushInterval: 30`, `MaxSize: 100` MB,
`MaxBackups: 0`, and `MaxAge: 0`. `Level` values outside `DebugLevel` through
`ErrorLevel` are clamped.

## Levels

The default threshold is `InfoLevel`. Use one function to change it at runtime:

```go
jlog.SetLevel(jlog.DebugLevel)
```

Valid values are `DebugLevel`, `InfoLevel`, `WarnLevel`, and `ErrorLevel`.
Values outside that range are clamped to the nearest valid level. Records below
the threshold are dropped. Fatal records are not level gated.

## Structured Logs

`Debug`, `Info`, `Warn`, and `Error` return an entry. At or above the configured
threshold, the returned entry is non-nil. Below the threshold, it returns nil,
and every chained method is safe to call on the nil result.

```go
jlog.Info().
    Bool("ok", true).
    Int64("size", 1024).
    Err(err).
    Str("requestId", requestId).
    Msg("request handled")
```

Structured records are written as JSON and string values are escaped. `Err`
writes a non-nil error under the `error` key and omits the field for nil.
`Msg` closes the JSON object and writes the record.
The built-in `time`, `level`, and `msg` keys are reserved for application
structured records.

## HTTP Logs

```go
start := time.Now().UnixNano()

jlog.Https(r, reqID, r.Host, start, http.StatusOK, "", size, err)
```

HTTP records are written to the `.http` file separately from application logs
and do not contain a level field. Plain-text `Http` records escape CR and LF in
request-derived fields so one request cannot inject additional log lines.
The `.http` destination is implemented with the same internal custom-log
mechanism as user-defined structured logs.

## Custom Logs

Register a custom structured log when its dedicated file is needed:

```go
audit, err := jlog.RegisterCustomLog(jlog.CustomLogConfig{
    Name:       "audit",
    FileSuffix: ".jsonl",
    Fields: map[string]interface{}{
        "service": "payments",
        "version": 2,
    },
})
if err != nil {
    panic(err)
}

audit.Event().
    Str("action", "refund").
    Int64("amount", 1999).
    Msg("")
```

The output path is `<LogDir>/<FileName>.<Name><FileSuffix>`, for example
`log/myapp.audit.jsonl`. A custom log uses the application configuration for
directory, rotation, compression, flush interval, stdout, external writers, and
local file output.

`Fields` are written to every record after `time` and before event fields. Keys
are sorted once during registration. Supported fixed-field values are `nil`,
`string`, `[]byte`, `bool`, signed and unsigned integers, and `float32` or
`float64`. Keys named `time`, `msg`, or `level` are rejected, and event fields
cannot override fixed fields.

Custom-log events use structured entry methods such as `Str`, `Int`, `Bool`,
and `Msg`; they do not use `Infof` or `Infoln`. They are not level gated by
`SetLevel` and do not contain a `level` field. Register custom logs once during
startup. `Flush`, `Shutdown`, and a later `InitWithConfig` include registered
custom logs; records are ignored after `Shutdown` until initialization runs
again.

## Time Format

```go
jlog.TimeFormat(time.RFC3339Nano)
```

The default format is `2006-01-02T15:04:05.000` and does not depend on the local
timezone suffix.

## Historical Benchmark

On Linux/amd64 with an Intel Xeon E312xx, a previous jlog benchmark reported:

```
BenchmarkJlog      280686    4041 ns/op    301 B/op     4 allocs/op
BenchmarkJlogJson  317008    3773 ns/op      0 B/op     0 allocs/op
```

The benchmark did not include rotation or compression costs.
