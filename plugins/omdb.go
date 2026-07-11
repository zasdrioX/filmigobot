// (c) Jisin0
// Functions and types to search using Hybrid APIs.

package plugins

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Jisin0/filmigo/omdb"
	"github.com/PaulSonOfLars/gotgbot/v2"
)

const (
	omdbBanner   = "https://telegra.ph/file/e810982a269773daa42a9.png"
	omdbHomepage = "https://imdb.com"
	notAvailable = "N/A"

	apiFallback = "https://api.balloonerismm.workers.dev"

	topCastLimit    = 30
	enableTelegraph = true
)

var (
	omdbClient       *omdb.OmdbClient
	searchMethodOMDb = "omdb"
	telegraphToken   string
)

func init() {
	if OmdbApiKey != "" {
		omdbClient = omdb.NewClient(OmdbApiKey)
		inlineSearchButtons = append(inlineSearchButtons, []gotgbot.InlineKeyboardButton{{Text: "🔍 Search OMDb", SwitchInlineQueryCurrentChat: &inlineOMDbSwitch}})
	}
	if enableTelegraph { go ensureTelegraphToken() }
}

type UniversalSearchResult struct { ID string; Title string; Year int; Poster string; Type string; Rating float64 }

func ensureTelegraphToken() {
	if telegraphToken != "" { return }
	resp, err := http.Get("https://api.telegra.ph/createAccount?short_name=FilmigoBot&author_name=Filmigo+Bot")
	if err == nil {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		var res struct { Ok bool `json:"ok"`; Result struct { AccessToken string `json:"access_token"` } `json:"result"` }
		json.Unmarshal(body, &res)
		if res.Ok { telegraphToken = res.Result.AccessToken }
	}
}

type tgNode struct { Tag string `json:"tag"`; Attrs *tgAttrs `json:"attrs,omitempty"`; Children []any `json:"children,omitempty"` }
type tgAttrs struct { Src string `json:"src,omitempty"`; Href string `json:"href,omitempty"` }

func createTelegraphPage(title string, nodes []tgNode) string {
	ensureTelegraphToken()
	if telegraphToken == "" { return "" }
	contentBytes, err := json.Marshal(nodes)
	if err != nil { return "" }
	data := url.Values{}
	data.Set("access_token", telegraphToken)
	data.Set("title", title)
	data.Set("content", string(contentBytes))
	data.Set("return_content", "false")
	resp, err := http.PostForm("https://api.telegra.ph/createPage", data)
	if err != nil { return "" }
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var res struct { Ok bool `json:"ok"`; Result struct { Url string `json:"url"` } `json:"result"` }
	json.Unmarshal(body, &res)
	return res.Result.Url
}

func makeRow(label, value string) tgNode { return tgNode{Tag: "p", Children: []any{tgNode{Tag: "b", Children: []any{label + ": "}}, value}} }
func makeHeader(text string) tgNode { return tgNode{Tag: "h4", Children: []any{text}} }

// --- API STRUCTS ---
type bRes struct { Results []struct { MediaType string `json:"media_type"`; ID string `json:"id"`; Title string `json:"title"`; Name string `json:"name"`; ReleaseDate string `json:"release_date"`; FirstAirDate string `json:"first_air_date"`; PosterPath string `json:"poster_path"`; VoteAverage float64 `json:"vote_average"` } `json:"results"` }
type bTrailer struct { URL string `json:"url"` }
type bMovie struct { ID string `json:"id"`; Title string `json:"title"`; OriginalTitle string `json:"original_title"`; Overview string `json:"overview"`; Tagline string `json:"tagline"`; ReleaseDate string `json:"release_date"`; Runtime int `json:"runtime"`; VoteAverage float64 `json:"vote_average"`; VoteCount int `json:"vote_count"`; Genres []struct{ Name string `json:"name"` } `json:"genres"`; PosterPath string `json:"poster_path"`; SpokenLanguages []struct{ Name string `json:"name"` } `json:"spoken_languages"`; ProductionCountries []struct{ Iso3166_1 string `json:"iso_3166_1"`; Name string `json:"name"` } `json:"production_countries"`; ProductionCompanies []struct{ Name string `json:"name"` } `json:"production_companies"`; Budget int `json:"budget"`; Revenue int `json:"revenue"`; Status string `json:"status"`; Trailer *bTrailer `json:"trailer"` }
type bTV struct { ID string `json:"id"`; Name string `json:"name"`; OriginalName string `json:"original_name"`; Overview string `json:"overview"`; FirstAirDate string `json:"first_air_date"`; LastAirDate string `json:"last_air_date"`; NumberOfEpisodes int `json:"number_of_episodes"`; NumberOfSeasons int `json:"number_of_seasons"`; EpisodeRunTime []int `json:"episode_run_time"`; VoteAverage float64 `json:"vote_average"`; VoteCount int `json:"vote_count"`; Genres []struct{ Name string `json:"name"` } `json:"genres"`; PosterPath string `json:"poster_path"`; SpokenLanguages []struct{ Name string `json:"name"` } `json:"spoken_languages"`; ProductionCountries []struct{ Iso3166_1 string `json:"iso_3166_1"`; Name string `json:"name"` } `json:"production_countries"`; ProductionCompanies []struct{ Name string `json:"name"` } `json:"production_companies"`; OriginCountry []string `json:"origin_country"`; Networks []struct{ Name string `json:"name"` } `json:"networks"`; Status string `json:"status"`; Trailer *bTrailer `json:"trailer"` }
type bCredits struct { Cast []struct{ ID string `json:"id"`; Name string `json:"name"`; Character string `json:"character"` } `json:"cast"`; Crew []struct{ ID string `json:"id"`; Name string `json:"name"`; Job string `json:"job"`; Department string `json:"department"` } `json:"crew"` }
type bAkas struct { Titles []struct{ Title string `json:"title"`; Iso3166_1 string `json:"iso_3166_1"` } `json:"titles"` }
type bKeywords struct { Keywords []struct{ Name string `json:"name"` } `json:"keywords"` }

type omdbFillData struct { Released string `json:"Released"`; Awards string `json:"Awards"`; TotalSeasons string `json:"totalSeasons"`; Country string `json:"Country"`; Poster string `json:"Poster"`; BoxOffice string `json:"BoxOffice"`; Rated string `json:"Rated"` }

func parseYear(d string) int { if len(d) >= 4 { y, _ := strconv.Atoi(d[:4]); return y }; return 0 }

func fetchBalloon(id string) (*bMovie, *bTV, error) {
	var m bMovie; var t bTV; var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); if r, e := http.Get(fmt.Sprintf("%s/movie/%s", apiFallback, id)); e == nil && r.StatusCode == 200 { json.NewDecoder(r.Body).Decode(&m); r.Body.Close() } }()
	go func() { defer wg.Done(); if r, e := http.Get(fmt.Sprintf("%s/tv/%s", apiFallback, id)); e == nil && r.StatusCode == 200 { json.NewDecoder(r.Body).Decode(&t); r.Body.Close() } }()
	wg.Wait()
	if t.NumberOfSeasons > 0 || t.NumberOfEpisodes > 0 || t.LastAirDate != "" { return nil, &t, nil }
	if m.ID != "" { return &m, nil, nil }
	if t.ID != "" { return nil, &t, nil }
	return nil, nil, errors.New("not found")
}

func SearchOMDb(query string) ([]UniversalSearchResult, error) {
	var imdbID string
	if strings.Contains(query, "imdb.com/title/tt") || (strings.HasPrefix(query, "tt") && len(query) >= 7) {
		s := strings.Index(query, "tt"); idPart := query[s:]; e := strings.IndexAny(idPart, "/? \n\t"); if e == -1 { e = len(idPart) }; imdbID = idPart[:e]
	}
	if imdbID != "" {
		m, t, err := fetchBalloon(imdbID)
		if err == nil {
			res := UniversalSearchResult{ID: imdbID}
			if t != nil { res.Title = t.Name; res.Year = parseYear(t.FirstAirDate); res.Poster = t.PosterPath; res.Type = "TV Series"; res.Rating = t.VoteAverage } else if m != nil { res.Title = m.Title; res.Year = parseYear(m.ReleaseDate); res.Poster = m.PosterPath; res.Type = "Movie"; res.Rating = m.VoteAverage }
			return []UniversalSearchResult{res}, nil
		}
	}
	r, err := http.Get(fmt.Sprintf("%s/search/multi?query=%s", apiFallback, url.QueryEscape(query)))
	if err != nil { return nil, err }
	defer r.Body.Close()
	var data bRes; if err := json.NewDecoder(r.Body).Decode(&data); err != nil { return nil, err }
	var results []UniversalSearchResult
	for _, i := range data.Results {
		if i.MediaType == "person" { continue }
		title := i.Title; if title == "" { title = i.Name }
		date := i.ReleaseDate; if date == "" { date = i.FirstAirDate }
		typeTag := "Movie"; if i.MediaType == "tv" { typeTag = "TV Series" }
		results = append(results, UniversalSearchResult{ID: i.ID, Title: title, Year: parseYear(date), Poster: i.PosterPath, Type: typeTag, Rating: i.VoteAverage})
	}
	if len(results) == 0 { return nil, errors.New("no results") }
	return results, nil
}

func OMDbInlineSearch(query string) []gotgbot.InlineQueryResult {
	results, err := SearchOMDb(query)
	if err != nil { return nil }
	tgResults := make([]gotgbot.InlineQueryResult, 0, len(results))
	for _, item := range results {
		posterURL := item.Poster; if posterURL == "" || posterURL == "N/A" { posterURL = omdbBanner }
		title := item.Title; if item.Year > 0 { title = fmt.Sprintf("%s [%d]", item.Title, item.Year) }
		description := item.Type; if item.Rating > 0 { description = fmt.Sprintf("%s | Ratings: %.1f ⭐", item.Type, item.Rating) } else { description = fmt.Sprintf("%s | Ratings: N/A", item.Type) }
		tgResults = append(tgResults, gotgbot.InlineQueryResultArticle{
			Id: searchMethodOMDb + "_" + item.ID, Title: title, Description: description, ThumbnailUrl: posterURL,
			InputMessageContent: gotgbot.InputTextMessageContent{ MessageText: fmt.Sprintf("<i>Loading details for %s...</i>", item.Title), ParseMode: gotgbot.ParseModeHTML },
			ReplyMarkup: &gotgbot.InlineKeyboardMarkup{InlineKeyboard: [][]gotgbot.InlineKeyboardButton{ {{Text: "Open IMDb", CallbackData: fmt.Sprintf("open_%s_%s", searchMethodOMDb, item.ID)}} }},
		})
	}
	return tgResults
}

func GetOMDbTitle(id string, progress func(string)) (string, string, [][]gotgbot.InlineKeyboardButton, error) {
	if progress != nil { go progress("<i>Fetching High Quality Details...</i>") }
	var buttons [][]gotgbot.InlineKeyboardButton
	mDetail, tDetail, err := fetchBalloon(id)
	if err != nil { return "", "", buttons, err }

	isSeries := (tDetail != nil)
	endpoint := "movie"; if isSeries { endpoint = "tv" }

	var creds bCredits; var akas bAkas; var kwds bKeywords; var omdbFill omdbFillData; var wg sync.WaitGroup
	wg.Add(4)
	go func() { defer wg.Done(); if r, e := http.Get(fmt.Sprintf("%s/%s/%s/credits", apiFallback, endpoint, id)); e == nil { json.NewDecoder(r.Body).Decode(&creds); r.Body.Close() } }()
	go func() { defer wg.Done(); if r, e := http.Get(fmt.Sprintf("%s/%s/%s/alternative_titles", apiFallback, endpoint, id)); e == nil { json.NewDecoder(r.Body).Decode(&akas); r.Body.Close() } }()
	go func() { defer wg.Done(); if r, e := http.Get(fmt.Sprintf("%s/%s/%s/keywords", apiFallback, endpoint, id)); e == nil { json.NewDecoder(r.Body).Decode(&kwds); r.Body.Close() } }()
	go func() { defer wg.Done(); if r, e := http.Get(fmt.Sprintf("https://www.omdbapi.com/?i=%s&apikey=%s", id, OmdbApiKey)); e == nil { json.NewDecoder(r.Body).Decode(&omdbFill); r.Body.Close() } }()
	wg.Wait()

	var sb strings.Builder
	var title, origTitle, dateStr, overview, poster, tagline string
	var year, lastYear, runtime, seasons, episodes int
	var vote float64; var vCount int; var typeStr = "Movie"
	var dirs, writers, prods, stars, cast, genres []string
	var trailer *bTrailer

	if isSeries {
		title = tDetail.Name; origTitle = tDetail.OriginalName; dateStr = tDetail.FirstAirDate; year = parseYear(dateStr); lastYear = parseYear(tDetail.LastAirDate); overview = tDetail.Overview; poster = tDetail.PosterPath; if len(tDetail.EpisodeRunTime) > 0 { runtime = tDetail.EpisodeRunTime[0] }
		vote = tDetail.VoteAverage; vCount = tDetail.VoteCount; seasons = tDetail.NumberOfSeasons; episodes = tDetail.NumberOfEpisodes; typeStr = "TV Series"; trailer = tDetail.Trailer
		for _, g := range tDetail.Genres { genres = append(genres, g.Name) }
	} else {
		title = mDetail.Title; origTitle = mDetail.OriginalTitle; dateStr = mDetail.ReleaseDate; year = parseYear(dateStr); overview = mDetail.Overview; poster = mDetail.PosterPath; tagline = mDetail.Tagline; runtime = mDetail.Runtime
		vote = mDetail.VoteAverage; vCount = mDetail.VoteCount; trailer = mDetail.Trailer
		for _, g := range mDetail.Genres { genres = append(genres, g.Name) }
	}

	yearStr := ""
	if isSeries {
		if lastYear > year { yearStr = fmt.Sprintf("[%d-%d]", year, lastYear) } else if lastYear == 0 && year > 0 { yearStr = fmt.Sprintf("[%d-Present]", year) } else if year > 0 { yearStr = fmt.Sprintf("[%d]", year) }
	} else if year > 0 { yearStr = fmt.Sprintf("[%d]", year) }

	sb.WriteString(fmt.Sprintf("<i>%s: </i><b>%s %s</b> | <a href=\"https://imdb.com/title/%s\">IMDb Link</a>\n", typeStr, title, yearStr, id))
	if origTitle != "" && origTitle != title { sb.WriteString(fmt.Sprintf("<i>(Original Title: %s)</i>\n", origTitle)) }

	akaStr := ""
	if len(akas.Titles) > 0 {
		for _, a := range akas.Titles { if (a.Iso3166_1 == "US" || a.Iso3166_1 == "IN") && a.Title != title { akaStr = a.Title; break } }
		if akaStr == "" && akas.Titles[0].Title != title { akaStr = akas.Titles[0].Title }
	}
	if akaStr != "" { sb.WriteString(fmt.Sprintf("<i>(AKA %s)</i>\n", akaStr)) }

	if isSeries && seasons > 0 { sb.WriteString(fmt.Sprintf("<b>%d Seasons (%d Episodes)</b>\n", seasons, episodes)) } else if isSeries && omdbFill.TotalSeasons != "" && omdbFill.TotalSeasons != notAvailable { sb.WriteString(fmt.Sprintf("<b>%s Seasons</b>\n", omdbFill.TotalSeasons)) }

	if runtime > 0 { dur := fmt.Sprintf("%dh %dm", runtime/60, runtime%60); if isSeries { dur += "/Episode" }; sb.WriteString(fmt.Sprintf("<i>Duration: </i>%s\n", dur)) }

	if dateStr != "" { 
		if p, err := time.Parse("2006-01-02", dateStr); err == nil { dateStr = p.Format("02 January 2006") }
		flag := ""
		if isSeries && len(tDetail.OriginCountry) > 0 { flag = getFlag(tDetail.OriginCountry[0]) } else if !isSeries && len(mDetail.ProductionCountries) > 0 { flag = getFlag(mDetail.ProductionCountries[0].Iso3166_1) } else if omdbFill.Country != "" && omdbFill.Country != notAvailable { flag = getFlag(omdbFill.Country) }
		if flag != "" { dateStr += fmt.Sprintf(" (%s)", flag) }
		if isSeries { dateStr += " - First Episode" }
		sb.WriteString(fmt.Sprintf("<i>Release Date: </i>%s\n", dateStr)) 
	}

	sb.WriteString("<blockquote>")
	if vote > 0 { sb.WriteString(fmt.Sprintf("<i>Rating ⭐️ </i><b>%.1f / 10</b> (from %d votes)\n", vote, vCount)) }

	var gEmojiMap = map[string]string{ "Action": "💥", "Adventure": "🗺️", "Sci-Fi": "🚀", "Science Fiction": "🚀", "Comedy": "🤣", "Drama": "🎭", "Romance": "🌹", "Thriller": "🔪", "Horror": "👻", "Fantasy": "✨", "Mystery": "❓", "Music": "🎶" }
	if len(genres) > 0 {
		var gs []string
		for _, g := range genres {
			emoji := "- "
			if e, ok := gEmojiMap[g]; ok { emoji = e + " " }
			gs = append(gs, fmt.Sprintf("%s#%s", emoji, strings.ReplaceAll(g, " ", "_")))
		}
		sb.WriteString(fmt.Sprintf("<i>Genres: </i>%s\n", strings.Join(gs, " ")))
	}

	var themes []string
	for i, k := range kwds.Keywords {
		if i >= 6 { break }
		themes = append(themes, "#" + strings.ReplaceAll(strings.Title(k.Name), " ", "_"))
	}
	if len(themes) > 0 { sb.WriteString(fmt.Sprintf("<i>Themes: </i>%s\n", strings.Join(themes, " "))) }

	var lgs, cgs []string
	if isSeries {
		for _, l := range tDetail.SpokenLanguages { lgs = append(lgs, "#"+strings.ReplaceAll(l.Name, " ", "_")) }
		for _, c := range tDetail.ProductionCountries { f := getFlag(c.Iso3166_1); if f != "" { f += " " }; cgs = append(cgs, fmt.Sprintf("%s#%s", f, strings.ReplaceAll(c.Name, " ", "_"))) }
		if len(cgs) == 0 { for _, c := range tDetail.OriginCountry { f := getFlag(c); if f != "" { f += " " }; cgs = append(cgs, fmt.Sprintf("%s#%s", f, c)) } }
	} else {
		for _, l := range mDetail.SpokenLanguages { lgs = append(lgs, "#"+strings.ReplaceAll(l.Name, " ", "_")) }
		for _, c := range mDetail.ProductionCountries { f := getFlag(c.Iso3166_1); if f != "" { f += " " }; cgs = append(cgs, fmt.Sprintf("%s#%s", f, strings.ReplaceAll(c.Name, " ", "_"))) }
	}
	if len(lgs) > 0 || len(cgs) > 0 { sb.WriteString(fmt.Sprintf("<i>Language (Country): </i>%s (%s)\n", strings.Join(lgs, " "), strings.Join(cgs, " "))) }
	sb.WriteString("</blockquote>\n")

	if tagline != "" { sb.WriteString(fmt.Sprintf("<b>\"%s\"</b>\n", tagline)) }
	if overview != "" { sb.WriteString(fmt.Sprintf("<blockquote><b>Story Line: </b><i>%s</i></blockquote>\n", overview)) }

	for _, c := range creds.Crew {
		if c.Job == "Director" || (isSeries && (c.Job == "Executive Producer" || c.Job == "Creator")) { if len(dirs) < 3 { dirs = append(dirs, link(c.Name, c.ID)) } }
		if c.Department == "Writing" || c.Job == "Writer" || c.Job == "Screenplay" { if len(writers) < 3 { writers = append(writers, link(c.Name, c.ID)) } }
		if c.Job == "Producer" { if len(prods) < 3 { prods = append(prods, link(c.Name, c.ID)) } }
	}
	for i, c := range creds.Cast {
		if i < 4 { stars = append(stars, link(c.Name, c.ID)) }
		if i >= 4 && i < topCastLimit+4 { cast = append(cast, link(c.Name, c.ID)) }
	}

	sb.WriteString("<blockquote>")
	if len(dirs) > 0 { sb.WriteString(fmt.Sprintf("<i><b>Directors:</b></i> %s\n", strings.Join(dirs, ", "))) }
	if len(writers) > 0 { sb.WriteString(fmt.Sprintf("<i><b>Writers:</b></i> %s\n", strings.Join(writers, ", "))) }
	if len(prods) > 0 { sb.WriteString(fmt.Sprintf("<i><b>Producers:</b></i> %s\n", strings.Join(prods, ", "))) }
	if len(stars) > 0 { sb.WriteString(fmt.Sprintf("<i><b>Stars:</b></i> %s\n", strings.Join(stars, ", "))) }
	if len(cast) > 0 { sb.WriteString(fmt.Sprintf("<i><b>Top Cast:</b></i> %s\n", strings.Join(cast, ", "))) }
	sb.WriteString("</blockquote>\n")

	sb.WriteString("<blockquote>")
	if omdbFill.Awards != "" && omdbFill.Awards != notAvailable {
		sb.WriteString(fmt.Sprintf("<b>Awards: </b><a href=\"https://imdb.com/title/%s/awards\">%s</a>\n", id, omdbFill.Awards))
	}
	sb.WriteString(fmt.Sprintf("<b>OTT Info: </b><a href=\"https://www.justwatch.com/in/search?q=%s\">Find on JustWatch</a></blockquote>", url.QueryEscape(title)))

	// Fetch native 3000px IMAX-level poster straight from IMDB database
	imdbPoster := omdbFill.Poster
	dl := ""
	if imdbPoster != "" && imdbPoster != notAvailable {
		if strings.Contains(imdbPoster, "._V1_") {
			base := strings.Split(imdbPoster, "._V1_")[0]
			dl = base + "._V1_FMjpg_UX3000_.jpg"
		} else { dl = imdbPoster }
	} else if poster != "" { dl = "https://image.tmdb.org/t/p/original" + poster } else { dl = omdbBanner }

	if enableTelegraph {
		var nodes []tgNode
		nodes = append(nodes, tgNode{Tag: "h3", Children: []any{fmt.Sprintf("%s %s", title, yearStr)}})
		if dl != omdbBanner { nodes = append(nodes, tgNode{Tag: "figure", Children: []any{tgNode{Tag: "img", Attrs: &tgAttrs{Src: dl}}}}) }
		nodes = append(nodes, makeHeader("Info"), makeRow("Type", typeStr))
		
		if omdbFill.Rated != "" && omdbFill.Rated != notAvailable { nodes = append(nodes, makeRow("Content Rating", omdbFill.Rated)) }
		if mDetail != nil && mDetail.Status != "" { nodes = append(nodes, makeRow("Status", mDetail.Status)) }
		if tDetail != nil && tDetail.Status != "" { nodes = append(nodes, makeRow("Status", tDetail.Status)) }
		if mDetail != nil && mDetail.Budget > 0 { nodes = append(nodes, makeRow("Budget", fmt.Sprintf("$%d", mDetail.Budget))) }
		if mDetail != nil && mDetail.Revenue > 0 { nodes = append(nodes, makeRow("Revenue", fmt.Sprintf("$%d", mDetail.Revenue))) }
		if omdbFill.BoxOffice != "" && omdbFill.BoxOffice != notAvailable { nodes = append(nodes, makeRow("Box Office", omdbFill.BoxOffice)) }
		
		var pComps []string
		if isSeries { for _, pc := range tDetail.ProductionCompanies { pComps = append(pComps, pc.Name) } } else { for _, pc := range mDetail.ProductionCompanies { pComps = append(pComps, pc.Name) } }
		if len(pComps) > 0 { nodes = append(nodes, makeRow("Production", strings.Join(pComps, ", "))) }
		if isSeries && len(tDetail.Networks) > 0 { var nets []string; for _, n := range tDetail.Networks { nets = append(nets, n.Name) }; nodes = append(nodes, makeRow("Networks", strings.Join(nets, ", "))) }
		
		nodes = append(nodes, makeRow("Plot", overview))
		if len(creds.Cast) > 0 {
			nodes = append(nodes, makeHeader("Full Cast"))
			var castList []string
			for _, c := range creds.Cast {
				role := ""; if c.Character != "" { role = " as " + c.Character }
				castList = append(castList, c.Name+role)
			}
			nodes = append(nodes, tgNode{Tag: "p", Children: []any{strings.Join(castList, ", ")}})
		}
		page := createTelegraphPage(title+" Details", nodes)
		sb.WriteString(fmt.Sprintf("\n\n<a href=\"https://imdb.com/title/%s\">Read More...</a>", id))
		if page != "" { sb.WriteString(fmt.Sprintf(" | <a href=\"%s\">Full Details</a>", page)) }
	} else {
		sb.WriteString(fmt.Sprintf("\n\n<a href=\"https://imdb.com/title/%s\">Read More...</a>", id))
	}

	if trailer != nil && trailer.URL != "" { sb.WriteString(fmt.Sprintf(" | <a href=\"%s\">Trailer</a>", trailer.URL)) }
	sb.WriteString(fmt.Sprintf(" | <a href=\"%s\">Download Poster</a>", dl))

	// STRICT REQUIREMENT: Preview thumbnail MUST be the poster, NOT a video
	return dl, sb.String(), buttons, nil
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
    if val, ok := flagMap[country]; ok { return val }
    for k, v := range flagMap { if strings.Contains(country, k) { return v } }
    return ""
}

func link(name string, id any) string {
	if idStr, ok := id.(string); ok { return fmt.Sprintf("<a href=\"https://imdb.com/name/%s\">%s</a>", idStr, name) }
	if idInt, ok := id.(int); ok { return fmt.Sprintf("<a href=\"https://www.themoviedb.org/person/%d\">%s</a>", idInt, name) }
	return name
}
