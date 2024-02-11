package db

import (
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

// getMemosRows returns active and done within the last 24 hours memos
func (d *Database) getMemosRows(usr int64) (*sql.Rows, error) {
	query := `SELECT memo_id, text, state, timestamp, priority
FROM memos
WHERE chat_id=$1 AND (state=$2 OR (state=$3 AND timestamp>$4))
ORDER BY priority ASC`

	return d.db.Query(query, usr, memoStateActive, memoStateDone, clk.Now().UTC().Add(minus24Hours))
}

// extractMemos splits raw rows of memos into active and done memos
func extractMemos(rows *sql.Rows) ([]Memo, []Memo, error) {
	var activeMemos []Memo
	var doneMemos []Memo
	for rows.Next() {
		var memo Memo
		var ts sql.NullTime

		err := rows.Scan(&memo.ID, &memo.Text, &memo.State, &ts, &memo.Priority)
		if err != nil {
			return nil, nil, errors.Wrap(err, "failed scanning text, state, ts, priority")
		}

		if ts.Valid {
			memo.TS = ts.Time
		} else {
			memo.TS = never
		}

		switch memo.State {
		case memoStateActive:
			activeMemos = append(activeMemos, memo)
		case memoStateDone:
			doneMemos = append(doneMemos, memo)
		}
	}

	return activeMemos, doneMemos, nil
}

// AddMemo inserts new memo at the end of the memo list
func (d *Database) AddMemo(c int64, text string) error {
	if _, err := d.db.Exec(`INSERT INTO memos(chat_id, text, state, priority, timestamp)
VALUES($1, $2, $3, COALESCE(
(SELECT max(priority) FROM memos WHERE chat_id=$1 AND state=$3), 0)+1, $4)`, c, text, memoStateActive, clk.Now().UTC()); err != nil {
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
WHERE chat_id=$1 AND state=$2`, c, memoStateActive); err != nil {
		return errors.Wrap(err, "failed to update priorities")
	}
	if _, err = tx.Exec(`INSERT INTO memos(chat_id, text, state, priority, timestamp)
VALUES($1, $2, $3, 1, $4)`, c, text, memoStateActive, clk.Now().UTC()); err != nil {
		return errors.Wrap(err, "failed to insert memo")
	}
	if err = tx.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit")
	}

	return nil
}

// markAs updates memo status of the given memo
func (d *Database) markAs(state uint, usr int64, n int) error {
	if n < 0 {
		return errors.New("argument can't be negative")
	}

	tx, err := d.db.BeginTx(context.Background(), repeatableReadIsoLevel)
	if err != nil {
		return errors.Wrap(err, "failed to begin transaction")
	}
	defer tx.Rollback()

	if _, err = tx.Exec(`UPDATE memos
SET state=$1, timestamp=$2
WHERE chat_id=$3 AND state=$4 AND priority=$5`, state, clk.Now().UTC(), usr, memoStateActive, n); err != nil {
		return errors.Wrap(err, "failed to update memo state")
	}

	if _, err = tx.Exec(`UPDATE memos
SET priority=priority-1
WHERE chat_id=$1 AND state=$2 AND priority>$3`, usr, memoStateActive, n); err != nil {
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
