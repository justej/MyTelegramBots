package tgbot

import (
	"botfarm/bots/AlainDelon/db"
	"botfarm/bots/AlainDelon/log"
	"fmt"
	"strconv"
	"strings"
	"time"

	tg "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const (
	introductionMessage = "Let me introduce myself. I'm Alain Fabien Maurice Marcel Delon bot, I take care of movies. With my help you can add movies you would like to watch, rate your and other's movies."
	mainMessage         = "So what you're gonna do?"
	startMessage        = introductionMessage + "\n\n" + mainMessage
	helpMessage         = introductionMessage + `

For CLI nerds there's a bunch of one-line commands. These commands, however, require following the precise message format. []'d values are optional.
/add - add new movie. Format: /add "<title>"/["<alternative title>"][(<year>)]
/del - delete movie. Format: /del <movie ID>
/rate - rate movie (1-5). Format: /rate <movie ID> <rating>
/unrate - unrate movie. Format: /unrate <movie ID>
/amazeme - show a random unseen movie
/unseen - list unseen movies
/seen - list seen movies
/all - list both unseen and seen movies
/top - list top 10 movies
/latest - list 10 latest movies
/find - find movie by name or year. Format: /find <case-insensitive name or year>
/help - this help`
)

var bot *tg.BotAPI

func Init(token string) {
	b, err := tg.NewBotAPI(token)
	if err != nil {
		log.Error(0, err, "Failed to initialize bot")
		return
	}

	b.Debug = false
	log.Infof("Successfully initialized bot as %s", b.Self.UserName)

	bot = b
}

func Run() {
	if bot == nil {
		log.Error(0, nil, "Bot can't run")
		return
	}

	uCfg := tg.NewUpdate(0)
	uCfg.Timeout = 60

	for u := range bot.GetUpdatesChan(uCfg) {
		if u.Message != nil {
			if u.Message.IsCommand() {
				go handleCommand(&u)
			} else {
				go handleUpdate(&u)
			}
		}
		if u.CallbackQuery != nil {
			go handleCallbackQuery(&u)
		}
	}

}

type command struct {
	name string
	len  int
}

var (
	cmdStart = makeCommand("start")
)

type stage uint

type state struct {
	stage
	mainMessageID int
	movie         db.Movie
}

var states = make(map[int64]*state)

const (
	stageIdle stage = iota
	stageAdd
	stageDel
	stageRate
	stageUnrate
	stageAmazeMe
	stageFind
	stageList
	stageHelp

	stageTitle
	stageAltTitle
	stageYear

	stageChooseDel
	stageChooseRate
	stageChooseUnrate
)

func makeCommand(c string) *command {
	return &command{
		name: c,
		len:  len(c) + 2,
	}
}

func handleCommand(upd *tg.Update) {
	msg := upd.Message
	cmd := msg.Command()
	usr := msg.From.ID
	cht := msg.Chat.ID

	if states[usr] == nil {
		states[usr] = &state{stage: stageIdle}
	}

	switch cmd {
	case cmdStart.name:
		err := db.AddUser(usr, cht)
		if err != nil {
			log.Error(usr, err, "failed adding user")
			return
		}

		m := tg.NewMessage(cht, startMessage)
		m.ReplyMarkup = mainKeyboard
		if _, err := bot.Send(m); err != nil {
			log.Error(usr, err, "failed sending response to user")
			return
		}

		states[usr] = &state{mainMessageID: msg.MessageID}
	}

	dm := tg.NewDeleteMessage(cht, msg.MessageID)
	// ignore bot.Send errors because it always fails to deserialize response
	bot.Send(dm)
}

func joinMovies(movies []*db.Movie, showID bool, prefix ...string) string {
	prefixLen := 0
	for _, s := range prefix {
		prefixLen += len(s)
	}

	var sb strings.Builder
	sb.Grow(len(movies)*20 + prefixLen)
	sb.WriteString(strings.Join(prefix, ""))

	for _, movie := range movies {
		line := formatMovie(movie, showID)
		sb.WriteString(line)
	}

	return sb.String()
}

func formatMovie(mv *db.Movie, showID bool) string {
	fmtStr := []string{}
	args := []any{}
	if showID {
		fmtStr = append(fmtStr, "%d: ")
		args = append(args, mv.ID)
	}

	fmtStr = append(fmtStr, "\"%s\"")
	args = append(args, mv.Title)

	if len(mv.AltTitle) > 0 {
		fmtStr = append(fmtStr, "/\"%s\"")
		args = append(args, mv.AltTitle)
	}

	if mv.Year > 0 {
		fmtStr = append(fmtStr, " (%d)")
		args = append(args, mv.Year)
	}

	if mv.Rating < 0 {
		fmtStr = append(fmtStr, " - no ⭐ yet\n")
	} else {
		fmtStr = append(fmtStr, " - %.2f ⭐\n")
		args = append(args, mv.Rating)
	}

	return fmt.Sprintf(strings.Join(fmtStr, ""), args...)
}

func handleUpdate(upd *tg.Update) {
	msg := upd.Message
	txt := strings.TrimSpace(msg.Text)
	usr := msg.From.ID
	cht := msg.Chat.ID

	state := states[usr]

	var keyboard *tg.InlineKeyboardMarkup

	switch state.stage {
	case stageTitle:
		keyboard = &keyboardAdd
		state.stage = stageAdd
		state.movie.Title = txt

	case stageAltTitle:
		keyboard = &keyboardAdd
		state.stage = stageAdd
		state.movie.AltTitle = txt

	case stageYear:
		keyboard = &keyboardAdd
		state.stage = stageAdd

		year, err := strconv.Atoi(txt)
		if err != nil || year < 1850 || year > time.Now().UTC().Year()+2 {
			cb := tg.NewCallbackWithAlert(time.Now().UTC().String(), fmt.Sprintf("The value %s doesn't seem a valid year, isn't it?", txt))
			if _, err = bot.Request(cb); err != nil {
				log.Error(usr, err, "failed sending alert message")
			}
		} else {
			state.movie.Year = int16(year)
		}

	case stageChooseDel:
		keyboard = &keyboardDel
		state.movie = *movieByID(usr, txt)
		state.stage = stageDel

	case stageChooseRate:
		keyboard = &keyboardRate
		state.movie = *movieByID(usr, txt)
		state.stage = stageRate

	case stageChooseUnrate:
		keyboard = &keyboardUnrate
		state.movie = *movieByID(usr, txt)
		state.stage = stageUnrate
	}

	states[usr] = state
	replaceMessage(usr, cht, state.mainMessageID, formatMovieWithHeaders(&state.movie), keyboard, state.stage)

	dm := tg.NewDeleteMessage(cht, msg.MessageID)
	// ignore bot.Send errors because it always fails to deserialize response
	bot.Send(dm)
}

func movieByID(usr int64, strID string) *db.Movie {
	var mv *db.Movie
	id, err := strconv.Atoi(strID)
	if err != nil {
		cb := tg.NewCallbackWithAlert(time.Now().UTC().String(), fmt.Sprintf("The value %s doesn't seem a valid movie ID, isn't it?", strID))
		if _, err = bot.Request(cb); err != nil {
			log.Error(usr, err, "failed sending alert message")
		}
		mv = &db.Movie{}
	} else {
		mv, _ = db.GetMovie(usr, id)
	}

	return mv
}
