package discogs

import (
	"context"
	"os"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	md "github.com/ytsiuryn/ds-audiomd"
	srv "github.com/ytsiuryn/ds-microservice"
)

type DiscogsTestSuite struct {
	suite.Suite
	cl     *srv.RPCClient
	ctx    context.Context
	cancel context.CancelFunc
}

func (suite *DiscogsTestSuite) SetupSuite() {
	suite.ctx, suite.cancel = context.WithCancel(context.Background())
	suite.startTestService(suite.ctx)
	suite.cl = srv.NewRPCClient()
}

func (suite *DiscogsTestSuite) TearDownSuite() {
	suite.cancel()
	suite.cl.Close()
}

func (suite *DiscogsTestSuite) TestBaseServiceCommands() {
	correlationID, data, err := srv.CreateCmdRequest("ping")
	require.NoError(suite.T(), err)
	suite.cl.Request(ServiceName, correlationID, data)
	respData := suite.cl.Result(correlationID)
	suite.Empty(respData)

	correlationID, data, err = srv.CreateCmdRequest("x")
	require.NoError(suite.T(), err)
	suite.cl.Request(ServiceName, correlationID, data)
	vInfo, err := srv.ParseErrorAnswer(suite.cl.Result(correlationID))
	require.NoError(suite.T(), err)
	// {"error": "Unknown command: x", "context": "Message dispatcher"}
	suite.Equal(vInfo.Error, "Unknown command: x")
}

func (suite *DiscogsTestSuite) TestSearchRelease() {
	r := md.NewRelease()
	r.Title = "The Dark Side Of The Moon"
	r.Year = 1977
	r.ActorRoles.Add("Pink Floyd", "performer")
	r.Publishing = append(r.Publishing, &md.Publishing{Name: "Harvest", Catno: "SHVL 804"})

	correlationID, data, err := CreateReleaseRequest(r)
	require.NoError(suite.T(), err)
	suite.cl.Request(ServiceName, correlationID, data)

	set, err := ParseReleaseAnswer(suite.cl.Result(correlationID))
	require.NoError(suite.T(), err)

	suite.NotEmpty(set)
	suite.Equal(set.Suggestions[0].Release.Title, "The Dark Side Of The Moon")
}

func (suite *DiscogsTestSuite) startTestService(ctx context.Context) {
	testService := New(
		os.Getenv("DISCOGS_APP"),
		os.Getenv("DISCOGS_PERSONAL_TOKEN"))
	msgs := testService.ConnectToMessageBroker("amqp://guest:guest@localhost:5672/")
	testService.Log.SetLevel(log.DebugLevel)
	go testService.Start(msgs)
}

func TestDiscogsSuite(t *testing.T) {
	suite.Run(t, new(DiscogsTestSuite))
}
