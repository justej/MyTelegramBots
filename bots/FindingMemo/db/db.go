package db

import (
	"botfarm/bot"
	"database/sql"
	"strconv"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
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

func Init(connStr string) (*sql.DB, error) {
	// connection string should look like postgresql://localhost:5432/finding_memo?user=admn&password=passwd
	d, err := sql.Open("pgx", connStr)
	if err != nil {
		return nil, err
	}

	if err = d.Ping(); err != nil {
		return nil, err
	}

	return d, nil
}

func ListAllMemos(ctx *bot.Context, cht int64, short bool) (string, string) {
	rows, err := getMemosRows(ctx, cht)
	if err != nil {
		ctx.Logger.Errorw("failed querying memos", "err", err)
		return "", ""
	}
	defer rows.Close()

	active, done := extractMemos(ctx, rows)
	return listMemos(active, short), listMemos(done, short)
}

func ListFullMemos(ctx *bot.Context, cht int64, short bool) string {
	rows, err := getMemosRows(ctx, cht)
	if err != nil {
		ctx.Logger.Errorw("failed querying memos", "err", err)
		return ""
	}
	defer rows.Close()

	activeMemos, _ := extractMemos(ctx, rows)
	return listMemos(activeMemos, short)
}

func ListFirstMemos(ctx *bot.Context, cht int64, n int, short bool) string {
	rows, err := getMemosRows(ctx, cht)
	if err != nil {
		ctx.Logger.Errorw("failed querying memos", "err", err)
		return ""
	}
	defer rows.Close()

	activeMemos, _ := extractMemos(ctx, rows)
	if len(activeMemos) < n {
		n = len(activeMemos)
	}

	list := listMemos(activeMemos[:n], short)
	if len(activeMemos) > n {
		list += "\n..."
	}

	return list
}

func ListActiveMemos(ctx *bot.Context, cht int64, short bool) []Memo {
	rows, err := getMemosRows(ctx, cht)
	if err != nil {
		ctx.Logger.Errorw("failed querying memos", "err", err)
		return []Memo{}
	}
	defer rows.Close()

	activeMemos, _ := extractMemos(ctx, rows)
	return activeMemos
}

// Done marks the task as done
func Done(ctx *bot.Context, cht int64, n int) {
	markAs(ctx, memoStateDone, cht, n)
}

// Delete soft-deletes the task
func Delete(ctx *bot.Context, cht int64, n int) {
	markAs(ctx, memoStateDeleted, cht, n)
}

func GetLenMemos(ctx *bot.Context, cht int64) int {
	rows, err := getMemosRows(ctx, cht)
	if err != nil {
		ctx.Logger.Errorw("failed to count done memos", "err", err)
		return 0
	}
	defer rows.Close()

	activeMemos, _ := extractMemos(ctx, rows)
	return len(activeMemos)
}

func listMemos(memos []Memo, short bool) string {
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
			if maxLineLen < len(memo.Text) {
				maxLineLen = len(memo.Text)
			}
		}
	}

	// grow string builder to accommodate all lines
	for _, memo := range memos {
		// up to 3 symbols per line
		n += 3 + min(len(memo.Text), maxLineLen)
	}
	sb.Grow(n)

	// compose the string
	for i, memo := range (memos)[:len(memos)-1] {
		writeMemo(&sb, strconv.Itoa(i+1), maxLineLen, memo.Text)
		sb.WriteString("\n")
	}
	writeMemo(&sb, strconv.Itoa(len(memos)), maxLineLen, memos[len(memos)-1].Text)

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
