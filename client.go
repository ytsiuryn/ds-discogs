package discogs

import (
	"encoding/json"

	"github.com/gofrs/uuid"

	md "github.com/ytsiuryn/ds-audiomd"
)

type AudioOnlineRequest struct {
	Cmd     string      `json:"cmd"`
	Release *md.Release `json:"release"`
	// Actor
	// *md.Publishing
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
func ParseReleaseAnswer(data []byte) ([]*md.Suggestion, error) {
	suggestions := []*md.Suggestion{}
	if err := json.Unmarshal(data, &suggestions); err != nil {
		return nil, err
	}
	return suggestions, nil
}
