# go-discogs #

Микросервис-клиент [Discogs](https://www.discogs.com/developers). Обмен сообщениями реализован с использованием [RabbitMQ](https://www.rabbitmq.com).

Пример использования:

    package main

    import (
	    "flag"
	    "fmt"

	    log "github.com/sirupsen/logrus"

	    discogs "github.com/gtyrin/go-discogs"
	    srv "github.com/gtyrin/go-service"
    )

    func main() {
	    connstr := flag.String(
		    "msg-server",
		    "amqp://guest:guest@localhost:5672/",
		    "Message server connection string")
	    idle := flag.Bool(
		    "idle",
		    false,
		    "Free-running mode of the service to the message queue cleaning")
	    flag.Parse()

	    log.Info(
		    fmt.Sprintf("%s %s starting in %s mode..",
			    discogs.ServiceName, srv.RunModeName(*idle)))

	    cl, err := discogs.NewDiscogsClient(*connstr)
	    srv.FailOnError(err, "Failed to create Discogs client")

	    err = cl.TestPollingFrequency()
	    srv.FailOnError(err, "Failed to test polling frequency")

	    cl.Idle = *idle

	    defer cl.Close()

	    cl.Dispatch(cl)
    }

