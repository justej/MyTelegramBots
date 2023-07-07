package database

import (
	"strconv"
	"strings"
	"sync"
)

const (
	shortLineLen int = 40

	// default configuration
	defaultHour   = 9
	defaultMinute = 0
)

type UserID = int64

// A Memo keeps user's to-do with specified priority
//
// Implementation details:
// * number of memos: 255
// * priority is the position of a memo in the Memos list
type Memo struct {
	Owner UserID
	Text  string
}

// Memos represents user's Memo records
type Memos []Memo

type Config struct {
	RemindHour int   // remind time hour
	RemindMin  int   // remind time minute
	ChatID     int64 // TODO: can several users have the same chat ID?
}

func NewConfig() *Config {
	return &Config{RemindHour: defaultHour, RemindMin: defaultMinute}
}

// Data is synchronizable Memos
type Data struct {
	*Memos
	Done *Memos
	mux  sync.Mutex
	*Config
}

// Database represents a real database even though this is just a map
type Database map[UserID]Data

func NewDatabase() Database {
	return make(map[UserID]Data)
}

func NewData() *Data {
	var data Data
	data.Memos = new(Memos)
	data.Done = new(Memos)
	data.Config = NewConfig()
	return &data
}

func min(x, y int) int {
	if x < y {
		return x
	}
	return y
}

func (db *Database) StoreChatID(u UserID, chatID int64) {
	data, ok := (*db)[u]
	if !ok {
		data = *NewData()
	}

	data.Config.ChatID = chatID
	(*db)[u] = data
}

func (db *Database) ListAllMemos(u UserID, short bool) string {
	memos := (*db)[u].Memos
	if memos == nil || len(*memos) == 0 {
		return ""
	}
	return db.listMemos(memos, short)
}

func (db *Database) ListFirstMemos(u UserID, n int, short bool) string {
	memos := (*db)[u].Memos
	if memos == nil || len(*memos) == 0 {
		return ""
	}
	n = min(n, len(*memos))
	if n <= 0 {
		return ""
	}

	m := (*memos)[:n]
	list := db.listMemos(&m, short)
	if len(*memos) > n {
		list += "\n..."
	}

	return list
}

func (db *Database) listMemos(memos *Memos, short bool) string {
	if len(*memos) < 0 {
		return ""
	}

	var n int
	var maxLineLen int
	var sb strings.Builder

	if short {
		maxLineLen = shortLineLen
	} else {
		for _, memo := range *memos {
			if maxLineLen < len(memo.Text) {
				maxLineLen = len(memo.Text)
			}
		}
	}

	// grow string builder to accommodate all lines
	for _, memo := range *memos {
		// up to 3 symbols per line
		n += 3 + min(len(memo.Text), maxLineLen)
	}
	sb.Grow(n)

	// compose the string
	for i, memo := range (*memos)[:len(*memos)-1] {
		writeMemo(&sb, strconv.Itoa(i+1), maxLineLen, memo.Text)
		sb.WriteString("\n")
	}
	writeMemo(&sb, strconv.Itoa(len(*memos)), maxLineLen, (*memos)[len(*memos)-1].Text)

	return sb.String()
}

func writeMemo(sb *strings.Builder, n string, maxLineLen int, text string) {
	sb.WriteString(n)
	sb.WriteString(". ")
	if len(text) > maxLineLen {
		sb.WriteString(text[:maxLineLen])
		sb.WriteString("...")
	} else {
		sb.WriteString(text)
	}
}

func (db *Database) AddMemo(u UserID, text string) {
	data, ok := (*db)[u]
	if !ok {
		data = *NewData()
		(*db)[u] = data
	}

	*data.Memos = append(*data.Memos, Memo{u, text})
}

func (db *Database) Reorder(u UserID, old, new int) {
	memos := (*db)[u].Memos
	if old == new || old < 1 || old > len(*memos) || new < 1 || new > len(*memos) {
		return
	}

	mux := (*db)[u].mux
	mux.Lock()
	// move to back
	m := (*memos)[old]
	if old < new {
		for old < new {
			(*memos)[old] = (*memos)[old+1]
			old++
		}
		(*memos)[new] = m
	} else {
		for old > new {
			(*memos)[old] = (*memos)[old-1]
			old--
		}
		(*memos)[new] = m
	}
	mux.Unlock()
}

func (db *Database) Done(u UserID, n int) {
	memos := (*db)[u].Memos
	done := (*db)[u].Done
	if memos == nil || len(*memos) == 0 || n < 0 || n > len(*memos) {
		return
	}

	*done = append(*done, (*memos)[n])
}

func (db *Database) Delete(u UserID, n int) {
	memos := (*db)[u].Memos
	if memos == nil || len(*memos) == 0 || n < 0 || n > len(*memos) {
		return
	}

	for i := n; i < len(*memos)-1; i++ {
		(*memos)[i] = (*memos)[i+1]
	}

	*memos = (*memos)[:len(*memos)-1]
}

func (db *Database) GetLenDone(u UserID) int {
	done := (*db)[u].Done
	if done == nil {
		return 0
	}
	return len(*done)
}

func (db *Database) GetLenMemos(u UserID) int {
	memos := (*db)[u].Memos
	if memos == nil {
		return 0
	}
	return len(*memos)
}
