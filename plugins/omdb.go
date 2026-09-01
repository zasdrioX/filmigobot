// (c) Jisin0
// Hybrid IMDb Direct + Python Bridge + TMDB/OMDb Fallback Engine.

package plugins

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Jisin0/filmigo/omdb"
	"github.com/PaulSonOfLars/gotgbot/v2"
)

const (
	omdbBanner   = "https://telegra.ph/file/e810982a269773daa42a9.png"
	omdbHomepage = "https://imdb.com"
	notAvailable = "N/A"

	topCastLimit    = 30
	enableTelegraph = true
	tmdbKey         = "1b4ba621cf09ae9752dd659e6e55307b"
)

var (
	omdbClient       *omdb.OmdbClient
	searchMethodOMDb = "omdb"
	telegraphToken   string
	httpClient       = &http.Client{Timeout: 10 * time.Second}
)

func init() {
	if OmdbApiKey != "" {
		omdbClient = omdb.NewClient(OmdbApiKey)
		inlineSearchButtons = append(inlineSearchButtons, []gotgbot.InlineKeyboardButton{{Text: "🔍 Search OMDb", SwitchInlineQueryCurrentChat: &inlineOMDbSwitch}})
	}
	if enableTelegraph {
		go ensureTelegraphToken()
	}
}

type UniversalSearchResult struct {
	ID     string
	Title  string
	Year   int
	Poster string
	Type   string
	Rating float64
}

func getPythonCmd() string {
	if _, err := os.Stat("./venv/bin/python3"); err == nil {
		return "./venv/bin/python3"
	}
	if _, err := os.Stat("./venv/Scripts/python.exe"); err == nil {
		return "./venv/Scripts/python.exe"
	}
	return "python3"
}

func ensureTelegraphToken() {
	if telegraphToken != "" {
		return
	}
	if resp, err := httpClient.Get("https://api.telegra.ph/createAccount?short_name=FilmigoBot&author_name=Filmigo+Bot"); err == nil {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		var res struct {
			Ok     bool `json:"ok"`
			Result struct {
				AccessToken string `json:"access_token"`
			} `json:"result"`
		}
		json.Unmarshal(body, &res)
		if res.Ok {
			telegraphToken = res.Result.AccessToken
		}
	}
}

type tgNode struct {
	Tag      string   `json:"tag"`
	Attrs    *tgAttrs `json:"attrs,omitempty"`
	Children []any    `json:"children,omitempty"`
}
type tgAttrs struct {
	Src  string `json:"src,omitempty"`
	Href string `json:"href,omitempty"`
}

func createTelegraphPage(title string, nodes []tgNode) string {
	ensureTelegraphToken()
	if telegraphToken == "" {
		return ""
	}
	contentBytes, err := json.Marshal(nodes)
	if err != nil {
		return ""
	}
	data := url.Values{}
	data.Set("access_token", telegraphToken)
	data.Set("title", title)
	data.Set("content", string(contentBytes))
	data.Set("return_content", "false")
	resp, err := httpClient.PostForm("https://api.telegra.ph/createPage", data)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var res struct {
		Ok     bool `json:"ok"`
		Result struct {
			Url string `json:"url"`
		} `json:"result"`
	}
	json.Unmarshal(body, &res)
	return res.Result.Url
}

func makeRow(label, value string) tgNode {
	return tgNode{Tag: "p", Children: []any{tgNode{Tag: "b", Children: []any{label + ": "}}, value}}
}
func makeHeader(text string) tgNode { return tgNode{Tag: "h4", Children: []any{text}} }

type imdbGqlSearchRes struct {
	Data struct {
		Results struct {
			Edges []struct {
				Node struct {
					Entity struct {
						ID        string `json:"id"`
						TitleText struct {
							Text string `json:"text"`
						} `json:"titleText"`
						ReleaseYear struct {
							Year int `json:"year"`
						} `json:"releaseYear"`
						TitleType struct {
							Text string `json:"text"`
						} `json:"titleType"`
						PrimaryImage struct {
							URL string `json:"url"`
						} `json:"primaryImage"`
						RatingsSummary struct {
							AggregateRating float64 `json:"aggregateRating"`
						} `json:"ratingsSummary"`
					} `json:"entity"`
				} `json:"node"`
			} `json:"edges"`
		} `json:"results"`
	} `json:"data"`
}

type imdbPerson struct {
	Name   string `json:"name"`
	ImdbID string `json:"imdbId"`
}

type pythonImdbRes struct {
	ImdbID           string       `json:"imdbId"`
	Title            string       `json:"title"`
	TitleAkas        []string     `json:"title_akas"`
	Kind             string       `json:"kind"`
	CoverUrl         string       `json:"cover_url"`
	Plot             string       `json:"plot"`
	ReleaseDate      string       `json:"release_date"`
	LanguagesText    []string     `json:"languages_text"`
	Mpaa             string       `json:"mpaa"`
	Year             int          `json:"year"`
	YearEnd          *int         `json:"year_end"`
	Duration         int          `json:"duration"`
	Countries        []string     `json:"countries"`
	Rating           float64      `json:"rating"`
	MetacriticRating int          `json:"metacritic_rating"`
	Votes            int          `json:"votes"`
	Awards           struct {
		Wins        int `json:"wins"`
		Nominations int `json:"nominations"`
	} `json:"awards"`
	Trailers          []string     `json:"trailers"`
	Genres            []string     `json:"genres"`
	StorylineKeywords []string     `json:"storyline_keywords"`
	WorldwideGross    string       `json:"worldwide_gross"`
	ProductionBudget  string       `json:"production_budget"`
	Directors         []imdbPerson `json:"directors"`
	Stars             []imdbPerson `json:"stars"`
	Categories        struct {
		Cast []struct {
			Name       string   `json:"name"`
			ImdbID     string   `json:"imdbId"`
			Characters []string `json:"characters"`
		} `json:"cast"`
		Writer   []imdbPerson `json:"writer"`
		Producer []imdbPerson `json:"producer"`
	} `json:"categories"`
}

type tmdbMultiRes struct {
	Results []struct {
		ID           int     `json:"id"`
		MediaType    string  `json:"media_type"`
		Title        string  `json:"title"`
		Name         string  `json:"name"`
		ReleaseDate  string  `json:"release_date"`
		FirstAirDate string  `json:"first_air_date"`
		PosterPath   string  `json:"poster_path"`
		VoteAverage  float64 `json:"vote_average"`
	} `json:"results"`
}
type tmdbFindRes struct {
	MovieResults []struct{ ID int `json:"id"` } `json:"movie_results"`
	TvResults    []struct{ ID int `json:"id"` } `json:"tv_results"`
}
type tmdbDetail struct {
	ID                  int      `json:"id"`
	Title               string   `json:"title"`
	Name                string   `json:"name"`
	OriginalTitle       string   `json:"original_title"`
	OriginalName        string   `json:"original_name"`
	Overview            string   `json:"overview"`
	Tagline             string   `json:"tagline"`
	ReleaseDate         string   `json:"release_date"`
	FirstAirDate        string   `json:"first_air_date"`
	LastAirDate         string   `json:"last_air_date"`
	Runtime             int      `json:"runtime"`
	EpisodeRunTime      []int    `json:"episode_run_time"`
	NumberOfSeasons     int      `json:"number_of_seasons"`
	NumberOfEpisodes    int      `json:"number_of_episodes"`
	VoteAverage         float64  `json:"vote_average"`
	VoteCount           int      `json:"vote_count"`
	Popularity          float64  `json:"popularity"`
	Genres              []struct{ Name string `json:"name"` } `json:"genres"`
	PosterPath          string   `json:"poster_path"`
	SpokenLanguages     []struct{ EnglishName string `json:"english_name"`; Name string `json:"name"` } `json:"spoken_languages"`
	ProductionCountries []struct{ Iso3166_1 string `json:"iso_3166_1"`; Name string `json:"name"` } `json:"production_countries"`
	ProductionCompanies []struct{ Name string `json:"name"` } `json:"production_companies"`
	Networks            []struct{ Name string `json:"name"` } `json:"networks"`
	Budget              int      `json:"budget"`
	Revenue             int      `json:"revenue"`
	Status              string   `json:"status"`
	Credits struct {
		Cast []struct{ ID int `json:"id"`; Name string `json:"name"`; Character string `json:"character"` } `json:"cast"`
		Crew []struct{ ID int `json:"id"`; Name string `json:"name"`; Job string `json:"job"`; Department string `json:"department"` } `json:"crew"`
	} `json:"credits"`
	Keywords struct {
		Keywords []struct{ Name string `json:"name"` } `json:"keywords"`
		Results  []struct{ Name string `json:"name"` } `json:"results"`
	} `json:"keywords"`
	Videos struct {
		Results []struct{ Key string `json:"key"`; Site string `json:"site"`; Type string `json:"type"` } `json:"results"`
	} `json:"videos"`
	ExternalIds struct {
		ImdbId string `json:"imdb_id"`
	} `json:"external_ids"`
}
type omdbRating struct {
	Source string `json:"Source"`
	Value  string `json:"Value"`
}
type omdbFillData struct {
	Released     string       `json:"Released"`
	Awards       string       `json:"Awards"`
	TotalSeasons string       `json:"totalSeasons"`
	Country      string       `json:"Country"`
	Poster       string       `json:"Poster"`
	BoxOffice    string       `json:"BoxOffice"`
	Rated        string       `json:"Rated"`
	Metascore    string       `json:"Metascore"`
	ImdbRating   string       `json:"imdbRating"`
	ImdbVotes    string       `json:"imdbVotes"`
	Ratings      []omdbRating `json:"Ratings"`
}

func parseYear(d string) int {
	if len(d) >= 4 {
		y, _ := strconv.Atoi(d[:4])
		return y
	}
	return 0
}

func extractIMDbID(text string) string {
	re := regexp.MustCompile(`tt\d+`)
	return re.FindString(text)
}

func SearchIMDbDirect(query string) ([]UniversalSearchResult, error) {
	reqBody := map[string]any{
		"variables": map[string]any{
			"includeAdult":      false,
			"isExactMatch":      false,
			"locale":            "en-US",
			"numResults":        25,
			"originalTitleText": false,
			"searchTerm":        query,
			"skipHasExact":      true,
			"typeFilter":        "TITLE",
		},
		"extensions": map[string]any{
			"persistedQuery": map[string]any{
				"version":    1,
				"sha256Hash": "600c8ca2deb61df89fced826818a7b5bdfc5539c39402a8bd285221aedbfa99a",
			},
		},
	}
	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", "https://caching.graphql.imdb.com/", bytes.NewBuffer(jsonBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("accept", "application/graphql+json, application/json")
	req.Header.Set("content-type", "application/json")
	req.Header.Set("referer", "https://www.imdb.com/")
	req.Header.Set("origin", "https://www.imdb.com")
	req.Header.Set("user-agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/148.0.0.0 Safari/537.36")
	req.Header.Set("x-imdb-client-name", "imdb-web-next-localized")
	req.Header.Set("x-imdb-user-country", "US")
	req.Header.Set("x-imdb-user-language", "en-US")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("imdb status %d", resp.StatusCode)
	}

	var data imdbGqlSearchRes
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	var results []UniversalSearchResult
	for _, edge := range data.Data.Results.Edges {
		ent := edge.Node.Entity
		if ent.ID != "" && ent.TitleText.Text != "" {
			tType := ent.TitleType.Text
			if tType == "" {
				tType = "Title"
			}
			results = append(results, UniversalSearchResult{
				ID:     ent.ID,
				Title:  ent.TitleText.Text,
				Year:   ent.ReleaseYear.Year,
				Poster: ent.PrimaryImage.URL,
				Type:   tType,
				Rating: ent.RatingsSummary.AggregateRating,
			})
		}
	}
	if len(results) == 0 {
		return nil, errors.New("empty imdb graphql results")
	}
	return results, nil
}

func SearchOMDb(query string) ([]UniversalSearchResult, error) {
	imdbID := extractIMDbID(query)
	if imdbID != "" {
		if r, err := httpClient.Get(fmt.Sprintf("https://api.themoviedb.org/3/find/%s?external_source=imdb_id&api_key=%s", imdbID, tmdbKey)); err == nil {
			defer r.Body.Close()
			var d tmdbFindRes
			json.NewDecoder(r.Body).Decode(&d)
			var id int
			var mType string
			if len(d.MovieResults) > 0 {
				id = d.MovieResults[0].ID
				mType = "movie"
			} else if len(d.TvResults) > 0 {
				id = d.TvResults[0].ID
				mType = "tv"
			}
			if id != 0 {
				if r2, err2 := httpClient.Get(fmt.Sprintf("https://api.themoviedb.org/3/%s/%d?api_key=%s", mType, id, tmdbKey)); err2 == nil {
					defer r2.Body.Close()
					var det tmdbDetail
					json.NewDecoder(r2.Body).Decode(&det)
					t := det.Title
					if t == "" {
						t = det.Name
					}
					date := det.ReleaseDate
					if date == "" {
						date = det.FirstAirDate
					}
					tag := "Movie"
					if mType == "tv" {
						tag = "TV Series"
					}
					return []UniversalSearchResult{{
						ID:     imdbID,
						Title:  t,
						Year:   parseYear(date),
						Poster: det.PosterPath,
						Type:   tag,
						Rating: det.VoteAverage,
					}}, nil
				}
			}
		}
	}

	if res, err := SearchIMDbDirect(query); err == nil && len(res) > 0 {
		return res, nil
	}

	r, err := httpClient.Get(fmt.Sprintf("https://api.themoviedb.org/3/search/multi?query=%s&api_key=%s", url.QueryEscape(query), tmdbKey))
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()
	var data tmdbMultiRes
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		return nil, err
	}
	var results []UniversalSearchResult
	for _, i := range data.Results {
		if i.MediaType == "person" {
			continue
		}
		title := i.Title
		if title == "" {
			title = i.Name
		}
		date := i.ReleaseDate
		if date == "" {
			date = i.FirstAirDate
		}
		typeTag := "Movie"
		if i.MediaType == "tv" {
			typeTag = "TV Series"
		}
		results = append(results, UniversalSearchResult{
			ID:     fmt.Sprintf("%s-%d", i.MediaType, i.ID),
			Title:  title,
			Year:   parseYear(date),
			Poster: i.PosterPath,
			Type:   typeTag,
			Rating: i.VoteAverage,
		})
	}
	if len(results) == 0 {
		return nil, errors.New("no results")
	}
	return results, nil
}

func OMDbInlineSearch(query string) []gotgbot.InlineQueryResult {
	results, err := SearchOMDb(query)
	if err != nil {
		return nil
	}
	tgResults := make([]gotgbot.InlineQueryResult, 0, len(results))
	for _, item := range results {
		posterURL := item.Poster
		if posterURL == "" || posterURL == "N/A" {
			posterURL = omdbBanner
		} else if !strings.HasPrefix(posterURL, "http") {
			posterURL = "https://image.tmdb.org/t/p/w200" + posterURL
		}
		title := item.Title
		if item.Year > 0 {
			title = fmt.Sprintf("%s [%d]", item.Title, item.Year)
		}
		description := item.Type
		if item.Rating > 0 {
			description = fmt.Sprintf("%s | Ratings: %.1f ⭐", item.Type, item.Rating)
		} else {
			description = fmt.Sprintf("%s | Ratings: N/A", item.Type)
		}
		tgResults = append(tgResults, gotgbot.InlineQueryResultArticle{
			Id:          searchMethodOMDb + "_" + item.ID,
			Title:       title,
			Description: description,
			ThumbnailUrl: posterURL,
			InputMessageContent: gotgbot.InputTextMessageContent{
				MessageText: fmt.Sprintf("<i>Loading details for %s...</i>", item.Title),
				ParseMode:   gotgbot.ParseModeHTML,
			},
			ReplyMarkup: &gotgbot.InlineKeyboardMarkup{
				InlineKeyboard: [][]gotgbot.InlineKeyboardButton{
					{{Text: "Open IMDb", CallbackData: fmt.Sprintf("open_%s_%s", searchMethodOMDb, item.ID)}},
				},
			},
		})
	}
	return tgResults
}

func buildPythonDetails(imdbID string) (string, string, [][]gotgbot.InlineKeyboardButton, error) {
	var buttons [][]gotgbot.InlineKeyboardButton
	pyScript := `
import sys, json
try:
    from imdbinfo.services import get_movie, _load_waf_cookies, request_handler
    waf_cookies = _load_waf_cookies() or {}
    if not waf_cookies:
        try:
            request_handler("https://www.imdb.com/")
            waf_cookies = _load_waf_cookies() or {}
        except Exception:
            pass
    movie = get_movie(sys.argv[1])
    if movie:
        res = movie.model_dump() if hasattr(movie, "model_dump") else movie
        print(json.dumps(res, default=str))
except Exception as e:
    import traceback
    traceback.print_exc(file=sys.stderr)
`
	pyCmd := getPythonCmd()
	cmd := exec.Command(pyCmd, "-c", pyScript, imdbID)
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil || out.Len() == 0 {
		return "", "", buttons, errors.New("python bridge failed")
	}

	var p pythonImdbRes
	if err := json.Unmarshal(out.Bytes(), &p); err != nil || p.Title == "" {
		return "", "", buttons, errors.New("python bridge parse failed")
	}

	var sb strings.Builder
	isSeries := strings.Contains(strings.ToLower(p.Kind), "series") || strings.Contains(strings.ToLower(p.Kind), "tv")
	typeStr := "Movie"
	if isSeries {
		typeStr = "TV Series"
	}

	yearStr := ""
	if isSeries {
		if p.YearEnd != nil && *p.YearEnd > p.Year {
			yearStr = fmt.Sprintf("[%d-%d]", p.Year, *p.YearEnd)
		} else if p.Year > 0 {
			yearStr = fmt.Sprintf("[%d-Present]", p.Year)
		}
	} else if p.Year > 0 {
		yearStr = fmt.Sprintf("[%d]", p.Year)
	}

	displayImdb := p.ImdbID
	if displayImdb == "" {
		displayImdb = imdbID
	}
	sb.WriteString(fmt.Sprintf("<i>%s: </i><b>%s %s</b> | <a href=\"%s\">IMDb Link</a>\n", typeStr, p.Title, yearStr, "https://imdb.com/title/"+displayImdb))

	var akas []string
	if len(p.TitleAkas) > 0 && p.TitleAkas[0] != p.Title {
		akas = append(akas, p.TitleAkas[0])
	}
	if len(akas) > 0 {
		sb.WriteString(fmt.Sprintf("<i>(AKA %s)</i>\n", strings.Join(akas, ", ")))
	}

	if p.Duration > 0 {
		dur := fmt.Sprintf("%dh %dm", p.Duration/60, p.Duration%60)
		if isSeries {
			dur += "/Episode"
		}
		sb.WriteString(fmt.Sprintf("<i>Duration: </i>%s\n", dur))
	}

	if p.ReleaseDate != "" {
		dateStr := p.ReleaseDate
		if pt, err := time.Parse("2006-01-02", p.ReleaseDate); err == nil {
			dateStr = pt.Format("02 January 2006")
		}
		flag := ""
		if len(p.Countries) > 0 {
			flag = getFlag(p.Countries[0])
		}
		if flag != "" {
			dateStr += fmt.Sprintf(" (%s)", flag)
		}
		if isSeries {
			dateStr += " - First Episode"
		}
		sb.WriteString(fmt.Sprintf("<i>Release Date: </i>%s\n", dateStr))
	}

	ratingLine := ""
	if p.Rating > 0 {
		ratingLine += fmt.Sprintf("Rating ⭐️ %.1f / 10 (from %d votes)", p.Rating, p.Votes)
	}
	if p.MetacriticRating > 0 {
		if ratingLine != "" {
			ratingLine += " | "
		}
		ratingLine += fmt.Sprintf("Ⓜ️ %d/100", p.MetacriticRating)
	}
	if p.Mpaa != "" {
		mpaaClean := p.Mpaa
		if strings.HasPrefix(mpaaClean, "Rated ") {
			parts := strings.Split(mpaaClean, " ")
			if len(parts) > 1 {
				mpaaClean = parts[1]
			}
		}
		if ratingLine != "" {
			ratingLine += " | "
		}
		ratingLine += mpaaClean
	}
	if ratingLine != "" {
		sb.WriteString(ratingLine + "\n")
	}

	var bq1 []string
	var gEmojiMap = map[string]string{
		"Action": "💥", "Adventure": "🗺️", "Sci-Fi": "🚀", "Science Fiction": "🚀",
		"Comedy": "🤣", "Drama": "🎭", "Romance": "🌹", "Thriller": "🔪",
		"Horror": "👻", "Fantasy": "✨", "Mystery": "❓", "Music": "🎶",
	}
	if len(p.Genres) > 0 {
		var gs []string
		for _, g := range p.Genres {
			emoji := "- "
			if e, ok := gEmojiMap[g]; ok {
				emoji = e + " "
			}
			gs = append(gs, fmt.Sprintf("%s#%s", emoji, strings.ReplaceAll(g, " ", "_")))
		}
		bq1 = append(bq1, fmt.Sprintf("<i>Genres: </i>%s", strings.Join(gs, " ")))
	}

	var themes []string
	for i, k := range p.StorylineKeywords {
		if i >= 6 {
			break
		}
		themes = append(themes, "#"+strings.ReplaceAll(strings.Title(k), " ", "_"))
	}
	if len(themes) > 0 {
		bq1 = append(bq1, fmt.Sprintf("<i>Themes: </i>%s", strings.Join(themes, " ")))
	}

	var lgs, cgs []string
	for _, l := range p.LanguagesText {
		lgs = append(lgs, "#"+strings.ReplaceAll(l, " ", "_"))
	}
	for _, c := range p.Countries {
		f := getFlag(c)
		if f != "" {
			f += " "
		}
		cgs = append(cgs, fmt.Sprintf("%s#%s", f, strings.ReplaceAll(c, " ", "_")))
	}
	if len(lgs) > 0 || len(cgs) > 0 {
		bq1 = append(bq1, fmt.Sprintf("<i>Language (Country): </i>%s (%s)", strings.Join(lgs, " "), strings.Join(cgs, " ")))
	}

	if len(bq1) > 0 {
		sb.WriteString(fmt.Sprintf("<blockquote>%s</blockquote>\n\n", strings.Join(bq1, "\n")))
	}

	shortOverview := p.Plot
	if rs := []rune(p.Plot); len(rs) > 800 {
		shortOverview = string(rs[:797]) + "..."
	}
	if p.Plot != "" {
		sb.WriteString(fmt.Sprintf("<blockquote><b>Story Line: </b><i>%s</i></blockquote>\n\n", shortOverview))
	}

	var bq3 []string
	var dirs []string
	for _, d := range p.Directors {
		if len(dirs) < 3 {
			dirs = append(dirs, link(d.Name, d.ImdbID))
		}
	}
	if len(dirs) > 0 {
		bq3 = append(bq3, fmt.Sprintf("<i><b>Directors:</b></i> %s", strings.Join(dirs, ", ")))
	}

	var writers []string
	for _, w := range p.Categories.Writer {
		if len(writers) < 3 {
			writers = append(writers, link(w.Name, w.ImdbID))
		}
	}
	if len(writers) > 0 {
		bq3 = append(bq3, fmt.Sprintf("<i><b>Writers:</b></i> %s", strings.Join(writers, ", ")))
	}

	var prods []string
	for _, pr := range p.Categories.Producer {
		if len(prods) < 3 {
			prods = append(prods, link(pr.Name, pr.ImdbID))
		}
	}
	if len(prods) > 0 {
		bq3 = append(bq3, fmt.Sprintf("<i><b>Producers:</b></i> %s", strings.Join(prods, ", ")))
	}

	var stars []string
	if len(p.Stars) > 0 {
		for i, s := range p.Stars {
			if i < 4 {
				stars = append(stars, link(s.Name, s.ImdbID))
			}
		}
	} else if len(p.Categories.Cast) > 0 {
		for i, c := range p.Categories.Cast {
			if i < 4 {
				stars = append(stars, link(c.Name, c.ImdbID))
			}
		}
	}
	if len(stars) > 0 {
		bq3 = append(bq3, fmt.Sprintf("<i><b>Stars:</b></i> %s", strings.Join(stars, ", ")))
	}

	var cast []string
	for i, c := range p.Categories.Cast {
		if i < topCastLimit {
			cast = append(cast, link(c.Name, c.ImdbID))
		}
	}
	if len(cast) > 0 {
		bq3 = append(bq3, fmt.Sprintf("<i><b>Top Cast:</b></i> %s", strings.Join(cast, ", ")))
	}

	if len(bq3) > 0 {
		sb.WriteString(fmt.Sprintf("<blockquote>%s</blockquote>\n\n", strings.Join(bq3, "\n")))
	}

	if p.Awards.Wins > 0 || p.Awards.Nominations > 0 {
		sb.WriteString(fmt.Sprintf("<b>Awards: </b><a href=\"https://imdb.com/title/%s/awards\">Won %d Awards. %d Nominations</a>\n", displayImdb, p.Awards.Wins, p.Awards.Nominations))
	}
	sb.WriteString(fmt.Sprintf("<b>OTT Info: </b><a href=\"https://www.justwatch.com/in/search?q=%s\">Find on JustWatch</a>\n", url.QueryEscape(p.Title)))

	dl := omdbBanner
	if p.CoverUrl != "" {
		dl = p.CoverUrl
	}

	if enableTelegraph {
		var nodes []tgNode
		nodes = append(nodes, tgNode{Tag: "h3", Children: []any{tgNode{Tag: "b", Children: []any{fmt.Sprintf("%s %s", p.Title, yearStr)}}}})
		if dl != omdbBanner {
			nodes = append(nodes, tgNode{Tag: "figure", Children: []any{tgNode{Tag: "img", Attrs: &tgAttrs{Src: dl}}}})
		}

		nodes = append(nodes, makeHeader("Overview"), tgNode{Tag: "p", Children: []any{p.Plot}})
		nodes = append(nodes, makeHeader("General Information"), makeRow("Type", typeStr))

		if len(akas) > 0 {
			nodes = append(nodes, makeRow("Alternate Titles", strings.Join(akas, ", ")))
		}
		nodes = append(nodes, makeRow("Runtime", fmt.Sprintf("%d minutes", p.Duration)))

		nodes = append(nodes, makeHeader("Ratings"))
		nodes = append(nodes, makeRow("IMDb Rating", fmt.Sprintf("%.1f/10 (from %d votes)", p.Rating, p.Votes)))
		if p.MetacriticRating > 0 {
			nodes = append(nodes, makeRow("Metascore", strconv.Itoa(p.MetacriticRating)))
		}

		nodes = append(nodes, makeHeader("Genres & Themes"))
		if len(p.Genres) > 0 {
			nodes = append(nodes, makeRow("Genres", strings.Join(p.Genres, ", ")))
		}

		nodes = append(nodes, makeHeader("Financials"))
		if p.ProductionBudget != "" {
			nodes = append(nodes, makeRow("Budget", p.ProductionBudget))
		}
		if p.WorldwideGross != "" {
			nodes = append(nodes, makeRow("Worldwide Gross", p.WorldwideGross))
		}

		var castList []string
		for _, c := range p.Categories.Cast {
			role := ""
			if len(c.Characters) > 0 {
				role = " as " + c.Characters[0]
			}
			castList = append(castList, c.Name+role)
		}
		if len(castList) > 0 {
			nodes = append(nodes, makeHeader("Full Cast"), tgNode{Tag: "p", Children: []any{strings.Join(castList, ", ")}})
		}

		page := createTelegraphPage(p.Title+" Details", nodes)
		sb.WriteString(fmt.Sprintf("\n<a href=\"https://imdb.com/title/%s\">Read More...</a>", displayImdb))
		if page != "" {
			sb.WriteString(fmt.Sprintf(" | <a href=\"%s\">Full Details</a>", page))
		}
	} else {
		sb.WriteString(fmt.Sprintf("\n<a href=\"https://imdb.com/title/%s\">Read More...</a>", displayImdb))
	}

	trailerLink := ""
	if len(p.Trailers) > 0 {
		trailerLink = p.Trailers[0]
	}
	if trailerLink != "" {
		sb.WriteString(fmt.Sprintf(" | <a href=\"%s\">Trailer</a>", trailerLink))
	}
	sb.WriteString(fmt.Sprintf(" | <a href=\"%s\">Download Poster</a>", dl))

	return dl, sb.String(), buttons, nil
}

func buildTMDBDetails(imdbID, mType, tmdbID string) (string, string, [][]gotgbot.InlineKeyboardButton, error) {
	var buttons [][]gotgbot.InlineKeyboardButton
	var t tmdbDetail
	if tmdbID != "" {
		r, err := httpClient.Get(fmt.Sprintf("https://api.themoviedb.org/3/%s/%s?append_to_response=credits,keywords,videos,external_ids&api_key=%s", mType, tmdbID, tmdbKey))
		if err == nil {
			defer r.Body.Close()
			json.NewDecoder(r.Body).Decode(&t)
		}
	}

	if imdbID == "" && t.ExternalIds.ImdbId != "" {
		imdbID = t.ExternalIds.ImdbId
	}

	var omdbFill omdbFillData
	if imdbID != "" && OmdbApiKey != "" {
		if ro, eo := httpClient.Get(fmt.Sprintf("https://www.omdbapi.com/?i=%s&apikey=%s", imdbID, OmdbApiKey)); eo == nil {
			json.NewDecoder(ro.Body).Decode(&omdbFill)
			ro.Body.Close()
		}
	}

	if t.Title == "" && t.Name == "" && omdbFill.Released == "" {
		return "", "", buttons, errors.New("not found")
	}

	isSeries := (mType == "tv")
	title := t.Title
	if title == "" {
		title = t.Name
	}
	origTitle := t.OriginalTitle
	if origTitle == "" {
		origTitle = t.OriginalName
	}
	dateStr := t.ReleaseDate
	if dateStr == "" {
		dateStr = t.FirstAirDate
	}
	year := parseYear(dateStr)
	lastYear := parseYear(t.LastAirDate)

	var sb strings.Builder
	typeStr := "Movie"
	if isSeries {
		typeStr = "TV Series"
	}

	yearStr := ""
	if isSeries {
		if lastYear > year {
			yearStr = fmt.Sprintf("[%d-%d]", year, lastYear)
		} else if lastYear == 0 && year > 0 {
			yearStr = fmt.Sprintf("[%d-Present]", year)
		} else if year > 0 {
			yearStr = fmt.Sprintf("[%d]", year)
		}
	} else if year > 0 {
		yearStr = fmt.Sprintf("[%d]", year)
	}

	displayImdb := imdbID
	if displayImdb == "" {
		displayImdb = "tt0000000"
	}

	sb.WriteString(fmt.Sprintf("<i>%s: </i><b>%s %s</b> | <a href=\"https://imdb.com/title/%s\">IMDb Link</a>\n", typeStr, title, yearStr, displayImdb))
	if origTitle != "" && origTitle != title {
		sb.WriteString(fmt.Sprintf("<i>(AKA %s)</i>\n", origTitle))
	}

	if isSeries && t.NumberOfSeasons > 0 {
		sb.WriteString(fmt.Sprintf("<b>%d Seasons (%d Episodes)</b>\n", t.NumberOfSeasons, t.NumberOfEpisodes))
	} else if isSeries && omdbFill.TotalSeasons != "" && omdbFill.TotalSeasons != notAvailable {
		sb.WriteString(fmt.Sprintf("<b>%s Seasons</b>\n", omdbFill.TotalSeasons))
	}

	runtime := t.Runtime
	if len(t.EpisodeRunTime) > 0 {
		runtime = t.EpisodeRunTime[0]
	}
	if runtime > 0 {
		dur := fmt.Sprintf("%dh %dm", runtime/60, runtime%60)
		if isSeries {
			dur += "/Episode"
		}
		sb.WriteString(fmt.Sprintf("<i>Duration: </i>%s\n", dur))
	}

	if dateStr != "" {
		if p, err := time.Parse("2006-01-02", dateStr); err == nil {
			dateStr = p.Format("02 January 2006")
		}
		flag := ""
		if len(t.ProductionCountries) > 0 {
			flag = getFlag(t.ProductionCountries[0].Iso3166_1)
		} else if omdbFill.Country != "" && omdbFill.Country != notAvailable {
			flag = getFlag(omdbFill.Country)
		}
		if flag != "" {
			dateStr += fmt.Sprintf(" (%s)", flag)
		}
		if isSeries {
			dateStr += " - First Episode"
		}
		sb.WriteString(fmt.Sprintf("<i>Release Date: </i>%s\n", dateStr))
	}

	ratingLine := ""
	if omdbFill.ImdbRating != "" && omdbFill.ImdbRating != notAvailable && omdbFill.ImdbRating != "0" {
		if omdbFill.ImdbVotes != "" && omdbFill.ImdbVotes != notAvailable {
			ratingLine += fmt.Sprintf("Rating ⭐️ %s / 10 (from %s votes)", omdbFill.ImdbRating, omdbFill.ImdbVotes)
		} else {
			ratingLine += fmt.Sprintf("Rating ⭐️ %s / 10", omdbFill.ImdbRating)
		}
	} else if t.VoteAverage > 0 {
		ratingLine += fmt.Sprintf("Rating ⭐️ %.1f / 10 (from %d votes)", t.VoteAverage, t.VoteCount)
	}

	var rtScore string
	for _, rVal := range omdbFill.Ratings {
		if rVal.Source == "Rotten Tomatoes" {
			rtScore = rVal.Value
			break
		}
	}
	if rtScore != "" {
		if ratingLine != "" {
			ratingLine += " | "
		}
		ratingLine += fmt.Sprintf("🍅 %s", rtScore)
	}

	if omdbFill.Metascore != "" && omdbFill.Metascore != notAvailable {
		if ratingLine != "" {
			ratingLine += " | "
		}
		ratingLine += fmt.Sprintf("Ⓜ️ %s/100", omdbFill.Metascore)
	}

	if omdbFill.Rated != "" && omdbFill.Rated != notAvailable && omdbFill.Rated != "Not Rated" {
		if ratingLine != "" {
			ratingLine += " | "
		}
		ratingLine += fmt.Sprintf("%s", omdbFill.Rated)
	}
	if ratingLine != "" {
		sb.WriteString(ratingLine + "\n")
	}

	var bq1 []string
	var gEmojiMap = map[string]string{
		"Action": "💥", "Adventure": "🗺️", "Sci-Fi": "🚀", "Science Fiction": "🚀",
		"Comedy": "🤣", "Drama": "🎭", "Romance": "🌹", "Thriller": "🔪",
		"Horror": "👻", "Fantasy": "✨", "Mystery": "❓", "Music": "🎶",
	}
	if len(t.Genres) > 0 {
		var gs []string
		for _, g := range t.Genres {
			emoji := "- "
			if e, ok := gEmojiMap[g.Name]; ok {
				emoji = e + " "
			}
			gs = append(gs, fmt.Sprintf("%s#%s", emoji, strings.ReplaceAll(g.Name, " ", "_")))
		}
		bq1 = append(bq1, fmt.Sprintf("<i>Genres: </i>%s", strings.Join(gs, " ")))
	}

	var themes []string
	kws := t.Keywords.Keywords
	if len(kws) == 0 {
		kws = t.Keywords.Results
	}
	for i, k := range kws {
		if i >= 6 {
			break
		}
		themes = append(themes, "#"+strings.ReplaceAll(strings.Title(k.Name), " ", "_"))
	}
	if len(themes) > 0 {
		bq1 = append(bq1, fmt.Sprintf("<i>Themes: </i>%s", strings.Join(themes, " ")))
	}

	var lgs, cgs []string
	for _, l := range t.SpokenLanguages {
		langName := l.EnglishName
		if langName == "" {
			langName = l.Name
		}
		if langName != "" {
			lgs = append(lgs, "#"+strings.ReplaceAll(langName, " ", "_"))
		}
	}
	for _, c := range t.ProductionCountries {
		f := getFlag(c.Iso3166_1)
		if f != "" {
			f += " "
		}
		cgs = append(cgs, fmt.Sprintf("%s#%s", f, strings.ReplaceAll(c.Name, " ", "_")))
	}
	if len(lgs) > 0 || len(cgs) > 0 {
		bq1 = append(bq1, fmt.Sprintf("<i>Language (Country): </i>%s (%s)", strings.Join(lgs, " "), strings.Join(cgs, " ")))
	}

	if len(bq1) > 0 {
		sb.WriteString(fmt.Sprintf("<blockquote>%s</blockquote>\n\n", strings.Join(bq1, "\n")))
	}

	if t.Tagline != "" {
		sb.WriteString(fmt.Sprintf("<b>\"%s\"</b>\n\n", t.Tagline))
	}

	shortOverview := t.Overview
	if rs := []rune(t.Overview); len(rs) > 800 {
		shortOverview = string(rs[:797]) + "..."
	}
	if t.Overview != "" {
		sb.WriteString(fmt.Sprintf("<blockquote><b>Story Line: </b><i>%s</i></blockquote>\n\n", shortOverview))
	}

	var dirs, writers, prods, stars, cast []string
	for _, c := range t.Credits.Crew {
		if c.Job == "Director" || (isSeries && (c.Job == "Executive Producer" || c.Job == "Creator")) {
			if len(dirs) < 3 {
				dirs = append(dirs, link(c.Name, c.ID))
			}
		}
		if c.Department == "Writing" || c.Job == "Writer" || c.Job == "Screenplay" {
			if len(writers) < 3 {
				writers = append(writers, link(c.Name, c.ID))
			}
		}
		if c.Job == "Producer" {
			if len(prods) < 3 {
				prods = append(prods, link(c.Name, c.ID))
			}
		}
	}
	for i, c := range t.Credits.Cast {
		if i < 4 {
			stars = append(stars, link(c.Name, c.ID))
		}
		if i >= 4 && i < topCastLimit+4 {
			cast = append(cast, link(c.Name, c.ID))
		}
	}

	var bq3 []string
	if len(dirs) > 0 {
		bq3 = append(bq3, fmt.Sprintf("<i><b>Directors:</b></i> %s", strings.Join(dirs, ", ")))
	}
	if len(writers) > 0 {
		bq3 = append(bq3, fmt.Sprintf("<i><b>Writers:</b></i> %s", strings.Join(writers, ", ")))
	}
	if len(prods) > 0 {
		bq3 = append(bq3, fmt.Sprintf("<i><b>Producers:</b></i> %s", strings.Join(prods, ", ")))
	}
	if len(stars) > 0 {
		bq3 = append(bq3, fmt.Sprintf("<i><b>Stars:</b></i> %s", strings.Join(stars, ", ")))
	}
	if len(cast) > 0 {
		bq3 = append(bq3, fmt.Sprintf("<i><b>Top Cast:</b></i> %s", strings.Join(cast, ", ")))
	}

	if len(bq3) > 0 {
		sb.WriteString(fmt.Sprintf("<blockquote>%s</blockquote>\n\n", strings.Join(bq3, "\n")))
	}

	if omdbFill.Awards != "" && omdbFill.Awards != notAvailable {
		sb.WriteString(fmt.Sprintf("<b>Awards: </b><a href=\"https://imdb.com/title/%s/awards\">%s</a>\n", displayImdb, omdbFill.Awards))
	}
	sb.WriteString(fmt.Sprintf("<b>OTT Info: </b><a href=\"https://www.justwatch.com/in/search?q=%s\">Find on JustWatch</a>\n", url.QueryEscape(title)))

	dl := omdbBanner
	if t.PosterPath != "" {
		dl = "https://image.tmdb.org/t/p/original" + t.PosterPath
	} else if omdbFill.Poster != "" && omdbFill.Poster != notAvailable {
		dl = omdbFill.Poster
	}

	if enableTelegraph {
		var nodes []tgNode
		nodes = append(nodes, tgNode{Tag: "h3", Children: []any{tgNode{Tag: "b", Children: []any{fmt.Sprintf("%s %s", title, yearStr)}}}})
		if dl != omdbBanner {
			nodes = append(nodes, tgNode{Tag: "figure", Children: []any{tgNode{Tag: "img", Attrs: &tgAttrs{Src: dl}}}})
		}
		if t.Tagline != "" {
			nodes = append(nodes, tgNode{Tag: "blockquote", Children: []any{tgNode{Tag: "i", Children: []any{t.Tagline}}}})
		}

		nodes = append(nodes, makeHeader("Overview"), tgNode{Tag: "p", Children: []any{t.Overview}})
		nodes = append(nodes, makeHeader("General Information"), makeRow("Type", typeStr))

		if origTitle != "" && origTitle != title {
			nodes = append(nodes, makeRow("Original Title", origTitle))
		}
		if omdbFill.Rated != "" && omdbFill.Rated != notAvailable {
			nodes = append(nodes, makeRow("Content Rating", omdbFill.Rated))
		}
		if isSeries && t.NumberOfSeasons > 0 {
			nodes = append(nodes, makeRow("Seasons", strconv.Itoa(t.NumberOfSeasons)))
			nodes = append(nodes, makeRow("Episodes", strconv.Itoa(t.NumberOfEpisodes)))
		}
		if runtime > 0 {
			nodes = append(nodes, makeRow("Runtime", fmt.Sprintf("%d minutes", runtime)))
		}
		if t.Status != "" {
			nodes = append(nodes, makeRow("Status", t.Status))
		}

		nodes = append(nodes, makeHeader("Ratings & Popularity"))
		if omdbFill.ImdbRating != "" && omdbFill.ImdbRating != notAvailable {
			nodes = append(nodes, makeRow("IMDb Rating", fmt.Sprintf("%s/10 (from %s votes)", omdbFill.ImdbRating, omdbFill.ImdbVotes)))
		} else {
			nodes = append(nodes, makeRow("IMDb Rating", fmt.Sprintf("%.1f/10 (from %d votes)", t.VoteAverage, t.VoteCount)))
		}
		if rtScore != "" {
			nodes = append(nodes, makeRow("Rotten Tomatoes", rtScore))
		}
		if omdbFill.Metascore != "" && omdbFill.Metascore != notAvailable {
			nodes = append(nodes, makeRow("Metascore", omdbFill.Metascore))
		}
		if t.Popularity > 0 {
			nodes = append(nodes, makeRow("Popularity Score", fmt.Sprintf("%.2f", t.Popularity)))
		}

		nodes = append(nodes, makeHeader("Genres & Themes"))
		if len(t.Genres) > 0 {
			var gList []string
			for _, g := range t.Genres {
				gList = append(gList, g.Name)
			}
			nodes = append(nodes, makeRow("Genres", strings.Join(gList, ", ")))
		}
		if len(themes) > 0 {
			nodes = append(nodes, makeRow("Themes", strings.Join(themes, " ")))
		}

		nodes = append(nodes, makeHeader("Financials & Production"))
		if t.Budget > 0 {
			nodes = append(nodes, makeRow("Budget", fmt.Sprintf("$%d", t.Budget)))
		}
		if omdbFill.BoxOffice != "" && omdbFill.BoxOffice != notAvailable {
			nodes = append(nodes, makeRow("Domestic Box Office", omdbFill.BoxOffice))
		}
		if t.Revenue > 0 {
			nodes = append(nodes, makeRow("Worldwide Gross", fmt.Sprintf("$%d", t.Revenue)))
		}

		var pComps []string
		for _, pc := range t.ProductionCompanies {
			pComps = append(pComps, pc.Name)
		}
		if len(pComps) > 0 {
			nodes = append(nodes, makeRow("Production Companies", strings.Join(pComps, ", ")))
		}
		var nets []string
		for _, n := range t.Networks {
			nets = append(nets, n.Name)
		}
		if len(nets) > 0 {
			nodes = append(nodes, makeRow("Networks", strings.Join(nets, ", ")))
		}

		var lgsTel []string
		for _, l := range t.SpokenLanguages {
			langName := l.EnglishName
			if langName == "" {
				langName = l.Name
			}
			if langName != "" {
				lgsTel = append(lgsTel, langName)
			}
		}
		if len(lgsTel) > 0 {
			nodes = append(nodes, makeRow("Spoken Languages", strings.Join(lgsTel, ", ")))
		}
		var cgsTel []string
		for _, c := range t.ProductionCountries {
			cgsTel = append(cgsTel, c.Name)
		}
		if len(cgsTel) > 0 {
			nodes = append(nodes, makeRow("Production Countries", strings.Join(cgsTel, ", ")))
		}

		if len(t.Credits.Cast) > 0 {
			nodes = append(nodes, makeHeader("Full Cast"))
			var castList []string
			for _, c := range t.Credits.Cast {
				role := ""
				if c.Character != "" {
					role = " as " + c.Character
				}
				castList = append(castList, c.Name+role)
			}
			nodes = append(nodes, tgNode{Tag: "p", Children: []any{strings.Join(castList, ", ")}})
		}

		trailerLink := ""
		for _, v := range t.Videos.Results {
			if v.Site == "YouTube" && v.Type == "Trailer" {
				trailerLink = "https://www.youtube.com/watch?v=" + v.Key
				break
			}
		}
		if trailerLink != "" {
			nodes = append(nodes, makeHeader("Media"), tgNode{Tag: "p", Children: []any{fmt.Sprintf("Trailer: %s", trailerLink)}})
		}

		page := createTelegraphPage(title+" Details", nodes)
		sb.WriteString(fmt.Sprintf("\n<a href=\"https://imdb.com/title/%s\">Read More...</a>", displayImdb))
		if page != "" {
			sb.WriteString(fmt.Sprintf(" | <a href=\"%s\">Full Details</a>", page))
		}
	} else {
		sb.WriteString(fmt.Sprintf("\n<a href=\"https://imdb.com/title/%s\">Read More...</a>", displayImdb))
	}

	trailerLink := ""
	for _, v := range t.Videos.Results {
		if v.Site == "YouTube" && v.Type == "Trailer" {
			trailerLink = "https://www.youtube.com/watch?v=" + v.Key
			break
		}
	}
	if trailerLink != "" {
		sb.WriteString(fmt.Sprintf(" | <a href=\"%s\">Trailer</a>", trailerLink))
	}
	sb.WriteString(fmt.Sprintf(" | <a href=\"%s\">Download Poster</a>", dl))

	return dl, sb.String(), buttons, nil
}

func GetOMDbTitle(id string, progress func(string)) (string, string, [][]gotgbot.InlineKeyboardButton, error) {
	if progress != nil {
		go progress("<i>Fetching High Quality Details...</i>")
	}

	id = strings.TrimPrefix(id, "open_")
	id = strings.TrimPrefix(id, "omdb_")

	var mType string
	var tmdbID string
	var directImdbID string

	if strings.HasPrefix(id, "tt") {
		directImdbID = id
		if r, err := httpClient.Get(fmt.Sprintf("https://api.themoviedb.org/3/find/%s?external_source=imdb_id&api_key=%s", id, tmdbKey)); err == nil {
			defer r.Body.Close()
			var d tmdbFindRes
			json.NewDecoder(r.Body).Decode(&d)
			if len(d.MovieResults) > 0 {
				tmdbID = strconv.Itoa(d.MovieResults[0].ID)
				mType = "movie"
			} else if len(d.TvResults) > 0 {
				tmdbID = strconv.Itoa(d.TvResults[0].ID)
				mType = "tv"
			}
		}
	} else if strings.Contains(id, "-") {
		parts := strings.Split(id, "-")
		mType = parts[0]
		tmdbID = parts[1]
	} else if strings.Contains(id, "_") {
		parts := strings.Split(id, "_")
		mType = parts[0]
		tmdbID = parts[1]
	} else {
		mType = "movie"
		tmdbID = id
		if r, err := httpClient.Get(fmt.Sprintf("https://api.themoviedb.org/3/movie/%s?api_key=%s", tmdbID, tmdbKey)); err == nil {
			defer r.Body.Close()
			var testDet tmdbDetail
			json.NewDecoder(r.Body).Decode(&testDet)
			if testDet.ID == 0 {
				mType = "tv"
			}
		}
	}

	if tmdbID == "" && directImdbID == "" {
		return "", "", nil, errors.New("not found")
	}

	imdbSearchID := directImdbID
	if imdbSearchID == "" && tmdbID != "" {
		r, err := httpClient.Get(fmt.Sprintf("https://api.themoviedb.org/3/%s/%s?api_key=%s", mType, tmdbID, tmdbKey))
		if err == nil {
			defer r.Body.Close()
			var t tmdbDetail
			json.NewDecoder(r.Body).Decode(&t)
			imdbSearchID = t.ExternalIds.ImdbId
		}
	}

	if imdbSearchID != "" {
		dl, text, btns, err := buildPythonDetails(imdbSearchID)
		if err == nil && text != "" {
			return dl, text, btns, nil
		}
	}

	return buildTMDBDetails(directImdbID, mType, tmdbID)
}

func getFlag(country string) string {
	flagMap := map[string]string{
		"United States": "🇺🇸 US", "USA": "🇺🇸 US", "US": "🇺🇸 US",
		"United Kingdom": "🇬🇧 UK", "UK": "🇬🇧 UK", "GB": "🇬🇧 UK",
		"India": "🇮🇳 IN", "IN": "🇮🇳 IN", "France": "🇫🇷 FR", "FR": "🇫🇷 FR",
		"Japan": "🇯🇵 JP", "JP": "🇯🇵 JP", "Canada": "🇨🇦 CA", "CA": "🇨🇦 CA",
		"Germany": "🇩🇪 DE", "DE": "🇩🇪 DE", "Australia": "🇦🇺 AU", "AU": "🇦🇺 AU",
		"Korea": "🇰🇷 KR", "South Korea": "🇰🇷 KR", "KR": "🇰🇷 KR",
		"China": "🇨🇳 CN", "CN": "🇨🇳 CN", "Russia": "🇷🇺 RU", "RU": "🇷🇺 RU",
		"Italy": "🇮🇹 IT", "IT": "🇮🇹 IT", "Spain": "🇪🇸 ES", "ES": "🇪🇸 ES",
		"Brazil": "🇧🇷 BR", "BR": "🇧🇷 BR",
	}
	if val, ok := flagMap[country]; ok {
		return val
	}
	for k, v := range flagMap {
		if strings.Contains(country, k) {
			return v
		}
	}
	return ""
}

func link(name string, id any) string {
	if idStr, ok := id.(string); ok {
		return fmt.Sprintf("<a href=\"https://imdb.com/name/%s\">%s</a>", idStr, name)
	}
	if idInt, ok := id.(int); ok {
		return fmt.Sprintf("<a href=\"https://www.themoviedb.org/person/%d\">%s</a>", idInt, name)
	}
	return name
}