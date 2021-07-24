package discogs

import (
	"encoding/json"

	"github.com/gofrs/uuid"

	md "github.com/ytsiuryn/ds-audiomd"
	srv "github.com/ytsiuryn/ds-microservice"
)

type AudioOnlineRequest struct {
	Cmd     string      `json:"cmd"`
	Release *md.Release `json:"release"`
	// Actor
	// *md.Publishing
}

type AudioOnlineResponse struct {
	SuggestionSet *md.SuggestionSet `json:"suggestion_set,omitempty"`
	Error         srv.ErrorResponse `json:"error,omitempty"`
}

// CreateReleaseRequest формирует данные запроса поиска релиза по указанным метаданным.
func CreateReleaseRequest(r *md.Release) (string, []byte, error) {
	correlationID, _ := uuid.NewV4()
	req := AudioOnlineRequest{
		Cmd:     "release",
		Release: r}
	data, err := json.Marshal(&req)
	if err != nil {
		return "", nil, err
	}
	return correlationID.String(), data, nil
}

// ParseReleaseAnswer разбирает ответ с предложением метаданных релиза.
func ParseReleaseAnswer(data []byte) (*AudioOnlineResponse, error) {
	resp := AudioOnlineResponse{}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
