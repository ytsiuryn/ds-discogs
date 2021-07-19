package discogs

import (
	"context"
	"os"
	"sync"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

	correlationID, data, err := srv.CreateCmdRequest("ping")
	require.NoError(t, err)
	cl.Request(ServiceName, correlationID, data)
	respData := cl.Result(correlationID)
	assert.Equal(t, len(respData), 0)

	correlationID, data, err = srv.CreateCmdRequest("x")
	require.NoError(t, err)
	cl.Request(ServiceName, correlationID, data)
	vInfo, err := srv.ParseErrorAnswer(cl.Result(correlationID))
	require.NoError(t, err)
	// {"error": "Unknown command: x", "context": "Message dispatcher"}
	assert.Equal(t, vInfo.Error, "Unknown command: x")
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

	correlationID, data, err := CreateReleaseRequest(r)
	require.NoError(t, err)
	cl.Request(ServiceName, correlationID, data)

	suggestions, err := ParseReleaseAnswer(cl.Result(correlationID))
	require.NoError(t, err)

	assert.NotEmpty(t, suggestions)
	assert.Equal(t, suggestions[0].Release.Title, "The Dark Side Of The Moon")
}

func startTestService(ctx context.Context) {
	mut.Lock()
	defer mut.Unlock()
	if testService == nil {
		testService = New(
			os.Getenv("DISCOGS_APP"),
			os.Getenv("DISCOGS_PERSONAL_TOKEN"))
		msgs := testService.ConnectToMessageBroker("amqp://guest:guest@localhost:5672/")
		testService.Log.SetLevel(log.DebugLevel)
		go testService.Start(msgs)
	}
}
