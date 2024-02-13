package db

import (
	"botfarm/bot"
	"botfarm/bots/FindingMemo/timezone"
	"context"
	"database/sql"
	"time"

	"github.com/jmhodges/clock"
	"github.com/pkg/errors"

	_ "github.com/jackc/pgx/v5/stdlib"
)

var (
	repeatableReadIsoLevel = &sql.TxOptions{Isolation: sql.LevelRepeatableRead}
	never                  = time.Unix(0, 0)
	clk                    = clock.New()
	minus24Hours           = -24 * time.Hour
)

const (
	shortLineLen    = 40
	DefaultTime     = 9 * 60 // 9:00
	DefaultTimeZone = "UTC"
)

type Database struct {
	db            *sql.DB
	RetryAttempts int
	RetryDelay    time.Duration
	Timeout       time.Duration
}

func NewDatabase(connStr string, attempts int, delay, timeout time.Duration) (*Database, error) {
	// connection string should look like postgresql://localhost:5432/finding_memo?user=admn&password=passwd
	d, err := sql.Open("pgx", connStr)
	if err != nil {
		return nil, err
	}

	if err = d.Ping(); err != nil {
		return nil, err
	}

	return &Database{db: d, RetryAttempts: attempts, RetryDelay: delay, Timeout: timeout}, nil
}

func (d *Database) GetAllMemos(usr int64, short bool) ([]Memo, error) {
	query := `SELECT memo_id, text, state, timestamp, priority
FROM memos
WHERE chat_id=$1 AND (state=$2 OR (state IN ($3, $4) AND timestamp>$5))
ORDER BY priority ASC`
	rows, err := d.db.Query(query, usr, MemoStateActive, MemoStateDone,
		MemoStateDeleted, clk.Now().UTC().Add(minus24Hours))
	if err != nil {
		return []Memo{}, err
	}
	defer rows.Close()

	memos, err := extractMemos(rows)
	if err != nil {
		return []Memo{}, err
	}
	return memos, nil
}

// MarkAsDone marks the task as done
func (d *Database) MarkAsDone(usr int64, n int) error {
	return d.markAs(MemoStateDone, usr, n)
}

// DeleteMemo soft-deletes the task
func (d *Database) DeleteMemo(usr int64, n int) error {
	return d.markAs(MemoStateDeleted, usr, n)
}

// GetActiveMemoCount returns the count of active memos for a user
func (d *Database) GetActiveMemoCount(usr int64) (int, error) {
	var n int
	err := d.db.QueryRow(`SELECT count(*) FROM memos WHERE chat_id=$1 AND state=$2`,
		usr, MemoStateActive).Scan(&n)
	if err != nil {
		return 0, err
	}
	return n, nil
}

// CreateUser creates a new user or updates chat ID for the case when the bot was deleted earlier
// UTC timezone is used by default
func (d *Database) CreateUser(usr int64) error {
	tx, err := d.db.BeginTx(context.Background(), repeatableReadIsoLevel)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var cID int64
	err = tx.QueryRow(`SELECT chat_id FROM users WHERE user_id=$1`, usr).Scan(&cID)

	switch {
	case err == sql.ErrNoRows:
		if _, err := tx.Exec(`INSERT INTO users(user_id, remind, remind_at, timezone)
VALUES($1, $2, $3, $4, $5)`, usr, true, DefaultTime, DefaultTimeZone); err != nil {
			return errors.Wrap(err, "failed inserting user")
		}

	case err != nil:
		return errors.Wrap(err, "failed creating user")
	}

	if err := tx.Commit(); err != nil {
		return errors.Wrap(err, "failed adding user")
	}
	return nil
}

func extractMemos(rows *sql.Rows) ([]Memo, error) {
	var memos []Memo
	var m Memo

	for rows.Next() {
		var ts sql.NullTime

		err := rows.Scan(&m.ID, &m.Text, &m.State, &ts, &m.Priority)
		if err != nil {
			return nil, errors.Wrap(err, "failed scanning text, state, ts, priority")
		}

		if ts.Valid {
			m.TS = ts.Time
		} else {
			m.TS = never
		}
		memos = append(memos, m)
	}

	return memos, nil
}

// AddMemo inserts new memo at the end of the memo list
func (d *Database) AddMemo(c int64, text string) error {
	if _, err := d.db.Exec(`INSERT INTO memos(chat_id, text, state, priority, timestamp)
VALUES($1, $2, $3, COALESCE(
(SELECT max(priority) FROM memos WHERE chat_id=$1 AND state=$3), 0)+1, $4)`, c, text, MemoStateActive, clk.Now().UTC()); err != nil {
		return errors.Wrap(err, "failed to add memo")
	}

	return nil
}

// InsertMemo inserts new memo at the beginning of the memo list
func (d *Database) InsertMemo(c int64, text string) error {
	tx, err := d.db.BeginTx(context.Background(), repeatableReadIsoLevel)
	if err != nil {
		return errors.Wrap(err, "failed to begin transaction")
	}
	defer tx.Rollback()

	if _, err = tx.Exec(`UPDATE memos SET priority=priority+1
WHERE chat_id=$1 AND state=$2`, c, MemoStateActive); err != nil {
		return errors.Wrap(err, "failed to update priorities")
	}
	if _, err = tx.Exec(`INSERT INTO memos(chat_id, text, state, priority, timestamp)
VALUES($1, $2, $3, $4, $5)`, c, text, MemoStateActive, priorityMinValue, clk.Now().UTC()); err != nil {
		return errors.Wrap(err, "failed to insert memo")
	}
	if err = tx.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit")
	}

	return nil
}

// markAs updates memo status of the given memo
func (d *Database) markAs(state uint, usr int64, n int) error {
	if n < priorityMinValue {
		return errors.New("argument can't be negative")
	}

	tx, err := d.db.BeginTx(context.Background(), repeatableReadIsoLevel)
	if err != nil {
		return errors.Wrap(err, "failed to begin transaction")
	}
	defer tx.Rollback()

	if _, err = tx.Exec(`UPDATE memos
SET state=$1, timestamp=$2
WHERE chat_id=$3 AND state=$4 AND priority=$5`, state, clk.Now().UTC(), usr, MemoStateActive, n); err != nil {
		return errors.Wrap(err, "failed to update memo state")
	}

	if _, err = tx.Exec(`UPDATE memos
SET priority=priority-1
WHERE chat_id=$1 AND state=$2 AND priority>$3`, usr, MemoStateActive, n); err != nil {
		return errors.Wrap(err, "failed to update priorities")
	}

	if err = tx.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit")
	}
	return nil
}

// GetUsers returns a list of all user IDs
func (d *Database) GetUsers() ([]int64, error) {
	rows, err := d.db.Query(`SELECT user_id FROM users`)
	if err != nil {
		return nil, errors.Wrap(err, "failed fetching list of users")
	}
	defer rows.Close()

	var users []int64
	for rows.Next() {
		var usr int64
		err = rows.Scan(&usr)
		if err != nil {
			return nil, errors.Wrap(err, "failed reading user ID")
		}

		users = append(users, usr)
	}

	return users, nil
}

// GetRemindParams returns the time
func (d *Database) GetRemindParams(usr int64) (*RemindParams, error) {
	var rp RemindParams
	err := d.db.QueryRow(`SELECT remind, remind_at, chat_id, timezone
FROM users
WHERE user_id=$1`, usr).Scan(&rp.Set, &rp.RemindAt, &rp.ChatID, &rp.TimeZone)

	switch {
	case err == sql.ErrNoRows:
		return nil, nil
	case err != nil:
		return nil, errors.Wrap(err, "failed to fetch remind parameters")
	}

	return &rp, nil
}

// SetRemindAt updates reminder time in DB
func (d *Database) SetRemindAt(usr int64, at int) error {
	_, err := d.db.Exec(`UPDATE users SET remind_at=$1, remind=TRUE
WHERE user_id = $2`, at, usr)
	if err != nil {
		return errors.Wrap(err, "failed updating reminder")
	}
	return nil
}

func (d *Database) UpdateTZ(usr int64, loc *timezone.GeoLocation, tz string) error {
	_, err := d.db.Exec(`UPDATE users SET latitude=$1, longitude=$2, timezone=$3 WHERE user_id=$4`, loc.Latitude, loc.Longitude, tz, usr)
	if err != nil {
		return errors.Wrap(err, "failed updating time zone")
	}
	return nil
}

func (d *Database) MakeFirst(usr int64, n int) error {
	if n < priorityMinValue {
		return errors.New("argument can't be negative")
	}

	if n == priorityMinValue {
		return nil
	}

	var err error
	bot.RobustExecute(d.RetryAttempts, d.RetryDelay, func() bool {
		d.db.Exec(`UPDATE memos
SET priority=CASE
	WHEN priority=$1 THEN $2
	ELSE priority+1
END
WHERE chat_id=$3 AND state=$4 AND priority<=$1`, n, priorityMinValue, usr, MemoStateActive)
		return err == nil
	})

	return err
}

func (d *Database) MakeLast(usr int64, n int) error {
	if n < priorityMinValue {
		return errors.New("argument can't be negative")
	}

	var err error
	bot.RobustExecute(d.RetryAttempts, d.RetryDelay, func() bool {
		d.db.Exec(`WITH max_priority AS (
	SELECT MAX(priority) AS value FROM memos WHERE chat_id=$2 AND state=$3
)
UPDATE memos
SET priority=CASE
	WHEN priority=$1 THEN (SELECT value FROM max_priority)
	ELSE priority-1
END
WHERE chat_id=$2 AND state=$3 AND priority>=$1`, n, usr, MemoStateActive)
		return err == nil
	})

	return err
}
