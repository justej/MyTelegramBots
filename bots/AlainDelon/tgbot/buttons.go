package tgbot

import (
	"botfarm/bot"
	"botfarm/bots/AlainDelon/db"
	"fmt"
	"strconv"
	"strings"

	tg "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const (
	cbqAdd       = "cbqAdd"
	cbqDel       = "cbqDel"
	cbqRate      = "cbqRate"
	cbqUnrate    = "cbqUnrate"
	cbqAmazeMe   = "cbqAmazeMe"
	cbqFind      = "cbqFind"
	cbqWatched   = "cbqWatched"
	cbqUnwatched = "cbqUnwatched"
	cbqAll       = "cbqAll"
	cbqMy        = "cbqMy"
	cbqTop       = "cbqTop"
	cbqLast      = "cbqLast"
	cbqHelp      = "cbqHelp"

	cbqBack    = "cbqBack"
	cbqExecute = "cbqExecute"
	cbqSkip    = "cbqSkip"

	cbqTitle    = "cbqTitle"
	cbqAltTitle = "cbqAltTitle"
	cbqYear     = "cbqYear"

	cbq1Star = "1star"
	cbq2Star = "2stars"
	cbq3Star = "3stars"
	cbq4Star = "4stars"
	cbq5Star = "5stars"
)

var (
	mainKeyboard = tg.NewInlineKeyboardMarkup(
		tg.NewInlineKeyboardRow(
			tg.NewInlineKeyboardButtonData("Add", cbqAdd),
			tg.NewInlineKeyboardButtonData("Delete", cbqDel),
		),
		tg.NewInlineKeyboardRow(
			tg.NewInlineKeyboardButtonData("Rate", cbqRate),
			tg.NewInlineKeyboardButtonData("Unrate", cbqUnrate),
		),
		tg.NewInlineKeyboardRow(
			tg.NewInlineKeyboardButtonData("List watched", cbqWatched),
			tg.NewInlineKeyboardButtonData("List unwatched", cbqUnwatched),
		),
		tg.NewInlineKeyboardRow(
			tg.NewInlineKeyboardButtonData("List all", cbqAll),
			tg.NewInlineKeyboardButtonData("List my", cbqMy),
		),
		tg.NewInlineKeyboardRow(
			tg.NewInlineKeyboardButtonData("List top 10", cbqTop),
			tg.NewInlineKeyboardButtonData("List latest 10", cbqLast),
		),
		tg.NewInlineKeyboardRow(
			tg.NewInlineKeyboardButtonData("Amaze me", cbqAmazeMe),
			tg.NewInlineKeyboardButtonData("Find", cbqFind),
		),
		tg.NewInlineKeyboardRow(
			tg.NewInlineKeyboardButtonData("I need help", cbqHelp),
		),
	)

	keyboardSkip = tg.NewInlineKeyboardMarkup(
		tg.NewInlineKeyboardRow(
			tg.NewInlineKeyboardButtonData("Back", cbqBack),
		),
		tg.NewInlineKeyboardRow(
			tg.NewInlineKeyboardButtonData("Skip", cbqSkip),
		),
	)

	keyboardAdd = tg.NewInlineKeyboardMarkup(
		tg.NewInlineKeyboardRow(
			tg.NewInlineKeyboardButtonData("Back", cbqBack),
		),
		tg.NewInlineKeyboardRow(
			tg.NewInlineKeyboardButtonData("Add movie", cbqExecute),
		),
	)

	keyboardBack = tg.NewInlineKeyboardMarkup(
		tg.NewInlineKeyboardRow(
			tg.NewInlineKeyboardButtonData("Back", cbqBack),
		),
	)

	keyboardRateOptions = tg.NewInlineKeyboardMarkup(
		tg.NewInlineKeyboardRow(
			tg.NewInlineKeyboardButtonData("Back", cbqBack),
		),
		tg.NewInlineKeyboardRow(
			tg.NewInlineKeyboardButtonData("⭐", cbq1Star),
			tg.NewInlineKeyboardButtonData("⭐⭐", cbq2Star),
			tg.NewInlineKeyboardButtonData("⭐⭐⭐", cbq3Star),
		),
		tg.NewInlineKeyboardRow(
			tg.NewInlineKeyboardButtonData("⭐⭐⭐⭐", cbq4Star),
			tg.NewInlineKeyboardButtonData("⭐⭐⭐⭐⭐", cbq5Star),
		),
	)
)

func HandleCallbackQuery(ctx *bot.Context, upd *tg.Update) {
	cbq := upd.CallbackQuery
	usr := cbq.From.ID
	cht := cbq.Message.Chat.ID
	mID := cbq.Message.MessageID

	if states[usr] == nil {
		states[usr] = &state{stage: stageIdle, mainMessageID: mID}
	}

	state := states[usr]

	switch cbq.Data {
	case cbqBack:
		replaceMessage(ctx, usr, cht, mID, mainMessage, &mainKeyboard, stageIdle)

	case cbqAdd:
		if state.stage != stageIdle {
			fixState(ctx, cbq)
			return
		}
		states[usr].movie = db.Movie{}
		replaceMessage(ctx, usr, cht, mID, "Enter the title of the movie", &keyboardBack, stageTitle)

	case cbqSkip:
		if state.stage != stageAltTitle && state.stage != stageYear {
			fixState(ctx, cbq)
			return
		}

		var keyboard *tg.InlineKeyboardMarkup
		var prefix string
		var stage stage
		switch state.stage {
		case stageAltTitle:
			stage = stageYear
			keyboard = &keyboardSkip
			prefix = "Maybe you know the year of release?\n\n"

		case stageYear:
			db.AddMovie(ctx, usr, &state.movie)
			stage = stageIdle
			keyboard = &mainKeyboard
			prefix = mainMessage
		}

		replaceMessage(ctx, usr, cht, state.mainMessageID, prefix, keyboard, stage)

	case cbqDel:
		if state.stage != stageIdle {
			fixState(ctx, cbq)
			return
		}
		lst, _ := db.ListAllMovies(ctx, usr)
		keyboard := makeChooseMovieKeyboard(ctx, lst)
		replaceMessage(ctx, usr, cht, mID, "Pick the movie to delete", &keyboard, stageChooseDel)

	case cbqRate:
		if state.stage != stageIdle {
			fixState(ctx, cbq)
			return
		}
		lst, _ := db.ListAllMovies(ctx, usr)
		keyboard := makeChooseMovieKeyboard(ctx, lst)
		replaceMessage(ctx, usr, cht, mID, "Which one do you want to rate?", &keyboard, stageChooseRate)

	case cbqUnrate:
		if state.stage != stageIdle {
			fixState(ctx, cbq)
			return
		}
		lst, _ := db.ListAllMovies(ctx, usr)
		keyboard := makeChooseMovieKeyboard(ctx, lst)
		replaceMessage(ctx, usr, cht, mID, "Unrate? Which one?", &keyboard, stageChooseUnrate)

	case cbqAmazeMe:
		if state.stage != stageIdle {
			fixState(ctx, cbq)
			return
		}

		mv, err := db.RandomMovie(ctx, usr)
		if err != nil {
			return
		}

		movieStr := "Random movie:\n\n" + formatMovie(mv, false)
		replaceMessage(ctx, usr, cht, mID, movieStr, &keyboardBack, stageAmazeMe)

	case cbqWatched:
		lst, _ := db.ListSeenMovies(ctx, usr)
		replaceMessage(ctx, usr, cht, mID, joinMovies(lst, false, "List of watched movies\n\n"), &keyboardBack, stageList)

	case cbqUnwatched:
		lst, _ := db.ListUnseenMovies(ctx, usr)
		replaceMessage(ctx, usr, cht, mID, joinMovies(lst, false, "List of unwatched movies\n\n"), &keyboardBack, stageList)

	case cbqAll:
		lst, _ := db.ListAllMovies(ctx, usr)
		replaceMessage(ctx, usr, cht, mID, joinMovies(lst, false, "All movies on the list\n\n"), &keyboardBack, stageList)

	case cbqMy:
		lst, _ := db.ListMyMovies(ctx, usr)
		replaceMessage(ctx, usr, cht, mID, joinMovies(lst, false, "These are movies you added. The rates are also yours\n\n"), &keyboardBack, stageList)

	case cbqTop:
		lst, _ := db.ListTopMovies(ctx, usr)
		replaceMessage(ctx, usr, cht, mID, joinMovies(lst, false, "Top 10 movies by rate\n\n"), &keyboardBack, stageList)

	case cbqLast:
		lst, _ := db.ListLatestMovies(ctx, usr)
		replaceMessage(ctx, usr, cht, mID, joinMovies(lst, false, "10 latest movies added\n\n"), &keyboardBack, stageList)

	case cbqHelp:
		replaceMessage(ctx, usr, cht, mID, helpMessage, &keyboardBack, stageHelp)
	case cbqTitle:
		if state.stage != stageAdd {
			fixState(ctx, cbq)
			return
		}
		replaceMessage(ctx, usr, cht, mID, "What's the name of the movie?", &keyboardBack, stageTitle)

	case cbqAltTitle:
		if state.stage != stageAdd {
			fixState(ctx, cbq)
			return
		}
		replaceMessage(ctx, usr, cht, mID, "Oh, the movie has an alternative name? What's that?", &keyboardBack, stageAltTitle)

	case cbqYear:
		if state.stage != stageAdd {
			fixState(ctx, cbq)
			return
		}
		replaceMessage(ctx, usr, cht, mID, "Do you know the issue year?", &keyboardBack, stageYear)

	case cbq1Star:
		fallthrough
	case cbq2Star:
		fallthrough
	case cbq3Star:
		fallthrough
	case cbq4Star:
		fallthrough
	case cbq5Star:
		r, err := strconv.Atoi(cbq.Data[0:1])
		if err != nil {
			ctx.Logger.Errorw("impossible came true", "err", err)
		} else {
			state.movie.Rating = float32(r)
			db.RateMovie(ctx, usr, state.movie.ID, int(state.movie.Rating))
		}

		replaceMessage(ctx, usr, cht, mID, mainMessage, &mainKeyboard, stageIdle)

	default:
		// you only can get here when you chose movie
		txt := cbq.Data
		keyboard := &mainKeyboard
		prefix := mainMessage
		state.movie = *movieByID(ctx, usr, txt)

		switch state.stage {
		case stageChooseRate:
			keyboard = &keyboardRateOptions
			state.stage = stageRate
			prefix = fmt.Sprintf("How many starts for %q?", state.movie.Title)

		case stageChooseUnrate:
			db.UnrateMovie(ctx, usr, state.movie.ID)
			state.stage = stageIdle

		case stageChooseDel:
			db.DelMovie(ctx, usr, state.movie.ID)
			state.stage = stageIdle
		}

		states[usr] = state
		replaceMessage(ctx, usr, cht, state.mainMessageID, prefix, keyboard, state.stage)
	}
}

func makeChooseMovieKeyboard(ctx *bot.Context, lst []*db.Movie) tg.InlineKeyboardMarkup {
	rows := make([][]tg.InlineKeyboardButton, len(lst)+1)
	rows[0] = tg.NewInlineKeyboardRow(keyboardBack.InlineKeyboard[0][0])
	for i, mv := range lst {
		text := formatMovie(mv, false)
		rows[i+1] = tg.NewInlineKeyboardRow(tg.NewInlineKeyboardButtonData(text, strconv.Itoa(mv.ID)))
	}
	keyboard := tg.NewInlineKeyboardMarkup(rows...)
	return keyboard
}

func alertIncompleteData(ctx *bot.Context, s string) {
	// TODO: show alert
	ctx.Logger.Warn("movie doesn't have a title")
}

func replaceMessage(ctx *bot.Context, usr, cht int64, msgID int, msg string, kbMarkup *tg.InlineKeyboardMarkup, stg stage) bool {
	var upd tg.EditMessageTextConfig
	if kbMarkup == nil {
		upd = tg.NewEditMessageText(cht, msgID, msg)
	} else {
		upd = tg.NewEditMessageTextAndMarkup(cht, msgID, msg, *kbMarkup)
	}

	if _, err := ctx.Bot.Send(upd); err != nil {
		ctx.Logger.Errorw("failed updating message", "err", err)
		return false
	}

	state := states[usr]
	state.stage = stg
	state.mainMessageID = msgID
	states[usr] = state

	return true
}

func formatMovieWithHeaders(mv *db.Movie, showRate bool) string {
	fmtStr := []string{}
	args := []any{}

	if len(mv.Title) > 0 {
		fmtStr = append(fmtStr, "Title: %s\n")
		args = append(args, mv.Title)
	}
	if len(mv.AltTitle) > 0 {
		fmtStr = append(fmtStr, "Alternative title:%s\n")
		args = append(args, mv.AltTitle)
	}

	if mv.Year > 0 {
		fmtStr = append(fmtStr, "Year: %d\n")
		args = append(args, mv.Year)
	}

	if showRate && mv.Rating > 0 {
		fmtStr = append(fmtStr, "Rate: %.0f ⭐")
		args = append(args, mv.Rating)
	}

	return fmt.Sprintf(strings.Join(fmtStr, ""), args...)
}

func fixState(ctx *bot.Context, cbq *tg.CallbackQuery) {
	usr := cbq.From.ID
	cht := cbq.Message.From.ID

	replaceMessage(ctx, usr, cht, cbq.Message.MessageID, mainMessage, &mainKeyboard, stageIdle)
}
