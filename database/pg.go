package database

import (
	"context"
	"errors"
	"telecho/logger"
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
	- remindAt: time - time to remind memos
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
)

const (
	minus24Hours = -24 * time.Hour
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

// CreateUser creates a new user or updates chat ID for the case when the bot was deleted earlier
// UTC timezone is used by default
func (db *Database) CreateUser(u int64, c int64) error {
	tx, err := db.Conn.BeginTx(noCtx, repeatableReadIsoLevel)
	if err != nil {
		return err
	}
	defer tx.Rollback(noCtx)

	var cID int64
	err = tx.QueryRow(noCtx, `SELECT chat_id FROM users WHERE user_id=$1`, u).Scan(&cID)

	switch {
	case err == pgx.ErrNoRows:
		if _, err := tx.Exec(noCtx, `INSERT INTO users(user_id, chat_id, remind, remind_at, utc_offset)
VALUES($1, $2, $3, $4, $5)`, u, c, true, DefaultTime, DefaultOffset); err != nil {
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
	return err
}

// getMemosRows returns active and done within the last 24 hours memos
func (db *Database) getMemosRows(u int64, c int64) (pgx.Rows, error) {
	query := `SELECT text, state, timestamp, priority
FROM memos
WHERE chat_id=$1 AND (state=$2 OR (state=$3 AND timestamp>$4))
ORDER BY priority ASC`

	return db.Conn.Query(noCtx, query, c, stateActive, stateDone, clk.Now().Add(minus24Hours))
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
		case stateActive:
			activeMemos = append(activeMemos, memo)
		case stateDone:
			doneMemos = append(doneMemos, memo)
		}
	}

	return activeMemos, doneMemos
}

// AddMemo inserts new memo at the end of the memo list
func (db *Database) AddMemo(u int64, c int64, text string) {
	if _, err := db.Conn.Exec(noCtx, `INSERT INTO memos(chat_id, text, state, priority, timestamp)
VALUES($1, $2, $3, COALESCE(
(SELECT max(priority) FROM memos WHERE chat_id=$1 AND state=$3), 0)+1, $4)`, c, text, stateActive, clk.Now()); err != nil {
		logger.ForUser(u, "failed to add memo", err)
	}
}

// InsertMemo inserts new memo at the beginning of the memo list
func (db *Database) InsertMemo(u int64, c int64, text string) {
	tx, err := db.Conn.BeginTx(noCtx, repeatableReadIsoLevel)
	if err != nil {
		logger.ForUser(u, "failed to begin transaction", err)
		return
	}
	defer tx.Rollback(noCtx)

	if _, err = tx.Exec(noCtx, `UPDATE memos SET priority=priority+1
WHERE chate_id=$1 AND state=$2`, c, stateActive); err != nil {
		logger.ForUser(u, "failed to update priorities", err)
		return
	}
	if _, err = tx.Exec(noCtx, `INSERT INTO memos(chat_id, text, state, priority, timestamp)
VALUES($1, $2, $3, 1, $4)`, c, text, stateActive, clk.Now()); err != nil {
		logger.ForUser(u, "failed to insert memo", err)
		return
	}
	if err = tx.Commit(noCtx); err != nil {
		logger.ForUser(u, "failed to commit", err)
	}
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
WHERE chat_id=$3 AND priority=$4`, state, clk.Now(), c, n); err != nil {
		logger.ForUser(u, "failed to update memo state", err)
		return
	}
	if _, err = tx.Exec(noCtx, `UPDATE memos SET priority=priority-1
WHERE chat_id=$1 AND priority>$2`, c, n); err != nil {
		logger.ForUser(u, "failed to update priorities", err)
		return
	}
	if err = tx.Commit(noCtx); err != nil {
		logger.ForUser(u, "failed to commit", err)
	}
}

// GetRemindParams returns the time
func (db *Database) GetRemindParams(u int64) (*RemindParams, bool) {
	var remindAt pgtype.Time
	var remindParams RemindParams
	err := db.Conn.QueryRow(noCtx, `SELECT remind, remind_at, chat_id, utc_offset
FROM users
WHERE user_id=$1`, u).Scan(&remindParams.Set, &remindAt, &remindParams.UTCOffset, &remindParams.ChatID)
	switch {
	case err == pgx.ErrNoRows:
		return nil, true
	case err != nil:
		logger.ForUser(u, "failed to fetch remind parameters", err)
		return nil, false
	}

	if remindAt.Valid {
		remindParams.RemindAt = time.UnixMicro(remindAt.Microseconds)
	} else {
		remindParams.RemindAt = never
	}

	return &remindParams, true
}

// SetRemindAt updates reminder time in DB
func (db *Database) SetRemindAt(u, c int64, t time.Time) bool {
	_, err := db.Conn.Exec(noCtx, `UPDATE users SET chat_id=$1, remind_at=$2, remind=TRUE
WHERE user_id = $3`, c, t, u)
	return err == nil
}
