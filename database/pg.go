package database

import (
	"context"
	"errors"
	"findingmemo/logger"
	"findingmemo/timezone"
	"log"
	"time"

	"github.com/jmhodges/clock"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

/**
DB tables:
- users:
	- user ID: bigint - user ID
	- chat ID: bigint - chat ID
	- remind: boolean - on/off reminder
	- remindAt: smallint - remind time as number of minutes since midnight
	- latitude: float4 - latitude for TZ identification
	- longitude: float4 - longitude for TZ identification
	- utcOffset: int4 - UTC offset in minutes

- memos:
	- user ID: bigint - memo owner
	- text: text - memo text
	- priority: smallint - memo's ordinal number
	- state: smallint - memo state:
		- 0 - active
		- 1 - done
		- 2 - deleted
	- priority: smallint - memo order
	- timestamp: timestamp - timestamp of the last operation

Indexes:
- users:
	- user ID - primary key
- memos:
	- user ID - primary key
*/

var (
	noCtx                  = context.Background()
	repeatableReadIsoLevel = pgx.TxOptions{IsoLevel: pgx.RepeatableRead}
	errFetchingReminder    = errors.New("failed to fetch remind parameters")
	errFailedCreateUser    = errors.New("failed to add or update user")
	never                  = time.Unix(0, 0)
	clk                    = clock.New()
	minus24Hours           = -24 * time.Hour
)

type Timetz struct {
	time.Time
	Offset int16 // offset in minutes
}
type Database struct {
	Conn *pgxpool.Pool
}

func newDatabase(connStr string) (*Database, error) {
	dbpool, err := pgxpool.New(noCtx, connStr)
	if err != nil {
		return nil, err
	}

	return &Database{Conn: dbpool}, err
}

// TODO: make all operations with DB robust (retry several times on error)

// CreateUser creates a new user or updates chat ID for the case when the bot was deleted earlier
// UTC timezone is used by default
func (db *Database) CreateUser(u, c int64) error {
	tx, err := db.Conn.BeginTx(noCtx, repeatableReadIsoLevel)
	if err != nil {
		return err
	}
	defer tx.Rollback(noCtx)

	var cID int64
	err = tx.QueryRow(noCtx, `SELECT chat_id FROM users WHERE user_id=$1`, u).Scan(&cID)

	switch {
	case err == pgx.ErrNoRows:
		if _, err := tx.Exec(noCtx, `INSERT INTO users(user_id, chat_id, remind, remind_at, timezone)
VALUES($1, $2, $3, $4, $5)`, u, c, true, DefaultTime, DefaultTimeZone); err != nil {
			logger.ForUser(u, "failed inserting user", err)
			return errFailedCreateUser
		}

	case err != nil:
		logger.ForUser(u, "failed creating user", err)
		return errFailedCreateUser

	default:
		if c == cID {
			return nil
		}
		if _, err := tx.Exec(noCtx, `UPDATE users SET chat_id=$1 WHERE user_id=$2`, c, u); err != nil {
			logger.ForUser(u, "failed updating chat ID", err)
			return errFailedCreateUser
		}
	}

	if err := tx.Commit(noCtx); err != nil {
		logger.ForUser(u, "failed adding user", err)
		return errFailedCreateUser
	}
	return nil
}

// getMemosRows returns active and done within the last 24 hours memos
func (db *Database) getMemosRows(u int64, c int64) (pgx.Rows, error) {
	query := `SELECT text, state, timestamp, priority
FROM memos
WHERE chat_id=$1 AND (state=$2 OR (state=$3 AND timestamp>$4))
ORDER BY priority ASC`

	return db.Conn.Query(noCtx, query, c, memoStateActive, memoStateDone, clk.Now().UTC().Add(minus24Hours))
}

// extractMemos splits raw rows of memos into active and done memos
func extractMemos(rows pgx.Rows, u int64) ([]memo, []memo) {
	var activeMemos []memo
	var doneMemos []memo
	for rows.Next() {
		var memo memo
		var ts pgtype.Timestamp

		err := rows.Scan(&memo.text, &memo.state, &ts, &memo.priority)
		if err != nil {
			logger.ForUser(u, "failed scanning text, state, ts, priority", err)
			continue
		}

		if ts.Valid {
			memo.ts = ts.Time
		} else {
			memo.ts = never
		}

		switch memo.state {
		case memoStateActive:
			activeMemos = append(activeMemos, memo)
		case memoStateDone:
			doneMemos = append(doneMemos, memo)
		}
	}

	return activeMemos, doneMemos
}

// AddMemo inserts new memo at the end of the memo list
func (db *Database) AddMemo(u int64, c int64, text string) bool {
	if _, err := db.Conn.Exec(noCtx, `INSERT INTO memos(chat_id, text, state, priority, timestamp)
VALUES($1, $2, $3, COALESCE(
(SELECT max(priority) FROM memos WHERE chat_id=$1 AND state=$3), 0)+1, $4)`, c, text, memoStateActive, clk.Now().UTC()); err != nil {
		logger.ForUser(u, "failed to add memo", err)
		return false
	}

	return true
}

// InsertMemo inserts new memo at the beginning of the memo list
func (db *Database) InsertMemo(u int64, c int64, text string) bool {
	tx, err := db.Conn.BeginTx(noCtx, repeatableReadIsoLevel)
	if err != nil {
		logger.ForUser(u, "failed to begin transaction", err)
		return false
	}
	defer tx.Rollback(noCtx)

	if _, err = tx.Exec(noCtx, `UPDATE memos SET priority=priority+1
WHERE chat_id=$1 AND state=$2`, c, memoStateActive); err != nil {
		logger.ForUser(u, "failed to update priorities", err)
		return false
	}
	if _, err = tx.Exec(noCtx, `INSERT INTO memos(chat_id, text, state, priority, timestamp)
VALUES($1, $2, $3, 1, $4)`, c, text, memoStateActive, clk.Now().UTC()); err != nil {
		logger.ForUser(u, "failed to insert memo", err)
		return false
	}
	if err = tx.Commit(noCtx); err != nil {
		logger.ForUser(u, "failed to commit", err)
		return false
	}

	return true
}

// markAs updates memo status of the given memo
func (db *Database) markAs(state uint, u, c int64, n int) {
	if n < 0 {
		return
	}

	tx, err := db.Conn.BeginTx(noCtx, repeatableReadIsoLevel)
	if err != nil {
		logger.ForUser(u, "failed to begin transaction", err)
	}
	defer tx.Rollback(noCtx)

	if _, err = tx.Exec(noCtx, `UPDATE memos
SET state=$1, timestamp=$2
WHERE chat_id=$3 AND state=$4 AND priority=$5`, state, clk.Now().UTC(), c, memoStateActive, n); err != nil {
		logger.ForUser(u, "failed to update memo state", err)
		return
	}
	if _, err = tx.Exec(noCtx, `UPDATE memos
SET priority=priority-1
WHERE chat_id=$1 AND state=$2 AND priority>$3`, c, memoStateActive, n); err != nil {
		logger.ForUser(u, "failed to update priorities", err)
		return
	}
	if err = tx.Commit(noCtx); err != nil {
		logger.ForUser(u, "failed to commit", err)
	}
}

// GetUsers returns a list of all user IDs
func (db *Database) GetUsers() (users []int64) {
	rows, err := db.Conn.Query(noCtx, `SELECT user_id FROM users`)
	if err != nil {
		log.Println("failed fetching list of users:", err)
		return nil
	}
	defer rows.Close()

	for rows.Next() {
		var u int64
		err = rows.Scan(&u)
		if err != nil {
			log.Println("failed reading user ID", err)
			continue
		}

		users = append(users, u)
	}

	return users
}

// GetRemindParams returns the time
func (db *Database) GetRemindParams(u int64) (*RemindParams, bool) {
	var remindParams RemindParams
	err := db.Conn.QueryRow(noCtx, `SELECT remind, remind_at, chat_id, timezone
FROM users
WHERE user_id=$1`, u).Scan(&remindParams.Set, &remindParams.RemindAt, &remindParams.ChatID, &remindParams.TimeZone)
	switch {
	case err == pgx.ErrNoRows:
		return nil, true
	case err != nil:
		logger.ForUser(u, "failed to fetch remind parameters", err)
		return nil, false
	}

	return &remindParams, true
}

// SetRemindAt updates reminder time in DB
func (db *Database) SetRemindAt(u int64, t int) bool {
	_, err := db.Conn.Exec(noCtx, `UPDATE users SET remind_at=$1, remind=TRUE
WHERE user_id = $2`, t, u)
	if err != nil {
		logger.ForUser(u, "failed updating reminder", err)
	}
	return err == nil
}

func (db *Database) UpdateTZ(u int64, loc *timezone.GeoLocation, tz string) bool {
	_, err := db.Conn.Exec(noCtx, `UPDATE users SET latitude=$1, longitude=$2, timezone=$3`, loc.Latitude, loc.Longitude, tz)
	if err != nil {
		logger.ForUser(u, "failed updating time zone", err)
	}
	return err == nil
}
