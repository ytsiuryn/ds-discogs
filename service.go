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

// Константы микросервиса.
const (
	ServiceName = "discogs"
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

	BaseURL       = "https://api.discogs.com/"
	RateHeaderKey = "X-Discogs-Ratelimit"
)

// Discogs описывает внутреннее состояние клиента Discogs.
type Discogs struct {
	*srv.Service
	headers map[string]string
	poller  *srv.WebPoller
}

// New создает объект нового клиента Discogs.
func New(app, token string) *Discogs {
	ret := &Discogs{
		Service: srv.NewService(ServiceName),
		headers: map[string]string{
			"User-Agent":    app,
			"Authorization": "Discogs token=" + token,
		},
		poller: srv.NewWebPoller(time.Second)}
	ret.poller.Log = ret.Log
	return ret
}

// AnswerWithError заполняет структуру ответа информацией об ошибке.
func (d *Discogs) AnswerWithError(delivery *amqp.Delivery, err error, context string) {
	d.LogOnErrorWithContext(err, context)
	req := &AudioOnlineResponse{
		Error: &srv.ErrorResponse{
			Error:   err.Error(),
			Context: context,
		},
	}
	data, err := json.Marshal(req)
	srv.FailOnError(err, "Answer marshalling error")
	d.Answer(delivery, data)
}

// TestPollingInterval выполняет определение частоты опроса сервера на примере тестового запроса.
// Периодичность расчитывается в наносекундах.
func (d *Discogs) TestPollingInterval() {
	resource := d.poller.Head(BaseURL, d.headers)
	if resource.Err != nil {
		srv.FailOnError(resource.Err, "Polling interval testing")
	}

	if len(resource.Response.Header) == 0 {
		d.Log.Warn(`Define DISCOGS_APP/DISCOGS_PERSONAL_TOKEN environment variables
	to achieve the maximum number of requests per minute`)
		d.poller.SetPollingInterval(time.Duration(2400) * time.Millisecond)
		return
	}

	rate, err := strconv.Atoi(string(resource.Response.Header[RateHeaderKey][0]))
	if err != nil {
		d.LogOnErrorWithContext(err, "header 'X-Discogs-Ratelimit' conversion")
		return
	}
	pollingInterval := time.Duration(60*1000/rate) * time.Millisecond

	d.poller.SetPollingInterval(pollingInterval)
	d.Log.Info("Polling interval: ", pollingInterval)
}

// StartWithConnection запускает Web Poller и цикл обработки входящих запросов.
// Контролирует сигнал завершения цикла и последующего освобождения ресурсов микросервиса.
func (d *Discogs) StartWithConnection(connstr string) {
	msgs := d.Service.ConnectToMessageBroker(connstr)

	d.poller.Start()
	go d.TestPollingInterval()

	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		for delivery := range msgs {
			req := NewAudioOnlineRequest()
			if err := json.Unmarshal(delivery.Body, req); err != nil {
				d.AnswerWithError(&delivery, err, "Message dispatcher")
				continue
			}
			d.logRequest(req)
			d.RunCmd(req, &delivery)
		}
	}()

	d.Log.Info("Awaiting RPC requests")
	<-c

	d.cleanup()
}

func (d *Discogs) cleanup() {
	d.Service.Cleanup()
}

// Отображение сведений о выполняемом запросе.
func (d *Discogs) logRequest(req *AudioOnlineRequest) {
	if req.Release != nil {
		if _, ok := req.Release.IDs[md.DiscogsReleaseID]; ok {
			d.Log.WithField("release", req.Release.IDs[md.DiscogsReleaseID]).Info(req.Cmd + "()")
		} else { // TODO: может стоит офомить метод String() для md.Release?
			var args []string
			if actor := req.Release.ActorRoles.Filter(md.IsPerformer).First(); actor != "" {
				args = append(args, actor)
			}
			if req.Release.Title != "" {
				args = append(args, req.Release.Title)
			}
			if req.Release.Year != 0 {
				args = append(args, strconv.Itoa(req.Release.Year))
			}
			d.Log.WithField("release", strings.Join(args, "-")).Info(req.Cmd + "()")
		}
	} else {
		d.Log.Info(req.Cmd + "()")
	}
}

// RunCmd вызывает командам  запроса методы сервиса и возвращает результат клиенту.
func (d *Discogs) RunCmd(req *AudioOnlineRequest, delivery *amqp.Delivery) {
	var data []byte
	var err error

	switch req.Cmd {
	case "release":
		data, err = d.release(req)
	default:
		d.Service.RunCmd(req.Cmd, delivery)
		return
	}

	if err != nil {
		d.AnswerWithError(delivery, err, req.Cmd)
	} else {
		d.Log.Debug(string(data))
		d.Answer(delivery, data)
	}
}

// Обрабатываются следующие сущности: release (actor и label будут добавлены позже).
func (d *Discogs) release(request *AudioOnlineRequest) ([]byte, error) {
	var err error
	var set *md.SuggestionSet

	if _, ok := request.Release.IDs[md.DiscogsReleaseID]; ok {
		set, err = d.searchReleaseByID(request.Release.IDs[md.DiscogsReleaseID])
	} else {
		set, err = d.searchReleaseByIncompleteData(request.Release)
	}
	if err != nil {
		return nil, err
	}

	set.Optimize()

	return json.Marshal(AudioOnlineResponse{SuggestionSet: set})
}

func (d *Discogs) searchReleaseByID(id string) (*md.SuggestionSet, error) {
	r := md.NewRelease()
	if err := d.releaseByID(id, r); err != nil {
		return nil, err
	}
	set := md.NewSuggestionSet()
	set.Suggestions = []*md.Suggestion{
		{
			Release:          r,
			ServiceName:      ServiceName,
			SourceSimilarity: 1.,
		}}
	return set, nil
}

func (d *Discogs) searchReleaseByIncompleteData(release *md.Release) (*md.SuggestionSet, error) {
	var suggestions []*md.Suggestion
	// discogs release search...
	var preResult searchResponse
	if err := d.poller.DecodeJSON(searchURL(release, "release"), d.headers, &preResult); err != nil {
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
					SourceSimilarity: score,
				})
		}
	}
	suggestions = md.BestNResults(suggestions, MaxPreSuggestions)
	d.Log.WithField("results", len(suggestions)).Debug("Preliminary search")
	// окончательные предложения
	for i := len(suggestions) - 1; i >= 0; i-- {
		r := suggestions[i].Release
		if err := d.releaseByID(r.IDs[md.DiscogsReleaseID], r); err != nil {
			return nil, err
		}
		if score = release.Compare(r); score > MinSearchFullResult {
			suggestions[i].SourceSimilarity = score
		} else {
			suggestions = append(suggestions[:i], suggestions[i+1:]...)
		}
	}
	suggestions = md.BestNResults(suggestions, MaxSuggestions)
	d.Log.WithField("results", len(suggestions)).Debug("Suggestions")

	set := md.NewSuggestionSet()
	set.Suggestions = suggestions

	return set, nil
}

func (d *Discogs) releaseByID(id string, release *md.Release) error {
	// сведения о релизе...
	var releaseResp releaseInfo
	if err := d.poller.DecodeJSON(BaseURL+"releases/"+id, d.headers, &releaseResp); err != nil {
		return err
	}
	releaseResp.Release(release)
	// сведения о мастер-релизе...
	if releaseResp.MasterURL != "" {
		var masterResp masterInfo
		if err := d.poller.DecodeJSON(releaseResp.MasterURL, d.headers, &masterResp); err != nil {
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
	builder.WriteString("&title=")
	builder.WriteString(release.Title)
	if performers := release.ActorRoles.Filter(md.IsPerformer); len(performers) > 0 {
		for actorName := range performers {
			builder.WriteString("&artist=")
			builder.WriteString(actorName)
		}
	}
	if len(release.Publishing.Labels) > 0 {
		if len(release.Publishing.Labels[0].Label) > 0 {
			builder.WriteString("&label=")
			builder.WriteString(release.Publishing.Labels[0].Label)
		}
		if len(release.Publishing.Labels[0].Catno) > 0 {
			builder.WriteString("&catno=")
			builder.WriteString(release.Publishing.Labels[0].Catno)
		}
	}
	if release.Year != 0 {
		builder.WriteString("&year=")
		builder.WriteString(strconv.Itoa(int(release.Year)))
	}
	return builder.String()
}
