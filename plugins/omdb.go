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

topCastLimit    = 30
enableTelegraph = true
tmdbKey         = "1b4ba621cf09ae9752dd659e6e55307b"
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
if resp, err := http.Get("https://api.telegra.ph/createAccount?short_name=FilmigoBot&author_name=Filmigo+Bot"); err == nil {
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
data.Set("access_token", telegraphToken); data.Set("title", title); data.Set("content", string(contentBytes)); data.Set("return_content", "false")
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

// --- TMDB & OMDB STRUCTS ---
type tmdbMultiRes struct {
Results []struct {
ID int `json:"id"`; MediaType string `json:"media_type"`; Title string `json:"title"`; Name string `json:"name"`; ReleaseDate string `json:"release_date"`; FirstAirDate string `json:"first_air_date"`; PosterPath string `json:"poster_path"`; VoteAverage float64 `json:"vote_average"`
} `json:"results"`
}
type tmdbFindRes struct {
MovieResults []struct{ ID int `json:"id"` } `json:"movie_results"`; TvResults []struct{ ID int `json:"id"` } `json:"tv_results"`
}
type tmdbDetail struct {
ID int `json:"id"`; Title string `json:"title"`; Name string `json:"name"`; OriginalTitle string `json:"original_title"`; OriginalName string `json:"original_name"`; Overview string `json:"overview"`; Tagline string `json:"tagline"`; ReleaseDate string `json:"release_date"`; FirstAirDate string `json:"first_air_date"`; LastAirDate string `json:"last_air_date"`; Runtime int `json:"runtime"`; EpisodeRunTime []int `json:"episode_run_time"`; NumberOfSeasons int `json:"number_of_seasons"`; NumberOfEpisodes int `json:"number_of_episodes"`; VoteAverage float64 `json:"vote_average"`; VoteCount int `json:"vote_count"`; Popularity float64 `json:"popularity"`; Genres []struct{ Name string `json:"name"` } `json:"genres"`; PosterPath string `json:"poster_path"`; SpokenLanguages []struct{ EnglishName string `json:"english_name"`; Name string `json:"name"` } `json:"spoken_languages"`; ProductionCountries []struct{ Iso3166_1 string `json:"iso_3166_1"`; Name string `json:"name"` } `json:"production_countries"`; ProductionCompanies []struct{ Name string `json:"name"` } `json:"production_companies"`; Networks []struct{ Name string `json:"name"` } `json:"networks"`; Budget int `json:"budget"`; Revenue int `json:"revenue"`; Status string `json:"status"`
Credits struct { Cast []struct{ ID int `json:"id"`; Name string `json:"name"`; Character string `json:"character"` } `json:"cast"`; Crew []struct{ ID int `json:"id"`; Name string `json:"name"`; Job string `json:"job"`; Department string `json:"department"` } `json:"crew"` } `json:"credits"`
Keywords struct { Keywords []struct{ Name string `json:"name"` } `json:"keywords"`; Results []struct{ Name string `json:"name"` } `json:"results"` } `json:"keywords"`
Videos struct { Results []struct{ Key string `json:"key"`; Site string `json:"site"`; Type string `json:"type"` } `json:"results"` } `json:"videos"`
ExternalIds struct { ImdbId string `json:"imdb_id"` } `json:"external_ids"`
}
type omdbFillData struct { Released string `json:"Released"`; Awards string `json:"Awards"`; TotalSeasons string `json:"totalSeasons"`; Country string `json:"Country"`; Poster string `json:"Poster"`; BoxOffice string `json:"BoxOffice"`; Rated string `json:"Rated"`; Metascore string `json:"Metascore"` }

func parseYear(d string) int { if len(d) >= 4 { y, _ := strconv.Atoi(d[:4]); return y }; return 0 }

func SearchOMDb(query string) ([]UniversalSearchResult, error) {
var imdbID string
if strings.Contains(query, "imdb.com/title/tt") || (strings.HasPrefix(query, "tt") && len(query) >= 7) {
s := strings.Index(query, "tt"); idPart := query[s:]; e := strings.IndexAny(idPart, "/? \n\t"); if e == -1 { e = len(idPart) }; imdbID = idPart[:e]
}

if imdbID != "" {
if r, err := http.Get(fmt.Sprintf("https://api.themoviedb.org/3/find/%s?external_source=imdb_id&api_key=%s", imdbID, tmdbKey)); err == nil {
defer r.Body.Close()
var d tmdbFindRes
json.NewDecoder(r.Body).Decode(&d)
var id int; var mType string
if len(d.MovieResults) > 0 { id = d.MovieResults[0].ID; mType = "movie" } else if len(d.TvResults) > 0 { id = d.TvResults[0].ID; mType = "tv" }
if id != 0 {
if r2, err2 := http.Get(fmt.Sprintf("https://api.themoviedb.org/3/%s/%d?api_key=%s", mType, id, tmdbKey)); err2 == nil {
defer r2.Body.Close()
var det tmdbDetail
json.NewDecoder(r2.Body).Decode(&det)
t := det.Title; if t == "" { t = det.Name }
date := det.ReleaseDate; if date == "" { date = det.FirstAirDate }
tag := "Movie"; if mType == "tv" { tag = "TV Series" }
return []UniversalSearchResult{{ID: fmt.Sprintf("%s-%d", mType, id), Title: t, Year: parseYear(date), Poster: det.PosterPath, Type: tag, Rating: det.VoteAverage}}, nil
}
}
}
}

r, err := http.Get(fmt.Sprintf("https://api.themoviedb.org/3/search/multi?query=%s&api_key=%s", url.QueryEscape(query), tmdbKey))
if err != nil { return nil, err }
defer r.Body.Close()
var data tmdbMultiRes; if err := json.NewDecoder(r.Body).Decode(&data); err != nil { return nil, err }
var results []UniversalSearchResult
for _, i := range data.Results {
if i.MediaType == "person" { continue }
title := i.Title; if title == "" { title = i.Name }
date := i.ReleaseDate; if date == "" { date = i.FirstAirDate }
typeTag := "Movie"; if i.MediaType == "tv" { typeTag = "TV Series" }
results = append(results, UniversalSearchResult{ID: fmt.Sprintf("%s-%d", i.MediaType, i.ID), Title: title, Year: parseYear(date), Poster: i.PosterPath, Type: typeTag, Rating: i.VoteAverage})
}
if len(results) == 0 { return nil, errors.New("no results") }
return results, nil
}

func OMDbInlineSearch(query string) []gotgbot.InlineQueryResult {
results, err := SearchOMDb(query)
if err != nil { return nil }
tgResults := make([]gotgbot.InlineQueryResult, 0, len(results))
for _, item := range results {
posterURL := item.Poster; if posterURL == "" || posterURL == "N/A" { posterURL = omdbBanner } else if !strings.HasPrefix(posterURL, "http") { posterURL = "https://image.tmdb.org/t/p/w200" + posterURL }
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

id = strings.TrimPrefix(id, "open_")
id = strings.TrimPrefix(id, "omdb_")

var mType string; var tmdbID string

if strings.HasPrefix(id, "tt") {
if r, err := http.Get(fmt.Sprintf("https://api.themoviedb.org/3/find/%s?external_source=imdb_id&api_key=%s", id, tmdbKey)); err == nil {
defer r.Body.Close()
var d tmdbFindRes; json.NewDecoder(r.Body).Decode(&d)
if len(d.MovieResults) > 0 { tmdbID = strconv.Itoa(d.MovieResults[0].ID); mType = "movie" } else if len(d.TvResults) > 0 { tmdbID = strconv.Itoa(d.TvResults[0].ID); mType = "tv" }
}
} else if strings.Contains(id, "-") {
parts := strings.Split(id, "-"); mType = parts[0]; tmdbID = parts[1]
} else if strings.Contains(id, "_") {
parts := strings.Split(id, "_"); mType = parts[0]; tmdbID = parts[1]
} else {
mType = "movie"; tmdbID = id
if r, err := http.Get(fmt.Sprintf("https://api.themoviedb.org/3/movie/%s?api_key=%s", tmdbID, tmdbKey)); err == nil {
defer r.Body.Close()
var testDet tmdbDetail; json.NewDecoder(r.Body).Decode(&testDet)
if testDet.ID == 0 { mType = "tv" }
}
}

if tmdbID == "" { return "", "", buttons, errors.New("not found") }

r, err := http.Get(fmt.Sprintf("https://api.themoviedb.org/3/%s/%s?append_to_response=credits,keywords,videos,external_ids&api_key=%s", mType, tmdbID, tmdbKey))
if err != nil { return "", "", buttons, err }
defer r.Body.Close()
var t tmdbDetail
if err := json.NewDecoder(r.Body).Decode(&t); err != nil { return "", "", buttons, err }
if t.Title == "" && t.Name == "" { return "", "", buttons, errors.New("not found") }

var omdbFill omdbFillData
imdbID := t.ExternalIds.ImdbId
if imdbID != "" {
if ro, eo := http.Get(fmt.Sprintf("https://www.omdbapi.com/?i=%s&apikey=%s", imdbID, OmdbApiKey)); eo == nil { json.NewDecoder(ro.Body).Decode(&omdbFill); ro.Body.Close() }
} else { imdbID = id }

isSeries := (mType == "tv")
title := t.Title; if title == "" { title = t.Name }
origTitle := t.OriginalTitle; if origTitle == "" { origTitle = t.OriginalName }
dateStr := t.ReleaseDate; if dateStr == "" { dateStr = t.FirstAirDate }
year := parseYear(dateStr); lastYear := parseYear(t.LastAirDate)

var sb strings.Builder
typeStr := "Movie"
if isSeries { typeStr = "TV Series" }

yearStr := ""
if isSeries {
if lastYear > year { yearStr = fmt.Sprintf("[%d-%d]", year, lastYear) } else if lastYear == 0 && year > 0 { yearStr = fmt.Sprintf("[%d-Present]", year) } else if year > 0 { yearStr = fmt.Sprintf("[%d]", year) }
} else if year > 0 { yearStr = fmt.Sprintf("[%d]", year) }

sb.WriteString(fmt.Sprintf("<i>%s: </i><b>%s %s</b> | <a href=\"https://imdb.com/title/%s\">IMDb Link</a>\n", typeStr, title, yearStr, imdbID))
if origTitle != "" && origTitle != title { sb.WriteString(fmt.Sprintf("<i>(AKA %s)</i>\n", origTitle)) }

if isSeries && t.NumberOfSeasons > 0 {
sb.WriteString(fmt.Sprintf("<b>%d Seasons (%d Episodes)</b>\n", t.NumberOfSeasons, t.NumberOfEpisodes))
} else if isSeries && omdbFill.TotalSeasons != "" && omdbFill.TotalSeasons != notAvailable {
sb.WriteString(fmt.Sprintf("<b>%s Seasons</b>\n", omdbFill.TotalSeasons))
}

runtime := t.Runtime; if len(t.EpisodeRunTime) > 0 { runtime = t.EpisodeRunTime[0] }
if runtime > 0 { dur := fmt.Sprintf("%dh %dm", runtime/60, runtime%60); if isSeries { dur += "/Episode" }; sb.WriteString(fmt.Sprintf("<i>Duration: </i>%s\n", dur)) }

if dateStr != "" {
if p, err := time.Parse("2006-01-02", dateStr); err == nil { dateStr = p.Format("02 January 2006") }
flag := ""
if len(t.ProductionCountries) > 0 { flag = getFlag(t.ProductionCountries[0].Iso3166_1) } else if omdbFill.Country != "" && omdbFill.Country != notAvailable { flag = getFlag(omdbFill.Country) }
if flag != "" { dateStr += fmt.Sprintf(" (%s)", flag) }
if isSeries { dateStr += " - First Episode" }
sb.WriteString(fmt.Sprintf("<i>Release Date: </i>%s\n", dateStr))
}

ratingLine := ""
if t.VoteAverage > 0 { ratingLine += fmt.Sprintf("Rating ⭐️ %.1f / 10 (from %d votes)", t.VoteAverage, t.VoteCount) }
if omdbFill.Metascore != "" && omdbFill.Metascore != notAvailable {
if ratingLine != "" { ratingLine += " | " }
ratingLine += fmt.Sprintf("Ⓜ️ %s/100", omdbFill.Metascore)
}
if omdbFill.Rated != "" && omdbFill.Rated != notAvailable && omdbFill.Rated != "Not Rated" {
if ratingLine != "" { ratingLine += " | " }
ratingLine += fmt.Sprintf("%s", omdbFill.Rated)
}
if ratingLine != "" { sb.WriteString(ratingLine + "\n") }

sb.WriteString("<blockquote>")
var gEmojiMap = map[string]string{ "Action": "💥", "Adventure": "🗺️", "Sci-Fi": "🚀", "Science Fiction": "🚀", "Comedy": "🤣", "Drama": "🎭", "Romance": "🌹", "Thriller": "🔪", "Horror": "👻", "Fantasy": "✨", "Mystery": "❓", "Music": "🎶" }
if len(t.Genres) > 0 {
var gs []string
for _, g := range t.Genres { emoji := "- "; if e, ok := gEmojiMap[g.Name]; ok { emoji = e + " " }; gs = append(gs, fmt.Sprintf("%s#%s", emoji, strings.ReplaceAll(g.Name, " ", "_"))) }
sb.WriteString(fmt.Sprintf("<i>Genres: </i>%s\n", strings.Join(gs, " ")))
}

var themes []string
kws := t.Keywords.Keywords; if len(kws) == 0 { kws = t.Keywords.Results }
for i, k := range kws { if i >= 6 { break }; themes = append(themes, "#" + strings.ReplaceAll(strings.Title(k.Name), " ", "_")) }
if len(themes) > 0 { sb.WriteString(fmt.Sprintf("<i>Themes: </i>%s\n", strings.Join(themes, " "))) }

var lgs, cgs []string
for _, l := range t.SpokenLanguages { langName := l.EnglishName; if langName == "" { langName = l.Name }; if langName != "" { lgs = append(lgs, "#"+strings.ReplaceAll(langName, " ", "_")) } }
for _, c := range t.ProductionCountries { f := getFlag(c.Iso3166_1); if f != "" { f += " " }; cgs = append(cgs, fmt.Sprintf("%s#%s", f, strings.ReplaceAll(c.Name, " ", "_"))) }
if len(lgs) > 0 || len(cgs) > 0 { sb.WriteString(fmt.Sprintf("<i>Language (Country): </i>%s (%s)\n", strings.Join(lgs, " "), strings.Join(cgs, " "))) }
sb.WriteString("</blockquote>\n\n")

if t.Tagline != "" { sb.WriteString(fmt.Sprintf("<b>\"%s\"</b>\n\n", t.Tagline)) }

shortOverview := t.Overview
if rs := []rune(t.Overview); len(rs) > 800 { shortOverview = string(rs[:797]) + "..." }
if t.Overview != "" { sb.WriteString(fmt.Sprintf("<blockquote><b>Story Line: </b><i>%s</i></blockquote>\n\n", shortOverview)) }

var dirs, writers, prods, stars, cast []string
for _, c := range t.Credits.Crew {
if c.Job == "Director" || (isSeries && (c.Job == "Executive Producer" || c.Job == "Creator")) { if len(dirs) < 3 { dirs = append(dirs, link(c.Name, c.ID)) } }
if c.Department == "Writing" || c.Job == "Writer" || c.Job == "Screenplay" { if len(writers) < 3 { writers = append(writers, link(c.Name, c.ID)) } }
if c.Job == "Producer" { if len(prods) < 3 { prods = append(prods, link(c.Name, c.ID)) } }
}
for i, c := range t.Credits.Cast {
if i < 4 { stars = append(stars, link(c.Name, c.ID)) }
if i >= 4 && i < topCastLimit+4 { cast = append(cast, link(c.Name, c.ID)) }
}

sb.WriteString("<blockquote>")
if len(dirs) > 0 { sb.WriteString(fmt.Sprintf("<i><b>Directors:</b></i> %s\n", strings.Join(dirs, ", "))) }
if len(writers) > 0 { sb.WriteString(fmt.Sprintf("<i><b>Writers:</b></i> %s\n", strings.Join(writers, ", "))) }
if len(prods) > 0 { sb.WriteString(fmt.Sprintf("<i><b>Producers:</b></i> %s\n", strings.Join(prods, ", "))) }
if len(stars) > 0 { sb.WriteString(fmt.Sprintf("<i><b>Stars:</b></i> %s\n", strings.Join(stars, ", "))) }
if len(cast) > 0 { sb.WriteString(fmt.Sprintf("<i><b>Top Cast:</b></i> %s", strings.Join(cast, ", "))) }
sb.WriteString("</blockquote>\n\n")

if omdbFill.Awards != "" && omdbFill.Awards != notAvailable {
sb.WriteString(fmt.Sprintf("<b>Awards: </b><a href=\"https://imdb.com/title/%s/awards\">%s</a>\n", imdbID, omdbFill.Awards))
}
sb.WriteString(fmt.Sprintf("<b>OTT Info: </b><a href=\"https://www.justwatch.com/in/search?q=%s\">Find on JustWatch</a>\n", url.QueryEscape(title)))

dl := omdbBanner
if t.PosterPath != "" { dl = "https://image.tmdb.org/t/p/original" + t.PosterPath } else if omdbFill.Poster != "" && omdbFill.Poster != notAvailable { dl = omdbFill.Poster }

if enableTelegraph {
var nodes []tgNode
nodes = append(nodes, tgNode{Tag: "h3", Children: []any{tgNode{Tag: "b", Children: []any{fmt.Sprintf("%s %s", title, yearStr)}}}})
if dl != omdbBanner { nodes = append(nodes, tgNode{Tag: "figure", Children: []any{tgNode{Tag: "img", Attrs: &tgAttrs{Src: dl}}}}) }
if t.Tagline != "" { nodes = append(nodes, tgNode{Tag: "blockquote", Children: []any{tgNode{Tag: "i", Children: []any{t.Tagline}}}}) }

nodes = append(nodes, makeHeader("Overview"), tgNode{Tag: "p", Children: []any{t.Overview}})
nodes = append(nodes, makeHeader("General Information"), makeRow("Type", typeStr))

if origTitle != "" && origTitle != title { nodes = append(nodes, makeRow("Original Title", origTitle)) }
if omdbFill.Rated != "" && omdbFill.Rated != notAvailable { nodes = append(nodes, makeRow("Content Rating", omdbFill.Rated)) }
if isSeries && t.NumberOfSeasons > 0 { nodes = append(nodes, makeRow("Seasons", strconv.Itoa(t.NumberOfSeasons))) }
if isSeries && t.NumberOfEpisodes > 0 { nodes = append(nodes, makeRow("Episodes", strconv.Itoa(t.NumberOfEpisodes))) }
if runtime > 0 { nodes = append(nodes, makeRow("Runtime", fmt.Sprintf("%d minutes", runtime))) }
if t.Status != "" { nodes = append(nodes, makeRow("Status", t.Status)) }

nodes = append(nodes, makeHeader("Ratings & Popularity"))
nodes = append(nodes, makeRow("IMDb Rating", fmt.Sprintf("%.1f/10 (from %d votes)", t.VoteAverage, t.VoteCount)))
if omdbFill.Metascore != "" && omdbFill.Metascore != notAvailable { nodes = append(nodes, makeRow("Metascore", omdbFill.Metascore)) }
if t.Popularity > 0 { nodes = append(nodes, makeRow("Popularity Score", fmt.Sprintf("%.2f", t.Popularity))) }

nodes = append(nodes, makeHeader("Genres & Themes"))
if len(t.Genres) > 0 { var gList []string; for _, g := range t.Genres { gList = append(gList, g.Name) }; nodes = append(nodes, makeRow("Genres", strings.Join(gList, ", "))) }
if len(themes) > 0 { nodes = append(nodes, makeRow("Themes", strings.Join(themes, " "))) }

nodes = append(nodes, makeHeader("Financials & Production"))
if t.Budget > 0 { nodes = append(nodes, makeRow("Budget", fmt.Sprintf("$%d", t.Budget))) }
if omdbFill.BoxOffice != "" && omdbFill.BoxOffice != notAvailable { nodes = append(nodes, makeRow("Domestic Box Office", omdbFill.BoxOffice)) }
if t.Revenue > 0 { nodes = append(nodes, makeRow("Worldwide Gross", fmt.Sprintf("$%d", t.Revenue))) }

var pComps []string; for _, pc := range t.ProductionCompanies { pComps = append(pComps, pc.Name) }
if len(pComps) > 0 { nodes = append(nodes, makeRow("Production Companies", strings.Join(pComps, ", "))) }
var nets []string; for _, n := range t.Networks { nets = append(nets, n.Name) }
if len(nets) > 0 { nodes = append(nodes, makeRow("Networks", strings.Join(nets, ", "))) }

var lgsTel []string; for _, l := range t.SpokenLanguages { langName := l.EnglishName; if langName == "" { langName = l.Name }; if langName != "" { lgsTel = append(lgsTel, langName) } }
if len(lgsTel) > 0 { nodes = append(nodes, makeRow("Spoken Languages", strings.Join(lgsTel, ", "))) }
var cgsTel []string; for _, c := range t.ProductionCountries { cgsTel = append(cgsTel, c.Name) }
if len(cgsTel) > 0 { nodes = append(nodes, makeRow("Production Countries", strings.Join(cgsTel, ", "))) }

if len(t.Credits.Cast) > 0 {
nodes = append(nodes, makeHeader("Full Cast"))
var castList []string
for _, c := range t.Credits.Cast {
role := ""; if c.Character != "" { role = " as " + c.Character }
castList = append(castList, c.Name+role)
}
nodes = append(nodes, tgNode{Tag: "p", Children: []any{strings.Join(castList, ", ")}})
}

trailerLink := ""
for _, v := range t.Videos.Results {
if v.Site == "YouTube" && v.Type == "Trailer" { trailerLink = "https://www.youtube.com/watch?v=" + v.Key; break }
}
if trailerLink != "" { nodes = append(nodes, makeHeader("Media"), tgNode{Tag: "p", Children: []any{fmt.Sprintf("Trailer: %s", trailerLink)}}) }

page := createTelegraphPage(title+" Details", nodes)
sb.WriteString(fmt.Sprintf("\n<a href=\"https://imdb.com/title/%s\">Read More...</a>", imdbID))
if page != "" { sb.WriteString(fmt.Sprintf(" | <a href=\"%s\">Full Details</a>", page)) }
} else {
sb.WriteString(fmt.Sprintf("\n<a href=\"https://imdb.com/title/%s\">Read More...</a>", imdbID))
}

trailerLink := ""
for _, v := range t.Videos.Results {
if v.Site == "YouTube" && v.Type == "Trailer" { trailerLink = "https://www.youtube.com/watch?v=" + v.Key; break }
}
if trailerLink != "" { sb.WriteString(fmt.Sprintf(" | <a href=\"%s\">Trailer</a>", trailerLink)) }
sb.WriteString(fmt.Sprintf(" | <a href=\"%s\">Download Poster</a>", dl))

return dl, sb.String(), buttons, nil
}

func getFlag(country string) string {
    flagMap := map[string]string{ "United States": "🇺🇸 US", "USA": "🇺🇸 US", "US": "🇺🇸 US", "United Kingdom": "🇬🇧 UK", "UK": "🇬🇧 UK", "GB": "🇬🇧 UK", "India": "🇮🇳 IN", "IN": "🇮🇳 IN", "France": "🇫🇷 FR", "FR": "🇫🇷 FR", "Japan": "🇯🇵 JP", "JP": "🇯🇵 JP", "Canada": "🇨🇦 CA", "CA": "🇨🇦 CA", "Germany": "🇩🇪 DE", "DE": "🇩🇪 DE", "Australia": "🇦🇺 AU", "AU": "🇦🇺 AU", "Korea": "🇰🇷 KR", "South Korea": "🇰🇷 KR", "KR": "🇰🇷 KR", "China": "🇨🇳 CN", "CN": "🇨🇳 CN", "Russia": "🇷🇺 RU", "RU": "🇷🇺 RU", "Italy": "🇮🇹 IT", "IT": "🇮🇹 IT", "Spain": "🇪🇸 ES", "ES": "🇪🇸 ES", "Brazil": "🇧🇷 BR", "BR": "🇧🇷 BR" }
    if val, ok := flagMap[country]; ok { return val }
    for k, v := range flagMap { if strings.Contains(country, k) { return v } }
    return ""
}

func link(name string, id any) string {
if idStr, ok := id.(string); ok { return fmt.Sprintf("<a href=\"https://imdb.com/name/%s\">%s</a>", idStr, name) }
if idInt, ok := id.(int); ok { return fmt.Sprintf("<a href=\"https://www.themoviedb.org/person/%d\">%s</a>", idInt, name) }
return name
}
