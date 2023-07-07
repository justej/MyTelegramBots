package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"telecho/database"

	tg "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const (
	// states
	idle State = iota
	add
	del
	remindon
)

func RunBot(bot *tg.BotAPI) {
	u := tg.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message != nil {
			if update.Message.IsCommand() {
				handleCommand(bot, update.Message)
				continue
			}

			handleMessage(bot, update.Message)
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

func handleMessage(bot *tg.BotAPI, message *tg.Message) {
	u := message.From.ID
	chatID := message.Chat.ID

	switch states[u] {
	case idle:
		db.AddMemo(u, message.Text)
		msg := tg.NewMessage(chatID, "Looks like you wanted to add a memo. I did it.")
		msg.ReplyToMessageID = message.MessageID
		if _, err := bot.Send(msg); err != nil {
			logForUser(u, "failed on quick add:", err)
		}
		list := db.ListFirstMemos(u, 5, true)
		if _, err := sendMemosForToday(bot, chatID, list); err != nil {
			logForUser(u, "failed on sending today's list:", err)
		}

	case add:
		addMemo(bot, u, chatID, message.Text)

	case del:
		// TODO: set idle state after 3 attempts
		delMemo(bot, u, chatID, message.MessageID, message.Text)

	case remindon:
		states[u] = idle
		reminderTime := strings.Trim(message.Text, " ")
		if err := updateReminder(bot, u, chatID, reminderTime); err != nil {
			sendErrorMessage(bot, u, chatID, message.MessageID, "I didn't get it. I expected a valid time in the format HH:MM")
			return
		}

		msg := tg.NewMessage(chatID, "I got it, I'll remind you about your memos at "+reminderTime)
		if _, err := bot.Send(msg); err != nil {
			logForUser(u, "failed on setting reminder:", err)
		}
	}
}

func delMemo(bot *tg.BotAPI, u int64, chatID int64, replyID int, text string) {
	val, err := validateInt(text, 1, db.GetLenMemos(u))
	if err != nil {
		sendErrorMessage(bot, u, chatID, replyID, "I expected a number in the range of 1-"+strconv.Itoa(db.GetLenMemos(u)))
		return
	}

	db.Delete(u, val-1)
	list := db.ListFirstMemos(u, 5, true)
	if _, err = sendMemosForToday(bot, chatID, list); err != nil {
		logForUser(u, "failed on sanding today's list:", err)
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

func handleCommand(bot *tg.BotAPI, message *tg.Message) {
	chatID := message.Chat.ID
	u := message.From.ID

	state, ok := states[u]
	if ok && state != idle {
		resetState(bot, u, chatID)
	}

	switch cmd := message.Command(); cmd {
	case "start":
		db.StoreChatID(u, chatID)
		setReminder(u)
		msg := tg.NewMessage(chatID, "Hello, I'm an experienced memo keeper. I write down your memos and remind about them from time to time. By the way, I can tell me when to send you the reminder, so I won't wake you up when you decided to stay in bed ;)")
		if _, err := bot.Send(msg); err != nil {
			logForUser(u, "failed on sending hello message:", err)
		}
		list := db.ListAllMemos(u, false)
		if _, err := sendMemosForToday(bot, chatID, list); err != nil {
			logForUser(u, "failed on '/start:'", err)
		}

	case "help":
		msg := tg.NewMessage(chatID, `As you may know, I keep your memos in order and periodically remind about them. You can send me a message or one of these commands:
/list - to see short list of your memos
/listall - to see full list of your memos
/add - to add a new memo
/del - to immediately delete the memo
/done - to mark the memo as done, I'll delete done memos before next reminder
/reorder - to change the display order of your memos
/remindon - to let me know when to send you a reminder
/settings - to list settings
/stop - to stop sending reminders`)
		if _, err := bot.Send(msg); err != nil {
			logForUser(u, "failed on '/help':", err)
		}

	case "list":
		list := db.ListFirstMemos(u, 5, true)
		if _, err := sendMemosForToday(bot, chatID, list); err != nil {
			logForUser(u, "failed on '/list:'", err)
		}

	case "listall":
		list := db.ListAllMemos(u, false)
		if _, err := sendMemosForToday(bot, chatID, list); err != nil {
			logForUser(u, "failed on '/listall:'", err)
		}

	case "add":
		if len(message.Text) > len("/add") {
			text := message.Text[len("/add "):]
			addMemo(bot, u, chatID, text)
			return
		}

		msg := tg.NewMessage(chatID, "What's your memo?")
		if _, err := bot.Send(msg); err != nil {
			logForUser(u, "failed on '/add':", err)
			return
		}
		states[u] = add

	case "del":
		if len(message.Text) > len("/del") {
			memo := strings.Trim(message.Text[len("/del "):], " ")
			delMemo(bot, u, chatID, message.MessageID, memo)
			return
		}

		list := db.ListAllMemos(u, true) + "\n\nWhich memo do you want to delete?"
		if _, err := sendMemosForToday(bot, chatID, list); err != nil {
			logForUser(u, "failed on '/del':", err)
			return
		}
		states[u] = del

	case "done":
		msg := tg.NewMessage(chatID, "not implemented yet")
		bot.Send(msg)

	case "reorder":
		msg := tg.NewMessage(chatID, "not implemented yet")
		bot.Send(msg)

	case "remindon":
		if len(message.Text) > len("/remindon") {
			text := message.Text[len("/remindon "):]
			updateReminder(bot, u, chatID, text)
			return
		}

		msg := tg.NewMessage(chatID, "Enter hour and minute to send you a reminder in the format HH:MM")
		if _, err := bot.Send(msg); err != nil {
			logForUser(u, "failed on '/add':", err)
			return
		}
		states[u] = remindon

	case "settings":
		data, ok := db[u]
		if !ok {
			data = *database.NewData()
			db[u] = data
		} 

		cfg := data.Config
		txt := fmt.Sprintf("Your settings:\nRemind memos every day on %02d:%02d", cfg.RemindHour, cfg.RemindMin)
		msg := tg.NewMessage(chatID, txt)
		if _, err := bot.Send(msg); err != nil {
			logForUser(u, "failed on '/settings':", err)
		}

	default:
		sendErrorMessage(bot, u, chatID, message.MessageID, "I don't known this command. Use /help to list commands I know")
	}
}

func addMemo(bot *tg.BotAPI, u int64, chatID int64, text string) {
	db.AddMemo(u, text)
	list := db.ListFirstMemos(u, 5, true)
	if _, err := sendMemosForToday(bot, chatID, list); err != nil {
		logForUser(u, "failed on sending today's list:", err)
	} else {
		states[u] = idle
	}
}

func logForUser(u int64, msg string, err error) {
	log.Println("user:", u, msg, err)
}

func sendMemosForToday(bot *tg.BotAPI, chatID int64, text string) (tg.Message, error) {
	if len(text) == 0 {
		text = "No memos to remind. Add ones now!"
	} else {
		text = "Your memos for today:\n" + text
	}
	msg := tg.NewMessage(chatID, text)
	return bot.Send(msg)
}

func resetState(bot *tg.BotAPI, u int64, chatID int64) {
	msg := tg.NewMessage(chatID, "You've started another command. Cancelling ongoing operation")
	if _, err := bot.Send(msg); err != nil {
		logForUser(u, "failed resetting state:", err)
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
		logForUser(u, errMsg.String(), err)
	}
}
