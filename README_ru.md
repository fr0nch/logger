[![English](https://img.shields.io/badge/English-%F0%9F%87%AC%F0%9F%87%A7-blue?style=for-the-badge)](README.md)

Простой асинхронный файловый логгер для плагинов [Plugify](https://github.com/untrustedmodders/go-plugify).

### Возможности

- Асинхронная запись через буферизированный канал
- Автоматическая ротация лог-файлов по дням
- Уровни логирования: `LOG`, `INFO`, `WARN`, `ERROR`, `DEBUG`
- Уровень `DEBUG` доступен только с build-тегом `debug`
- Настраиваемые форматы даты/времени и размер буфера

### Установка

```bash
go get github.com/fr0nch/logger
```

### Быстрый старт

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

        log.Info("Плагин запущен.")
        log.Warnf("Таймаут подключения через %d секунд", 30)
        log.Error("Что-то пошло не так.")
    })
    
    plugify.OnPluginEnd(func() {
        log.Info("Плагин выключается.")
        log.Close()
    })
}
```

### Директория логов

Все лог-файлы записываются в директорию `logs/` Plugify.

- По умолчанию (когда `Folder` пустой) логи сохраняются прямо в `logs/` с именем плагина в качестве префикса файла: `logs/<имя_плагина>_<дата>.log`
- Если указан параметр `Folder`, внутри `logs/` создаётся подпапка и лог-файлы размещаются в ней без префикса: `logs/<folder>/<дата>.log`

### Кастомные настройки

```go
log, err := logger.NewWithOptions(logger.Options{
    Folder:      "my-plugin",  // логи будут записываться в logs/my-plugin/
    DateFmt:     "2006-01-02",
    TimeFmt:     "2006-01-02 15:04:05",
    ChanBufSize: 256,
})
```