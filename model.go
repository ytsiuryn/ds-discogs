package discogs

import (
	"log"
	"strconv"
	"strings"

	md "github.com/ytsiuryn/ds-audiomd"
	collection "github.com/ytsiuryn/go-collection"
	intutils "github.com/ytsiuryn/go-intutils"
	tp "github.com/ytsiuryn/go-stringutils"
)

type label struct {
	Name           string `json:"name"`
	EntityType     string `json:"entity_type"`
	Catno          string `json:"catno"`
	ResourceURL    string `json:"resource_url"`
	ID             int32  `json:"id"`
	EntityTypeName string `json:"entity_type_name"`
}

type artist struct {
	Join        string `json:"join"`
	Name        string `json:"name"`
	Anv         string `json:"anv"`
	Tracks      string `json:"tracks"`
	Role        string `json:"role"`
	ResourceURL string `json:"resource_url"`
	ID          int32  `json:"id"`
}

type image struct {
	URI         string `json:"uri"`
	Height      int16  `json:"height"`
	Width       int16  `json:"width"`
	ResourceURL string `json:"resource_url"`
	Type        string `json:"type"`
	URI150      string `json:"uri150"`
}

type track struct {
	Duration     string   `json:"duration"`
	Position     string   `json:"position"`
	Type         string   `json:"type_"`
	Title        string   `json:"title"`
	SubTracks    []track  `json:"sub_tracks"`
	ExtraArtists []artist `json:"extraartists"`
}

// type identifier struct {
// 	Type  string `json:"type"`
// 	Value string `json:"value"`
// }

type format struct {
	Descriptions []string `json:"descriptions"`
	Name         string   `json:"name"`
	Qty          string   `json:"qty"`
	Text         string   `json:"text"`
}

// TODO: new structure, research it
// type company struct {
// 	Name           string `json:"name"`
// 	EntityType     string `json:"entity_type"`
// 	ThumbnailURL   string `json:"thumbnail_url"`
// 	Catno          string `json:"catno"`
// 	ResourceURL    string `json:"resource_url"`
// 	ID             int32  `json:"id"`
// 	EntityTypeName string `json:"entity_type_name"`
// }

type serie struct {
	Name           string `json:"name"`
	EntityType     string `json:"entity_type"`
	ThumbnailURL   string `json:"thumbnail_url"`
	Catno          string `json:"catno"`
	ResourceURL    string `json:"resource_url"`
	ID             int32  `json:"id"`
	EntityTypeName string `json:"entity_type_name"`
}

// releaseInfo is the common master/release structure for json release info conversion.
type releaseInfo struct {
	Styles      []string `json:"styles"`
	Series      []serie  `json:"series"`
	Labels      []label  `json:"labels"`
	Year        int32    `json:"year"`
	Artists     []artist `json:"artists"`
	Images      []image  `json:"images"`
	ID          int32    `json:"id"`
	ArtistsSort string   `json:"artists_sort"`
	Genres      []string `json:"genres"`
	// Thumb             string   `json:"thumb"`
	Title    string `json:"title"`
	MasterID int32  `json:"master_id"`
	// ReleasedFormatted string   `json:"released_formatted"`
	EstimatedWeight int16    `json:"estimated_weight"`
	MasterURL       string   `json:"master_url"`
	Released        string   `json:"released"`
	Tracklist       []track  `json:"tracklist"`
	ExtraArtists    []artist `json:"extraartists"`
	Country         string   `json:"country"`
	Notes           string   `json:"notes"`
	// Companies   []company `json:"companies"` - список вовлеченных в производство релиза компаний
	URL         string   `json:"uri"`
	Formats     []format `json:"formats"`
	ResourceURL string   `json:"resource_url"`
	MainRelease int32    `json:"main_release"`
}

type searchResult struct {
	Style       []string `json:"style"`
	Barcode     []string `json:"barcode"`
	Thumb       string   `json:"thumb"`
	URI         string   `json:"uri"`
	Title       string   `json:"title"`
	Country     string   `json:"country"`
	Format      []string `json:"format"`
	Label       []string `json:"label"`
	CoverImage  string   `json:"cover_image"`
	CatNo       string   `json:"catno"`
	MasterURL   string   `json:"master_url"`
	Year        string   `json:"year"`
	Genre       []string `json:"genre"`
	ResourceURL string   `json:"resource_url"`
	MasterID    int32    `json:"master_id"`
	Type        string   `json:"type"`
	ID          int32    `json:"id"`
}

type masterInfo struct {
	ID                   int32    `json:"id"`
	MainRelease          int32    `json:"main_release"`
	MostRecentRelease    int32    `json:"most_recent_release"`
	ResourceURL          string   `json:"resource_url"`
	URI                  string   `json:"uri"`
	VersionsURL          string   `json:"versions_url"`
	MainReleaseURL       string   `json:"main_release_url"`
	MostRecentReleaseURL string   `json:"most_recent_release_url"`
	Images               []image  `json:"images"`
	Genres               []string `json:"genres"`
	Styles               []string `json:"styles"`
	Year                 int32    `json:"year"`
	Tracklist            []track  `json:"tracklist"`
	Artists              []artist `json:"artists"`
	Title                string   `json:"title"`
	Notes                string   `json:"notes"`
}

// searchResponse is the search master list response.
type searchResponse struct {
	Results []searchResult `json:"results"`
}

// Search gatheres the common release info results.
func (sr *searchResponse) Search() []*md.Release {
	var releases []*md.Release
	for _, result := range sr.Results {
		r := md.NewRelease()
		r.IDs.Add(ServiceName, strconv.Itoa(int(result.ID)))
		r.Title = result.Title
		r.Year = tp.NaiveStringToInt(result.Year)
		for _, lblName := range result.Label {
			r.Publishing = append(
				r.Publishing,
				&md.Publishing{
					Name:  lblName,
					Catno: result.CatNo,
				},
			)
		}
		releases = append(releases, r)
	}
	return releases
}

// Master updates release with master page data.
func (mi *masterInfo) Master(r *md.Release) {
	r.Original.Year = int(mi.Year)
	r.Original.Notes = mi.Notes
}

// Release converts data to common release format.
func (ai *releaseInfo) Release(r *md.Release) {
	r.Title = ai.Title
	genres := append(ai.Genres, ai.Styles...)
	r.Country = ai.Country
	r.Year = int(ai.Year)
	r.Notes = ai.Notes
	r.IDs.Add(ServiceName, strconv.Itoa(int(ai.ID)))
	if ai.MasterID != 0 {
		r.Original.IDs.Add(ServiceName, strconv.Itoa(int(ai.MasterID)))
	}
	for _, artist := range ai.Artists {
		artist.ReleaseActor(r)
	}
	for _, tr := range ai.Tracklist {
		track := tr.Track()
		dn := md.DiscNumberByTrackPos(track.Position)
		track.LinkWithDisc(r.Disc(dn))
		r.Tracks = append(r.Tracks, track)
		r.TotalTracks++
	}
	for _, artist := range ai.ExtraArtists {
		if positions := artist.TrackPositions(); len(positions) > 0 {
			for _, pos := range positions {
				if tr := r.TrackByPosition(pos); tr != nil {
					artist.TrackActor(tr)
					tr.Record.Genres = append(tr.Record.Genres, genres...)
				}
			}
		}
	}
	r.Publishing = r.Publishing[:0] // to reset after preliminary search
	for _, lbl := range ai.Labels {
		lbl := lbl.Publishing()
		if !collection.Contains(lbl, r.Publishing) {
			r.Publishing = append(r.Publishing, lbl)
		}
	}
	for i, fmt := range ai.Formats {
		r.ReleaseType.DecodeSlice(&fmt.Descriptions)
		r.ReleaseStatus.DecodeSlice(&fmt.Descriptions)
		r.ReleaseRepeat.DecodeSlice(&fmt.Descriptions)
		r.ReleaseRemake.DecodeSlice(&fmt.Descriptions)
		r.ReleaseOrigin.DecodeSlice(&fmt.Descriptions)
		r.Discs[i].Format = fmt.DiscFormat()
		r.TotalDiscs++
	}
	var pia *md.PictureInAudio
	for _, img := range ai.Images {
		if pia = img.Cover(); pia != nil {
			r.Pictures = append(r.Pictures)
		}
	}
}

func (a *artist) TrackPositions() []string {
	positions := collection.SplitWithTrim(a.Tracks, ",")
	return positions
}

func (a *artist) ReleaseActor(r *md.Release) {
	if a.Role == "" {
		r.ActorRoles.Add(a.Name, "performer")
	} else {
		// TODO: РЕАЛиЗОВАТЬ!
		log.Panicf("НЕ РЕАЛИЗОВАНО ДЛЯ НЕ ПЕРФОРМЕРОВ: %s", a.Role)
		// actors := ActorsByRole(track, a.Role)
		// actor := actors.AddRole(a.Name, a.Role)
		// actor.IDs.Add(ServiceName, strconv.Itoa(int(a.ID)))
	}
}

func (a *artist) TrackActor(track *md.Track) {
	if a.Role == "" {
		track.Record.ActorRoles.Add(a.Name, "performer")
	} else {
		ActorsByRole(track, a.Role).Add(a.Name, a.Role)
	}
	track.Actors.Add(a.Name, ServiceName, strconv.Itoa(int(a.ID)))
}

func (tr *track) Track() *md.Track {
	track := md.NewTrack()
	if len(tr.SubTracks) > 0 {
		for _, sTrack := range tr.SubTracks {
			if len(tr.Position) > 0 {
				track.SetPosition(md.ComplexPosition(tr.Position, sTrack.Position))
			} else {
				track.SetPosition(sTrack.Position)
			}
			track.SetTitle(md.ComplexTitle(tr.Title, sTrack.Title))
			track.Duration = intutils.NewDurationFromString(sTrack.Duration)
			for _, artist := range tr.ExtraArtists {
				artist.TrackActor(track)
				track.Record.ActorRoles.Add(artist.Name, artist.Role)
			}
			for _, artist := range sTrack.ExtraArtists {
				artist.TrackActor(track)
				track.Record.ActorRoles.Add(artist.Name, artist.Role)
			}
		}
	} else {
		track.SetPosition(tr.Position)
		track.SetTitle(tr.Title)
		track.Duration = intutils.NewDurationFromString(tr.Duration)
		for _, artist := range tr.ExtraArtists {
			artist.TrackActor(track)
			track.Record.ActorRoles.Add(artist.Name, artist.Role)
		}
	}
	return track
}

func (lbl *label) Publishing() *md.Publishing {
	return &md.Publishing{
		Name:  lbl.Name,
		Catno: lbl.Catno,
		IDs:   map[string]string{ServiceName: strconv.Itoa(int(lbl.ID))},
	}
}

func (fmt *format) DiscFormat() *md.DiscFormat {
	return &md.DiscFormat{
		Media: md.DecodeMedia(fmt.Name),
		Attrs: append(fmt.Descriptions, fmt.Text),
	}
}

func (img *image) Cover() *md.PictureInAudio {
	if img.Type == "primary" {
		pictType := md.PictTypeCoverFront
		return &md.PictureInAudio{PictType: pictType, CoverURL: img.URI}
	}
	return nil
}

// ActorsByRole определяет коллекцию для размещения описания по наименованию роли.
func ActorsByRole(track *md.Track, roles string) *md.ActorRoles {
	var ret *md.ActorRoles
	if strings.Contains(roles, "Artwork") ||
		strings.Contains(roles, "Design") ||
		strings.Contains(roles, "Photography") {
		ret = &track.ActorRoles
	} else if strings.Contains(roles, "Composer") || // TODO: проверить!
		strings.Contains(roles, "Lyricist") ||
		strings.Contains(roles, "Written-By") {
		ret = &track.Composition.ActorRoles
	} else {
		ret = &track.Record.ActorRoles
	}
	return ret
}
