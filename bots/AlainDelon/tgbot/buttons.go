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

	cbqTitle       = "cbqTitle"
	cbqAltTitle    = "cbqAltTitle"
	cbqYear        = "cbqYear"
	cbqChooseMovie = "cbqChooseMovie"
	cbqSetRating   = "cbqSetRating"

	cbq1Star = "1"
	cbq2Star = "2"
	cbq3Star = "3"
	cbq4Star = "4"
	cbq5Star = "5"
)

const (
	prefixMovieToAdd    = "Fill out the fields (title is required, the rest is optional)\n\n"
	prefixMovieToDelete = "Movie to delete:\n\n"
	prefixMovieToRate   = "Movie to rate:\n\n"
	prefixMovieToUnrate = "Movie to unrate:\n\n"
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

	keyboardAdd = tg.NewInlineKeyboardMarkup(
		tg.NewInlineKeyboardRow(
			tg.NewInlineKeyboardButtonData("Back", cbqBack),
		),
		tg.NewInlineKeyboardRow(
			tg.NewInlineKeyboardButtonData("Set title", cbqTitle),
			tg.NewInlineKeyboardButtonData("Set year", cbqYear),
		),
		tg.NewInlineKeyboardRow(
			tg.NewInlineKeyboardButtonData("Set alternative title", cbqAltTitle),
		),
		tg.NewInlineKeyboardRow(
			tg.NewInlineKeyboardButtonData("Add movie", cbqExecute),
		),
	)

	keyboardDel = tg.NewInlineKeyboardMarkup(
		tg.NewInlineKeyboardRow(
			tg.NewInlineKeyboardButtonData("Back", cbqBack),
		),
		tg.NewInlineKeyboardRow(
			tg.NewInlineKeyboardButtonData("Choose movie", cbqChooseMovie),
		),
		tg.NewInlineKeyboardRow(
			tg.NewInlineKeyboardButtonData("Delete movie", cbqExecute),
		),
	)

	keyboardRate = tg.NewInlineKeyboardMarkup(
		tg.NewInlineKeyboardRow(
			tg.NewInlineKeyboardButtonData("Back", cbqBack),
		),
		tg.NewInlineKeyboardRow(
			tg.NewInlineKeyboardButtonData("Choose movie", cbqChooseMovie),
			tg.NewInlineKeyboardButtonData("Set rate", cbqSetRating),
		),
		tg.NewInlineKeyboardRow(
			tg.NewInlineKeyboardButtonData("Rate", cbqExecute),
		),
	)

	keyboardUnrate = tg.NewInlineKeyboardMarkup(
		tg.NewInlineKeyboardRow(
			tg.NewInlineKeyboardButtonData("Back", cbqBack),
		),
		tg.NewInlineKeyboardRow(
			tg.NewInlineKeyboardButtonData("Choose movie", cbqChooseMovie),
		),
		tg.NewInlineKeyboardRow(
			tg.NewInlineKeyboardButtonData("Unrate", cbqExecute),
		),
	)

	keyboardBack = tg.NewInlineKeyboardMarkup(
		tg.NewInlineKeyboardRow(
			tg.NewInlineKeyboardButtonData("Back", cbqBack),
		),
	)

	keyboardRateOptions = tg.NewInlineKeyboardMarkup(
		tg.NewInlineKeyboardRow(
			tg.NewInlineKeyboardButtonData("⭐", cbq1Star),
			tg.NewInlineKeyboardButtonData("⭐⭐", cbq2Star),
			tg.NewInlineKeyboardButtonData("⭐⭐⭐", cbq3Star),
		),
		tg.NewInlineKeyboardRow(
			tg.NewInlineKeyboardButtonData("⭐⭐⭐⭐", cbq4Star),
			tg.NewInlineKeyboardButtonData("⭐⭐⭐⭐⭐", cbq5Star),
		),
		tg.NewInlineKeyboardRow(
			tg.NewInlineKeyboardButtonData("Back", cbqBack),
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
		var keyboard *tg.InlineKeyboardMarkup
		var stage stage
		var message string
		switch state.stage {
		case stageTitle:
			fallthrough
		case stageAltTitle:
			fallthrough
		case stageYear:
			keyboard = &keyboardAdd
			stage = stageAdd
			message = prefixMovieToAdd + formatMovieWithHeaders(&state.movie)

		case stageChooseDel:
			keyboard = &keyboardDel
			stage = stageDel
			message = prefixMovieToDelete + formatMovieWithHeaders(&state.movie)

		case stageChooseRate:
			keyboard = &keyboardRate
			stage = stageRate
			message = prefixMovieToRate + formatMovieWithHeaders(&state.movie)

		case stageChooseUnrate:
			keyboard = &keyboardUnrate
			stage = stageUnrate
			message = prefixMovieToUnrate + formatMovieWithHeaders(&state.movie)

		default:
			keyboard = &mainKeyboard
			stage = stageIdle
			message = mainMessage
		}

		replaceMessage(ctx, usr, cht, mID, message, keyboard, stage)

	case cbqExecute:
		switch state.stage {
		case stageAdd:
			if len(state.movie.Title) > 0 {
				db.AddMovie(ctx, usr, &state.movie)
			} else {
				alertIncompleteData(ctx, "Movie needs a non-empty title")
			}
		case stageDel:
			if len(state.movie.Title) != 0 {
				db.DelMovie(ctx, usr, state.movie.ID)
			}
		case stageRate:
			db.RateMovie(ctx, usr, state.movie.ID, int(state.movie.Rating))
		case stageUnrate:
			db.UnrateMovie(ctx, usr, state.movie.ID)
		default:
			fixState(ctx, cbq)
			return
		}

		states[usr].movie = db.Movie{}
		replaceMessage(ctx, usr, cht, mID, mainMessage, &mainKeyboard, stageIdle)

	case cbqAdd:
		if state.stage != stageIdle {
			fixState(ctx, cbq)
			return
		}
		replaceMessage(ctx, usr, cht, mID, prefixMovieToAdd+formatMovieWithHeaders(&state.movie), &keyboardAdd, stageAdd)

	case cbqDel:
		if state.stage != stageIdle {
			fixState(ctx, cbq)
			return
		}
		replaceMessage(ctx, usr, cht, mID, prefixMovieToDelete+formatMovieWithHeaders(&state.movie), &keyboardDel, stageDel)

	case cbqRate:
		if state.stage != stageIdle {
			fixState(ctx, cbq)
			return
		}
		replaceMessage(ctx, usr, cht, mID, prefixMovieToRate+formatMovieWithHeaders(&state.movie), &keyboardRate, stageRate)

	case cbqUnrate:
		if state.stage != stageIdle {
			fixState(ctx, cbq)
			return
		}
		replaceMessage(ctx, usr, cht, mID, prefixMovieToUnrate+formatMovieWithHeaders(&state.movie), &keyboardUnrate, stageUnrate)

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

	case cbqChooseMovie:
		if state.stage != stageDel && state.stage != stageRate && state.stage != stageUnrate {
			fixState(ctx, cbq)
			return
		}

		lst, _ := db.ListAllMovies(ctx, usr)

		var stage stage
		switch state.stage {
		case stageDel:
			stage = stageChooseDel
		case stageRate:
			stage = stageChooseRate
		case stageUnrate:
			stage = stageChooseUnrate
		}
		replaceMessage(ctx, usr, cht, mID, joinMovies(lst, true, "Enter movie ID:\n\n"), &keyboardBack, stage)

	case cbqSetRating:
		if state.stage != stageRate {
			fixState(ctx, cbq)
			return
		}
		replaceMessage(ctx, usr, cht, mID, "Set your rating", &keyboardRateOptions, stageRate)

	case cbq1Star:
		fallthrough
	case cbq2Star:
		fallthrough
	case cbq3Star:
		fallthrough
	case cbq4Star:
		fallthrough
	case cbq5Star:
		r, err := strconv.Atoi(cbq.Data)
		if err != nil {
			ctx.Logger.Errorw("impossible came true", "err", err)
		}

		state.movie.Rating = float32(r)
		replaceMessage(ctx, usr, cht, mID, prefixMovieToRate+formatMovieWithHeaders(&state.movie), &keyboardRate, stageRate)

	}
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

	states[usr].stage = stg
	states[usr].mainMessageID = msgID
	// for mID := range states[usr].tmpMsgIDs {
	// 	if mID == states[usr].mainMessageID {
	// 		continue
	// 	}

	// 	d := tg.NewDeleteMessage(cht, mID)
	// 	if _, err := bot.Send(d); err != nil {
	// 		log.Error(usr, err, "failed deleting outdated messages")
	// 	}
	// }

	return true
}

func formatMovieWithHeaders(mv *db.Movie) string {
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
		fmtStr = append(fmtStr, "Year: %d")
		args = append(args, mv.Year)
	}

	return fmt.Sprintf(strings.Join(fmtStr, ""), args...)
}

func fixState(ctx *bot.Context, cbq *tg.CallbackQuery) {
	usr := cbq.From.ID
	cht := cbq.Message.From.ID

	cb := tg.NewCallback(cbq.ID, "Message is outdated, can't continue")
	if _, err := ctx.Bot.Request(cb); err != nil {
		ctx.Logger.Errorw("failed sending callback", "err", err)
		// no return
	}

	replaceMessage(ctx, usr, cht, cbq.Message.MessageID, mainMessage, &mainKeyboard, stageIdle)
}
