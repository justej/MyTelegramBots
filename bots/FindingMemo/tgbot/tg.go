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

const numShortCount = 5
const numAssumedAvgMemo = 100

const (
	cbqShowAll = "cbqShowAll"
	cbqRetry   = "cbqRetry"
)

var (
	keyboardShowAll = tg.NewInlineKeyboardMarkup(tg.NewInlineKeyboardRow(tg.NewInlineKeyboardButtonData("Show all", cbqShowAll)))
	keyboardRetry   = tg.NewInlineKeyboardMarkup(tg.NewInlineKeyboardRow(tg.NewInlineKeyboardButtonData("Retry", cbqRetry)))
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
	txtWelcomeMessage = "Hello, I'm an experienced memo keeper. I write down your memos and remind about them from time to time. By the way, you can tell me when to send you the reminder, so I won't wake you up when you decided to stay in bed ;) Send me your location so I'll know in which time zone is your time"
	txtHelpMessage    = `As you may know, I keep your memos in order and periodically remind about them. You can send me a message or one of these commands:
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
	txtUnknownCommand              = "I don't known this command. Use /help to list commands I know"
	txtDoNotUnderstandWhatHappened = "E-mm, I don't understood what have just happened"
	txtWhatWasThatText             = "Looks like you wanted to insert a memo from a media. Saved the caption as a memo"
	txtWhatWasThatCaption          = "Looks like you wanted to insert a memo. Saved the caption as a memo"
	txtErrorAccessingDatabase      = "Oops, I couldn't get your memos. Retry again. If it didn't help, try again later"
	txtNothingToDelete             = "There's nothing to delete"
	txtNothingToMarkDone           = "There's nothing to mark as done"
	txtNothingToMove               = "There's no memos to move"
	txtFailedDeleMemo              = "I failed to delete the memo. Please retry now or later"
	txtFailedAddMemo               = "I failed to add the memo. Please retry now or later"
	txtFailedInsertMemo            = "I failed to insert the memo. Please retry now or later"
	txtFailedFetchMemos            = "I'm sorry, I couldn't fetch the list of memos"
	txtFailedStartingBot           = "Hey, I couldn't start. Let's try again!"
	txtFailedSetReminder           = "Hm. I couldn't set a reminder"
	txtFailedReorder               = "Argh, I failed to move the memo!"
	txtFailedUpdateReminder        = "Oh, no! I couldn't update the reminder! Try again!"
	txtFailedFetchRemindParameters = "I'm sorry, I couldn't fetch the reminder parameters"
	txtExpectedValidTimeFormat     = "I expect a valid time in the format HH:MM. Please repeat the command and enter correct value"
	txtNoRemindTimeHere            = "I don't see remind time here"
	txtWhatToDelete                = "Which memo do you want to delete?"
	txtWhatToMarkDone              = "Which memo do you want to mark as done?"
	txtWhatToMakeFirst             = "Which memo do you want to move to the beginning of the list?"
	txtWhatToMakeLast              = "Which memo do you want to move to the end of the list?"
	txtGotRemindTime               = "Gotcha, I'll remind at "
	txtSendMeMemo                  = "Send me your memo"
	txtEnterRemindTime             = "Enter hour and minute to send you a reminder in the format HH:MM. Send location to update timezone"
	txtNoActiveMemos               = "Congrats, you don't have any active memos at the moment!\n"
	txtYourActiveMemos             = "Your active memos:\n"
	txtYourDoneMemos               = "\nMemos you've recently done:\n"
	txtYourDeletedMemos            = "\nMemos you've recently deleted:\n"

	fmtTimeZoneAccepted      = "Time zone identified as %s, it will be used in time offset and transition to daylight saving time if any"
	fmtRemindTimeUpdated     = "I got it, I'll remind you about your memos at %s in %s time zone"
	fmtYourSettings          = "Reminder time: %02d:%02d (%s).\n\nUse command '/%s' to change the time.\nYou can send location to update the time zone"
	fmtNumberInRangeExpected = "I expected a number in the range of 1-%d. Please repeat the command and enter correct value"
	fmtMemo                  = "[<code>%d</code>] %s\n"
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

			txt := fmt.Sprintf(fmtTimeZoneAccepted, tzName)
			b.SendMessage(usr, txt, msg.MessageID, nil)

		case msg.Text != "":
			if err := b.DB.InsertMemo(usr, msg.Text); err != nil {
				b.Logger.Errorw("failed inserting memo", "err", err)
				return
			}

			b.SendMessage(usr, txtWhatWasThatText, msg.MessageID, nil)

		case msg.Caption != "":
			if err := b.DB.InsertMemo(usr, msg.Caption); err != nil {
				b.Logger.Errorw("failed inserting memo", "err", err)
				return
			}

			b.SendMessage(usr, txtWhatWasThatCaption, msg.MessageID, nil)

		default:
			b.SendMessage(usr, txtDoNotUnderstandWhatHappened, msg.MessageID, nil)
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
			b.SendMessage(usr, txtExpectedValidTimeFormat, msg.MessageID, nil)
			return
		}

		rp, err := b.DB.GetRemindParams(usr)
		if err != nil {
			b.Logger.Warn("failed on setting reminder:")
			b.SendMessage(usr, txtErrorAccessingDatabase, msg.MessageID, nil)
			return
		}

		txt := fmt.Sprintf(fmtRemindTimeUpdated, remindAt, rp.TimeZone)
		b.SendMessage(usr, txt, -1, nil)
		userState.stage = stageIdle

	case stageMakeFirst:
		b.reorder(usr, msg.MessageID, msg.Text, b.DB.MakeFirst)
		userState.stage = stageIdle

	case stageMakeLast:
		b.reorder(usr, msg.MessageID, msg.Text, b.DB.MakeLast)
		userState.stage = stageIdle
	}
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
			b.SendMessage(usr, txtFailedStartingBot, msg.MessageID, nil)
			return
		}

		err = b.ReminderManager.Set(usr)
		if err != nil {
			b.Logger.Warn("failed setting reminder")
			b.SendMessage(usr, txtFailedSetReminder, msg.MessageID, nil)
			return
		}

		b.Logger.Info("user has started the bot")

		b.SendMessage(usr, txtWelcomeMessage, -1, nil)

		memos, err := b.DB.GetAllMemos(usr, true)
		if err != nil {
			b.Logger.Errorw("failed listing memos", "err", err)
			b.SendMessage(usr, txtFailedFetchMemos, msg.MessageID, nil)
			return
		}

		b.sendMemosForToday(usr, memos, true)

	case cmdHelp.Name:
		b.SendMessage(usr, txtHelpMessage, -1, nil)

	case cmdList.Name:
		memos, err := b.DB.GetAllMemos(usr, true)
		if err != nil {
			b.Logger.Errorw("failed listing memos", "err", err)
			b.SendMessage(usr, txtFailedFetchMemos, msg.MessageID, nil)
			return
		}

		b.sendMemosForToday(usr, memos, false)

	case cmdListAll.Name:
		memos, err := b.DB.GetAllMemos(usr, true)
		if err != nil {
			b.Logger.Errorw("failed listing memos", "err", err)
			b.SendMessage(usr, txtFailedFetchMemos, msg.MessageID, nil)
			return
		}

		b.sendMemosForToday(usr, memos, true)

	case cmdAdd.Name:
		if len(msg.Text) > cmdAdd.Len {
			txt := strings.TrimSpace(msg.Text[cmdAdd.Len:])
			b.addMemo(usr, txt)
			return
		}

		err := b.SendMessage(usr, txtSendMeMemo, -1, nil)
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

		if b.SendMessage(usr, txtSendMeMemo, -1, nil) != nil {
			return
		}

		userState.stage = stageIns

	case cmdDel.Name:
		if len(msg.Text) > cmdDel.Len {
			txt := strings.TrimSpace(msg.Text[cmdDel.Len:])
			b.delMemo(usr, msg.MessageID, txt)
			return
		}

		memos, err := b.DB.GetAllMemos(usr, true)
		if err != nil {
			b.Logger.Errorw("failed listing memos", "err", err)
			b.SendMessage(usr, txtFailedFetchMemos, msg.MessageID, nil)
			return
		}

		if len(memos) == 0 {
			b.SendMessage(usr, txtNothingToDelete, -1, nil)
			return
		}

		b.sendMemosForToday(usr, memos, true)
		if b.SendMessage(usr, txtWhatToDelete, msg.MessageID, nil) != nil {
			return
		}

		userState.stage = stageDel

	case cmdDone.Name:
		if len(msg.Text) > cmdDone.Len {
			txt := strings.TrimSpace(msg.Text[cmdDel.Len:])
			b.markAsDone(usr, msg.MessageID, txt)
			return
		}

		memos, err := b.DB.GetAllMemos(usr, true)
		if err != nil {
			b.Logger.Errorw("failed listing memos", "err", err)
			b.SendMessage(usr, txtFailedFetchMemos, msg.MessageID, nil)
			return
		}

		if len(memos) == 0 {
			b.SendMessage(usr, txtNothingToMarkDone, -1, nil)
			return
		}

		b.sendMemosForToday(usr, memos, true)
		if b.SendMessage(usr, txtWhatToMarkDone, -1, nil) != nil {
			return
		}

		userState.stage = stageDone

	case cmdMakeFirst.Name:
		if len(msg.Text) > cmdMakeFirst.Len {
			txt := strings.TrimSpace(msg.Text[cmdMakeFirst.Len:])
			b.reorder(usr, msg.MessageID, txt, b.DB.MakeFirst)
			return
		}

		memos, err := b.DB.GetAllMemos(usr, true)
		if err != nil {
			b.Logger.Errorw("failed listing memos", "err", err)
			b.SendMessage(usr, txtFailedFetchMemos, msg.MessageID, nil)
			return
		}

		if len(memos) == 0 {
			b.SendMessage(usr, txtNothingToMove, -1, nil)
			return
		}

		b.sendMemosForToday(usr, memos, true)
		if b.SendMessage(usr, txtWhatToMakeFirst, -1, nil) != nil {
			return
		}

		userState.stage = stageMakeFirst

	case cmdMakeLast.Name:
		if len(msg.Text) > cmdMakeLast.Len {
			txt := strings.TrimSpace(msg.Text[cmdMakeLast.Len:])
			b.reorder(usr, msg.MessageID, txt, b.DB.MakeLast)
			return
		}

		memos, err := b.DB.GetAllMemos(usr, true)
		if err != nil {
			b.Logger.Errorw("failed listing memos", "err", err)
			b.SendMessage(usr, txtFailedFetchMemos, msg.MessageID, nil)
			return
		}

		if len(memos) == 0 {
			b.SendMessage(usr, txtNothingToMove, -1, nil)
			return
		}

		b.sendMemosForToday(usr, memos, true)
		if b.SendMessage(usr, txtWhatToMakeLast, -1, nil) != nil {
			return
		}

		userState.stage = stageMakeLast

	case cmdRemindAt.Name:
		if len(msg.Text) > cmdRemindAt.Len {
			remindTimeTxt := strings.TrimSpace(msg.Text[cmdRemindAt.Len:])
			if b.updateReminder(usr, remindTimeTxt) != nil {
				b.Logger.Error("failed updating reminder")
				b.SendMessage(usr, txtFailedUpdateReminder, msg.MessageID, nil)
				return
			}

			b.SendMessage(usr, txtGotRemindTime+remindTimeTxt, -1, nil)
			return
		}

		if b.SendMessage(usr, txtEnterRemindTime, -1, nil) != nil {
			return
		}

		userState.stage = stageRemindAt

	case cmdSettings.Name:
		rp, err := b.DB.GetRemindParams(usr)
		if err != nil {
			b.Logger.Warn("failed getting user config")
			b.SendMessage(usr, txtFailedFetchRemindParameters, msg.MessageID, nil)
			return
		}

		var txt string
		if rp == nil {
			b.Logger.Errorw("no remind params found")
			txt = txtNoRemindTimeHere
		} else {
			h := rp.RemindAt / 60
			m := rp.RemindAt - h*60
			txt = fmt.Sprintf(fmtYourSettings, h, m, cmdRemindAt.Name, rp.TimeZone)
		}

		b.SendMessage(usr, txt, -1, nil)

	default:
		b.SendMessage(usr, txtUnknownCommand, msg.MessageID, nil)
	}
}

func (b *TBot) HandleCallback(cbq *tg.CallbackQuery) {
	usr := cbq.From.ID

	userState := b.states[usr]
	if userState == nil {
		userState = &state{stageIdle}
		b.states[usr] = userState
	}

	switch cbq.Data {
	case cbqShowAll, cbqRetry:
		memos, err := b.DB.GetAllMemos(usr, true)
		if err != nil {
			b.Logger.Errorw("failed listing memos", "err", err)
			b.ReplaceMessage(usr, txtFailedFetchMemos, cbq.Message.MessageID, &keyboardRetry)
			return
		}

		active, done, deleted := groupByState(memos)
		var sb strings.Builder
		formatAllMemos(&sb, active, done, deleted)
		b.ReplaceMessage(usr, sb.String(), cbq.Message.MessageID, nil)
	}
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
	n, err := b.DB.GetActiveMemoCount(usr)
	if err != nil {
		b.Logger.Errorw("failed getting number of memos", "err", err)
		b.SendMessage(usr, txtErrorAccessingDatabase, -1, nil)
		return
	}

	if n == 0 {
		b.SendMessage(usr, txtNothingToDelete, -1, nil)
		return
	}

	val, err := validateInt(txt, 1, n)
	if err != nil {
		txt := fmt.Sprintf(fmtNumberInRangeExpected, n)
		b.SendMessage(usr, txt, replyID, nil)
		return
	}

	err = b.DB.DeleteMemo(usr, val)
	if err != nil {
		b.Logger.Errorw("failed deleted memo", "err", err)
		b.SendMessage(usr, txtFailedDeleMemo, replyID, nil)
		return
	}

	memos, err := b.DB.GetAllMemos(usr, true)
	if err != nil {
		b.Logger.Errorw("failed listing memos", "err", err)
		b.SendMessage(usr, txtFailedFetchMemos, -1, nil)
		return
	}

	b.sendMemosForToday(usr, memos, false)
}

func (b *TBot) markAsDone(usr int64, replyID int, txt string) {
	n, err := b.DB.GetActiveMemoCount(usr)
	if err != nil {
		b.Logger.Errorw("failed getting number of memos", "err", err)
		b.SendMessage(usr, txtErrorAccessingDatabase, replyID, nil)
		return
	}

	if n == 0 {
		b.SendMessage(usr, txtNothingToMarkDone, -1, nil)
	}

	val, err := validateInt(txt, 1, n)
	if err != nil {
		txt := fmt.Sprintf(fmtNumberInRangeExpected, n)
		b.SendMessage(usr, txt, replyID, nil)
		return
	}

	err = b.DB.MarkAsDone(usr, val)
	if err != nil {
		b.Logger.Errorw("failed marking memo as done", "err", err)
		b.SendMessage(usr, txtErrorAccessingDatabase, replyID, nil)
		return
	}

	memos, err := b.DB.GetAllMemos(usr, true)
	if err != nil {
		b.Logger.Errorw("failed listing memos", "err", err)
		b.SendMessage(usr, txtFailedFetchMemos, -1, nil)
		return
	}

	b.sendMemosForToday(usr, memos, false)
}

func (b *TBot) addMemo(usr int64, txt string) {
	err := b.DB.AddMemo(usr, txt)
	if err != nil {
		b.Logger.Errorw("failed adding memo", "err", err)
		b.SendMessage(usr, txtFailedAddMemo, -1, nil)
		return
	}

	memos, err := b.DB.GetAllMemos(usr, true)
	if err != nil {
		b.Logger.Errorw("failed listing memos", "err", err)
		b.SendMessage(usr, txtFailedFetchMemos, -1, nil)
		return
	}

	b.sendMemosForToday(usr, memos, false)
}

func (b *TBot) insertMemo(usr int64, txt string) {
	err := b.DB.InsertMemo(usr, txt)
	if err != nil {
		b.Logger.Errorw("failed inserting memo", "err", err)
		b.SendMessage(usr, txtFailedInsertMemo, -1, nil)
		return
	}

	memos, err := b.DB.GetAllMemos(usr, true)
	if err != nil {
		b.Logger.Errorw("failed listing memos", "err", err)
		b.SendMessage(usr, txtFailedFetchMemos, -1, nil)
		return
	}

	b.sendMemosForToday(usr, memos, false)
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
	memos, err := b.DB.GetAllMemos(usr, true)
	if err != nil {
		b.Logger.Errorw("failed listing memos", "err", err)
		b.SendMessage(usr, txtFailedFetchMemos, -1, nil)
		return
	}

	b.sendMemosForToday(usr, memos, false)
}

func (b *TBot) reorder(usr int64, replyID int, txt string, f func(int64, int) error) {
	n, err := b.DB.GetActiveMemoCount(usr)
	if err != nil {
		b.Logger.Errorw("failed getting number of memos", "err", err)
		b.SendMessage(usr, txtErrorAccessingDatabase, replyID, nil)
		return
	}

	if n == 0 {
		b.SendMessage(usr, txtNothingToMove, -1, nil)
	}

	val, err := validateInt(txt, 1, n)
	if err != nil {
		txt := fmt.Sprintf(fmtNumberInRangeExpected, n)
		b.SendMessage(usr, txt, replyID, nil)
		return
	}

	err = f(usr, val)
	if err != nil {
		b.Logger.Errorw("failed reordering memo", "err", err)
		b.SendMessage(usr, txtFailedReorder, replyID, nil)
		return
	}

	memos, err := b.DB.GetAllMemos(usr, true)
	if err != nil {
		b.Logger.Errorw("failed listing memos", "err", err)
		b.SendMessage(usr, txtFailedFetchMemos, -1, nil)
		return
	}

	b.sendMemosForToday(usr, memos, false)
}

func (b *TBot) SendMessage(usr int64, txt string, replyTo int, kbMarkup *tg.InlineKeyboardMarkup) error {
	m := tg.NewMessage(usr, txt)
	if replyTo >= 0 {
		m.ReplyToMessageID = replyTo
	}
	m.ParseMode = tg.ModeHTML
	m.DisableWebPagePreview = true
	if kbMarkup != nil {
		m.BaseChat.ReplyMarkup = kbMarkup
	}

	var err error
	bot.RobustExecute(b.RetryAttempts, b.RetryDelay, func() bool {
		_, err = b.Bot.Request(m)
		return err == nil
	})
	if err != nil {
		b.Logger.Errorw("failed sending message", "err", err)
	}
	return err
}

func (b TBot) ReplaceMessage(usr int64, txt string, msgID int, kbMarkup *tg.InlineKeyboardMarkup) bool {
	updText := tg.EditMessageTextConfig{
		BaseEdit: tg.BaseEdit{
			ChatID:      usr,
			MessageID:   msgID,
			ReplyMarkup: kbMarkup,
		},
		DisableWebPagePreview: true,
		ParseMode:             tg.ModeHTML,
		Text:                  txt,
	}

	var err error
	ok := bot.RobustExecute(b.RetryAttempts, b.RetryDelay, func() bool {
		_, err := b.Bot.Request(updText)
		if err != nil && strings.HasPrefix(err.Error(), "Bad Request: message is not modified") {
			err = nil
		}
		return err == nil
	})
	if !ok {
		b.Logger.Errorw("failed updating message text", "err", err)
	}

	return ok
}

func (b *TBot) sendMemosForToday(usr int64, memos []db.Memo, showAll bool) error {
	activeMemos, doneMemos, deletedMemos := groupByState(memos)

	var sb strings.Builder
	var kb *tg.InlineKeyboardMarkup
	if showAll {
		formatAllMemos(&sb, activeMemos, doneMemos, deletedMemos)
	} else {
		formatFirstMemos(&sb, activeMemos)
		if len(activeMemos) > numShortCount || len(doneMemos) > 0 || len(deletedMemos) > 0 {
			kb = &keyboardShowAll
		}
	}

	return b.SendMessage(usr, sb.String(), -1, kb)
}

func formatAllMemos(sb *strings.Builder, activeMemos []string, doneMemos []string, deletedMemos []string) {
	sb.Grow(numAssumedAvgMemo * (len(activeMemos) + len(doneMemos) + len(deletedMemos)))

	if len(activeMemos) == 0 {
		sb.WriteString(txtNoActiveMemos)
	} else {
		sb.WriteString(txtYourActiveMemos)
	}

	for i, txt := range activeMemos {
		sb.WriteString(fmt.Sprintf(fmtMemo, i+1, txt))
	}

	if len(doneMemos) > 0 {
		sb.WriteString(txtYourDoneMemos)
		for i, txt := range doneMemos {
			sb.WriteString(fmt.Sprintf(fmtMemo, i+1, txt))
		}
	}

	if len(deletedMemos) > 0 {
		sb.WriteString(txtYourDeletedMemos)
		for i, txt := range deletedMemos {
			sb.WriteString(fmt.Sprintf(fmtMemo, i+1, txt))
		}
	}
}

func formatFirstMemos(sb *strings.Builder, activeMemos []string) {
	n := numShortCount
	if len(activeMemos) < numShortCount {
		n = len(activeMemos)
	}

	if len(activeMemos) == 0 {
		sb.WriteString(txtNoActiveMemos)
	} else {
		sb.WriteString(txtYourActiveMemos)
	}

	for i, txt := range activeMemos[:n] {
		sb.WriteString(fmt.Sprintf(fmtMemo, i+1, txt))
	}
}

func groupByState(memos []db.Memo) ([]string, []string, []string) {
	var activeMemos []string
	var doneMemos []string
	var deletedMemos []string
	for _, m := range memos {
		switch m.State {
		case db.MemoStateActive:
			activeMemos = append(activeMemos, m.Text)
		case db.MemoStateDone:
			doneMemos = append(doneMemos, m.Text)
		case db.MemoStateDeleted:
			deletedMemos = append(deletedMemos, m.Text)
		}
	}
	return activeMemos, doneMemos, deletedMemos
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
