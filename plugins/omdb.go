// (c) Jisin0
// Functions and types to search using Hybrid APIs.

package plugins

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
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

	// API Endpoints
	apiPrimary  = "https://imdb.iamidiotareyoutoo.com/search" // Used for Details only
	apiFallback = "https://api.balloonerismm.workers.dev"     // NEW: Balloonerism API for Search & Fallback

	// Configuration
	topCastLimit    = 30
	enableAIReview  = true
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
	if enableTelegraph {
		go ensureTelegraphToken()
	}
}

// --- SHARED HELPER STRUCT ---
type UniversalSearchResult struct {
	ID     string
	Title  string
	Year   int
	Poster string
	Type   string
	Rating float64
}

// ==========================================
// 1. TELEGRAPH HELPERS
// ==========================================

func ensureTelegraphToken() {
	if telegraphToken != "" {
		return
	}
	resp, err := http.Get("https://api.telegra.ph/createAccount?short_name=FilmigoBot&author_name=Filmigo+Bot")
	if err == nil {
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
	resp, err := http.PostForm("https://api.telegra.ph/createPage", data)
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
func makeHeader(text string) tgNode {
	return tgNode{Tag: "h4", Children: []any{text}}
}
func makeSubHeader(text string) tgNode {
	return tgNode{Tag: "h5", Children: []any{text}}
}

// ==========================================
// 2. PRIMARY API STRUCTS
// ==========================================
// (Kept unchanged for your Main API connection)
type primaryDetailData struct {
	Ok    bool `json:"ok"`
	Short struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Trailer     struct {
			EmbedURL string `json:"embedUrl"`
		} `json:"trailer"`
	} `json:"short"`
	ReviewSummary *struct {
		Overall struct {
			Medium struct {
				Value struct {
					PlaidHtml string `json:"plaidHtml"`
				} `json:"value"`
			} `json:"medium"`
		} `json:"overall"`
	} `json:"reviewSummary"`
	Top struct {
		TitleText   struct{ Text string `json:"text"` } `json:"titleText"`
		TitleType   struct{ Text string `json:"text"` } `json:"titleType"`
		ReleaseYear struct {
			Year    int `json:"year"`
			EndYear int `json:"endYear"`
		} `json:"releaseYear"`
		ReleaseDate struct {
			Day     int `json:"day"`
			Month   int `json:"month"`
			Year    int `json:"year"`
			Country struct{ Text string `json:"text"` } `json:"country"`
		} `json:"releaseDate"`
		Runtime struct {
			DisplayableProperty struct {
				Value struct{ PlainText string `json:"plainText"` } `json:"value"`
			} `json:"displayableProperty"`
		} `json:"runtime"`
		RatingsSummary struct {
			AggregateRating float64 `json:"aggregateRating"`
			VoteCount       int     `json:"voteCount"`
		} `json:"ratingsSummary"`
		Metacritic *struct {
			Metascore struct{ Score int `json:"score"` } `json:"metascore"`
		} `json:"metacritic"`
		Genres    struct{ Genres []struct{ Text string `json:"text"` } `json:"genres"` } `json:"genres"`
		Interests struct {
			Edges []struct {
				Node struct {
					PrimaryText struct{ Text string `json:"text"` } `json:"primaryText"`
				} `json:"node"`
			} `json:"edges"`
		} `json:"interests"`
		Plot struct {
			PlotText struct{ PlainText string `json:"plainText"` } `json:"plotText"`
		} `json:"plot"`
		PrimaryImage struct{ URL string `json:"url"` } `json:"primaryImage"`
		Directors    []struct {
			Credits []struct {
				Name struct {
					NameText struct{ Text string `json:"text"` } `json:"nameText"`
					ID       string `json:"id"`
				} `json:"name"`
			} `json:"credits"`
		} `json:"directorsPageTitle"`
		PrincipalCredits []struct {
			Grouping struct{ Text string `json:"text"` } `json:"grouping"`
			Credits  []struct {
				Name struct {
					NameText struct{ Text string `json:"text"` } `json:"nameText"`
					ID       string `json:"id"`
				} `json:"name"`
			} `json:"credits"`
		} `json:"principalCreditsV2"`
		Cast []struct {
			Grouping struct{ Text string `json:"text"` } `json:"grouping"`
			Credits  []struct {
				Name struct {
					NameText struct{ Text string `json:"text"` } `json:"nameText"`
					ID       string `json:"id"`
				} `json:"name"`
			} `json:"credits"`
		} `json:"castV2"`
		Certificate struct{ Rating string `json:"rating"` } `json:"certificate"`
		ProductionStatus struct {
			CurrentProductionStage struct{ Text string `json:"text"` } `json:"currentProductionStage"`
		} `json:"productionStatus"`
		FeaturedReviews *struct {
			Edges []struct {
				Node struct {
					Author       struct{ NickName string `json:"nickName"` } `json:"author"`
					Summary      struct{ OriginalText string `json:"originalText"` } `json:"summary"`
					Text         struct{ OriginalText struct{ PlainHtml string `json:"plaidHtml"` } `json:"originalText"` } `json:"text"`
					AuthorRating int `json:"authorRating"`
				} `json:"node"`
			} `json:"edges"`
		} `json:"featuredReviews"`
		TriviaTotal struct{ Total int `json:"total"` } `json:"triviaTotal"`
		Trivia      struct {
			Edges []struct {
				Node struct {
					Text struct{ PlaidHtml string `json:"plaidHtml"` } `json:"text"`
				} `json:"node"`
			} `json:"edges"`
		} `json:"trivia"`
		GoofsTotal struct{ Total int `json:"total"` } `json:"goofsTotal"`
		Goofs      struct {
			Edges []struct {
				Node struct {
					Text struct{ PlaidHtml string `json:"plaidHtml"` } `json:"text"`
				} `json:"node"`
			} `json:"edges"`
		} `json:"goofs"`
		QuotesTotal struct{ Total int `json:"total"` } `json:"quotesTotal"`
		Quotes      struct {
			Edges []struct {
				Node struct {
					Lines []struct{ Text string `json:"text"` } `json:"lines"`
				} `json:"node"`
			} `json:"edges"`
		} `json:"quotes"`
		FilmingLocations struct {
			Edges []struct {
				Node struct{ Text string `json:"text"` } `json:"node"`
			} `json:"edges"`
		} `json:"filmingLocations"`
		Production struct {
			Edges []struct {
				Node struct {
					Company struct {
						CompanyText struct{ Text string `json:"text"` } `json:"companyText"`
					} `json:"company"`
				} `json:"node"`
			} `json:"edges"`
		} `json:"production"`
		Soundtrack struct {
			Edges []struct {
				Node struct{ Text string `json:"text"` } `json:"node"`
			} `json:"edges"`
		} `json:"soundtrack"`
	} `json:"top"`
	Main struct {
		PrestigiousAwardSummary *struct {
			Nominations int `json:"nominations"`
			Wins        int `json:"wins"`
		} `json:"prestigiousAwardSummary"`
		Wins        struct{ Total int `json:"total"` } `json:"wins"`
		Nominations struct{ Total int `json:"total"` } `json:"nominationsExcludeWins"`
		Languages   struct {
			Languages []struct{ Text string `json:"text"` } `json:"spokenLanguages"`
		} `json:"spokenLanguages"`
		Countries struct {
			Countries []struct{ Text string `json:"text"` } `json:"countries"`
		} `json:"countriesDetails"`
		Akas struct {
			Edges []struct {
				Node struct{ Text string `json:"text"` } `json:"node"`
			} `json:"edges"`
		} `json:"akas"`
		Cast []struct {
			Grouping struct{ Text string `json:"text"` } `json:"grouping"`
			Credits  []struct {
				Name struct {
					NameText struct{ Text string `json:"text"` } `json:"nameText"`
					ID       string `json:"id"`
				} `json:"name"`
				Characters []struct {
					Name string `json:"name"`
				} `json:"characters"`
			} `json:"credits"`
		} `json:"castV2"`
		Episodes *struct {
			Seasons []struct {
				Number int `json:"number"`
			} `json:"seasons"`
			TotalEpisodes struct{ Total int `json:"total"` } `json:"totalEpisodes"`
		} `json:"episodes"`
		ProductionBudget *struct {
			Budget struct {
				Amount   int64  `json:"amount"`
				Currency string `json:"currency"`
			} `json:"budget"`
		} `json:"productionBudget"`
		LifetimeGross *struct {
			Total struct {
				Amount   int64  `json:"amount"`
				Currency string `json:"currency"`
			} `json:"total"`
		} `json:"lifetimeGross"`
		WorldwideGross *struct {
			Total struct {
				Amount   int64  `json:"amount"`
				Currency string `json:"currency"`
			} `json:"total"`
		} `json:"worldwideGross"`
		TechnicalSpecifications *struct {
			SoundMixes struct {
				Items []struct{ Text string `json:"text"` } `json:"items"`
			} `json:"soundMixes"`
			AspectRatios struct {
				Items []struct{ AspectRatio string `json:"aspectRatio"` } `json:"items"`
			} `json:"aspectRatios"`
		} `json:"technicalSpecifications"`
	} `json:"main"`
}

// ==========================================
// 3. NEW BALLOONERISM API STRUCTS
// ==========================================

type balloonSearchRes struct {
	Results []struct {
		MediaType    string  `json:"media_type"`
		ID           string  `json:"id"`
		Title        string  `json:"title"`
		Name         string  `json:"name"`
		ReleaseDate  string  `json:"release_date"`
		FirstAirDate string  `json:"first_air_date"`
		PosterPath   string  `json:"poster_path"`
		VoteAverage  float64 `json:"vote_average"`
	} `json:"results"`
}

type balloonMovieDetail struct {
	ID                  string  `json:"id"`
	Title               string  `json:"title"`
	OriginalTitle       string  `json:"original_title"`
	Overview            string  `json:"overview"`
	Tagline             string  `json:"tagline"`
	ReleaseDate         string  `json:"release_date"`
	Runtime             int     `json:"runtime"`
	VoteAverage         float64 `json:"vote_average"`
	VoteCount           int     `json:"vote_count"`
	Genres              []struct{ Name string `json:"name"` } `json:"genres"`
	PosterPath          string `json:"poster_path"`
	SpokenLanguages     []struct{ Name string `json:"name"` } `json:"spoken_languages"`
	ProductionCountries []struct{ Iso3166_1 string `json:"iso_3166_1"`; Name string `json:"name"` } `json:"production_countries"`
}

type balloonTVDetail struct {
	ID                  string  `json:"id"`
	Name                string  `json:"name"`
	OriginalName        string  `json:"original_name"`
	Overview            string  `json:"overview"`
	FirstAirDate        string  `json:"first_air_date"`
	NumberOfEpisodes    int     `json:"number_of_episodes"`
	NumberOfSeasons     int     `json:"number_of_seasons"`
	EpisodeRunTime      []int   `json:"episode_run_time"`
	VoteAverage         float64 `json:"vote_average"`
	VoteCount           int     `json:"vote_count"`
	Genres              []struct{ Name string `json:"name"` } `json:"genres"`
	PosterPath          string `json:"poster_path"`
	SpokenLanguages     []struct{ Name string `json:"name"` } `json:"spoken_languages"`
	ProductionCountries []struct{ Iso3166_1 string `json:"iso_3166_1"`; Name string `json:"name"` } `json:"production_countries"`
	OriginCountry       []string `json:"origin_country"`
}

type balloonCredits struct {
	Cast []struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		Character string `json:"character"`
	} `json:"cast"`
	Crew []struct {
		ID         string `json:"id"`
		Name       string `json:"name"`
		Job        string `json:"job"`
		Department string `json:"department"`
	} `json:"crew"`
}

type balloonAkas struct {
	Titles []struct {
		Title     string `json:"title"`
		Iso3166_1 string `json:"iso_3166_1"`
	} `json:"titles"`
}

type omdbFillData struct {
	Released     string `json:"Released"`
	Awards       string `json:"Awards"`
	TotalSeasons string `json:"totalSeasons"`
	Country      string `json:"Country"`
}

// Helpers for the new API
func parseYear(date string) int {
	if len(date) >= 4 {
		y, _ := strconv.Atoi(date[:4])
		return y
	}
	return 0
}

func fetchBalloonerismDetail(id string) (*balloonMovieDetail, *balloonTVDetail, error) {
	// Try Movie first
	resp, err := http.Get(fmt.Sprintf("%s/movie/%s", apiFallback, id))
	if err == nil && resp.StatusCode == 200 {
		var m balloonMovieDetail
		json.NewDecoder(resp.Body).Decode(&m)
		resp.Body.Close()
		if m.ID != "" {
			return &m, nil, nil
		}
	}
	// Try TV Show
	resp, err = http.Get(fmt.Sprintf("%s/tv/%s", apiFallback, id))
	if err == nil && resp.StatusCode == 200 {
		var t balloonTVDetail
		json.NewDecoder(resp.Body).Decode(&t)
		resp.Body.Close()
		if t.ID != "" {
			return nil, &t, nil
		}
	}
	return nil, nil, errors.New("not found in fallback API")
}


// ==========================================
// 4. UNIFIED SEARCH FUNCTION
// ==========================================

func SearchOMDb(query string) ([]UniversalSearchResult, error) {
	// --- SMART ID EXTRACTOR ---
	var imdbID string
	if strings.Contains(query, "imdb.com/title/tt") || (strings.HasPrefix(query, "tt") && len(query) >= 7) {
		start := strings.Index(query, "tt")
		idPart := query[start:]
		end := strings.IndexAny(idPart, "/? \n\t")
		if end == -1 {
			end = len(idPart)
		}
		imdbID = idPart[:end]
	}

	if imdbID != "" {
		m, t, err := fetchBalloonerismDetail(imdbID)
		if err == nil {
			res := UniversalSearchResult{ID: imdbID}
			if m != nil {
				res.Title = m.Title
				res.Year = parseYear(m.ReleaseDate)
				res.Poster = m.PosterPath
				res.Type = "Movie"
				res.Rating = m.VoteAverage
			} else if t != nil {
				res.Title = t.Name
				res.Year = parseYear(t.FirstAirDate)
				res.Poster = t.PosterPath
				res.Type = "TV Series"
				res.Rating = t.VoteAverage
			}
			return []UniversalSearchResult{res}, nil
		}
	}

	// --- REGULAR MULTI-SEARCH ---
	apiURL := fmt.Sprintf("%s/search/multi?query=%s", apiFallback, url.QueryEscape(query))
	resp, err := http.Get(apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var bData balloonSearchRes
	if err := json.NewDecoder(resp.Body).Decode(&bData); err != nil {
		return nil, err
	}

	var results []UniversalSearchResult
	for _, item := range bData.Results {
		if item.MediaType == "person" {
			continue // Skip actor profiles
		}

		title := item.Title
		if title == "" {
			title = item.Name
		}
		
		dateStr := item.ReleaseDate
		if dateStr == "" {
			dateStr = item.FirstAirDate
		}
		year := parseYear(dateStr)

		typeTag := "Movie"
		if item.MediaType == "tv" {
			typeTag = "TV Series"
		}

		results = append(results, UniversalSearchResult{
			ID: item.ID, Title: title, Year: year, Poster: item.PosterPath, Type: typeTag, Rating: item.VoteAverage,
		})
	}
	
	if len(results) == 0 {
		return nil, errors.New("no results found")
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
			Id:           searchMethodOMDb + "_" + item.ID,
			Title:        title,
			Description:  description,
			ThumbnailUrl: posterURL,
			InputMessageContent: gotgbot.InputTextMessageContent{
				MessageText: fmt.Sprintf("<i>Loading details for %s...</i>", item.Title),
				ParseMode:   gotgbot.ParseModeHTML,
			},
			ReplyMarkup: &gotgbot.InlineKeyboardMarkup{InlineKeyboard: [][]gotgbot.InlineKeyboardButton{
				{{Text: "Open IMDb", CallbackData: fmt.Sprintf("open_%s_%s", searchMethodOMDb, item.ID)}},
			}},
		})
	}
	return tgResults
}

// ==========================================
// 5. UNIFIED DETAILS FUNCTION
// ==========================================

func GetOMDbTitle(id string, progress func(string)) (string, string, [][]gotgbot.InlineKeyboardButton, error) {
	if progress != nil {
		go progress("<i>Using Primary API...</i>")
	}
	p, c, b, err := getDetailsPrimary(id)
	if err == nil {
		return p, c, b, nil
	}
	if progress != nil {
		go progress("<i>Primary API is offline. Using Fallback...</i>")
	}
	return getDetailsFallback(id)
}

func getDetailsPrimary(id string) (string, string, [][]gotgbot.InlineKeyboardButton, error) {
	var buttons [][]gotgbot.InlineKeyboardButton
	apiURL := fmt.Sprintf("%s?tt=%s", apiPrimary, id)
	resp, err := http.Get(apiURL)
	if err != nil {
		return "", "", buttons, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var t primaryDetailData
	if err := json.Unmarshal(body, &t); err != nil {
		return "", "", buttons, err
	}
	if !t.Ok || t.Top.TitleText.Text == "" {
		return "", "", buttons, errors.New("Not found in Primary")
	}

	// Maps
	isSeries := (t.Top.TitleType.Text == "TV Series" || t.Top.TitleType.Text == "TV Mini Series")
	monthMap := map[int]string{1: "January", 2: "February", 3: "March", 4: "April", 5: "May", 6: "June", 7: "July", 8: "August", 9: "September", 10: "October", 11: "November", 12: "December"}
	genreEmojiMap := map[string]string{
		"Action": "💥", "Adventure": "🗺️", "Sci-Fi": "🚀", "Comedy": "🤣", "Drama": "🎭", "Romance": "🌹",
		"Thriller": "🔪", "Horror": "👻", "Fantasy": "✨",
		"Mystery": "❓", "Crime": "-", "Animation": "-",
		"War": "-", "History": "-","Music": "🎶",
	}
	countryFlagMap := map[string]string{
		"United States": "🇺🇸", "USA": "🇺🇸",
		"United Kingdom": "🇬🇧", "UK": "🇬🇧",
		"India": "🇮🇳", "France": "🇫🇷",
		"Japan": "🇯🇵", "Canada": "🇨🇦",
		"Germany": "🇩🇪",
	}
	genreMap := make(map[string]bool)

	var sb strings.Builder
	imdbURL := omdbHomepage + "/title/" + id

	// Title
	var yearString string
	if isSeries {
		if t.Top.ReleaseYear.EndYear > 0 {
			yearString = fmt.Sprintf("[%d-%d]", t.Top.ReleaseYear.Year, t.Top.ReleaseYear.EndYear)
		} else {
			yearString = fmt.Sprintf("[%d-Present]", t.Top.ReleaseYear.Year)
		}
	} else {
		yearString = fmt.Sprintf("[%d]", t.Top.ReleaseYear.Year)
	}
	sb.WriteString(fmt.Sprintf("<i>%s: </i><b>%s %s</b> | <a href=\"%s\">IMDb Link</a>\n", t.Top.TitleType.Text, t.Top.TitleText.Text, yearString, imdbURL))

	if len(t.Main.Akas.Edges) > 0 {
		sb.WriteString(fmt.Sprintf("<i>(AKA: %s)</i>\n", t.Main.Akas.Edges[0].Node.Text))
	}

	if isSeries && t.Main.Episodes != nil {
		seasonCount := len(t.Main.Episodes.Seasons)
		episodeCount := t.Main.Episodes.TotalEpisodes.Total
		if seasonCount > 0 && episodeCount > 0 {
			sb.WriteString(fmt.Sprintf("<b>%d Seasons (%d Episodes)</b>\n", seasonCount, episodeCount))
		}
	}

	if t.Top.Runtime.DisplayableProperty.Value.PlainText != "" {
		dur := t.Top.Runtime.DisplayableProperty.Value.PlainText
		if isSeries {
			dur += "/Episode"
		}
		sb.WriteString(fmt.Sprintf("<i>Duration: </i>%s\n", dur))
	}

	rd := t.Top.ReleaseDate
	if rd.Year > 0 {
		date := fmt.Sprintf("%d %s %d", rd.Day, monthMap[rd.Month], rd.Year)
		if rd.Country.Text != "" {
			date += " (" + rd.Country.Text + ")"
			flag := getFlag(rd.Country.Text)
			if flag != "" {
				date += " " + flag
			}
		}
		if isSeries {
			date += " - For First Episode"
		}
		sb.WriteString(fmt.Sprintf("<i>Release Date: </i>%s\n", date))
	}

	rating := ""
	if t.Top.RatingsSummary.AggregateRating > 0 {
		rating = fmt.Sprintf("<i>Rating ⭐️ </i><b>%.1f / 10</b> (from %d votes)", t.Top.RatingsSummary.AggregateRating, t.Top.RatingsSummary.VoteCount)
	}
	if t.Top.Metacritic != nil && t.Top.Metacritic.Metascore.Score > 0 {
		if rating != "" {
			rating += " | "
		}
		rating += fmt.Sprintf("<b>Ⓜ️ %d/100</b>", t.Top.Metacritic.Metascore.Score)
	}
	if rating != "" {
		sb.WriteString(rating + "\n")
	}

	sb.WriteString("<blockquote>")
	if len(t.Top.Genres.Genres) > 0 {
		var gs []string
		for _, g := range t.Top.Genres.Genres {
			emoji := "- "
			if e, ok := genreEmojiMap[g.Text]; ok {
				emoji = e + " "
			}
			gs = append(gs, fmt.Sprintf("%s#%s", emoji, g.Text))
			genreMap[g.Text] = true
		}
		sb.WriteString(fmt.Sprintf("<i>Genres: </i>%s\n", strings.Join(gs, " ")))
	}
	if len(t.Top.Interests.Edges) > 0 {
		var ts []string
		for _, tx := range t.Top.Interests.Edges {
			name := tx.Node.PrimaryText.Text
			if !genreMap[name] {
				ts = append(ts, "#"+strings.ReplaceAll(name, " ", "_"))
			}
		}
		if len(ts) > 0 {
			sb.WriteString(fmt.Sprintf("<i>Themes: </i>%s\n", strings.Join(ts, " ")))
		}
	}
	var langs, countries []string
	for _, l := range t.Main.Languages.Languages {
		langs = append(langs, "#"+l.Text)
	}
	for _, c := range t.Main.Countries.Countries {
		flag := ""
		if f, ok := countryFlagMap[c.Text]; ok {
			flag = f + " "
		}
		countries = append(countries, fmt.Sprintf("%s#%s", flag, strings.ReplaceAll(c.Text, " ", "_")))
	}
	sb.WriteString(fmt.Sprintf("<i>Language (Country): </i>%s (%s)", strings.Join(langs, " "), strings.Join(countries, " ")))
	sb.WriteString("</blockquote>\n\n")

	if t.Top.Plot.PlotText.PlainText != "" {
		sb.WriteString(fmt.Sprintf("<blockquote><b>Story Line: </b><i>%s</i></blockquote>\n\n", t.Top.Plot.PlotText.PlainText))
	}

	if enableAIReview && t.ReviewSummary != nil && t.ReviewSummary.Overall.Medium.Value.PlaidHtml != "" {
		sb.WriteString(fmt.Sprintf("<blockquote><b>AI Review: </b><i>%s</i></blockquote>\n\n", html.UnescapeString(t.ReviewSummary.Overall.Medium.Value.PlaidHtml)))
	}

	sb.WriteString("<blockquote>")
	var dirs []string
	if len(t.Top.Directors) > 0 {
		for _, d := range t.Top.Directors[0].Credits {
			dirs = append(dirs, link(d.Name.NameText.Text, d.Name.ID))
		}
	}
	if len(dirs) == 0 {
		for _, g := range t.Top.PrincipalCredits {
			if strings.Contains(g.Grouping.Text, "Director") {
				for _, c := range g.Credits {
					dirs = append(dirs, link(c.Name.NameText.Text, c.Name.ID))
				}
			}
		}
	}
	if isSeries && len(dirs) == 0 {
		for _, g := range t.Top.PrincipalCredits {
			if strings.Contains(g.Grouping.Text, "Creator") {
				for _, c := range g.Credits {
					dirs = append(dirs, link(c.Name.NameText.Text, c.Name.ID))
				}
			}
		}
	}
	if len(dirs) > 0 {
		sb.WriteString(fmt.Sprintf("<i><b>Directors:</b></i> %s\n", strings.Join(dirs, ", ")))
	}

	var writers, stars []string
	isStar := make(map[string]bool)
	for _, g := range t.Top.PrincipalCredits {
		if strings.Contains(g.Grouping.Text, "Writer") {
			for _, c := range g.Credits {
				writers = append(writers, link(c.Name.NameText.Text, c.Name.ID))
			}
		}
		if strings.Contains(g.Grouping.Text, "Star") {
			for _, c := range g.Credits {
				stars = append(stars, link(c.Name.NameText.Text, c.Name.ID))
				isStar[c.Name.NameText.Text] = true
			}
		}
	}
	if len(writers) > 0 {
		sb.WriteString(fmt.Sprintf("<i><b>Writers:</b></i> %s\n", strings.Join(writers, ", ")))
	}
	if len(stars) > 0 {
		sb.WriteString(fmt.Sprintf("<i><b>Stars:</b></i> %s\n", strings.Join(stars, ", ")))
	}

	var topCast []string
	for _, g := range t.Main.Cast {
		if g.Grouping.Text == "Top Cast" {
			for _, c := range g.Credits {
				if !isStar[c.Name.NameText.Text] {
					if len(topCast) < topCastLimit {
						topCast = append(topCast, link(c.Name.NameText.Text, c.Name.ID))
					} else {
						break
					}
				}
			}
			break
		}
	}
	if len(topCast) > 0 {
		sb.WriteString(fmt.Sprintf("<i><b>Top Cast:</b></i> %s", strings.Join(topCast, ", ")))
	}
	sb.WriteString("</blockquote>\n\n")

	sb.WriteString("<blockquote>")
	awardsURL := fmt.Sprintf("%s/title/%s/awards", omdbHomepage, id)
	awards := ""
	if t.Main.PrestigiousAwardSummary != nil {
		awards = fmt.Sprintf("Won %d Oscars. %d wins & %d nominations total.", t.Main.PrestigiousAwardSummary.Wins, t.Main.Wins.Total, t.Main.Nominations.Total)
	} else if t.Main.Wins.Total > 0 {
		awards = fmt.Sprintf("%d wins & %d nominations total.", t.Main.Wins.Total, t.Main.Nominations.Total)
	}
	if awards != "" {
		sb.WriteString(fmt.Sprintf("<b>Awards: </b><a href=\"%s\">%s</a>\n", awardsURL, awards))
	}
	sb.WriteString(fmt.Sprintf("<b>OTT Info: </b><a href=\"https://www.justwatch.com/in/search?q=%s\">Find on JustWatch</a></blockquote>", url.QueryEscape(t.Top.TitleText.Text)))

	// Telegraph Generation
	if enableTelegraph {
		var nodes []tgNode
		nodes = append(nodes, tgNode{Tag: "h3", Children: []any{fmt.Sprintf("%s (%d)", t.Top.TitleText.Text, t.Top.ReleaseYear.Year)}})
		if t.Top.PrimaryImage.URL != "" {
			nodes = append(nodes, tgNode{Tag: "figure", Children: []any{tgNode{Tag: "img", Attrs: &tgAttrs{Src: t.Top.PrimaryImage.URL}}}})
		}
		nodes = append(nodes, makeHeader("Info"))
		nodes = append(nodes, makeRow("Type", t.Top.TitleType.Text), makeRow("Rating", rating))
		if t.Top.Certificate.Rating != "" {
			nodes = append(nodes, makeRow("Content Rating", t.Top.Certificate.Rating))
		}
		if len(dirs) > 0 {
			nodes = append(nodes, makeRow("Directors", strings.Join(dirs, ", ")))
		}
		if len(writers) > 0 {
			nodes = append(nodes, makeRow("Writers", strings.Join(writers, ", ")))
		}

		if t.Top.Plot.PlotText.PlainText != "" {
			nodes = append(nodes, makeHeader("Plot"))
			nodes = append(nodes, tgNode{Tag: "p", Children: []any{t.Top.Plot.PlotText.PlainText}})
		}

		if t.ReviewSummary != nil && t.ReviewSummary.Overall.Medium.Value.PlaidHtml != "" {
			nodes = append(nodes, makeHeader("AI Review Summary"))
			nodes = append(nodes, tgNode{Tag: "i", Children: []any{html.UnescapeString(t.ReviewSummary.Overall.Medium.Value.PlaidHtml)}})
		}

		if len(t.Main.Cast) > 0 {
			nodes = append(nodes, makeHeader("Full Cast & Crew"))
			for _, g := range t.Main.Cast {
				nodes = append(nodes, makeSubHeader(g.Grouping.Text))
				var members []string
				c := 0
				for _, cr := range g.Credits {
					if c > 100 {
						break
					}
					ch := ""
					if len(cr.Characters) > 0 {
						ch = " as " + cr.Characters[0].Name
					}
					members = append(members, cr.Name.NameText.Text+ch)
					c++
				}
				nodes = append(nodes, tgNode{Tag: "p", Children: []any{strings.Join(members, ", ")}})
			}
		}

		if t.Top.FeaturedReviews != nil && len(t.Top.FeaturedReviews.Edges) > 0 {
			nodes = append(nodes, makeHeader("Featured Reviews"))
			for _, r := range t.Top.FeaturedReviews.Edges {
				txt := strings.ReplaceAll(html.UnescapeString(r.Node.Text.OriginalText.PlainHtml), "<br/>", "\n")
				nodes = append(nodes, tgNode{Tag: "blockquote", Children: []any{tgNode{Tag: "b", Children: []any{fmt.Sprintf("%d/10: ", r.Node.AuthorRating)}}, txt}})
			}
		}

		if t.Main.ProductionBudget != nil {
			nodes = append(nodes, makeHeader("Box Office"), makeRow("Budget", fmt.Sprintf("%d %s", t.Main.ProductionBudget.Budget.Amount, t.Main.ProductionBudget.Budget.Currency)))
		}

		if t.Top.TriviaTotal.Total > 0 {
			nodes = append(nodes, makeHeader("Trivia"))
			for i, x := range t.Top.Trivia.Edges {
				if i >= 50 {
					break
				}
				txt := strings.ReplaceAll(html.UnescapeString(x.Node.Text.PlaidHtml), "<br/>", "\n")
				txt = strings.ReplaceAll(txt, "</a>", "")
				if idx := strings.Index(txt, ">"); idx != -1 && strings.Contains(txt, "<a") {
					txt = txt[idx+1:]
				}
				nodes = append(nodes, tgNode{Tag: "blockquote", Children: []any{txt}})
			}
		}
		if t.Top.GoofsTotal.Total > 0 {
			nodes = append(nodes, makeHeader("Goofs"))
			for i, x := range t.Top.Goofs.Edges {
				if i >= 50 {
					break
				}
				nodes = append(nodes, tgNode{Tag: "p", Children: []any{"• " + html.UnescapeString(x.Node.Text.PlaidHtml)}})
			}
		}

		page := createTelegraphPage(t.Top.TitleText.Text+" Details", nodes)
		sb.WriteString(fmt.Sprintf("\n\n<a href=\"%s\">Read More...</a>", imdbURL))
		if page != "" {
			sb.WriteString(fmt.Sprintf(" | <a href=\"%s\">Full Details</a>", page))
		}
	} else {
		sb.WriteString(fmt.Sprintf("\n\n<a href=\"%s\">Read More...</a>", imdbURL))
	}

	trailer := t.Short.Trailer.EmbedURL
	if trailer == "" {
		trailer = fmt.Sprintf("https://www.youtube.com/results?search_query=%s", url.QueryEscape(t.Top.TitleText.Text+" trailer"))
	}
	sb.WriteString(fmt.Sprintf(" | <a href=\"%s\">Trailer</a>", trailer))

	poster := t.Top.PrimaryImage.URL
	dl := ""
	if poster != "" && poster != notAvailable {
		if strings.Contains(poster, "._V1_") {
			base := strings.Split(poster, "._V1_")[0]
			poster = base + "._V1_FMjpg_UX2000_.jpg"
			dl = base + "._V1_FMjpg_UX3000_.jpg"
		} else {
			dl = poster
		}
		sb.WriteString(fmt.Sprintf(" | <a href=\"%s\">Download Poster</a>", dl))
	} else {
		poster = omdbBanner
	}

	return poster, sb.String(), buttons, nil
}

func getDetailsFallback(id string) (string, string, [][]gotgbot.InlineKeyboardButton, error) {
	var buttons [][]gotgbot.InlineKeyboardButton

	// Fetch Base Detail from Balloonerism API
	mDetail, tDetail, err := fetchBalloonerismDetail(id)
	if err != nil {
		return "", "", buttons, err
	}

	isSeries := (tDetail != nil)
	endpointType := "movie"
	if isSeries {
		endpointType = "tv"
	}

	// Parallel Fetch: Credits, AKAs, OMDb
	var credits balloonCredits
	var akas balloonAkas
	var omdbFill omdbFillData
	var wg sync.WaitGroup

	wg.Add(3)
	go func() {
		defer wg.Done()
		if r, e := http.Get(fmt.Sprintf("%s/%s/%s/credits", apiFallback, endpointType, id)); e == nil {
			defer r.Body.Close()
			json.NewDecoder(r.Body).Decode(&credits)
		}
	}()
	go func() {
		defer wg.Done()
		if r, e := http.Get(fmt.Sprintf("%s/%s/%s/alternative_titles", apiFallback, endpointType, id)); e == nil {
			defer r.Body.Close()
			json.NewDecoder(r.Body).Decode(&akas)
		}
	}()
	go func() {
		defer wg.Done()
		if r, e := http.Get(fmt.Sprintf("https://www.omdbapi.com/?i=%s&apikey=%s", id, OmdbApiKey)); e == nil {
			defer r.Body.Close()
			json.NewDecoder(r.Body).Decode(&omdbFill)
		}
	}()
	wg.Wait()

	var sb strings.Builder

	// Extract unified fields
	var title, origTitle, dateStr, overview, posterPath, tagline string
	var startYear, runtime, seasons, episodes int
	var voteAverage float64
	var voteCount int
	var genres, languages, countries []string
	typeStr := "Movie"

	if isSeries {
		title = tDetail.Name
		origTitle = tDetail.OriginalName
		dateStr = tDetail.FirstAirDate
		startYear = parseYear(dateStr)
		overview = tDetail.Overview
		posterPath = tDetail.PosterPath
		if len(tDetail.EpisodeRunTime) > 0 {
			runtime = tDetail.EpisodeRunTime[0]
		}
		voteAverage = tDetail.VoteAverage
		voteCount = tDetail.VoteCount
		seasons = tDetail.NumberOfSeasons
		episodes = tDetail.NumberOfEpisodes
		typeStr = "TV Series"
		for _, g := range tDetail.Genres { genres = append(genres, g.Name) }
		for _, l := range tDetail.SpokenLanguages { languages = append(languages, l.Name) }
		for _, c := range tDetail.ProductionCountries { countries = append(countries, c.Iso3166_1) }
		if len(countries) == 0 { countries = tDetail.OriginCountry }
	} else {
		title = mDetail.Title
		origTitle = mDetail.OriginalTitle
		dateStr = mDetail.ReleaseDate
		startYear = parseYear(dateStr)
		overview = mDetail.Overview
		posterPath = mDetail.PosterPath
		tagline = mDetail.Tagline
		runtime = mDetail.Runtime
		voteAverage = mDetail.VoteAverage
		voteCount = mDetail.VoteCount
		for _, g := range mDetail.Genres { genres = append(genres, g.Name) }
		for _, l := range mDetail.SpokenLanguages { languages = append(languages, l.Name) }
		for _, c := range mDetail.ProductionCountries { countries = append(countries, c.Iso3166_1) }
	}

	yearStr := ""
	if startYear > 0 {
		yearStr = fmt.Sprintf("[%d]", startYear)
	}

	sb.WriteString(fmt.Sprintf("<i>%s: </i><b>%s %s</b> | <a href=\"%s\">IMDb Link</a>\n", typeStr, title, yearStr, omdbHomepage+"/title/"+id))

	if origTitle != "" && origTitle != title {
		sb.WriteString(fmt.Sprintf("<i>(Original Title: %s)</i>\n", origTitle))
	}

	// Smart AKA Logic
	akaStr := ""
	if len(akas.Titles) > 0 {
		for _, alt := range akas.Titles {
			if (alt.Iso3166_1 == "US" || alt.Iso3166_1 == "IN") && alt.Title != title {
				akaStr = alt.Title
				break
			}
		}
		if akaStr == "" && akas.Titles[0].Title != title {
			akaStr = akas.Titles[0].Title
		}
	}
	if akaStr != "" {
		sb.WriteString(fmt.Sprintf("<i>(AKA %s)</i>\n", akaStr))
	}

	if isSeries {
		if seasons > 0 {
			sb.WriteString(fmt.Sprintf("<b>%d Seasons (%d Episodes)</b>\n", seasons, episodes))
		} else if omdbFill.TotalSeasons != "" && omdbFill.TotalSeasons != notAvailable {
			sb.WriteString(fmt.Sprintf("<b>%s Seasons</b>\n", omdbFill.TotalSeasons))
		}
	}

	if runtime > 0 {
		h := runtime / 60
		m := runtime % 60
		dur := ""
		if h > 0 { dur += fmt.Sprintf("%dh ", h) }
		dur += fmt.Sprintf("%dm", m)
		if isSeries { dur += "/Episode" }
		sb.WriteString(fmt.Sprintf("<i>Duration: </i>%s\n", dur))
	}

	// Release Date & Flags
	if dateStr != "" {
		if parsed, err := time.Parse("2006-01-02", dateStr); err == nil {
			dateStr = parsed.Format("02 January 2006")
		}
		flag := ""
		if len(countries) > 0 {
			flag = getFlag(countries[0])
		} else if omdbFill.Country != "" && omdbFill.Country != notAvailable {
			flag = getFlag(omdbFill.Country)
		}
		if flag != "" {
			dateStr += fmt.Sprintf(" (%s)", flag)
		}
		if isSeries { dateStr += " - For First Episode" }
		sb.WriteString(fmt.Sprintf("<i>Release Date: </i>%s\n", dateStr))
	}

	if voteAverage > 0 {
		sb.WriteString(fmt.Sprintf("<i>Rating ⭐️ </i><b>%.1f / 10</b> (from %d votes)\n", voteAverage, voteCount))
	}

	sb.WriteString("<blockquote>")
	genreEmojiMap := map[string]string{"Action": "💥", "Adventure": "🗺️", "Science Fiction": "🚀", "Comedy": "🤣", "Drama": "🎭", "Romance": "🌹", "Thriller": "🔪", "Horror": "👻"}
	
	if len(genres) > 0 {
		var gs []string
		for _, g := range genres {
			emoji := "- "
			if e, ok := genreEmojiMap[g]; ok { emoji = e + " " }
			gs = append(gs, fmt.Sprintf("%s#%s", emoji, strings.ReplaceAll(g, " ", "_")))
		}
		sb.WriteString(fmt.Sprintf("<i>Genres: </i>%s\n", strings.Join(gs, " ")))
	}

	if len(languages) > 0 || len(countries) > 0 {
		var lgs, cgs []string
		for _, l := range languages { lgs = append(lgs, "#"+strings.ReplaceAll(l, " ", "_")) }
		for _, c := range countries {
			f := getFlag(c)
			if f != "" { f += " " }
			cgs = append(cgs, fmt.Sprintf("%s#%s", f, strings.ReplaceAll(c, " ", "_")))
		}
		sb.WriteString(fmt.Sprintf("<i>Language (Country): </i>%s (%s)", strings.Join(lgs, " "), strings.Join(cgs, " ")))
	}
	sb.WriteString("</blockquote>\n\n")

	if tagline != "" {
		sb.WriteString(fmt.Sprintf("<b>\"%s\"</b>\n\n", tagline))
	}

	if overview != "" {
		sb.WriteString(fmt.Sprintf("<blockquote><b>Story Line: </b><i>%s</i></blockquote>\n\n", overview))
	}

	sb.WriteString("<blockquote>")
	// Process Cast & Crew directly from API Credits
	var dirs, writers, stars, producers []string
	for _, c := range credits.Crew {
		if c.Job == "Director" || (isSeries && c.Job == "Executive Producer") {
			if len(dirs) < 3 { dirs = append(dirs, link(c.Name, c.ID)) }
		}
		if c.Department == "Writing" || c.Job == "Writer" || c.Job == "Screenplay" {
			if len(writers) < 3 { writers = append(writers, link(c.Name, c.ID)) }
		}
		if c.Job == "Producer" {
			if len(producers) < 4 { producers = append(producers, link(c.Name, c.ID)) }
		}
	}
	
	var topCast []string
	for i, c := range credits.Cast {
		if i < 4 {
			stars = append(stars, link(c.Name, c.ID))
		}
		if i >= 4 && i < topCastLimit+4 {
			topCast = append(topCast, link(c.Name, c.ID))
		}
	}

	if len(dirs) > 0 { sb.WriteString(fmt.Sprintf("<i><b>Directors:</b></i> %s\n", strings.Join(dirs, ", "))) }
	if len(writers) > 0 { sb.WriteString(fmt.Sprintf("<i><b>Writers:</b></i> %s\n", strings.Join(writers, ", "))) }
	if len(producers) > 0 { sb.WriteString(fmt.Sprintf("<i><b>Producers:</b></i> %s\n", strings.Join(producers, ", "))) }
	if len(stars) > 0 { sb.WriteString(fmt.Sprintf("<i><b>Stars:</b></i> %s\n", strings.Join(stars, ", "))) }
	if len(topCast) > 0 { sb.WriteString(fmt.Sprintf("<i><b>Top Cast:</b></i> %s", strings.Join(topCast, ", "))) }
	sb.WriteString("</blockquote>\n\n")

	sb.WriteString("<blockquote>")
	awardsURL := fmt.Sprintf("%s/title/%s/awards", omdbHomepage, id)
	if omdbFill.Awards != "" && omdbFill.Awards != notAvailable {
		sb.WriteString(fmt.Sprintf("<b>Awards: </b><a href=\"%s\">%s</a>\n", awardsURL, omdbFill.Awards))
	}
	sb.WriteString(fmt.Sprintf("<b>OTT Info: </b><a href=\"https://www.justwatch.com/in/search?q=%s\">Find on JustWatch</a></blockquote>", url.QueryEscape(title)))

	// Telegraph fallback generation
	if enableTelegraph {
		var nodes []tgNode
		nodes = append(nodes, tgNode{Tag: "h3", Children: []any{fmt.Sprintf("%s %s", title, yearStr)}})
		if posterPath != "" {
			nodes = append(nodes, tgNode{Tag: "figure", Children: []any{tgNode{Tag: "img", Attrs: &tgAttrs{Src: posterPath}}}})
		}
		nodes = append(nodes, makeHeader("Info"))
		nodes = append(nodes, makeRow("Type", typeStr))
		nodes = append(nodes, makeRow("Plot", overview))
		if len(credits.Cast) > 0 {
			nodes = append(nodes, makeHeader("Full Cast"))
			var castList []string
			for _, c := range credits.Cast {
				role := ""
				if c.Character != "" { role = " as " + c.Character }
				castList = append(castList, c.Name+role)
			}
			nodes = append(nodes, tgNode{Tag: "p", Children: []any{strings.Join(castList, ", ")}})
		}
		
		page := createTelegraphPage(title+" Details", nodes)
		sb.WriteString(fmt.Sprintf("\n\n<a href=\"%s\">Read More...</a>", omdbHomepage+"/title/"+id))
		if page != "" {
			sb.WriteString(fmt.Sprintf(" | <a href=\"%s\">Full Details</a>", page))
		}
	} else {
		sb.WriteString(fmt.Sprintf("\n\n<a href=\"%s\">Read More...</a>", omdbHomepage+"/title/"+id))
	}

	sb.WriteString(fmt.Sprintf(" | <a href=\"%s\">Trailer</a>", fmt.Sprintf("https://www.youtube.com/results?search_query=%s", url.QueryEscape(title+" trailer"))))

	poster := omdbBanner
	if posterPath != "" { poster = posterPath }
	sb.WriteString(fmt.Sprintf(" | <a href=\"%s\">Download Poster</a>", poster))

	return poster, sb.String(), buttons, nil
}

func getFlag(country string) string {
    flagMap := map[string]string{
        "United States": "🇺🇸 US", "USA": "🇺🇸 US", "US": "🇺🇸 US",
        "United Kingdom": "🇬🇧 UK", "UK": "🇬🇧 UK", "GB": "🇬🇧 UK",
        "India": "🇮🇳 IN", "IN": "🇮🇳 IN",
        "France": "🇫🇷 FR", "FR": "🇫🇷 FR",
        "Japan": "🇯🇵 JP", "JP": "🇯🇵 JP",
        "Canada": "🇨🇦 CA", "CA": "🇨🇦 CA",
        "Germany": "🇩🇪 DE", "DE": "🇩🇪 DE",
        "Australia": "🇦🇺 AU", "AU": "🇦🇺 AU",
        "Korea": "🇰🇷 KR", "South Korea": "🇰🇷 KR", "KR": "🇰🇷 KR",
        "China": "🇨🇳 CN", "CN": "🇨🇳 CN",
        "Russia": "🇷🇺 RU", "RU": "🇷🇺 RU",
        "Italy": "🇮🇹 IT", "IT": "🇮🇹 IT",
        "Spain": "🇪🇸 ES", "ES": "🇪🇸 ES",
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
