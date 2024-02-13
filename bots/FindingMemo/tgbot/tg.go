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

type Stage int

const (
	stageIdle Stage = iota
	stageAdd
	stageIns
	stageDel
	stageDone
	stageRemindAt
	stageMakeFirst
	stageMakeLast
)

const (
	txtNoMemosAddNow     = "No memos to remind. Add ones now!"
	txtYourMemosForToday = "Your memos for today:\n"
	txtDoneMemosForToday = "Memos you've done recently:\n"
	txtLineSep           = "\n\n"
)

const (
	txtUnknownCommand              = "I don't known this command. Use /help to list commands I know"
	txtErrorAccessingDatabase      = "Oops, I couldn't get your memos. Retry again. If it didn't help, try again later."
	txtNothingToDelete             = "There's nothing to delete."
	txtNothingToMarkDone           = "There's nothing to mark as done."
	txtNothingMove                 = "There's no memos to move."
	txtFailedDeleMemo              = "I failed to delete the memo. Please retry now or later."
	txtFailedAddMemo               = "I failed to add the memo. Please retry now or later."
	txtFailedInsertMemo            = "I failed to insert the memo. Please retry now or later."
	txtFailedFetchMemos            = "I'm sorry, I couldn't fetch the list of memos."
	txtFailedStartingBot           = "Hey, I couldn't start. Let's try again!"
	txtFailedSetReminder           = "Hm. I couldn't set a reminder."
	txtFailedReorder               = "Argh, I failed to move the memo!"
	txtExpectedValidTimeFormat     = "I expect a valid time in the format HH:MM."
	txtFailedUpdateReminder        = "Oh, no! I couldn't update the reminder! Try again!"
	txtFailedFetchRemindParameters = "I'm sorry, I couldn't fetch the reminder parameters."

	txtAddedMemo    = "Added at the end of the list."
	txtInsertedMemo = "Now it's your top priority memo."

	txtSendMeMemo      = "Send me your memo."
	txtEnterRemindTime = "Enter hour and minute to send you a reminder in the format HH:MM. Send location to update timezone."
)

var (
	errUnknownFormat = errors.New("unknown format")
	errOutOfRange    = errors.New("value is out of range")
)

type state struct {
	stage Stage
}

type Command struct {
	Name string
	Len  int
}

func makeCommand(name string) *Command {
	return &Command{
		Name: name,
		Len:  len(name) + 2, // leading '/' and trailing space
	}
}

var (
	cmdStart     = makeCommand("start")
	cmdAdd       = makeCommand("add")
	cmdIns       = makeCommand("ins")
	cmdDone      = makeCommand("done")
	cmdDel       = makeCommand("del")
	cmdList      = makeCommand("list")
	cmdListAll   = makeCommand("listall")
	cmdRemindAt  = makeCommand("remindat")
	cmdMakeFirst = makeCommand("makefirst")
	cmdMakeLast  = makeCommand("makelast")
	cmdHelp      = makeCommand("help")
	cmdSettings  = makeCommand("settings")
)

type TBot struct {
	Bot             *tg.BotAPI
	DB              *db.Database
	Logger          *zap.SugaredLogger
	ReminderManager *reminder.Manager
	RetryDelay      time.Duration
	RetryAttempts   int
	states          map[int64]*state
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
		states:        make(map[int64]*state),
	}, nil

}

func (b *TBot) HandleMessage(msg *tg.Message) {
	usr := msg.From.ID

	userState := b.states[usr]
	if userState == nil {
		userState = &state{stageIdle}
		b.states[usr] = userState
	}

	switch userState.stage {
	case stageIdle:
		switch {
		case msg.Location != nil:
			loc := msg.Location
			tzName, err := b.updateTimeZone(usr, loc)
			if err != nil {
				b.Logger.Errorw("couldn't update time zone", "err", err)
			}

			txt := fmt.Sprintf("Time zone identified as %s, it will be used in time offset and transition to daylight saving time if any.", tzName)
			b.SendMessage(usr, txt, msg.MessageID)

		case msg.Text != "":
			if err := b.DB.InsertMemo(usr, msg.Text); err != nil {
				b.Logger.Errorw("failed inserting memo", "err", err)
				return
			}

			b.SendMessage(usr, "Looks like you wanted to insert a memo. Added it.", msg.MessageID)

		case msg.Caption != "":
			if err := b.DB.InsertMemo(usr, msg.Caption); err != nil {
				b.Logger.Errorw("failed inserting memo", "err", err)
				return
			}

			b.SendMessage(usr, "Looks like you wanted to insert a memo from a media. Saved the caption as a memo.", msg.MessageID)

		default:
			b.SendMessage(usr, "Unfortunately, I couldn't recognize the memo.", msg.MessageID)
		}

	case stageAdd:
		var txt string
		switch {
		case msg.Text != "":
			txt = msg.Text
		case msg.Caption != "":
			txt = msg.Text
		}

		b.addMemo(usr, txt)
		userState.stage = stageIdle

	case stageIns:
		var txt string
		switch {
		case msg.Text != "":
			txt = msg.Text
		case msg.Caption != "":
			txt = msg.Text
		}

		b.insertMemo(usr, txt)
		userState.stage = stageIdle

	case stageDel:
		b.delMemo(usr, msg.MessageID, msg.Text)
		userState.stage = stageIdle

	case stageDone:
		b.markAsDone(usr, msg.MessageID, msg.Text)
		userState.stage = stageIdle

	case stageRemindAt:
		remindAt := strings.TrimSpace(msg.Text)
		err := b.updateReminder(usr, remindAt)
		if err != nil {
			b.Logger.Errorw("failed updating reminder", "err", err)
			b.SendMessage(usr, txtExpectedValidTimeFormat, msg.MessageID)
			return
		}

		rp, err := b.DB.GetRemindParams(usr)
		if err != nil {
			b.Logger.Warn("failed on setting reminder:")
			b.SendMessage(usr, txtErrorAccessingDatabase, -1)
			return
		}

		txt := fmt.Sprintf("I got it, I'll remind you about your memos at %s in %s time zone", remindAt, rp.TimeZone)
		b.SendMessage(usr, txt, -1)
		userState.stage = stageIdle

	case stageMakeFirst:
		b.reorder(usr, msg.MessageID, msg.Text, b.DB.MakeFirst)
		userState.stage = stageIdle

	case stageMakeLast:
		b.reorder(usr, msg.MessageID, msg.Text, b.DB.MakeLast)
		userState.stage = stageIdle
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
		return err == nil
	})
	if err != nil {
		b.Logger.Errorw("failed sending message", "err", err)
	}
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

func (b *TBot) delMemo(usr int64, replyID int, txt string) {
	n, err := b.DB.GetLenMemos(usr)
	if err != nil {
		b.Logger.Errorw("failed getting number of memos", "err", err)
		b.SendMessage(usr, txtErrorAccessingDatabase, -1)
		return
	}

	if n == 0 {
		b.SendMessage(usr, txtNothingToDelete, -1)
		return
	}

	val, err := validateInt(txt, 1, n)
	if err != nil {
		txt := fmt.Sprintf("I expected a number in the range of 1-%d", n)
		b.SendMessage(usr, txt, replyID)
		return
	}

	err = b.DB.Delete(usr, val)
	if err != nil {
		b.Logger.Errorw("failed deleted memo", "err", err)
		b.SendMessage(usr, txtFailedDeleMemo, replyID)
		return
	}

	active, done, err := b.DB.ListAllMemos(usr, true)
	if err != nil {
		b.Logger.Errorw("failed deleted memo", "err", err)
		b.SendMessage(usr, txtFailedFetchMemos, -1)
		return
	}

	b.SendMemosForToday(usr, active, done)
}

func (b *TBot) markAsDone(usr int64, replyID int, txt string) {
	n, err := b.DB.GetLenMemos(usr)
	if err != nil {
		b.Logger.Errorw("failed getting number of memos", "err", err)
		b.SendMessage(usr, txtErrorAccessingDatabase, replyID)
		return
	}

	if n == 0 {
		b.SendMessage(usr, txtNothingToMarkDone, -1)
	}

	val, err := validateInt(txt, 1, n)
	if err != nil {
		txt := fmt.Sprintf("I expected a number in the range of 1-%d", n)
		b.SendMessage(usr, txt, replyID)
		return
	}

	err = b.DB.Done(usr, val)
	if err != nil {
		b.Logger.Errorw("failed marking memo as done", "err", err)
		b.SendMessage(usr, txtErrorAccessingDatabase, replyID)
		return
	}

	active, done, err := b.DB.ListAllMemos(usr, true)
	if err != nil {
		b.Logger.Errorw("failed listing memos", "err", err)
		b.SendMessage(usr, txtFailedFetchMemos, replyID)
	}

	b.SendMemosForToday(usr, active, done)
}

func validateInt(txt string, min int, max int) (int, error) {
	val, err := strconv.Atoi(txt)
	if err != nil {
		return 0, err
	}

	if val < min || val > max {
		return 0, errOutOfRange
	}
	return val, nil
}

func (b *TBot) HandleCommand(msg *tg.Message) {
	usr := msg.From.ID

	userState := b.states[usr]
	if userState == nil {
		userState = &state{stageIdle}
		b.states[usr] = userState
	}

	if userState.stage != stageIdle {
		// Commands interrupt any ongoing command
		userState.stage = stageIdle
	}

	cmd := msg.Command()
	switch cmd {
	case cmdStart.Name:
		err := b.DB.CreateUser(usr)
		if err != nil {
			b.Logger.Errorw("failed creating user", "err", err)
			b.SendMessage(usr, txtFailedStartingBot, -1)
			return
		}

		err = b.ReminderManager.Set(usr)
		if err != nil {
			b.Logger.Warn("failed setting reminder")
			b.SendMessage(usr, txtFailedSetReminder, -1)
			return
		}

		b.Logger.Info("user has started the bot")

		txt := "Hello, I'm an experienced memo keeper. I write down your memos and remind about them from time to time. By the way, you can tell me when to send you the reminder, so I won't wake you up when you decided to stay in bed ;) Send me your location so I'll know in which time zone is your time"
		b.SendMessage(usr, txt, -1)

		list, err := b.DB.ListFullMemos(usr, false)
		if err != nil {
			b.Logger.Errorw("failed listing memos")
			b.SendMessage(usr, txtFailedFetchMemos, -1)
			return
		}

		b.SendMemosForToday(usr, list, "")

	case cmdHelp.Name:
		txt := `As you may know, I keep your memos in order and periodically remind about them. You can send me a message or one of these commands:
/list - to see short list of your memos
/listall - to see full list of your memos
/ins - to add a new memo at the beginning of the list
/add - to add a new memo at the end of the list
/del - to immediately delete the memo
/done - to mark the memo as done, I'll hide done memos in approximately 24 hours
/remindat - to let me know when to send you a reminder (send location to update time zone)
/makefirst - to move a memo to the beginning of the list
/makelast - to move a memo to the end of the list
/settings - to list settings`
		b.SendMessage(usr, txt, -1)

	case cmdList.Name:
		list, err := b.DB.ListFirstMemos(usr, 5, true)
		if err != nil {
			b.Logger.Errorw("failed listing memos", "err", err)
			b.SendMessage(usr, txtFailedFetchMemos, -1)
			return
		}

		b.SendMemosForToday(usr, list, "")

	case cmdListAll.Name:
		active, done, err := b.DB.ListAllMemos(usr, false)
		if err != nil {
			b.Logger.Errorw("failed listing memos", "err", err)
			b.SendMessage(usr, txtFailedFetchMemos, -1)
			return
		}

		b.SendMemosForToday(usr, active, done)

	case cmdAdd.Name:
		if len(msg.Text) > cmdAdd.Len {
			txt := strings.TrimSpace(msg.Text[cmdAdd.Len:])
			b.addMemo(usr, txt)
			return
		}

		err := b.SendMessage(usr, txtSendMeMemo, -1)
		if err != nil {
			return
		}

		userState.stage = stageAdd

	case cmdIns.Name:
		if len(msg.Text) > cmdIns.Len {
			txt := strings.TrimSpace(msg.Text[cmdIns.Len:])
			b.insertMemo(usr, txt)
			return
		}

		err := b.SendMessage(usr, txtSendMeMemo, -1)
		if err != nil {
			return
		}

		userState.stage = stageIns

	case cmdDel.Name:
		if len(msg.Text) > cmdDel.Len {
			txt := strings.TrimSpace(msg.Text[cmdDel.Len:])
			b.delMemo(usr, msg.MessageID, txt)
			return
		}

		list, err := b.DB.ListFullMemos(usr, true)
		if err != nil {
			b.Logger.Errorw("failed listing memos")
			b.SendMessage(usr, txtFailedFetchMemos, -1)
			return
		}
		list += "\n\nWhich memo do you want to delete?"

		err = b.SendMemosForToday(usr, list, "")
		if err != nil {
			return
		}

		userState.stage = stageDel

	case cmdDone.Name:
		if len(msg.Text) > cmdDone.Len {
			txt := strings.TrimSpace(msg.Text[cmdDel.Len:])
			b.markAsDone(usr, msg.MessageID, txt)
			return
		}

		list, err := b.DB.ListFullMemos(usr, false)
		if err != nil {
			b.Logger.Errorw("failed listing memos", "err", err)
			b.SendMessage(usr, txtFailedFetchMemos, -1)
			return
		}
		list += "\n\nWhich memo do you want to mark as done?"

		err = b.SendMemosForToday(usr, list, "")
		if err != nil {
			return
		}

		userState.stage = stageDone

	case cmdMakeFirst.Name:
		if len(msg.Text) > cmdMakeFirst.Len {
			txt := strings.TrimSpace(msg.Text[cmdMakeFirst.Len:])
			b.reorder(usr, msg.MessageID, txt, b.DB.MakeFirst)
			return
		}

		list, err := b.DB.ListFullMemos(usr, false)
		if err != nil {
			b.Logger.Errorw("failed listing memos", "err", err)
			b.SendMessage(usr, txtFailedFetchMemos, -1)
			return
		}
		list += "\n\nWhich memo do you want to move to the beginning of the list?"

		err = b.SendMemosForToday(usr, list, "")
		if err != nil {
			return
		}

		userState.stage = stageMakeFirst

	case cmdMakeLast.Name:
		if len(msg.Text) > cmdMakeLast.Len {
			txt := strings.TrimSpace(msg.Text[cmdMakeLast.Len:])
			b.reorder(usr, msg.MessageID, txt, b.DB.MakeLast)
			return
		}

		list, err := b.DB.ListFullMemos(usr, false)
		if err != nil {
			b.Logger.Errorw("failed listing memos", "err", err)
			b.SendMessage(usr, txtFailedFetchMemos, -1)
			return
		}
		list += "\n\nWhich memo do you want to move to the end of the list?"

		err = b.SendMemosForToday(usr, list, "")
		if err != nil {
			return
		}

		userState.stage = stageMakeLast

	case cmdRemindAt.Name:
		if len(msg.Text) > cmdRemindAt.Len {
			txt := msg.Text[cmdRemindAt.Len:]
			if b.updateReminder(usr, txt) != nil {
				b.Logger.Error("failed updating reminder")
				b.SendMessage(usr, txtFailedUpdateReminder, -1)
				return
			}

			b.SendMessage(usr, "Gotcha, I'll remind at "+txt, -1)
			return
		}

		err := b.SendMessage(usr, txtEnterRemindTime, -1)
		if err != nil {
			return
		}

		userState.stage = stageRemindAt

	case cmdSettings.Name:
		rp, err := b.DB.GetRemindParams(usr)
		if err != nil {
			b.Logger.Warn("failed getting user config")
			b.SendMessage(usr, txtFailedFetchRemindParameters, -1)
			return
		}

		var txt string
		if rp == nil {
			b.Logger.Errorw("no remind params found")
			txt = "I don't see remind time"
		} else {
			h := rp.RemindAt / 60
			m := rp.RemindAt - h*60
			txt = fmt.Sprintf("Reminder time: %02d:%02d (%s).\n\nUse command '/%s' to change the time.\nYou can send location to update the time zone.", h, m, cmdRemindAt.Name, rp.TimeZone)
		}
		b.SendMessage(usr, txt, -1)

	default:
		b.SendMessage(usr, txtUnknownCommand, msg.MessageID)
	}

	b.Logger.Infof("Command /%s was successfully handled", cmd)
}

func (b *TBot) addMemo(usr int64, txt string) {
	err := b.DB.AddMemo(usr, txt)
	if err != nil {
		b.Logger.Errorw("failed adding memo", "err", err)
		b.SendMessage(usr, txtFailedAddMemo, -1)
		return
	}

	list, err := b.DB.ListFirstMemos(usr, 5, true)
	if err != nil {
		b.Logger.Errorw("failed listing memos", "err", err)
		b.SendMessage(usr, txtFailedFetchMemos, -1)
		return
	}

	b.SendMemosForToday(usr, list, "")
}

func (b *TBot) insertMemo(usr int64, txt string) {
	err := b.DB.InsertMemo(usr, txt)
	if err != nil {
		b.Logger.Errorw("failed inserting memo", "err", err)
		b.SendMessage(usr, txtFailedInsertMemo, -1)
		return
	}

	list, err := b.DB.ListFirstMemos(usr, 5, true)
	if err != nil {
		b.Logger.Errorw("failed listing memos", "err", err)
		b.SendMessage(usr, txtFailedFetchMemos, -1)
		return
	}

	b.SendMemosForToday(usr, list, "")
}

func (b *TBot) SendMemosForToday(usr int64, active, done string) error {
	var sb strings.Builder

	n := len(txtNoMemosAddNow) + len(txtYourMemosForToday) + len(txtLineSep) + len(active) + len(done)
	sb.Grow(n)

	if len(active) == 0 {
		sb.WriteString(txtNoMemosAddNow)
	} else {
		sb.WriteString(txtYourMemosForToday)
		sb.WriteString(active)
	}

	if len(done) != 0 {
		sb.WriteString(txtLineSep)
		sb.WriteString(txtDoneMemosForToday)
		sb.WriteString(done)
	}

	return b.SendMessage(usr, sb.String(), -1)
}

func (b *TBot) updateReminder(usr int64, txt string) error {
	parts := strings.Split(txt, ":")
	if len(parts) != 2 {
		return errUnknownFormat
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
		b.SendMessage(usr, txtFailedFetchMemos, -1)
		return
	}

	b.SendMemosForToday(usr, list, "")
}

func (b *TBot) reorder(usr int64, replyID int, txt string, f func(int64, int) error) {
	n, err := b.DB.GetLenMemos(usr)
	if err != nil {
		b.Logger.Errorw("failed getting number of memos", "err", err)
		b.SendMessage(usr, txtErrorAccessingDatabase, replyID)
		return
	}

	if n == 0 {
		b.SendMessage(usr, txtNothingMove, -1)
	}

	val, err := validateInt(txt, 1, n)
	if err != nil {
		txt := fmt.Sprintf("I expected a number in the range of 1-%d", n)
		b.SendMessage(usr, txt, replyID)
		return
	}

	err = f(usr, val)
	if err != nil {
		b.Logger.Errorw("failed reordering memo", "err", err)
		b.SendMessage(usr, txtFailedReorder, replyID)
		return
	}

	active, done, err := b.DB.ListAllMemos(usr, true)
	if err != nil {
		b.Logger.Errorw("failed listing memos", "err", err)
		b.SendMessage(usr, txtFailedFetchMemos, replyID)
	}

	b.SendMemosForToday(usr, active, done)
}
