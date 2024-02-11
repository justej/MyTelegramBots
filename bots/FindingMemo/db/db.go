package db

import (
	"database/sql"
	"strconv"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pkg/errors"
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

type Database struct {
	db            *sql.DB
	RetryAttempts int
	Timeout       time.Duration
}

func NewDatabase(connStr string) (*Database, error) {
	// connection string should look like postgresql://localhost:5432/finding_memo?user=admn&password=passwd
	d, err := sql.Open("pgx", connStr)
	if err != nil {
		return nil, err
	}

	if err = d.Ping(); err != nil {
		return nil, err
	}

	return &Database{db: d}, nil
}

func (d *Database) ListAllMemos(cht int64, short bool) (string, string, error) {
	rows, err := d.getMemosRows(cht)
	if err != nil {
		return "", "", err
	}
	defer rows.Close()

	active, done, err := extractMemos(rows)
	if err != nil {
		return "", "", err
	}
	return listMemos(active, short), listMemos(done, short), nil
}

func (d *Database) ListFullMemos(usr int64, short bool) (string, error) {
	rows, err := d.getMemosRows(usr)
	if err != nil {
		return "", errors.Wrap(err, "failed querying memos")
	}
	defer rows.Close()

	activeMemos, _, err := extractMemos(rows)
	if err != nil {
		return "", err
	}
	return listMemos(activeMemos, short), nil
}

func (d *Database) ListFirstMemos(cht int64, n int, short bool) (string, error) {
	rows, err := d.getMemosRows(cht)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	activeMemos, _, err := extractMemos(rows)
	if err != nil {
		return "", err
	}
	if len(activeMemos) < n {
		n = len(activeMemos)
	}

	list := listMemos(activeMemos[:n], short)
	if len(activeMemos) > n {
		list += "\n..."
	}

	return list, nil
}

func (d *Database) ListActiveMemos(cht int64, short bool) ([]Memo, error) {
	rows, err := d.getMemosRows(cht)
	if err != nil {
		return []Memo{}, err
	}
	defer rows.Close()

	activeMemos, _, err := extractMemos(rows)
	if err != nil {
		return []Memo{}, err
	}
	return activeMemos, nil
}

// Done marks the task as done
func (d *Database) Done(cht int64, n int) error {
	return d.markAs(memoStateDone, cht, n)
}

// Delete soft-deletes the task
func (d *Database) Delete(cht int64, n int) error {
	return d.markAs(memoStateDeleted, cht, n)
}

func (d *Database) GetLenMemos(usr int64) (int, error) {
	rows, err := d.getMemosRows(usr)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	activeMemos, _, err := extractMemos(rows)
	if err != nil {
		return 0, err
	}
	return len(activeMemos), nil
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
