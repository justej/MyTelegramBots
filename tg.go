package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"telecho/database"
	"telecho/logger"

	tg "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const (
	// states
	idle State = iota
	add
	del
	done
	remindat

	noMemosAddNow = "No memos to remind. Add ones now!"
	yourMemosForToday = "Your memos for today:\n"
	doneMemosForToday = "Memos you've done recently:\n"
	lineSep = "\n\n"
)

func RunBot(bot *tg.BotAPI, db *database.Database) {
	u := tg.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message != nil {
			if update.Message.IsCommand() {
				go handleCommand(bot, db, update.Message)
				continue
			}

			go handleMessage(bot, db, update.Message)
		} else if update.CallbackQuery != nil {
			// TODO
		}
	}
}

func InitBot() *tg.BotAPI {
	tgToken := os.Getenv("TG_TOKEN")
	bot, err := tg.NewBotAPI(tgToken)
	if err != nil {
		log.Panic(err)
	}

	bot.Debug = false

	log.Printf("Authorized on account %s", bot.Self.UserName)
	return bot
}

func handleMessage(bot *tg.BotAPI, db *database.Database, message *tg.Message) {
	u := message.From.ID
	chatID := message.Chat.ID

	switch states[u] {
	case idle:
		db.AddMemo(u, chatID, message.Text)
		msg := tg.NewMessage(chatID, "Looks like you wanted to add a memo. I did it.")
		msg.ReplyToMessageID = message.MessageID
		if _, err := bot.Send(msg); err != nil {
			logger.ForUser(u, "failed on quick add:", err)
		}
		list := db.ListFirstMemos(u, chatID, 5, true)
		if _, err := sendMemosForToday(bot, chatID, &list, nil); err != nil {
			logger.ForUser(u, "failed on sending today's list:", err)
		}

	case add:
		addMemo(bot, db, u, chatID, message.Text)

	case del:
		// TODO: set idle state after 3 attempts
		delMemo(bot, db, u, chatID, message.MessageID, message.Text)

	case done:
		markAsDone(bot, db, u, chatID, message.MessageID, message.Text)

	case remindat:
		states[u] = idle
		reminderTime := strings.Trim(message.Text, " ")
		ok := updateReminder(bot, db, u, chatID, reminderTime)
		if !ok {
			sendErrorMessage(bot, u, chatID, message.MessageID, "I didn't get it. I expected a valid time in the format HH:MM")
			return
		}

		text := fmt.Sprintf("I got it, I'll remind you about your memos at %s.\n\nSend your location to update timezone", reminderTime)
		msg := tg.NewMessage(chatID, text)
		if _, err := bot.Send(msg); err != nil {
			logger.ForUser(u, "failed on setting reminder:", err)
			return
		}
	}
}

func delMemo(bot *tg.BotAPI, db *database.Database, u int64, chatID int64, replyID int, text string) {
	val, err := validateInt(text, 1, db.GetLenMemos(u, chatID))
	if err != nil {
		sendErrorMessage(bot, u, chatID, replyID, "I expected a number in the range of 1-"+strconv.Itoa(db.GetLenMemos(u, chatID)))
		return
	}

	db.Delete(u, chatID, val)
	list := db.ListFirstMemos(u, chatID, 5, true)
	if _, err = sendMemosForToday(bot, chatID, &list, nil); err != nil {
		logger.ForUser(u, "failed on sanding today's list:", err)
	}
	states[u] = idle
}

func markAsDone(bot *tg.BotAPI, db *database.Database, u int64, chatID int64, replyID int, text string) {
	val, err := validateInt(text, 1, db.GetLenMemos(u, chatID))
	if err != nil {
		sendErrorMessage(bot, u, chatID, replyID, "I expected a number in the range of 1-"+strconv.Itoa(db.GetLenMemos(u, chatID)))
		return
	}

	db.Done(u, chatID, val)
	active, done := db.ListAllMemos(u, chatID, true)
	if _, err = sendMemosForToday(bot, chatID, &active, &done); err != nil {
		logger.ForUser(u, "failed on sanding today's list:", err)
	}
	states[u] = idle
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

func handleCommand(bot *tg.BotAPI, db *database.Database, message *tg.Message) {
	chatID := message.Chat.ID
	u := message.From.ID

	state, ok := states[u]
	if ok && state != idle {
		resetState(bot, u, chatID)
	}

	switch cmd := message.Command(); cmd {
	case "start":
		err := db.CreateUser(u, chatID)
		if err != nil {
			logger.ForUser(u, "failed initializing user", err)
			return
		}

		ok := setReminder(u)
		if !ok {
			logger.ForUser(u, "failed setting reminder", nil)
			return
		}

		msg := tg.NewMessage(chatID, "Hello, I'm an experienced memo keeper. I write down your memos and remind about them from time to time. By the way, I can tell me when to send you the reminder, so I won't wake you up when you decided to stay in bed ;)")
		if _, err := bot.Send(msg); err != nil {
			logger.ForUser(u, "failed on sending hello message:", err)
		}
		list := db.ListFullMemos(u, chatID, false)
		if _, err := sendMemosForToday(bot, chatID, &list, nil); err != nil {
			logger.ForUser(u, "failed on '/start:'", err)
		}

	case "help":
		msg := tg.NewMessage(chatID, `As you may know, I keep your memos in order and periodically remind about them. You can send me a message or one of these commands:
/list - to see short list of your memos
/listall - to see full list of your memos
/add - to add a new memo
/del - to immediately delete the memo
/done - to mark the memo as done, I'll delete done memos before next reminder
/reorder - to change the display order of your memos
/remindat - to let me know when to send you a reminder
/settings - to list settings
/stop - to stop sending reminders`)
		if _, err := bot.Send(msg); err != nil {
			logger.ForUser(u, "failed on '/help':", err)
		}

	case "list":
		list := db.ListFirstMemos(u, chatID, 5, true)
		if _, err := sendMemosForToday(bot, chatID, &list, nil); err != nil {
			logger.ForUser(u, "failed on '/list:'", err)
		}

	case "listall":
		active, done := db.ListAllMemos(u, chatID, false)
		if _, err := sendMemosForToday(bot, chatID, &active, &done); err != nil {
			logger.ForUser(u, "failed on '/listall:'", err)
		}

	case "add":
		if len(message.Text) > len("/add") {
			text := message.Text[len("/add "):]
			addMemo(bot, db, u, chatID, text)
			return
		}

		msg := tg.NewMessage(chatID, "What's your memo?")
		if _, err := bot.Send(msg); err != nil {
			logger.ForUser(u, "failed on '/add':", err)
			return
		}
		states[u] = add

	case "del":
		if len(message.Text) > len("/del") {
			memo := strings.Trim(message.Text[len("/del "):], " ")
			delMemo(bot, db, u, chatID, message.MessageID, memo)
			return
		}

		list := db.ListFullMemos(u, chatID, true) + "\n\nWhich memo do you want to delete?"
		if _, err := sendMemosForToday(bot, chatID, &list, nil); err != nil {
			logger.ForUser(u, "failed on '/del':", err)
			return
		}
		states[u] = del

	case "done":
		if len(message.Text) > len("/done") {
			memo := strings.Trim(message.Text[len("/done "):], " ")
			markAsDone(bot, db, u, chatID, message.MessageID, memo)
			return
		}

		list := db.ListFullMemos(u, chatID, false) + "\n\nWhich memo do you want to mark as done?"
		if _, err := sendMemosForToday(bot, chatID, &list, nil); err != nil {
			logger.ForUser(u, "failed on '/done':", err)
			return
		}
		states[u] = done

	case "reorder":
		msg := tg.NewMessage(chatID, "not implemented yet")
		bot.Send(msg)

	case "remindat":
		if len(message.Text) > len("/remindat") {
			text := message.Text[len("/remindat "):]
			updateReminder(bot, db, u, chatID, text)
			return
		}

		msg := tg.NewMessage(chatID, "Enter hour and minute to send you a reminder in the format HH:MM. Send location to update timezone")
		if _, err := bot.Send(msg); err != nil {
			logger.ForUser(u, "failed on '/add':", err)
			return
		}
		states[u] = remindat

	case "settings":
		rp, ok := db.GetRemindParams(u)
		if !ok {
			logger.ForUser(u, "failed getting user config", nil)
		}

		var text string
		if rp == nil {
			logger.ForUser(u, "no remind params found", nil)
			text = "I don't see remind time"
		} else {
			t := rp.RemindAt
			zone, offset := t.Zone()
			text = fmt.Sprintf("Your settings:\nRemind memos every day on %02d:%02d, %s (%d)", t.Hour(), t.Minute(), zone, offset)
		}

		msg := tg.NewMessage(chatID, text)
		if _, err := bot.Send(msg); err != nil {
			logger.ForUser(u, "failed on '/settings':", err)
		}

	default:
		sendErrorMessage(bot, u, chatID, message.MessageID, "I don't known this command. Use /help to list commands I know")
	}
}

func addMemo(bot *tg.BotAPI, db *database.Database, u int64, chatID int64, text string) {
	db.AddMemo(u, chatID, text)
	list := db.ListFirstMemos(u, chatID, 5, true)
	if _, err := sendMemosForToday(bot, chatID, &list, nil); err != nil {
		logger.ForUser(u, "failed on sending today's list:", err)
	} else {
		states[u] = idle
	}
}

func sendMemosForToday(bot *tg.BotAPI, chatID int64, active, done *string) (tg.Message, error) {
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

	msg := tg.NewMessage(chatID, sb.String())
	return bot.Send(msg)
}

func resetState(bot *tg.BotAPI, u int64, chatID int64) {
	msg := tg.NewMessage(chatID, "You've started another command. Cancelling ongoing operation")
	if _, err := bot.Send(msg); err != nil {
		logger.ForUser(u, "failed resetting state:", err)
	}
	states[u] = idle
}

func sendErrorMessage(bot *tg.BotAPI, u int64, chatID int64, replyID int, s string) {
	msg := tg.NewMessage(chatID, s)
	msg.ReplyToMessageID = replyID
	if _, err := bot.Send(msg); err != nil {
		var errMsg strings.Builder
		errMsg.WriteString("failed on sending error message '")
		errMsg.WriteString(s)
		errMsg.WriteString("': ")
		logger.ForUser(u, errMsg.String(), err)
	}
}
