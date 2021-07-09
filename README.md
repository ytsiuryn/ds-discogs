# ds-discogs #

Микросервис-клиент [Discogs](https://www.discogs.com/developers). Обмен сообщениями с микросервисом реализован с использованием [RabbitMQ](https://www.rabbitmq.com).

Команды микросервиса:
---
|Команда|                    Назначение                         |
|-------|-------------------------------------------------------|
|search |поиск метаданных по неполным данным или ID в БД Discogs|
|ping   |проверка жизнеспособности микросервиса                 |
|info   |информация о микросервисе                              |

*Пример использования команд приведен в тестовом клиенте в [discogs.py](https://github.com/ytsiuryn/ds-discogs/blob/main/discogs.py)*.

Файл настроек (YAML):
---
|  Секция/параметр  |                            Назначение                         |
|-------------------|---------------------------------------------------------------|
|auth               |секция с данными авторизации на сервере Discogs                |
|auth.app           |наименование зарегистрированного приложения                    |
|auth.personal_token|секретный код клиента, полученный в ходе регистрации на Discogs|

Пример запуска микросервиса:
---
```go
    package main

    import (
	    "flag"
	    "fmt"

	    log "github.com/sirupsen/logrus"

	    discogs "github.com/ytsiuryn/ds-discogs"
	    srv "github.com/ytsiuryn/ds-service"
    )

    func main() {
	    connstr := flag.String(
		    "msg-server",
		    "amqp://guest:guest@localhost:5672/",
		    "Message server connection string")

		product := flag.Bool(
			"product",
			false,
			"product-режим запуска сервиса")

		flag.Parse()

	    log.Info(fmt.Sprintf("%s starting..", discogs.ServiceName))

	    cl := discogs.New(
			os.Getenv("DISCOGS_APP"),
			os.Getenv("DISCOGS_PERSONAL_TOKEN"))

	    msgs := testService.ConnectToMessageBroker("amqp://guest:guest@localhost:5672/")

		if *product {
			reader.Log.SetLevel(log.InfoLevel)
		} else {
			reader.Log.SetLevel(log.DebugLevel)
		}

		cl.Start(msgs)
    }
```

Пример клиента (Python тест)
---
См. файл [discogs.py](https://github.com/ytsiuryn/ds-discogs/blob/main/discogs.py)