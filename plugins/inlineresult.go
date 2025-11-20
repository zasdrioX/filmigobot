// (c) Jisin0
// Handle the chosen_inline_result event.

package plugins

import (
	"fmt"
	"strings"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
)

// Edit message after the result is chosen.
func InlineResultHandler(bot *gotgbot.Bot, ctx *ext.Context) error {
	var (
		update = ctx.ChosenInlineResult
		data   = update.ResultId
	)

	if data == notAvailable {
		return nil
	}

	args := strings.Split(data, "_")
	if len(args) < 2 {
		fmt.Println("bad resultid on choseninlineresult : " + data)
		return nil
	}

	var (
		method = args[0]
		id     = args[1]
	)

	posterURL, caption, buttons, err := getChosenResult(method, id)
	if err != nil {
		fmt.Println(err)
		return nil
	}

	messageText := fmt.Sprintf("<a href=\"%s\">&#8203;</a>%s", posterURL, caption)

	// --- FIX: Change from EditMessageMedia to EditMessageText ---
	_, _, err = bot.EditMessageText(
		messageText,
		&gotgbot.EditMessageTextOpts{
			InlineMessageId: update.InlineMessageId,
			ParseMode:       gotgbot.ParseModeHTML,
			ReplyMarkup:     gotgbot.InlineKeyboardMarkup{InlineKeyboard: buttons},
			LinkPreviewOptions: &gotgbot.LinkPreviewOptions{
				IsDisabled:      false,
				ShowAboveText: true, // <-- THIS IS THE FIX
				Url:             posterURL,
			},
		},
	)
	if err != nil {
		fmt.Println(err)
	}

	return nil
}

// Returns the content to edit with.
func getChosenResult(method, id string) (string, string, [][]gotgbot.InlineKeyboardButton, error) {
	switch method {
	// case searchMethodJW:
	// 	return GetJWTitle(id)
	case searchMethodIMDb:
		return GetIMDbTitle(id)
	case searchMethodOMDb:
		return GetOMDbTitle(id)
	default:
		fmt.Println("unknown method on choseninlineresult : " + method)
		return GetOMDbTitle(id)
	}
}

// CbOpen handles callbacks from open_ buttons in search results.
func CbOpen(bot *gotgbot.Bot, ctx *ext.Context) error {
	update := ctx.CallbackQuery

	split := strings.Split(update.Data, "_")
	if len(split) < 3 {
		update.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{Text: "Bad Callback Data !", ShowAlert: true})
		return ext.EndGroups
	}

	var (
		method = split[1]
		id     = split[2]
		
		posterURL string
		caption   string
		buttons   [][]gotgbot.InlineKeyboardButton
		err       error
	)

	switch method {
	// case searchMethodJW:
	// 	photo, buttons, err = GetJWTitle(id)
	case searchMethodIMDb:
		posterURL, caption, buttons, err = GetIMDbTitle(id)
	case searchMethodOMDb:
		posterURL, caption, buttons, err = GetOMDbTitle(id)
	default:
		fmt.Println("unknown method on cbopen: " + method)
		posterURL, caption, buttons, err = GetOMDbTitle(id)
	}

	if err != nil {
		fmt.Printf("cbopen: %v", err)
		update.Answer(bot, &gotgbot.AnswerCallbackQueryOpts{Text: "I Couldn't Fetch Data on That Movie 🤧\nPlease Try Again Later or Contact Admins !", ShowAlert: true})
		return nil
	}

	messageText := fmt.Sprintf("<a href=\"%s\">&#8203;</a>%s", posterURL, caption)

	// --- FIX: Change from EditMedia to EditMessageText ---
	_, _, err = update.Message.EditText(bot, messageText, &gotgbot.EditMessageTextOpts{
		ParseMode:   gotgbot.ParseModeHTML,
		ReplyMarkup: gotgbot.InlineKeyboardMarkup{InlineKeyboard: buttons},
		LinkPreviewOptions: &gotgbot.LinkPreviewOptions{
			IsDisabled:      false,
			ShowAboveText: true, // <-- THIS IS THE FIX
			Url:             posterURL,
		},
	})
	if err != nil {
		fmt.Printf("cbopen: %v", err)
	}

	return nil
}
