package db

import (
	"botfarm/bot"
	"botfarm/bots/FindingMemo/timezone"
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jmhodges/clock"

	_ "github.com/jackc/pgx/v5/stdlib"
)

var (
	repeatableReadIsoLevel = &sql.TxOptions{Isolation: sql.LevelRepeatableRead}
	errFailedCreateUser    = errors.New("failed to add or update user")
	never                  = time.Unix(0, 0)
	clk                    = clock.New()
	minus24Hours           = -24 * time.Hour
)

// CreateUser creates a new user or updates chat ID for the case when the bot was deleted earlier
// UTC timezone is used by default
func CreateUser(ctx *bot.Context, usr, cht int64) error {
	tx, err := ctx.DB.BeginTx(context.Background(), repeatableReadIsoLevel)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var cID int64
	err = tx.QueryRow(`SELECT chat_id FROM users WHERE user_id=$1`, usr).Scan(&cID)

	switch {
	case err == sql.ErrNoRows:
		if _, err := tx.Exec(`INSERT INTO users(user_id, chat_id, remind, remind_at, timezone)
VALUES($1, $2, $3, $4, $5)`, usr, cht, true, DefaultTime, DefaultTimeZone); err != nil {
			ctx.Logger.Errorw("failed inserting user", "err", err)
			return errFailedCreateUser
		}

	case err != nil:
		ctx.Logger.Errorw("failed creating user", "err", err)
		return errFailedCreateUser

	default:
		if cht == cID {
			return nil
		}
		if _, err := tx.Exec(`UPDATE users SET chat_id=$1 WHERE user_id=$2`, cht, usr); err != nil {
			ctx.Logger.Errorw("failed updating chat ID", "err", err)
			return errFailedCreateUser
		}
	}

	if err := tx.Commit(); err != nil {
		ctx.Logger.Errorw("failed adding user", "err", err)
		return errFailedCreateUser
	}
	return nil
}

// getMemosRows returns active and done within the last 24 hours memos
func getMemosRows(ctx *bot.Context, cht int64) (*sql.Rows, error) {
	query := `SELECT text, state, timestamp, priority
FROM memos
WHERE chat_id=$1 AND (state=$2 OR (state=$3 AND timestamp>$4))
ORDER BY priority ASC`

	return ctx.DB.Query(query, cht, memoStateActive, memoStateDone, clk.Now().UTC().Add(minus24Hours))
}

// extractMemos splits raw rows of memos into active and done memos
func extractMemos(ctx *bot.Context, rows *sql.Rows) ([]memo, []memo) {
	var activeMemos []memo
	var doneMemos []memo
	for rows.Next() {
		var memo memo
		var ts sql.NullTime

		err := rows.Scan(&memo.text, &memo.state, &ts, &memo.priority)
		if err != nil {
			ctx.Logger.Errorw("failed scanning text, state, ts, priority", "err", err)
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
func AddMemo(ctx *bot.Context, c int64, text string) bool {
	if _, err := ctx.DB.Exec(`INSERT INTO memos(chat_id, text, state, priority, timestamp)
VALUES($1, $2, $3, COALESCE(
(SELECT max(priority) FROM memos WHERE chat_id=$1 AND state=$3), 0)+1, $4)`, c, text, memoStateActive, clk.Now().UTC()); err != nil {
		ctx.Logger.Errorw("failed to add memo", "err", err)
		return false
	}

	return true
}

// InsertMemo inserts new memo at the beginning of the memo list
func InsertMemo(ctx *bot.Context, c int64, text string) bool {
	tx, err := ctx.DB.BeginTx(context.Background(), repeatableReadIsoLevel)
	if err != nil {
		ctx.Logger.Errorw("failed to begin transaction", "err", err)
		return false
	}
	defer tx.Rollback()

	if _, err = tx.Exec(`UPDATE memos SET priority=priority+1
WHERE chat_id=$1 AND state=$2`, c, memoStateActive); err != nil {
		ctx.Logger.Errorw("failed to update priorities", "err", err)
		return false
	}
	if _, err = tx.Exec(`INSERT INTO memos(chat_id, text, state, priority, timestamp)
VALUES($1, $2, $3, 1, $4)`, c, text, memoStateActive, clk.Now().UTC()); err != nil {
		ctx.Logger.Errorw("failed to insert memo", "err", err)
		return false
	}
	if err = tx.Commit(); err != nil {
		ctx.Logger.Errorw("failed to commit", "err", err)
		return false
	}

	return true
}

// markAs updates memo status of the given memo
func markAs(ctx *bot.Context, state uint, cht int64, n int) {
	if n < 0 {
		return
	}

	tx, err := ctx.DB.BeginTx(context.Background(), repeatableReadIsoLevel)
	if err != nil {
		ctx.Logger.Errorw("failed to begin transaction", "err", err)
	}
	defer tx.Rollback()

	if _, err = tx.Exec(`UPDATE memos
SET state=$1, timestamp=$2
WHERE chat_id=$3 AND state=$4 AND priority=$5`, state, clk.Now().UTC(), cht, memoStateActive, n); err != nil {
		ctx.Logger.Errorw("failed to update memo state", "err", err)
		return
	}

	if _, err = tx.Exec(`UPDATE memos
SET priority=priority-1
WHERE chat_id=$1 AND state=$2 AND priority>$3`, cht, memoStateActive, n); err != nil {
		ctx.Logger.Errorw("failed to update priorities", "err", err)
		return
	}

	if err = tx.Commit(); err != nil {
		ctx.Logger.Errorw("failed to commit", "err", err)
	}
}

// GetUsers returns a list of all user IDs
func GetUsers(ctx *bot.Context) ([]int64) {
	rows, err := ctx.DB.Query(`SELECT user_id FROM users`)
	if err != nil {
		ctx.Logger.Errorw("failed fetching list of users:", "err", err)
		return nil
	}
	defer rows.Close()

	var users []int64
	for rows.Next() {
		var usr int64
		err = rows.Scan(&usr)
		if err != nil {
			ctx.Logger.Errorw("failed reading user ID", "err", err)
			continue
		}

		users = append(users, usr)
	}

	return users
}

// GetRemindParams returns the time
func GetRemindParams(ctx *bot.Context, usr int64) (*RemindParams, bool) {
	var rp RemindParams
	err := ctx.DB.QueryRow(`SELECT remind, remind_at, chat_id, timezone
FROM users
WHERE user_id=$1`, usr).Scan(&rp.Set, &rp.RemindAt, &rp.ChatID, &rp.TimeZone)

	switch {
	case err == sql.ErrNoRows:
		return nil, true
	case err != nil:
		ctx.Logger.Errorw("failed to fetch remind parameters", "err", err)
		return nil, false
	}

	return &rp, true
}

// SetRemindAt updates reminder time in DB
func SetRemindAt(ctx *bot.Context, usr int64, at int) bool {
	_, err := ctx.DB.Exec(`UPDATE users SET remind_at=$1, remind=TRUE
WHERE user_id = $2`, at, usr)
	if err != nil {
		ctx.Logger.Errorw("failed updating reminder", "err", err)
	}
	return err == nil
}

func UpdateTZ(ctx *bot.Context, usr int64, loc *timezone.GeoLocation, tz string) bool {
	_, err := ctx.DB.Exec(`UPDATE users SET latitude=$1, longitude=$2, timezone=$3 WHERE user_id=$4`, loc.Latitude, loc.Longitude, tz, usr)
	if err != nil {
		ctx.Logger.Errorw("failed updating time zone", "err", err)
	}
	return err == nil
}
