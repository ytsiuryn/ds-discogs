package discogs

import (
	"context"
	"os"
	"sync"
	"testing"

	md "github.com/ytsiuryn/ds-audiomd"
	srv "github.com/ytsiuryn/ds-microservice"
)

var mut sync.Mutex
var testService *Discogs

func TestBaseServiceCommands(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	startTestService(ctx)

	cl := srv.NewRPCClient()
	defer cl.Close()

	correlationID, data, _ := srv.CreateCmdRequest("ping")
	cl.Request(ServiceName, correlationID, data)
	respData := cl.Result(correlationID)
	if len(respData) != 0 {
		t.Fail()
	}

	correlationID, data, _ = srv.CreateCmdRequest("x")
	cl.Request(ServiceName, correlationID, data)
	vInfo, _ := srv.ParseErrorAnswer(cl.Result(correlationID))
	// {"error": "Unknown command: x", "context": "Message dispatcher"}
	if vInfo.Error != "Unknown command: x" {
		t.Fail()
	}
}

func TestSearchRelease(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	startTestService(ctx)

	cl := srv.NewRPCClient()
	defer cl.Close()

	r := md.NewRelease()
	r.Title = "The Dark Side Of The Moon"
	r.Year = 1977
	r.ActorRoles.Add("Pink Floyd", "performer")
	r.Publishing = append(r.Publishing, &md.Publishing{Name: "Harvest", Catno: "SHVL 804"})

	correlationID, data, _ := CreateReleaseRequest(r)
	cl.Request(ServiceName, correlationID, data)

	suggestions, _ := ParseReleaseAnswer(cl.Result(correlationID))

	if len(suggestions) == 0 || suggestions[0].Release.Title != "The Dark Side Of The Moon" {
		t.Fail()
	}
}

func startTestService(ctx context.Context) {
	mut.Lock()
	defer mut.Unlock()
	if testService == nil {
		testService = New(
			os.Getenv("DISCOGS_APP"),
			os.Getenv("DISCOGS_PERSONAL_TOKEN"))
		msgs := testService.ConnectToMessageBroker("amqp://guest:guest@localhost:5672/")
		// defer test.Cleanup()
		go testService.Start(msgs)
	}
}
