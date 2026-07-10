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
	apiFallback = "https://api.imdbapi.dev"                   // Used for Search & Fallback Details
	apiTMDB     = "https://api.themoviedb.org/3"
	tmdbKey     = "1b4ba621cf09ae9752dd659e6e55307b"

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
	Rating float64 // --- ADDED RATINGS ---
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
// 3. FALLBACK API STRUCTS (Enhanced)
// ==========================================
type fallbackSearchRes struct {
	Results []struct {
		ID           string `json:"id"`
		PrimaryTitle string `json:"primaryTitle"`
		StartYear    int    `json:"startYear"`
		PrimaryImage *struct{ URL string `json:"url"` } `json:"primaryImage"`
		Type         string `json:"type"`
		Rating       *struct {                          // --- ADDED RATINGS EXTRACTION ---
			AggregateRating float64 `json:"aggregateRating"`
		} `json:"rating"`
	} `json:"titles"` // Uses titles mapping from imdbapi.dev
}

type fallbackDetailData struct {
	ID             string   `json:"id"`
	Type           string   `json:"type"`
	PrimaryTitle   string   `json:"primaryTitle"`
	StartYear      int      `json:"startYear"`
	EndYear        int      `json:"endYear"`
	RuntimeSeconds int      `json:"runtimeSeconds"`
	Plot           string   `json:"plot"`
	Genres         []string `json:"genres"`
	Rating         *struct {
		AggregateRating float64 `json:"aggregateRating"`
		VoteCount       int     `json:"voteCount"`
	} `json:"rating"`
	PrimaryImage *struct{ URL string `json:"url"` } `json:"primaryImage"`
	ReleaseDate  *string                            `json:"releaseDate"`
	Metacritic   *struct{ Score int `json:"score"` } `json:"metacritic"`
	Directors    []struct{ ID string `json:"id"`; Name string `json:"displayName"` } `json:"directors"`
	Writers      []struct{ ID string `json:"id"`; Name string `json:"displayName"` } `json:"writers"`
	Stars        []struct{ ID string `json:"id"`; Name string `json:"displayName"` } `json:"stars"`
	Interests    []struct{ Name string `json:"name"` }                               `json:"interests"`
	Countries    []struct{ Name string `json:"name"` }                               `json:"originCountries"`
	Languages    []struct{ Name string `json:"name"` }                               `json:"spokenLanguages"`
}
type fallbackCredits struct {
	Cast []struct {
		Name       struct{ ID string `json:"id"`; DisplayName string `json:"displayName"` } `json:"name"`
		Characters []struct{ Name string `json:"name"` }                                    `json:"characters"`
	} `json:"cast"`
}
type fallbackAKA struct {
	Titles []struct{ Title string `json:"title"` } `json:"titles"`
}

// --- NEW: STRUCTS FOR TMDB ---
type tmdbFindRes struct {
	MovieResults []struct{ ID int `json:"id"` } `json:"movie_results"`
	TVResults    []struct{ ID int `json:"id"` } `json:"tv_results"`
}
type tmdbDetailRes struct {
	Title         string `json:"title"`          
	OriginalTitle string `json:"original_title"` 
	PosterPath    string `json:"poster_path"`
	BackdropPath  string `json:"backdrop_path"`
	Tagline       string `json:"tagline"`
	ReleaseDate   string `json:"release_date"`   
	FirstAirDate  string `json:"first_air_date"` 
	OriginCountry []string `json:"origin_country"` 
	ProductionCountries []struct{ Name string `json:"name"` } `json:"production_countries"` 
	
	Credits       struct {
		Cast []struct {
			ID        int    `json:"id"` 
			Name      string `json:"name"`
			Character string `json:"character"`
		} `json:"cast"`
		Crew []struct {
			ID         int    `json:"id"` 
			Name       string `json:"name"`
			Job        string `json:"job"`
			Department string `json:"department"`
		} `json:"crew"`
	} `json:"credits"`
	AggregateCredits struct {
		Cast []struct {
			ID    int    `json:"id"`
			Name  string `json:"name"`
			Roles []struct {
				Character string `json:"character"`
			} `json:"roles"`
		} `json:"cast"`
	} `json:"aggregate_credits"`
	AlternativeTitles struct {
		Titles []struct {
			Title string `json:"title"`
			Iso   string `json:"iso_3166_1"`
		} `json:"titles"`
	} `json:"alternative_titles"`
	NumSeasons  int `json:"number_of_seasons"`
	NumEpisodes int `json:"number_of_episodes"`
	CreatedBy   []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"created_by"`
	Budget              int64 `json:"budget"`
	Revenue             int64 `json:"revenue"`
	ProductionCompanies []struct {
		Name string `json:"name"`
	} `json:"production_companies"`
}

// --- STRUCT for OMDb Fill-in Data ---
type omdbFillData struct {
	Released     string `json:"Released"`
	Awards       string `json:"Awards"`
	TotalSeasons string `json:"totalSeasons"`
	Country      string `json:"Country"`
}

// ==========================================
// 4. UNIFIED SEARCH FUNCTION
// ==========================================

func SearchOMDb(query string) ([]UniversalSearchResult, error) {
	// --- NEW CODE: EXTRACT ID FROM URL ---
	if strings.Contains(query, "imdb.com/title/tt") || (strings.HasPrefix(query, "tt") && len(query) >= 7) {
		start := strings.Index(query, "tt")
		idPart := query[start:]
		end := strings.IndexAny(idPart, "/? \n\t")
		if end == -1 {
			end = len(idPart)
		}
		imdbID := idPart[:end]

		apiURL := fmt.Sprintf("%s/titles/%s", apiFallback, imdbID)
		if resp, err := http.Get(apiURL); err == nil {
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			var directData struct {
				ID           string `json:"id"`
				PrimaryTitle string `json:"primaryTitle"`
				StartYear    int    `json:"startYear"`
				PrimaryImage *struct{ URL string `json:"url"` } `json:"primaryImage"`
				Type         string `json:"type"`
				Rating       *struct{ AggregateRating float64 `json:"aggregateRating"` } `json:"rating"`
			}
			if json.Unmarshal(body, &directData) == nil && directData.ID != "" {
				poster := ""
				if directData.PrimaryImage != nil { poster = directData.PrimaryImage.URL }
				typeTag := ""
				if directData.Type != "" { typeTag = strings.Title(directData.Type) }
				rating := 0.0
				if directData.Rating != nil { rating = directData.Rating.AggregateRating }
				return []UniversalSearchResult{{ID: directData.ID, Title: directData.PrimaryTitle, Year: directData.StartYear, Poster: poster, Type: typeTag, Rating: rating}}, nil
			}
		}
	}
	// --- END OF NEW CODE ---

	// EXCLUSIVE INLINE SEARCH: imdbapi.dev
	apiURL := fmt.Sprintf("%s/search/titles?query=%s", apiFallback, url.QueryEscape(query))
	if resp, err := http.Get(apiURL); err == nil {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		var fData fallbackSearchRes
		if json.Unmarshal(body, &fData) == nil && len(fData.Results) > 0 {
			var results []UniversalSearchResult
			for _, item := range fData.Results {
				poster := ""
				if item.PrimaryImage != nil {
					poster = item.PrimaryImage.URL
				}
				
				typeTag := ""
				if item.Type != "" { typeTag = strings.Title(item.Type) }

				// --- RATINGS EXTRACTION ---
				rating := 0.0
				if item.Rating != nil {
					rating = item.Rating.AggregateRating
				}

				results = append(results, UniversalSearchResult{
					ID: item.ID, Title: item.PrimaryTitle, Year: item.StartYear, Poster: poster, Type: typeTag, Rating: rating,
				})
			}
			return results, nil
		}
	}

	return nil, errors.New("No results found via imdbapi.dev")
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

		// --- FIX: Title and Subtext formatting ---
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

	// 1. ImdbApiDev (Base)
	resp, err := http.Get(fmt.Sprintf("%s/titles/%s", apiFallback, id))
	if err != nil {
		return "", "", buttons, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	type fbDetail struct {
		PrimaryTitle    string                                                      `json:"primaryTitle"`
		StartYear       int                                                         `json:"startYear"`
		EndYear         int                                                         `json:"endYear"`
		Plot            string                                                      `json:"plot"`
		Type            string                                                      `json:"type"`
		PrimaryImage    *struct{ URL string `json:"url"` }                          `json:"primaryImage"`
		Rating          *struct{ AggregateRating float64 `json:"aggregateRating"`
		                        VoteCount int `json:"voteCount"` } `json:"rating"`
		Genres          []string                                                    `json:"genres"`
		RuntimeSeconds  int                                                         `json:"runtimeSeconds"`
		ReleaseDate     *string                                                     `json:"releaseDate"`
		Metacritic      *struct{ Score int `json:"score"` }                         `json:"metacritic"`
		Directors       []struct{ ID string `json:"id"`; Name string `json:"displayName"` } `json:"directors"`
		Writers         []struct{ ID string `json:"id"`; Name string `json:"displayName"` } `json:"writers"`
		Stars           []struct{ ID string `json:"id"`; Name string `json:"displayName"` } `json:"stars"`
		Interests       []struct{ Name string `json:"name"` }                               `json:"interests"`
		OriginCountries []struct{ Name string `json:"name"` }                               `json:"originCountries"`
		SpokenLanguages []struct{ Name string `json:"name"` }                               `json:"spokenLanguages"`
	}
	var t fbDetail
	if json.Unmarshal(body, &t) != nil {
		return "", "", buttons, errors.New("Fallback parse error")
	}

	// 2. Parallel Fetch: AKAs, OMDb, TMDB
	var credits fallbackCredits
	var akas fallbackAKA
	var omdbFill omdbFillData
	var tmdbDetails tmdbDetailRes
	var tmdbFound bool
	var wg sync.WaitGroup

	wg.Add(3)

	// A. AKAs & Credits (from imdbapi.dev)
	go func() {
		defer wg.Done()
		if r, e := http.Get(fmt.Sprintf("%s/titles/%s/credits", apiFallback, id)); e == nil {
			defer r.Body.Close()
			b, _ := io.ReadAll(r.Body)
			json.Unmarshal(b, &credits)
		}
		if r, e := http.Get(fmt.Sprintf("%s/titles/%s/akas", apiFallback, id)); e == nil {
			defer r.Body.Close()
			b, _ := io.ReadAll(r.Body)
			json.Unmarshal(b, &akas)
		}
	}()
	// B. OMDb
	go func() {
		defer wg.Done()
		if r, e := http.Get(fmt.Sprintf("https://www.omdbapi.com/?i=%s&apikey=%s", id, OmdbApiKey)); e == nil {
			defer r.Body.Close()
			json.NewDecoder(r.Body).Decode(&omdbFill)
		}
	}()
	// C. TMDB (Find -> Details)
	go func() {
		defer wg.Done()
		findURL := fmt.Sprintf("%s/find/%s?api_key=%s&external_source=imdb_id", apiTMDB, id, tmdbKey)
		if r, e := http.Get(findURL); e == nil {
			defer r.Body.Close()
			b, _ := io.ReadAll(r.Body)
			var findRes tmdbFindRes
			if json.Unmarshal(b, &findRes) == nil {
				var tmdbID int
				var mediaType string
				if len(findRes.MovieResults) > 0 {
					tmdbID = findRes.MovieResults[0].ID
					mediaType = "movie"
				} else if len(findRes.TVResults) > 0 {
					tmdbID = findRes.TVResults[0].ID
					mediaType = "tv"
				}

				if tmdbID > 0 {
					// --- FIX: append_to_response adjusted for series ---
					appendQuery := "credits,release_dates,content_ratings,alternative_titles"
					if mediaType == "tv" {
						appendQuery = "aggregate_credits,content_ratings,alternative_titles"
					}
					
					detailURL := fmt.Sprintf("%s/%s/%d?api_key=%s&append_to_response=%s", apiTMDB, mediaType, tmdbID, tmdbKey, appendQuery)
					if r2, e2 := http.Get(detailURL); e2 == nil {
						defer r2.Body.Close()
						b2, _ := io.ReadAll(r2.Body)
						if json.Unmarshal(b2, &tmdbDetails) == nil {
							tmdbFound = true
						}
					}
				}
			}
		}
	}()
	wg.Wait()

	// --- BUILD CAPTION ---
	var sb strings.Builder
	isSeries := (t.Type == "tvSeries" || t.Type == "tvMiniSeries")
	typeStr := strings.Title(t.Type)
	if t.Type == "tvSeries" {
		typeStr = "TV Series"
	}
	
	// --- FIX: Use TMDB Title if available, else PrimaryTitle ---
	mainTitle := t.PrimaryTitle
	if tmdbFound && tmdbDetails.Title != "" {
		mainTitle = tmdbDetails.Title
	}

	var yearStr string
	if isSeries && t.EndYear > 0 {
		yearStr = fmt.Sprintf("[%d-%d]", t.StartYear, t.EndYear)
	} else if isSeries {
		yearStr = fmt.Sprintf("[%d-Present]", t.StartYear)
	} else {
		yearStr = fmt.Sprintf("[%d]", t.StartYear)
	}
	sb.WriteString(fmt.Sprintf("<i>%s: </i><b>%s %s</b> | <a href=\"%s\">IMDb Link</a>\n", typeStr, mainTitle, yearStr, omdbHomepage+"/title/"+id))
	
	// Original Title Logic
	orgTitle := ""
	if tmdbFound && tmdbDetails.OriginalTitle != "" {
		orgTitle = tmdbDetails.OriginalTitle
	}
	if orgTitle != "" && orgTitle != mainTitle {
		sb.WriteString(fmt.Sprintf("<i>(Original Title: %s)</i>\n", orgTitle))
	}

	// AKA Logic (Smart US/IN Fallback)
	akaStr := ""
	if tmdbFound && len(tmdbDetails.AlternativeTitles.Titles) > 0 {
		isUS := false
		if len(t.OriginCountries) > 0 {
			for _, c := range t.OriginCountries {
				if c.Name == "United States" { isUS = true; break }
			}
		}
		target := "US"
		if isUS { target = "IN" } // If US movie, prefer India AKA
		
		// 1. Try Target Region
		for _, alt := range tmdbDetails.AlternativeTitles.Titles {
			if alt.Iso == target && alt.Title != mainTitle {
				akaStr = alt.Title
				break
			}
		}
		// 2. Try any different title
		if akaStr == "" {
			for _, alt := range tmdbDetails.AlternativeTitles.Titles {
				if alt.Title != mainTitle {
					akaStr = alt.Title
					break
				}
			}
		}
	}
	if akaStr == "" && len(akas.Titles) > 0 {
		akaStr = akas.Titles[0].Title
	}
	if akaStr != "" && akaStr != mainTitle {
		sb.WriteString(fmt.Sprintf("<i>(AKA %s)</i>\n", akaStr))
	}

	// --- SEASONS (Prefer TMDB) ---
	if isSeries {
		if tmdbFound && tmdbDetails.NumSeasons > 0 {
			sb.WriteString(fmt.Sprintf("<b>%d Seasons (%d Episodes)</b>\n", tmdbDetails.NumSeasons, tmdbDetails.NumEpisodes))
		} else if omdbFill.TotalSeasons != "" && omdbFill.TotalSeasons != notAvailable {
			sb.WriteString(fmt.Sprintf("<b>%s Seasons</b>\n", omdbFill.TotalSeasons))
		}
	}

	if t.RuntimeSeconds > 0 {
		h := t.RuntimeSeconds / 3600
		m := (t.RuntimeSeconds % 3600) / 60
		dur := fmt.Sprintf("%dh %dm", h, m)
		if isSeries {
			dur += "/Episode"
		}
		sb.WriteString(fmt.Sprintf("<i>Duration: </i>%s\n", dur))
	}

	// --- RELEASE DATE ---
	var dateStr string
	if tmdbFound && (tmdbDetails.ReleaseDate != "" || tmdbDetails.FirstAirDate != "") {
		raw := tmdbDetails.ReleaseDate
		if isSeries {
			raw = tmdbDetails.FirstAirDate
		}
		if parsed, err := time.Parse("2006-01-02", raw); err == nil {
			dateStr = parsed.Format("02 January 2006")
		}
	} else if omdbFill.Released != "" && omdbFill.Released != notAvailable {
		dateStr = omdbFill.Released
	} else if t.ReleaseDate != nil {
		if parsed, err := time.Parse("2006-01-02", *t.ReleaseDate); err == nil {
			dateStr = parsed.Format("02 January 2006")
		}
	}
	if dateStr != "" {
		flag := ""
		countryName := ""
		
		// Priority 1: TMDB Production Countries (Movies)
		if tmdbFound && len(tmdbDetails.ProductionCountries) > 0 {
			countryName = tmdbDetails.ProductionCountries[0].Name
		}
		
		// Priority 2: Fallback Origin Countries
		if countryName == "" && len(t.OriginCountries) > 0 {
			countryName = t.OriginCountries[0].Name
		}
		
		// Priority 3: OMDb Country
		if countryName == "" && omdbFill.Country != "" && omdbFill.Country != notAvailable {
			countryName = omdbFill.Country
		}
		
		if countryName != "" {
			flag = getFlag(countryName)
		}
		
		if flag != "" {
			dateStr += fmt.Sprintf(" (%s)", flag)
		}
		
		if isSeries {
			dateStr += " - For First Episode"
		}
		sb.WriteString(fmt.Sprintf("<i>Release Date: </i>%s\n", dateStr))
	}

	ratingStr := ""
	if t.Rating != nil {
		ratingStr = fmt.Sprintf("<i>Rating ⭐️ </i><b>%.1f / 10</b> (from %d votes)", t.Rating.AggregateRating, t.Rating.VoteCount)
	}
	if t.Metacritic != nil && t.Metacritic.Score > 0 {
		if ratingStr != "" {
			ratingStr += " | "
		}
		ratingStr += fmt.Sprintf("<b>Ⓜ️ %d/100</b>", t.Metacritic.Score)
	}
	if ratingStr != "" {
		sb.WriteString(ratingStr + "\n")
	}

	sb.WriteString("<blockquote>")
	genreEmojiMap := map[string]string{"Action": "💥", "Adventure": "🗺️", "Sci-Fi": "🚀", "Comedy": "🤣", "Drama": "🎭", "Romance": "🌹", "Thriller": "🔪", "Horror": "👻"}
	countryFlagMap := map[string]string{"United States": "🇺🇸", "USA": "🇺🇸", "United Kingdom": "🇬🇧", "UK": "🇬🇧", "India": "🇮🇳", "France": "🇫🇷", "Japan": "🇯🇵", "Canada": "🇨🇦", "Germany": "🇩🇪"}

	if len(t.Genres) > 0 {
		var gs []string
		for _, g := range t.Genres {
			emoji := "- "
			if e, ok := genreEmojiMap[g]; ok {
				emoji = e + " "
			}
			gs = append(gs, fmt.Sprintf("%s#%s", emoji, g))
		}
		sb.WriteString(fmt.Sprintf("<i>Genres: </i>%s\n", strings.Join(gs, " ")))
	}
	if len(t.Interests) > 0 {
		var is []string
		for _, i := range t.Interests {
			is = append(is, "#"+strings.ReplaceAll(i.Name, " ", "_"))
		}
		sb.WriteString(fmt.Sprintf("<i>Themes: </i>%s\n", strings.Join(is, " ")))
	}

	var langs, countries []string
	for _, l := range t.SpokenLanguages {
		langs = append(langs, "#"+l.Name)
	}
	for _, c := range t.OriginCountries {
		flag := ""
		if f, ok := countryFlagMap[c.Name]; ok {
			flag = f + " "
		}
		countries = append(countries, fmt.Sprintf("%s#%s", flag, strings.ReplaceAll(c.Name, " ", "_")))
	}
	if len(langs) > 0 || len(countries) > 0 {
		sb.WriteString(fmt.Sprintf("<i>Language (Country): </i>%s (%s)", strings.Join(langs, " "), strings.Join(countries, " ")))
	}
	sb.WriteString("</blockquote>\n\n")

	// --- TAGLINE (TMDB) ---
	if tmdbFound && tmdbDetails.Tagline != "" {
		sb.WriteString(fmt.Sprintf("<b>\"%s\"</b>\n\n", tmdbDetails.Tagline))
	}

	if t.Plot != "" {
		sb.WriteString(fmt.Sprintf("<blockquote><b>Story Line: </b><i>%s</i></blockquote>\n\n", t.Plot))
	}

	sb.WriteString("<blockquote>")
	// Cast & Crew
	var dirs, writers, stars, producers []string

	// Prefer TMDB Cast
	if tmdbFound {
		if isSeries {
			for _, c := range tmdbDetails.CreatedBy {
				dirs = append(dirs, link(c.Name, c.ID))
			}
			// --- FIX: TV Producers (Search in aggregate credits crew) ---
			for _, c := range tmdbDetails.Credits.Crew {
				if (c.Job == "Executive Producer" || c.Job == "Producer") && len(producers) < 5 {
					producers = append(producers, link(c.Name, c.ID))
				}
			}
		} else {
			for _, c := range tmdbDetails.Credits.Crew {
				if c.Job == "Director" {
					dirs = append(dirs, link(c.Name, c.ID))
				}
				if c.Job == "Producer" && len(producers) < 5 {
					producers = append(producers, link(c.Name, c.ID))
				}
			}
		}

		for _, c := range tmdbDetails.Credits.Crew {
			if c.Department == "Writing" {
				writers = append(writers, link(c.Name, c.ID))
			}
		}
		for i, c := range tmdbDetails.Credits.Cast {
			if i < 4 {
				stars = append(stars, link(c.Name, c.ID))
			}
		}
	}

	// Fallback to imdbapi.dev if needed
	if len(dirs) == 0 {
		for _, d := range t.Directors {
			dirs = append(dirs, link(d.Name, d.ID))
		}
	}
	if len(writers) == 0 {
		for _, w := range t.Writers {
			writers = append(writers, link(w.Name, w.ID))
		}
	}
	if len(stars) == 0 {
		for _, s := range t.Stars {
			stars = append(stars, link(s.Name, s.ID))
		}
	}

	if len(dirs) > 0 {
		sb.WriteString(fmt.Sprintf("<i><b>Directors:</b></i> %s\n", strings.Join(dirs, ", ")))
	}
	if len(writers) > 0 {
		sb.WriteString(fmt.Sprintf("<i><b>Writers:</b></i> %s\n", strings.Join(writers, ", ")))
	}
	if len(producers) > 0 {
		sb.WriteString(fmt.Sprintf("<i><b>Producers:</b></i> %s\n", strings.Join(producers, ", ")))
	}
	if len(stars) > 0 {
		sb.WriteString(fmt.Sprintf("<i><b>Stars:</b></i> %s\n", strings.Join(stars, ", ")))
	}

	// Top Cast
	var topCast []string
	if tmdbFound {
		// Switch to Aggregate Credits for TV if available
		targetCast := tmdbDetails.Credits.Cast
		if isSeries && len(tmdbDetails.AggregateCredits.Cast) > 0 {
			targetCast = nil 
			for _, ac := range tmdbDetails.AggregateCredits.Cast {
				charName := ""
				if len(ac.Roles) > 0 { charName = ac.Roles[0].Character }
				targetCast = append(targetCast, struct{ID int `json:"id"`; Name string `json:"name"`; Character string `json:"character"`}{ac.ID, ac.Name, charName})
			}
		}

		for i, c := range targetCast {
			if i >= 4 && i < topCastLimit+4 {
				topCast = append(topCast, link(c.Name, c.ID))
			}
		}
	} else {
		for _, c := range credits.Cast {
			if len(topCast) < topCastLimit {
				topCast = append(topCast, link(c.Name.DisplayName, c.Name.ID))
			} else {
				break
			}
		}
	}
	if len(topCast) > 0 {
		sb.WriteString(fmt.Sprintf("<i><b>Top Cast:</b></i> %s", strings.Join(topCast, ", ")))
	}
	sb.WriteString("</blockquote>\n\n")

	sb.WriteString("<blockquote>")
	// Awards (OMDb)
	awardsURL := fmt.Sprintf("%s/title/%s/awards", omdbHomepage, id)
	if omdbFill.Awards != "" && omdbFill.Awards != notAvailable {
		// --- FIX: Attached URL to text ---
		sb.WriteString(fmt.Sprintf("<b>Awards: </b><a href=\"%s\">%s</a>\n", awardsURL, omdbFill.Awards))
	}
	sb.WriteString(fmt.Sprintf("<b>OTT Info: </b><a href=\"https://www.justwatch.com/in/search?q=%s\">Find on JustWatch</a></blockquote>", url.QueryEscape(t.PrimaryTitle)))

	// --- FALLBACK TELEGRAPH PAGE (USING TMDB & IMDBAPI DATA) ---
	if enableTelegraph && tmdbFound {
		var nodes []tgNode
		nodes = append(nodes, tgNode{Tag: "h3", Children: []any{fmt.Sprintf("%s (%d)", t.PrimaryTitle, t.StartYear)}})
		
		posterPath := t.PrimaryImage.URL
		if tmdbDetails.PosterPath != "" {
			posterPath = "https://image.tmdb.org/t/p/original" + tmdbDetails.PosterPath
		}
		if posterPath != "" {
			nodes = append(nodes, tgNode{Tag: "figure", Children: []any{tgNode{Tag: "img", Attrs: &tgAttrs{Src: posterPath}}}})
		}

		nodes = append(nodes, makeHeader("Info"))
		nodes = append(nodes, makeRow("Type", t.Type))
		nodes = append(nodes, makeRow("Plot", t.Plot))
		if tmdbDetails.Tagline != "" {
			nodes = append(nodes, makeRow("Tagline", tmdbDetails.Tagline))
		}
		if tmdbDetails.Budget > 0 {
			nodes = append(nodes, makeRow("Budget", fmt.Sprintf("$%d", tmdbDetails.Budget)))
		}
		if tmdbDetails.Revenue > 0 {
			nodes = append(nodes, makeRow("Revenue", fmt.Sprintf("$%d", tmdbDetails.Revenue)))
		}

		// Full Cast for Telegraph
		targetCast := tmdbDetails.Credits.Cast
		if isSeries && len(tmdbDetails.AggregateCredits.Cast) > 0 {
			targetCast = nil 
			for _, ac := range tmdbDetails.AggregateCredits.Cast {
				charName := ""
				if len(ac.Roles) > 0 { charName = ac.Roles[0].Character }
				targetCast = append(targetCast, struct{ID int `json:"id"`; Name string `json:"name"`; Character string `json:"character"`}{ac.ID, ac.Name, charName})
			}
		}

		if len(targetCast) > 0 {
			nodes = append(nodes, makeHeader("Full Cast"))
			var castList []string
			for _, c := range targetCast {
				role := ""
				if c.Character != "" {
					role = " as " + c.Character
				}
				castList = append(castList, c.Name+role)
			}
			nodes = append(nodes, tgNode{Tag: "p", Children: []any{strings.Join(castList, ", ")}})
		}
		
		if len(tmdbDetails.ProductionCompanies) > 0 {
			nodes = append(nodes, makeHeader("Production Companies"))
			var comps []string
			for _, c := range tmdbDetails.ProductionCompanies {
				comps = append(comps, c.Name)
			}
			nodes = append(nodes, tgNode{Tag: "p", Children: []any{strings.Join(comps, ", ")}})
		}

		page := createTelegraphPage(t.PrimaryTitle+" Details", nodes)
		sb.WriteString(fmt.Sprintf("\n\n<a href=\"%s\">Read More...</a>", omdbHomepage+"/title/"+id))
		if page != "" {
			sb.WriteString(fmt.Sprintf(" | <a href=\"%s\">Full Details</a>", page))
		}
	} else {
		sb.WriteString(fmt.Sprintf("\n\n<a href=\"%s\">Read More...</a>", omdbHomepage+"/title/"+id))
	}

	sb.WriteString(fmt.Sprintf(" | <a href=\"%s\">Trailer</a>", fmt.Sprintf("https://www.youtube.com/results?search_query=%s", url.QueryEscape(t.PrimaryTitle+" trailer"))))

	poster := omdbBanner
	if tmdbFound && tmdbDetails.PosterPath != "" {
		poster = "https://image.tmdb.org/t/p/original" + tmdbDetails.PosterPath
	} else if t.PrimaryImage != nil {
		poster = t.PrimaryImage.URL
	}
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
