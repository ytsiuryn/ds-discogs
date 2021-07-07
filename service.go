package discogs

import (
	"encoding/json"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/streadway/amqp"

	md "github.com/ytsiuryn/ds-audiomd"
	srv "github.com/ytsiuryn/ds-microservice"
)

const ServiceName = "discogs"

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

	BaseURL       = "https://api.discogs.com/"
	RateHeaderKey = "X-Discogs-Ratelimit"
)

// Discogs описывает внутреннее состояние клиента Discogs.
type Discogs struct {
	*srv.Service
	headers map[string]string
	poller  *srv.WebPoller
}

// NewDiscogsClient создает объект нового клиента Discogs.
func NewDiscogsClient(app, token string) *Discogs {
	d := &Discogs{
		Service: srv.NewService(ServiceName),
		headers: map[string]string{
			"User-Agent":    app,
			"Authorization": "Discogs token=" + token,
		},
		poller: srv.NewWebPoller(time.Second)}

	d.SetVersionInfo(
		srv.ServiceInfo{
			Subsystem:   "audio",
			Name:        ServiceName,
			Description: "Discogs client"})

	return d
}

// TestPollingFrequency выполняет определение частоты опроса сервера на примере тестового запроса.
// Периодичность расчитывается в наносекундах.
func (d *Discogs) TestPollingFrequency() {
	resource := d.poller.Head(BaseURL, d.headers)
	if resource.Err != nil {
		srv.FailOnError(resource.Err, "Polling frequency testing")
	}
	v := resource.Response.Header["X-Discogs-Ratelimit"]
	rate, err := strconv.Atoi(string(v[0]))
	if err != nil {
		d.LogOnError(err, "header 'X-Discogs-Ratelimit' conversion")
		return
	}
	pollingInterval := time.Duration(60*1000/rate) * time.Millisecond

	d.poller.SetPollingInterval(pollingInterval)
	d.Log.Info("Polling interval: ", pollingInterval)
}

// Start запускает Web Poller и цикл обработки взодящих запросов.
// Контролирует сигнал завершения цикла и последующего освобождения ресурсов микросервиса.
func (d *Discogs) Start(msgs <-chan amqp.Delivery) {
	d.poller.Start()
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		var req AudioOnlineRequest
		for delivery := range msgs {
			if err := json.Unmarshal(delivery.Body, &req); err != nil {
				d.AnswerWithError(&delivery, err, "Message dispatcher")
				continue
			}
			d.logRequest(&req)
			d.RunCmd(&req, &delivery)
		}
	}()
	d.Log.Info("Awaiting RPC requests")
	go d.TestPollingFrequency()
	<-c
	d.Cleanup()
}

// Cleanup ..
func (d *Discogs) Cleanup() {
	d.Service.Cleanup()
}

// Отображение сведений о выполняемом запросе.
func (d *Discogs) logRequest(req *AudioOnlineRequest) {
	if req.Release != nil {
		if _, ok := req.Release.IDs[ServiceName]; ok {
			d.Log.WithField("args", req.Release.IDs[ServiceName]).Debug(req.Cmd + "()")
		} else { // TODO: может стоит офомить метод String() для md.Release?
			args := strings.Builder{}
			actor := string(req.Release.ActorRoles.First())
			if actor != "" {
				args.WriteString(actor)
			}
			if req.Release.Title != "" {
				args.WriteRune('-')
				args.WriteString(req.Release.Title)
			}
			if req.Release.Year != 0 {
				args.WriteRune('-')
				args.WriteString(strconv.Itoa(req.Release.Year))
			}
			d.Log.WithField("args", args.String()).Debug(req.Cmd + "()")
		}
	} else {
		d.Log.Debug(req.Cmd + "()")
	}
}

// RunCmd вызывает командам  запроса методы сервиса и возвращает результат клиенту.
func (d *Discogs) RunCmd(req *AudioOnlineRequest, delivery *amqp.Delivery) {
	switch req.Cmd {
	case "release":
		go d.release(req, delivery)
	default:
		d.Service.RunCmd(req.Cmd, delivery)
	}
}

// Обрабатываются следующие сущности: release (actor и label будут добавлены позже).
func (d *Discogs) release(request *AudioOnlineRequest, delivery *amqp.Delivery) {
	// разбор параметров входного запроса
	var err error
	var suggestions []*md.Suggestion
	if _, ok := request.Release.IDs[ServiceName]; ok {
		suggestions, err = d.searchReleaseByID(request.Release.IDs[ServiceName])
	} else {
		suggestions, err = d.searchReleaseByIncompleteData(request.Release)
	}
	if err != nil {
		d.AnswerWithError(delivery, err, "Getting release data")
		return
	}
	for _, suggestion := range suggestions {
		suggestion.Optimize()
	}
	// отправка ответа
	if suggestionsJSON, err := json.Marshal(suggestions); err != nil {
		d.AnswerWithError(delivery, err, "Response")
	} else {
		d.Log.Debug(string(suggestionsJSON))
		d.Answer(delivery, suggestionsJSON)
	}
}

func (d *Discogs) searchReleaseByID(id string) ([]*md.Suggestion, error) {
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

func (d *Discogs) searchReleaseByIncompleteData(release *md.Release) ([]*md.Suggestion, error) {
	var suggestions []*md.Suggestion
	// discogs release search...
	var preResult searchResponse
	if err := d.poller.Decode(searchURL(release, "release"), d.headers, &preResult); err != nil {
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
	d.Log.WithField("results", len(suggestions)).Debug("Preliminary search")
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
	d.Log.WithField("results", len(suggestions)).Debug("Suggestions")
	return suggestions, nil
}

func (d *Discogs) releaseByID(id string, release *md.Release) error {
	// сведения о релизе...
	var releaseResp releaseInfo
	if err := d.poller.Decode(BaseURL+"releases/"+id, d.headers, &releaseResp); err != nil {
		return err
	}
	releaseResp.Release(release)
	// сведения о мастер-релизе...
	if releaseResp.MasterURL != "" {
		var masterResp masterInfo
		if err := d.poller.Decode(releaseResp.MasterURL, d.headers, &masterResp); err != nil {
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
	builder := strings.Builder{}
	builder.WriteString(BaseURL)
	builder.WriteString("database/search?type=")
	builder.WriteString(entityType)
	builder.WriteString(release.Title)
	if performers := release.ActorRoles.Filter(md.IsPerformer); len(performers) > 0 {
		for actorName := range performers {
			builder.WriteString("&artist=")
			builder.WriteString(string(actorName))
		}
	}
	if len(release.Publishing) > 0 {
		if len(release.Publishing[0].Name) > 0 {
			builder.WriteString("&label=")
			builder.WriteString(release.Publishing[0].Name)
		}
		if len(release.Publishing[0].Catno) > 0 {
			builder.WriteString("&catno=")
			builder.WriteString(release.Publishing[0].Catno)
		}
	}
	if release.Year != 0 {
		builder.WriteString("&year=")
		builder.WriteString(strconv.Itoa(int(release.Year)))
	}
	return builder.String()
}
