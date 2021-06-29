package client

import (
	"encoding/json"
	"fmt"
	"strconv"

	log "github.com/sirupsen/logrus"
	"github.com/streadway/amqp"

	md "github.com/ytsiuryn/ds-audiomd"
	srv "github.com/ytsiuryn/ds-service"
)

// Описание сервиса
const (
	ServiceSubsystem   = "audio"
	ServiceName        = "discogs"
	ServiceDescription = "Discogs client"
)

// Suggestion constants
const (
	MinSearchShortResult = .5
	MinSearchFullResult  = .75
	MaxPreSuggestions    = 7
	MaxSuggestions       = 3
)

// Client constants
const (
	// RequestTokenURL = "https://api.discogs.com/oauth/request_token"
	// AuthorizeURL    = "https://www.discogs.com/oauth/authorize"
	// AccessTokenURL  = "https://api.discogs.com/oauth/access_token"

	APIBaseURL    = "https://api.discogs.com/"
	RateHeaderKey = "X-Discogs-Ratelimit"
)

type config struct {
	Auth struct {
		App string `yaml:"app"`
		// Key           string `yaml:"key"`
		// Secret        string `yaml:"secret"`
		PersonalToken string `yaml:"personal_token"`
	}
	Product bool `yaml:"product"`
}

// Discogs описывает внутреннее состояние клиента Discogs.
type Discogs struct {
	*srv.PollingService
	conf *config
}

// NewDiscogsClient создает объект нового клиента Discogs.
func NewDiscogsClient(connstr, optFile string) (*Discogs, error) {
	conf := config{}

	srv.ReadConfig(optFile, &conf)

	log.SetLevel(srv.LogLevel(conf.Product))

	cl := &Discogs{
		conf: &conf,
		PollingService: srv.NewPollingService(
			map[string]string{
				"User-Agent":    conf.Auth.App,
				"Authorization": "Discogs token=" + conf.Auth.PersonalToken,
			}),
	}
	cl.ConnectToMessageBroker(connstr, ServiceName)

	return cl, nil
}

// TestPollingFrequency выполняет определение частоты опроса сервера на примере тестового запроса.
// Периодичность расчитывается в наносекундах.
func (d *Discogs) TestPollingFrequency() error {
	resp, err := d.TestResource(APIBaseURL)
	if err != nil {
		return err
	}
	v, ok := resp.Header[RateHeaderKey]
	if !ok {
		return fmt.Errorf("header '%s' does not exists", RateHeaderKey)
	}
	rate, err := strconv.Atoi(v[0])
	if err != nil {
		return err
	}
	d.SetPollingFrequency(int64(60 * 1000_000_000 / int64(rate)))
	return nil
}

// Cleanup ..
func (d *Discogs) Cleanup() {
	d.Service.Cleanup()
}

// RunCmdByName выполняет команды и возвращает результат клиенту в виде JSON-сообщения.
func (d *Discogs) RunCmdByName(cmd string, delivery *amqp.Delivery) {
	switch cmd {
	case "search":
		go d.search(delivery)
	case "info":
		version := srv.Version{
			Subsystem:   ServiceSubsystem,
			Name:        ServiceName,
			Description: ServiceDescription,
		}
		go d.Service.Info(delivery, &version)
	default:
		d.Service.RunCommonCmd(cmd, delivery)
	}
}

// Обрабатываются следующие сущности: release, actor и label.
//
// Параметры JSON-запроса:
//
// - для release используются ключи "release_id" или "release" (по частичным данным в ds.audio.metadata.Release)
//
// - для actor используются ключи "actor_id" или "actor" (по частичным данным в ds.audio.metadata.Actor)
//
// - для label используются ключи "label_id" или "label" (по частичным данным в ds.audio.metadata.ReleaseLabel)
func (d *Discogs) search(delivery *amqp.Delivery) {
	if d.Idle {
		res := []*md.Suggestion{}
		suggestionsJSON, err := json.Marshal(res)
		if err != nil {
			d.ErrorResult(delivery, err, "Response")
			return
		}
		d.Answer(delivery, suggestionsJSON)
		return
	}
	// прием входного запроса
	var request srv.Request
	err := json.Unmarshal(delivery.Body, &request)
	if err != nil {
		d.ErrorResult(delivery, err, "Request")
		return
	}
	// разбор параметров входного запроса
	var suggestions []*md.Suggestion
	if _, ok := request.Params["release_id"]; ok {
		suggestions, err = d.searchReleaseByID(&request)
		if err != nil {
			d.ErrorResult(delivery, err, "Release by ID")
			return
		}
	} else if _, ok := request.Params["release"]; ok {
		suggestions, err = d.searchReleaseByIncompleteData(&request)
		if err != nil {
			d.ErrorResult(delivery, err, "Release by incomplete data")
			return
		}
	}
	for _, suggestion := range suggestions {
		suggestion.Optimize()
	}
	// отправка ответа
	suggestionsJSON, err := json.Marshal(suggestions)
	if err != nil {
		d.ErrorResult(delivery, err, "Response")
		return
	}
	if !d.conf.Product {
		log.Println(string(suggestionsJSON))
	}
	d.Answer(delivery, suggestionsJSON)
}

func (d *Discogs) searchReleaseByID(request *srv.Request) ([]*md.Suggestion, error) {
	id := request.Params["release_id"]
	r := md.NewRelease()
	if err := d.releaseByID(id, r); err != nil {
		return nil, err
	}
	return []*md.Suggestion{
			{
				Release:          r,
				ServiceName:      ServiceName,
				OnlineSuggeston:  true,
				SourceSimilarity: 1.,
			}},
		nil
}

func (d *Discogs) searchReleaseByIncompleteData(request *srv.Request) ([]*md.Suggestion, error) {
	var suggestions []*md.Suggestion
	// params
	release, err := request.ParseRelease()
	if err != nil {
		return nil, err
	}
	// discogs release search...
	var preResult searchResponse
	if err := d.LoadAndDecode(searchURL(release, "release"), &preResult); err != nil {
		return nil, err
	}
	var score float64
	// предварительные предложения
	for _, r := range preResult.Search() {
		if score = release.Compare(r); score > MinSearchShortResult {
			suggestions = append(
				suggestions,
				&md.Suggestion{
					Release:          r,
					ServiceName:      ServiceName,
					OnlineSuggeston:  true,
					SourceSimilarity: score,
				})
		}
	}
	suggestions = md.BestNResults(suggestions, MaxPreSuggestions)
	log.WithField("results", len(suggestions)).Debug("Preliminary search")
	// окончательные предложения
	for i := len(suggestions) - 1; i >= 0; i-- {
		r := suggestions[i].Release
		if err := d.releaseByID(r.IDs[ServiceName], r); err != nil {
			return nil, err
		}
		if score = release.Compare(r); score > MinSearchFullResult {
			suggestions[i].Release = r
			suggestions[i].SourceSimilarity = score
		} else {
			suggestions = append(suggestions[:i], suggestions[i+1:]...)
		}
	}
	suggestions = md.BestNResults(suggestions, MaxSuggestions)
	log.WithField("results", len(suggestions)).Debug("Suggestions")
	return suggestions, nil
}

func (d *Discogs) releaseByID(id string, release *md.Release) error {
	// сведения о релизе...
	var releaseResp releaseInfo
	if err := d.LoadAndDecode(APIBaseURL+"releases/"+id, &releaseResp); err != nil {
		return err
	}
	releaseResp.Release(release)
	// сведения о мастер-релизе...
	if releaseResp.MasterURL != "" {
		var masterResp masterInfo
		if err := d.LoadAndDecode(releaseResp.MasterURL, &masterResp); err != nil {
			return err
		}
		masterResp.Master(release)
	}
	return nil
}

// All an artist releases
// /artists/{artist_id}/releases{?sort,sort_order}
// All a label releases
// /labels/{label_id}/releases{?page,per_page}
// GET /database/search?q={query}&{?type,title,release_title,credit,artist,anv,label,genre,style,country,year,format,catno,barcode,track,submitter,contributor}
// type: release, master, artist, label
func searchURL(release *md.Release, entityType string) string {
	ret := APIBaseURL + "database/search?type=" + entityType
	ret += "&release_title=" + release.Title
	if performers := release.ActorRoles.Filter(md.IsPerformer); len(performers) > 0 {
		for actorName := range performers {
			ret += "&artist=" + string(actorName)
		}
	}
	if len(release.Publishing) > 0 {
		if len(release.Publishing[0].Name) > 0 {
			ret += "&label=" + release.Publishing[0].Name
		}
		if len(release.Publishing[0].Catno) > 0 {
			ret += "&catno=" + release.Publishing[0].Catno
		}
	}
	if release.Year != 0 {
		ret += "&year=" + strconv.Itoa(int(release.Year))
	}
	return ret
}
