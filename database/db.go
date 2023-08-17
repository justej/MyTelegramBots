package database

import (
	"log"
	"os"
	"strconv"
	"strings"
	"telecho/logger"
)

const (
	shortLineLen    = 40
	DefaultTime     = 9 * 60 // 9:00
	DefaultTimeZone = "UTC"
)

func min(x, y int) int {
	if x < y {
		return x
	}
	return y
}

func Init() *Database {
	// connection string should look like postgresql://localhost:5432/finding_memo?user=admn&password=passwd
	connStr := os.Getenv("PG_CONN_STR")
	db, err := newDatabase(connStr)
	if err != nil {
		log.Panic(err)
	}

	return db
}

func (db *Database) ListAllMemos(u, c int64, short bool) (string, string) {
	rows, err := db.getMemosRows(u, c)
	if err != nil {
		logger.ForUser(u, "failed querying memos", err)
		return "", ""
	}
	defer rows.Close()

	active, done := extractMemos(rows, u)
	return listMemos(active, short), listMemos(done, short)
}

func (db *Database) ListFullMemos(u, c int64, short bool) string {
	rows, err := db.getMemosRows(u, c)
	if err != nil {
		logger.ForUser(u, "failed querying memos", err)
		return ""
	}
	defer rows.Close()

	activeMemos, _ := extractMemos(rows, u)
	return listMemos(activeMemos, short)
}

func (db *Database) ListFirstMemos(u, c int64, n int, short bool) string {
	rows, err := db.getMemosRows(u, c)
	if err != nil {
		logger.ForUser(u, "failed querying memos", err)
		return ""
	}
	defer rows.Close()

	activeMemos, _ := extractMemos(rows, u)
	list := listMemos(activeMemos[:n], short)
	if len(activeMemos) > n {
		list += "\n..."
	}

	return list
}

// Done marks the task as done
func (db *Database) Done(u, c int64, n int) {
	db.markAs(memoStateDone, u, c, n)
}

// Delete soft-deletes the task
func (db *Database) Delete(u, c int64, n int) {
	db.markAs(memoStateDeleted, u, c, n)
}

func (db *Database) GetLenDone(u, c int64) int {
	rows, err := db.getMemosRows(u, c)
	if err != nil {
		logger.ForUser(u, "failed to count done memos", err)
		return 0
	}
	defer rows.Close()

	_, doneMemos := extractMemos(rows, u)
	return len(doneMemos)
}

func (db *Database) GetLenMemos(u, c int64) int {
	rows, err := db.getMemosRows(u, c)
	if err != nil {
		logger.ForUser(u, "failed to count done memos", err)
		return 0
	}
	defer rows.Close()

	activeMemos, _ := extractMemos(rows, u)
	return len(activeMemos)
}

func listMemos(memos []memo, short bool) string {
	var n int
	var maxLineLen int
	var sb strings.Builder

	if len(memos) == 0 {
		return ""
	}

	if short {
		maxLineLen = shortLineLen
	} else {
		for _, memo := range memos {
			if maxLineLen < len(memo.text) {
				maxLineLen = len(memo.text)
			}
		}
	}

	// grow string builder to accommodate all lines
	for _, memo := range memos {
		// up to 3 symbols per line
		n += 3 + min(len(memo.text), maxLineLen)
	}
	sb.Grow(n)

	// compose the string
	for i, memo := range (memos)[:len(memos)-1] {
		writeMemo(&sb, strconv.Itoa(i+1), maxLineLen, memo.text)
		sb.WriteString("\n")
	}
	writeMemo(&sb, strconv.Itoa(len(memos)), maxLineLen, memos[len(memos)-1].text)

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
