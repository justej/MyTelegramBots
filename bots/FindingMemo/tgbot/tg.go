package tgbot

import (
	"botfarm/bot"
	"botfarm/bots/FindingMemo/db"
	"botfarm/bots/FindingMemo/reminder"
	"botfarm/bots/FindingMemo/timezone"
	"fmt"
	"strconv"
	"strings"
	"time"

	tg "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/pkg/errors"
	"go.uber.org/zap"
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

type TBot struct {
	Bot             *tg.BotAPI
	DB              *db.Database
	Logger          *zap.SugaredLogger
	ReminderManager *reminder.Manager
	RetryDelay      time.Duration
	RetryAttempts   int
}

func NewTBot(tgtoken string, d *db.Database, l *zap.SugaredLogger) (*TBot, error) {
	b, err := tg.NewBotAPI(tgtoken)
	if err != nil {
		l.Errorw("failed to initialize Telegram Bot", "err", err)
		return &TBot{}, err
	}

	b.Debug = false

	l.Infof("authorized on account %q (%q, %d)", b.Self.FirstName, b.Self.UserName, b.Self.ID)

	return &TBot{
		Bot:           b,
		DB:            d,
		Logger:        l,
		RetryAttempts: 3,
		RetryDelay:    1 * time.Second,
	}, nil

}

func (b *TBot) HandleMessage(message *tg.Message) {
	usr := message.From.ID

	switch states[usr] {
	case idle:
		loc := message.Location
		if loc != nil {
			tz, err := b.updateTimeZone(usr, loc)
			if err != nil {
				b.Logger.Errorw("couldn't update time zone", "err", err)
			}

			txt := fmt.Sprintf("Time zone identified as %s, it will be used in time offset and transition to daylight saving time if any.", tz)
			b.SendMessage(usr, txt, message.MessageID)
			return
		}

		if err := b.DB.AddMemo(usr, message.Text); err != nil {
			b.Logger.Errorw("failed adding memo", "err", err)
			return
		}

		msg := tg.NewMessage(usr, "Looks like you wanted to add a memo. Added it.")
		msg.ReplyToMessageID = message.MessageID
		if _, err := b.Bot.Send(msg); err != nil {
			b.Logger.Errorw("failed on quick add:", "err", err)
		}

	case add:
		b.addMemo(usr, message.Text)

	case ins:
		b.insertMemo(usr, message.Text)

	case del:
		// TODO: set idle state after 3 attempts
		b.delMemo(usr, message.MessageID, message.Text)

	case done:
		b.markAsDone(usr, message.MessageID, message.Text)

	case remindat:
		reminderAt := strings.TrimSpace(message.Text)
		err := b.updateReminder(usr, reminderAt)
		if err != nil {
			b.Logger.Errorw("failed updating reminder", "err", err)
			err := b.SendMessage(usr, "I didn't get it. I expected a valid time in the format HH:MM.", message.MessageID)
			if err != nil {
				b.Logger.Errorw("failed sending error message", "err", err)
			}
			return
		}

		rp, err := b.DB.GetRemindParams(usr)
		if err != nil {
			b.Logger.Warn("failed on setting reminder:")
			return
		}

		states[usr] = idle

		text := fmt.Sprintf("I got it, I'll remind you about your memos at %s in %s time zone", reminderAt, rp.TimeZone)
		msg := tg.NewMessage(usr, text)
		if _, err := b.Bot.Send(msg); err != nil {
			b.Logger.Errorw("failed on setting reminder:", "err", err)
			return
		}
	}
}

func (b *TBot) SendMessage(usr int64, txt string, replyMessageID int) error {
	m := tg.NewMessage(usr, txt)
	if replyMessageID >= 0 {
		m.ReplyToMessageID = replyMessageID
	}
	m.ParseMode = tg.ModeHTML
	m.DisableWebPagePreview = true

	var err error
	bot.RobustExecute(b.RetryAttempts, b.RetryDelay, func() bool {
		_, err = b.Bot.Send(m)
		if err != nil {
			b.Logger.Errorw("failed sending message", "err", err)
		}
		return err == nil
	})
	return err
}

func (b *TBot) updateTimeZone(usr int64, loc *tg.Location) (string, error) {
	l := timezone.GeoLocation{
		Latitude:  timezone.DegToRad(float32(loc.Latitude)),
		Longitude: timezone.DegToRad(float32(loc.Longitude)),
	}
	zone, err := l.FindZone()
	if err != nil {
		return "", errors.Wrap(err, "failed find time zone")
	}

	b.Logger.Debugf("handling location: (%f, %f) -> (%f; %f) -> %s",
		l.Latitude, l.Longitude, zone.GeoLocation.Latitude, zone.GeoLocation.Longitude, zone.TZ)

	err = b.DB.UpdateTZ(usr, zone.GeoLocation, zone.TZ)
	if err != nil {
		return "", errors.Wrap(err, "failed updating time zone in DB")
	}

	err = b.ReminderManager.Set(usr)
	if err != nil {
		return "", errors.Wrap(err, "failed setting reminder")
	}

	return zone.TZ, nil
}

func (b *TBot) delMemo(usr int64, replyID int, text string) error {
	n, err := b.DB.GetLenMemos(usr)
	if err != nil {
		return err
	}
	val, err := validateInt(text, 1, n)
	if err != nil {
		return b.SendMessage(usr, "I expected a number in the range of 1-"+strconv.Itoa(n), replyID)
	}

	err = b.DB.Delete(usr, val)
	if err != nil {
		return err
	}
	list, err := b.DB.ListFirstMemos(usr, 5, true)
	if err != nil {
		return err
	}
	err = b.SendMemosForToday(usr, &list, nil)
	if err != nil {
		b.Logger.Errorw("failed on sanding today's list:", "err", err)
	}
	states[usr] = idle
	return nil
}

func (b *TBot) markAsDone(usr int64, replyID int, text string) {
	n, err := b.DB.GetLenMemos(usr)
	if err != nil {
		b.Logger.Errorw("failed getting number of memos", "err", err)
		return
	}
	val, err := validateInt(text, 1, n)
	if err != nil {
		b.SendMessage(usr, "I expected a number in the range of 1-"+strconv.Itoa(n), replyID)
		return
	}

	err = b.DB.Done(usr, val)
	if err != nil {
		b.Logger.Errorw("failed marking memo as done", "err", err)
		return
	}

	active, done, err := b.DB.ListAllMemos(usr, true)
	if err != nil {
		b.Logger.Errorw("failed listing memos", "err", err)
	}

	err = b.SendMemosForToday(usr, &active, &done)
	if err != nil {
		b.Logger.Errorw("failed on sanding today's list:", "err", err)
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

func (b *TBot) HandleCommand(message *tg.Message) {
	usr := message.From.ID

	state, ok := states[usr]
	if ok && state != idle {
		b.resetState(usr)
	}

	// TODO: make sure that trailing spaces won't be interpreted as long command
	cmd := message.Command()
	switch cmd {
	case "start":
		err := b.DB.CreateUser(usr)
		if err != nil {
			b.Logger.Errorw("failed initializing user", "err", err)
			return
		}

		err = b.ReminderManager.Set(usr)
		if err != nil {
			b.Logger.Warn("failed setting reminder")
			return
		}

		b.Logger.Info("user has started the bot")

		txt := "Hello, I'm an experienced memo keeper. I write down your memos and remind about them from time to time. By the way, you can tell me when to send you the reminder, so I won't wake you up when you decided to stay in bed ;) Send me your location so I'll know in which time zone is your time"
		err = b.SendMessage(usr, txt, -1)
		if err != nil {
			b.Logger.Errorw("failed on sending hello message:", "err", err)
		}

		list, err := b.DB.ListFullMemos(usr, false)
		if err != nil {
			b.Logger.Errorw("failed listing memos")
			return
		}

		err = b.SendMemosForToday(usr, &list, nil)
		if err != nil {
			b.Logger.Errorw("failed on '/start:'", "err", err)
		}

	case "help":
		txt := `As you may know, I keep your memos in order and periodically remind about them. You can send me a message or one of these commands:
/list - to see short list of your memos
/listall - to see full list of your memos
/ins - to add a new memo at the beginning of the list
/add - to add a new memo at the end of the list
/del - to immediately delete the memo
/done - to mark the memo as done, I'll delete done memos in approximately 24 hours
/remindat - to let me know when to send you a reminder (send location to update time zone)
/settings - to list settings`
		err := b.SendMessage(usr, txt, -1)
		if err != nil {
			b.Logger.Errorw("failed on '/help':", "err", err)
		}

	case "list":
		list, err := b.DB.ListFirstMemos(usr, 5, true)
		if err != nil {
			b.Logger.Errorw("failed listing memos", "err", err)
			return
		}

		if err := b.SendMemosForToday(usr, &list, nil); err != nil {
			b.Logger.Errorw("failed on '/list:'", "err", err)
		}

	case "listall":
		active, done, err := b.DB.ListAllMemos(usr, false)
		if err != nil {
			b.Logger.Errorw("failed listing memos", "err", err)
			return
		}

		if err := b.SendMemosForToday(usr, &active, &done); err != nil {
			b.Logger.Errorw("failed on '/listall:'", "err", err)
		}

	case "add":
		if len(message.Text) > len("/add") {
			text := message.Text[len("/add "):]
			err := b.DB.AddMemo(usr, text)
			if err != nil {
				b.Logger.Errorw("failed adding memo", "err", err)
				return
			}
			err = b.SendMessage(usr, "Added to the end of the list", -1)
			return
		}

		err := b.SendMessage(usr, "What's your memo?", -1)
		if err != nil {
			b.Logger.Errorw("failed adding memo", "err", err)
			return
		}
		states[usr] = add

	case "ins":
		if len(message.Text) > len("/ins") {
			text := message.Text[len("/ins "):]
			err := b.DB.InsertMemo(usr, text)
			if err != nil {
				b.Logger.Errorw("failed inserting memo", "err", err)
				return
			}

			err = b.SendMessage(usr, "Now it's your top priority memo", message.MessageID)
			if err != nil {
				b.Logger.Errorw("failed sending confirmation message", "err", err)
			}
			return
		}

		err := b.SendMessage(usr, "What's your memo?", -1)
		if err != nil {
			b.Logger.Errorw("failed inserting memo", "err", err)
			return
		}
		states[usr] = ins

	case "del":
		if len(message.Text) > len("/del") {
			memo := strings.Trim(message.Text[len("/del "):], " ")
			err := b.delMemo(usr, message.MessageID, memo)
			if err != nil {
				b.Logger.Errorw("failed deleting memo", "err", err)
			}
			return
		}

		list, err := b.DB.ListFullMemos(usr, true)
		if err != nil {
			b.Logger.Errorw("failed listing memos")
			return
		}
		list += "\n\nWhich memo do you want to delete?"
		err = b.SendMemosForToday(usr, &list, nil)
		if err != nil {
			b.Logger.Errorw("failed on '/del':", "err", err)
			return
		}
		states[usr] = del

	case "done":
		if len(message.Text) > len("/done") {
			memo := strings.Trim(message.Text[len("/done "):], " ")
			b.markAsDone(usr, message.MessageID, memo)
			return
		}

		list, err := b.DB.ListFullMemos(usr, false)
		if err != nil {
			b.Logger.Errorw("failed listing memos", "err", err)
			return
		}
		list += "\n\nWhich memo do you want to mark as done?"

		err = b.SendMemosForToday(usr, &list, nil)
		if err != nil {
			b.Logger.Errorw("failed on '/done':", "err", err)
			return
		}
		states[usr] = done

	case "reorder":
		b.SendMessage(usr, "not implemented yet", -1)

	case "remindat":
		if len(message.Text) > len("/remindat") {
			text := message.Text[len("/remindat "):]
			if b.updateReminder(usr, text) != nil {
				b.Logger.Error("failed on '/remindat'")
				return
			}

			err := b.SendMessage(usr, "Gotcha, I'll remind at "+text, -1)
			if err != nil {
				b.Logger.Errorw("failed confirming '/remindat'", "err", err)
				return
			}

			return
		}

		err := b.SendMessage(usr, "Enter hour and minute to send you a reminder in the format HH:MM. Send location to update timezone", -1)
		if err != nil {
			b.Logger.Errorw("failed on '/remindat'", "err", err)
			return
		}
		states[usr] = remindat

	case "settings":
		rp, err := b.DB.GetRemindParams(usr)
		if err != nil {
			b.Logger.Warn("failed getting user config")
		}

		var text string
		if rp == nil {
			b.Logger.Errorw("no remind params found")
			text = "I don't see remind time"
		} else {
			h := rp.RemindAt / 60
			m := rp.RemindAt - h*60
			text = fmt.Sprintf("Reminder time: %02d:%02d (%s).\n\nUse command /remindat to change the time.\nSend location to update time zone.", h, m, rp.TimeZone)
		}

		err = b.SendMessage(usr, text, -1)
		if err != nil {
			b.Logger.Errorw("failed on '/settings':", "err", err)
		}

	default:
		b.SendMessage(usr, "I don't known this command. Use /help to list commands I know", message.MessageID)
	}

	b.Logger.Infof("Command %s was successfully handled", cmd)
}

func (b *TBot) addMemo(usr int64, text string) {
	b.DB.AddMemo(usr, text)
	list, err := b.DB.ListFirstMemos(usr, 5, true)
	if err != nil {
		b.Logger.Errorw("failed adding memo", "err", err)
		return
	}

	err = b.SendMemosForToday(usr, &list, nil)
	if err != nil {
		b.Logger.Errorw("failed on sending today's list:", "err", err)
	} else {
		states[usr] = idle
	}
}

func (b *TBot) insertMemo(usr int64, text string) {
	b.DB.InsertMemo(usr, text)
	list, err := b.DB.ListFirstMemos(usr, 5, true)
	if err != nil {
		b.Logger.Errorw("failed listing memos", "err", err)
		return
	}

	if err := b.SendMemosForToday(usr, &list, nil); err != nil {
		b.Logger.Errorw("failed on sending today's list:", "err", err)
	} else {
		states[usr] = idle
	}
}

func (b *TBot) SendMemosForToday(usr int64, active, done *string) error {
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

	return b.SendMessage(usr, sb.String(), -1)
}

func (b *TBot) resetState(usr int64) {
	b.SendMessage(usr, "You've started another command. Cancelling ongoing operation", -1)
	states[usr] = idle
}

func (b *TBot) updateReminder(usr int64, text string) error {
	parts := strings.Split(text, ":")
	if len(parts) != 2 {
		return errors.New("unknown format")
	}

	hour, err := validateInt(parts[0], 0, 23)
	if err != nil {
		return err
	}

	min, err := validateInt(parts[1], 0, 59)
	if err != nil {
		return err
	}

	t := hour*60 + min
	err = b.DB.SetRemindAt(usr, t)
	if err != nil {
		return err
	}

	return b.ReminderManager.Set(usr)
}

// SendReminder is a callback that's invoked by reminder
func (b *TBot) SendReminder(usr int64) {
	list, err := b.DB.ListFirstMemos(usr, 5, true)
	if err != nil {
		b.Logger.Errorw("failed fetching first memos", "err", err)
		return
	}

	err = b.SendMemosForToday(usr, &list, nil)
	if err != nil {
		b.Logger.Errorw("failed on '/list:'", "err", err)
	}
}
