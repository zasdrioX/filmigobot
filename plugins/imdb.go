// (c) Jisin0
// Functions and types to process imdb results.

package plugins

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/Jisin0/filmigo/imdb"
	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
)

var (
	imdbClient       = imdb.NewClient()
	searchMethodIMDb = "imdb"
)

const (
	imdbLogo     = "https://telegra.ph/file/1720930421ae2b00d9bab.jpg"
	imdbBanner   = "https://telegra.ph/file/2dd6f7c9ebfb237db4826.jpg"
	imdbHomepage = "https://imdb.com"
	// unofficialAPI const is removed (it's in omdb.go)
)

// unofficialSearchResult struct is removed (it's in omdb.go)


// ImdbInlineSearch now calls the working OMDbInlineSearch
func IMDbInlineSearch(query string) []gotgbot.InlineQueryResult {
	// This now calls the function in omdb.go
	results := OMDbInlineSearch(query)
	
	// We just need to change the ID prefix from "omdb_" to "imdb_"
	for i := range results {
		if photoResult, ok := results[i].(gotgbot.InlineQueryResultArticle); ok {
			photoResult.Id = strings.Replace(photoResult.Id, searchMethodOMDb, searchMethodIMDb, 1)
			
			// Also update the callback data
			if photoResult.ReplyMarkup != nil && len(photoResult.ReplyMarkup.InlineKeyboard) > 0 {
				callbackData := photoResult.ReplyMarkup.InlineKeyboard[0][0].CallbackData
				photoResult.ReplyMarkup.InlineKeyboard[0][0].CallbackData = strings.Replace(callbackData, searchMethodOMDb, searchMethodIMDb, 1)
			}
			results[i] = photoResult
		}
	}
	return results
}

// GetIMDbTitle is now just a wrapper for GetOMDbTitle
func GetIMDbTitle(id string) (string, string, [][]gotgbot.InlineKeyboardButton, error) {
	// This now calls the function in omdb.go
	return GetOMDbTitle(id)
}

// IMDbCommand handles the /imdb command.
func IMDbCommand(bot *gotgbot.Bot, ctx *ext.Context) error {
	update := ctx.EffectiveMessage

	split := strings.SplitN(update.GetText(), " ", 2)
	if len(split) < 2 {
		text := "<i>Please provide a search query or movie id along with this command !\nFor Example:</i>\n  <code>/imdb Inception</code>\n  <code>/imdb tt1375666</code>"
		update.Reply(bot, text, &gotgbot.SendMessageOpts{ParseMode: gotgbot.ParseModeHTML})
		return nil
	}

	input := split[1]

	var (
		messageText string
		buttons     [][]gotgbot.InlineKeyboardButton
		err         error
		posterURL   string = imdbBanner // Default to banner
	)

	if id := regexp.MustCompile(`tt\d+`).FindString(input); id != "" {
		// --- FIX: Use link preview method ---
		imgURL, caption, btns, e := GetOMDbTitle(id)
		if e != nil {
			err = e
		} else {
			// Invisible link hack to force poster preview
			posterURL = imgURL // Save the real poster URL
			messageText = fmt.Sprintf("<a href=\"%s\">&#8203;</a>%s", posterURL, caption)
			buttons = btns
		}
	} else {
		// --- FIX: Use unofficial API for search ---
		apiURL := fmt.Sprintf("%s?q=%s", unofficialAPI, url.QueryEscape(input))
		resp, e := http.Get(apiURL)
		if e != nil {
			err = e
		} else {
			defer resp.Body.Close()
			body, e := io.ReadAll(resp.Body)
			if e != nil {
				err = e
			} else {
				var searchData unofficialSearchResponse
				if e := json.Unmarshal(body, &searchData); e != nil {
					err = e
				} else if !searchData.Ok || len(searchData.Description) < 1 {
					err = errors.New("No results found")
				} else {
					// This is a search result, so we just send text + buttons
					posterURL = imdbBanner
					messageText = fmt.Sprintf("<a href=\"%s\">&#8203;</a><i>👋 Hey <tg-spoiler>%s</tg-spoiler> I've got %d Results for you 👇</i>", posterURL, mention(ctx.EffectiveUser), len(searchData.Description))
					for _, r := range searchData.Description {
						buttons = append(buttons, []gotgbot.InlineKeyboardButton{{Text: fmt.Sprintf("%s (%d)", r.Title, r.Year), CallbackData: fmt.Sprintf("open_%s_%s", searchMethodIMDb, r.ImdbID)}})
					}
				}
			}
		}
	}

	if err != nil {
		posterURL = imdbBanner // Use banner for errors
		messageText = fmt.Sprintf("<a href=\"%s\">&#8203;</a><i>I'm Sorry %s I Couldn't find Anything for <code>%s</code> 🤧</i>", posterURL, mention(ctx.EffectiveUser), input)
		buttons = [][]gotgbot.InlineKeyboardButton{{{Text: "Search On Google 🔎", Url: fmt.Sprintf("https://google.com/search?q=%s", url.QueryEscape(input))}}}
	}
	
	// --- FIX: Send as a Text Message with ShowAboveText ---
	_, err = bot.SendMessage(ctx.EffectiveChat.Id, messageText, &gotgbot.SendMessageOpts{
		ParseMode:   gotgbot.ParseModeHTML,
		ReplyMarkup: gotgbot.InlineKeyboardMarkup{InlineKeyboard: buttons},
		LinkPreviewOptions: &gotgbot.LinkPreviewOptions{
			IsDisabled:      false,
			ShowAboveText: true, // <-- THIS IS THE FIX
			Url:             posterURL,
		},
	})
	if err != nil {
		fmt.Printf("imdbcommand: %v", err)
	}

	return ext.EndGroups
}
