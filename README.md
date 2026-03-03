[![Русский](https://img.shields.io/badge/Русский-%F0%9F%87%B7%F0%9F%87%BA-green?style=for-the-badge)](README_ru.md)

Simple asynchronous file logger for [Plugify](https://github.com/untrustedmodders/go-plugify) plugins.

### Features

- Asynchronous writing via a buffered channel
- Automatic daily log file rotation
- Log levels: `LOG`, `INFO`, `WARN`, `ERROR`, `DEBUG`
- `DEBUG` level is available only with the `debug` build tag
- Customizable date/time formats and buffer size

### Installation

```bash
go get github.com/fr0nch/logger
```

### Quick Start

```go
import (
    "github.com/fr0nch/logger"
    "github.com/untrustedmodders/go-plugify"
)

var log *logger.Logger

func init() {
    plugify.OnPluginStart(func() {
        var err error
        log, err = logger.New()
        
        if err != nil {
            panic(err)
        }
        
        log.Info("Plugin started.")
        log.Warnf("Connection timeout in %d seconds", 30)
        log.Error("Something went wrong.")
    })
    
    plugify.OnPluginEnd(func() {
        log.Info("Plugin is shutting down.")
        log.Close()
    })
}
```

### Log Directory

All log files are written to the Plugify `logs/` directory.

- By default (when `Folder` is empty), logs are saved directly in `logs/` with the plugin name as a file prefix: `logs/<plugin_name>_<date>.log`
- If the `Folder` option is set, a subdirectory is created inside `logs/` and log files are placed there without a prefix: `logs/<folder>/<date>.log`

### Custom Options

```go
log, err := logger.NewWithOptions(logger.Options{
    Folder:      "my-plugin",
    DateFmt:     "2006-01-02",
    TimeFmt:     "2006-01-02 15:04:05",
    ChanBufSize: 256,
})
