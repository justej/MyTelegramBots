package tgbot

import (
	"botfarm/bot"
	"botfarm/bots/FindingMemo/db"
	"botfarm/bots/FindingMemo/reminder"
	"botfarm/bots/FindingMemo/timezone"
	"fmt"
	"strconv"
	"strings"

	tg "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type State int

const (
	idle State = iota
	add
	ins
	del
	done
	remindat

	noMemosAddNow     = "No memos to remind. Add ones now!"
	yourMemosForToday = "Your memos for today:\n"
	doneMemosForToday = "Memos you've done recently:\n"
	lineSep           = "\n\n"
)

var states = make(map[int64]State) // user to state

func HandleMessage(ctx *bot.Context, message *tg.Message) {
	usr := message.From.ID
	cht := message.Chat.ID

	switch states[usr] {
	case idle:
		loc := message.Location
		if loc != nil {
			tz := updateTimeZone(ctx, usr, loc)

			msg := tg.NewMessage(cht, fmt.Sprintf("Time zone identified as %s, it will be used in time offset and transition to daylight saving time if any.", tz))
			msg.ReplyToMessageID = message.MessageID
			if _, err := ctx.Bot.Send(msg); err != nil {
				ctx.Logger.Errorw("failed updating location", "err", err)
			}
			return
		}

		if !db.AddMemo(ctx, cht, message.Text) {
			return
		}

		msg := tg.NewMessage(cht, "Looks like you wanted to add a memo. Added it.")
		msg.ReplyToMessageID = message.MessageID
		if _, err := ctx.Bot.Send(msg); err != nil {
			ctx.Logger.Errorw("failed on quick add:", "err", err)
		}

	case add:
		addMemo(ctx, usr, cht, message.Text)

	case ins:
		insertMemo(ctx, usr, cht, message.Text)

	case del:
		// TODO: set idle state after 3 attempts
		delMemo(ctx, usr, cht, message.MessageID, message.Text)

	case done:
		markAsDone(ctx, usr, cht, message.MessageID, message.Text)

	case remindat:
		reminderAt := strings.TrimSpace(message.Text)
		ok := updateReminder(ctx, usr, reminderAt)
		if !ok {
			sendErrorMessage(ctx, cht, message.MessageID, "I didn't get it. I expected a valid time in the format HH:MM.")
			return
		}

		rp, ok := db.GetRemindParams(ctx, usr)
		if !ok {
			ctx.Logger.Warn("failed on setting reminder:")
			return
		}

		states[usr] = idle

		text := fmt.Sprintf("I got it, I'll remind you about your memos at %s in %s time zone", reminderAt, rp.TimeZone)
		msg := tg.NewMessage(cht, text)
		if _, err := ctx.Bot.Send(msg); err != nil {
			ctx.Logger.Errorw("failed on setting reminder:", "err", err)
			return
		}
	}
}

func updateTimeZone(ctx *bot.Context, usr int64, loc *tg.Location) string {
	l := timezone.GeoLocation{
		Latitude:  timezone.DegToRad(float32(loc.Latitude)),
		Longitude: timezone.DegToRad(float32(loc.Longitude)),
	}
	zone, err := l.FindZone()
	if err != nil {
		ctx.Logger.Error("failed updating time zone")
	}

	ctx.Logger.Debugf("handling location: (%f, %f) -> (%f; %f) -> %s",
		l.Latitude, l.Longitude, zone.GeoLocation.Latitude, zone.GeoLocation.Longitude, zone.TZ)

	ok := db.UpdateTZ(ctx, zone.GeoLocation, zone.TZ)
	if !ok {
		return ""
	}

	ok = reminder.Set(ctx, usr)
	if !ok {
		ctx.Logger.Warn("failed setting reminder")
	}

	return zone.TZ
}

func delMemo(ctx *bot.Context, usr, cht int64, replyID int, text string) {
	n := db.GetLenMemos(ctx, cht)
	val, err := validateInt(text, 1, n)
	if err != nil {
		sendErrorMessage(ctx, cht, replyID, "I expected a number in the range of 1-"+strconv.Itoa(n))
		return
	}

	db.Delete(ctx, cht, val)
	list := db.ListFirstMemos(ctx, cht, 5, true)
	if _, err = sendMemosForToday(ctx, cht, &list, nil); err != nil {
		ctx.Logger.Errorw("failed on sanding today's list:", "err", err)
	}
	states[usr] = idle
}

func markAsDone(ctx *bot.Context, usr, cht int64, replyID int, text string) {
	n := db.GetLenMemos(ctx, cht)
	val, err := validateInt(text, 1, n)
	if err != nil {
		sendErrorMessage(ctx, cht, replyID, "I expected a number in the range of 1-"+strconv.Itoa(n))
		return
	}

	db.Done(ctx, cht, val)
	active, done := db.ListAllMemos(ctx, cht, true)
	if _, err = sendMemosForToday(ctx, cht, &active, &done); err != nil {
		ctx.Logger.Errorw("failed on sanding today's list:", "err", err)
	}
	states[usr] = idle
}

type OutOfRange struct{}

func (oor OutOfRange) Error() string {
	return "out of range"
}

func validateInt(text string, min int, max int) (int, error) {
	val, err := strconv.Atoi(text)
	if err != nil {
		return 0, err
	}

	if val < min || val > max {
		return 0, OutOfRange{}
	}
	return val, nil
}

func HandleCommand(ctx *bot.Context, message *tg.Message) {
	usr := message.From.ID
	cht := message.Chat.ID

	state, ok := states[usr]
	if ok && state != idle {
		resetState(ctx, usr, cht)
	}

	// TODO: make sure that trailing spaces won't be interpreted as long command
	cmd := message.Command()
	switch cmd {
	case "start":
		err := db.CreateUser(ctx, usr, cht)
		if err != nil {
			ctx.Logger.Errorw("failed initializing user", "err", err)
			return
		}

		ok := reminder.Set(ctx, usr)
		if !ok {
			ctx.Logger.Warn("failed setting reminder")
			return
		}

		ctx.Logger.Info("user has started the bot")

		msg := tg.NewMessage(cht, "Hello, I'm an experienced memo keeper. I write down your memos and remind about them from time to time. By the way, you can tell me when to send you the reminder, so I won't wake you up when you decided to stay in bed ;) Send me your location so I'll know in which time zone is your time")
		if _, err := ctx.Bot.Send(msg); err != nil {
			ctx.Logger.Errorw("failed on sending hello message:", "err", err)
		}
		list := db.ListFullMemos(ctx, cht, false)
		if _, err := sendMemosForToday(ctx, cht, &list, nil); err != nil {
			ctx.Logger.Errorw("failed on '/start:'", "err", err)
		}

	case "help":
		msg := tg.NewMessage(cht, `As you may know, I keep your memos in order and periodically remind about them. You can send me a message or one of these commands:
/list - to see short list of your memos
/listall - to see full list of your memos
/ins - to add a new memo at the beginning of the list
/add - to add a new memo at the end of the list
/del - to immediately delete the memo
/done - to mark the memo as done, I'll delete done memos in approximately 24 hours
/remindat - to let me know when to send you a reminder (send location to update time zone)
/settings - to list settings`)
		if _, err := ctx.Bot.Send(msg); err != nil {
			ctx.Logger.Errorw("failed on '/help':", "err", err)
		}

	case "list":
		list := db.ListFirstMemos(ctx, cht, 5, true)
		if _, err := sendMemosForToday(ctx, cht, &list, nil); err != nil {
			ctx.Logger.Errorw("failed on '/list:'", "err", err)
		}

	case "listall":
		active, done := db.ListAllMemos(ctx, cht, false)
		if _, err := sendMemosForToday(ctx, cht, &active, &done); err != nil {
			ctx.Logger.Errorw("failed on '/listall:'", "err", err)
		}

	case "add":
		if len(message.Text) > len("/add") {
			text := message.Text[len("/add "):]
			ok := db.AddMemo(ctx, cht, text)
			if !ok {
				return
			}
			msg := tg.NewMessage(cht, "Added to the end of the list")
			msg.ReplyToMessageID = message.MessageID
			ctx.Bot.Send(msg)
			return
		}

		msg := tg.NewMessage(cht, "What's your memo?")
		if _, err := ctx.Bot.Send(msg); err != nil {
			ctx.Logger.Errorw("failed on '/add':", "err", err)
			return
		}
		states[usr] = add

	case "ins":
		if len(message.Text) > len("/ins") {
			text := message.Text[len("/ins "):]
			ok := db.InsertMemo(ctx, cht, text)
			if !ok {
				return
			}

			msg := tg.NewMessage(cht, "Now it's your top priority memo")
			msg.ReplyToMessageID = message.MessageID
			ctx.Bot.Send(msg)
			return
		}

		msg := tg.NewMessage(cht, "What's your memo?")
		if _, err := ctx.Bot.Send(msg); err != nil {
			ctx.Logger.Errorw("failed on '/ins':", "err", err)
			return
		}
		states[usr] = ins

	case "del":
		if len(message.Text) > len("/del") {
			memo := strings.Trim(message.Text[len("/del "):], " ")
			delMemo(ctx, usr, cht, message.MessageID, memo)
			return
		}

		list := db.ListFullMemos(ctx, cht, true) + "\n\nWhich memo do you want to delete?"
		if _, err := sendMemosForToday(ctx, cht, &list, nil); err != nil {
			ctx.Logger.Errorw("failed on '/del':", "err", err)
			return
		}
		states[usr] = del

	case "done":
		if len(message.Text) > len("/done") {
			memo := strings.Trim(message.Text[len("/done "):], " ")
			markAsDone(ctx, usr, cht, message.MessageID, memo)
			return
		}

		list := db.ListFullMemos(ctx, cht, false) + "\n\nWhich memo do you want to mark as done?"
		if _, err := sendMemosForToday(ctx, cht, &list, nil); err != nil {
			ctx.Logger.Errorw("failed on '/done':", "err", err)
			return
		}
		states[usr] = done

	case "reorder":
		msg := tg.NewMessage(cht, "not implemented yet")
		ctx.Bot.Send(msg)

	case "remindat":
		if len(message.Text) > len("/remindat") {
			text := message.Text[len("/remindat "):]
			if !updateReminder(ctx, usr, text) {
				ctx.Logger.Error("failed on '/remindat'")
				return
			}

			msg := tg.NewMessage(cht, "Gotcha, I'll remind at "+text)
			if _, err := ctx.Bot.Send(msg); err != nil {
				ctx.Logger.Errorw("failed confirming '/remindat'", "err", err)
				return
			}

			return
		}

		msg := tg.NewMessage(cht, "Enter hour and minute to send you a reminder in the format HH:MM. Send location to update timezone")
		if _, err := ctx.Bot.Send(msg); err != nil {
			ctx.Logger.Errorw("failed on '/remindat'", "err", err)
			return
		}
		states[usr] = remindat

	case "settings":
		rp, ok := db.GetRemindParams(ctx, usr)
		if !ok {
			ctx.Logger.Error("failed getting user config")
		}

		var text string
		if rp == nil {
			ctx.Logger.Errorw("no remind params found")
			text = "I don't see remind time"
		} else {
			h := rp.RemindAt / 60
			m := rp.RemindAt - h*60
			text = fmt.Sprintf("Reminder time: %02d:%02d (%s).\n\nUse command /remindat to change the time.\nSend location to update time zone.", h, m, rp.TimeZone)
		}

		msg := tg.NewMessage(cht, text)
		if _, err := ctx.Bot.Send(msg); err != nil {
			ctx.Logger.Errorw("failed on '/settings':", "err", err)
		}

	default:
		sendErrorMessage(ctx, cht, message.MessageID, "I don't known this command. Use /help to list commands I know")
	}

	ctx.Logger.Infof("Command %s was successfully handled", cmd)
}

func addMemo(ctx *bot.Context, usr, cht int64, text string) {
	db.AddMemo(ctx, cht, text)
	list := db.ListFirstMemos(ctx, cht, 5, true)
	if _, err := sendMemosForToday(ctx, cht, &list, nil); err != nil {
		ctx.Logger.Errorw("failed on sending today's list:", "err", err)
	} else {
		states[usr] = idle
	}
}

func insertMemo(ctx *bot.Context, usr, cht int64, text string) {
	db.InsertMemo(ctx, cht, text)
	list := db.ListFirstMemos(ctx, cht, 5, true)
	if _, err := sendMemosForToday(ctx, cht, &list, nil); err != nil {
		ctx.Logger.Errorw("failed on sending today's list:", "err", err)
	} else {
		states[usr] = idle
	}
}

func sendMemosForToday(ctx *bot.Context, cht int64, active, done *string) (tg.Message, error) {
	var sb strings.Builder

	n := len(noMemosAddNow) + len(yourMemosForToday) + len(lineSep)
	if active != nil {
		n += len(*active)
	}
	if done != nil {
		n += len(*done)
	}
	sb.Grow(n)

	if active == nil || len(*active) == 0 {
		sb.WriteString(noMemosAddNow)
	} else {
		sb.WriteString(yourMemosForToday)
		sb.WriteString(*active)
	}

	if done != nil && len(*done) != 0 {
		sb.WriteString(lineSep)
		sb.WriteString(doneMemosForToday)
		sb.WriteString(*done)
	}

	msg := tg.NewMessage(cht, sb.String())
	return ctx.Bot.Send(msg)
}

func resetState(ctx *bot.Context, usr, cht int64) {
	msg := tg.NewMessage(cht, "You've started another command. Cancelling ongoing operation")
	if _, err := ctx.Bot.Send(msg); err != nil {
		ctx.Logger.Errorw("failed resetting state:", "err", err)
	}
	states[usr] = idle
}

func sendErrorMessage(ctx *bot.Context, cht int64, replyID int, s string) {
	msg := tg.NewMessage(cht, s)
	msg.ReplyToMessageID = replyID
	if _, err := ctx.Bot.Send(msg); err != nil {
		var errMsg strings.Builder
		errMsg.WriteString("failed on sending error message '")
		errMsg.WriteString(s)
		errMsg.WriteString("': ")
		ctx.Logger.Errorw(errMsg.String(), "err", err)
	}
}

// SendReminder is a callback that's invoked by reminder
func SendReminder(ctx *bot.Context, cht int64) {
	list := db.ListFirstMemos(ctx, cht, 5, true)
	if _, err := sendMemosForToday(ctx, cht, &list, nil); err != nil {
		ctx.Logger.Errorw("failed on '/list:'", "err", err)
	}
}

func updateReminder(ctx *bot.Context, usr int64, text string) bool {
	parts := strings.Split(text, ":")
	if len(parts) != 2 {
		return false
	}

	hour, err := validateInt(parts[0], 0, 23)
	if err != nil {
		return false
	}

	min, err := validateInt(parts[1], 0, 59)
	if err != nil {
		return false
	}

	t := hour*60 + min
	ok := db.SetRemindAt(ctx, usr, t)
	if !ok {
		return false
	}

	reminder.Set(ctx, usr)
	return true
}
