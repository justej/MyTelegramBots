package db

import (
	"botfarm/bot"
	"context"
	"database/sql"
	"fmt"

	"github.com/jmhodges/clock"
)

var (
	clk = clock.New()
)

var txIsoRepeatableRead = &sql.TxOptions{Isolation: sql.LevelRepeatableRead}

func Init(connStr string) (*sql.DB, error) {
	d, err := sql.Open("pgx", connStr)
	if err != nil {
		return nil, err
	}

	if err = d.Ping(); err != nil {
		return nil, err
	}

	return d, nil
}

func AddUser(ctx *bot.Context, usr, cht int64) error {
	tx, err := ctx.DB.BeginTx(context.Background(), txIsoRepeatableRead)
	if err != nil {
		ctx.Logger.Errorw("failed starting transaction on adding user", "err", err)
		return err
	}
	defer tx.Rollback()

	var cID int64
	err = tx.QueryRow(`SELECT chat_id FROM users WHERE id=$1`, usr).Scan(&cID)
	switch {
	case err == sql.ErrNoRows:
		query := `INSERT INTO users (id, chat_id, created_on) VALUES ($1, $2, $3)`
		if _, err = tx.Exec(query, usr, cht, clk.Now().UTC()); err != nil {
			ctx.Logger.Errorw("failed adding user", "err", err)
			return err
		}

	case err != nil:
		ctx.Logger.Errorw("failed fetching chat ID", "err", err)
		return err

	default:
		if cID == cht {
			ctx.Logger.Info("user is already up-to-date")
			return nil
		}

		if _, err = tx.Exec(`UPDATE users SET chat_id=$1`, cht); err != nil {
			ctx.Logger.Errorw("failed updating chat_id", "err", err)
			return err
		}
	}

	if err = tx.Commit(); err != nil {
		ctx.Logger.Errorw("failed committing Tx for adding user", "err", err)
		return err
	}

	return nil
}

func AddMovie(ctx *bot.Context, usr int64, mv *Movie) error {
	query := `INSERT INTO movies (title, alt_title, year, created_on, created_by) VALUES ($1, $2, $3, $4, $5)`
	if _, err := ctx.DB.Exec(query, mv.Title, mv.AltTitle, mv.Year, clk.Now().UTC(), usr); err != nil {
		ctx.Logger.Errorw("failed inserting movie", "err", err)
		return err
	}

	return nil
}

func DelMovie(ctx *bot.Context, usr int64, movieID int) error {
	tx, err := ctx.DB.BeginTx(context.Background(), txIsoRepeatableRead)
	if err != nil {
		ctx.Logger.Errorw("failed starting delete movie transaction", "err", err)
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM ratings WHERE movie_id=$1`, movieID); err != nil {
		ctx.Logger.Errorw("failed deleting movie", "err", err)
		return err
	}

	if _, err := tx.Exec(`DELETE FROM movies WHERE id=$1`, movieID); err != nil {
		ctx.Logger.Errorw("failed deleting movie", "err", err)
		return err
	}

	if err := tx.Commit(); err != nil {
		ctx.Logger.Errorw("failed committing delete movie", "err", err)
		return err
	}

	return nil
}

func RandomMovie(ctx *bot.Context, usr int64) (*Movie, error) {
	query := `SELECT id, title, alt_title, year, rating
FROM movies m
	LEFT JOIN (
		SELECT movie_id, rating
		FROM ratings
	) r ON m.id=r.movie_id AND r.movie_id=$1
ORDER BY RANDOM() LIMIT 1`
	mv, err := getMovie(ctx, usr, query, usr)
	if err != nil {
		ctx.Logger.Errorw("failed fetching random movie", "err", err)
	}

	return mv, nil
}

func getMovie(ctx *bot.Context, usr int64, query string, args ...any) (*Movie, error) {
	var altTitle sql.NullString
	var year sql.NullInt16
	var rating sql.NullFloat64
	var mv Movie

	if err := ctx.DB.QueryRow(query, args...).Scan(&mv.ID, &mv.Title, &altTitle, &year, &rating); err != nil {
		return &Movie{}, err
	}

	if altTitle.Valid {
		mv.AltTitle = altTitle.String
	}

	if year.Valid {
		mv.Year = year.Int16
	} else {
		mv.Year = -1
	}

	if rating.Valid {
		mv.Rating = float32(rating.Float64)
	} else {
		mv.Rating = -1
	}
	return &mv, nil
}

func RateMovie(ctx *bot.Context, usr int64, movieID int, rating int) error {
	var r int
	err := ctx.DB.QueryRow(`SELECT rating FROM ratings WHERE user_id=$1 AND movie_id=$2`, usr, movieID).Scan(&r)
	switch {
	case err == sql.ErrNoRows:
		query := `INSERT INTO ratings (user_id, movie_id, rating, created_on) VALUES ($1, $2, $3, $4)`
		if _, err := ctx.DB.Exec(query, usr, movieID, rating, clk.Now().UTC()); err != nil {
			ctx.Logger.Errorw(fmt.Sprintf("failed rating movie %d", movieID), "err", err)
			return err
		}

	case err != nil:
		ctx.Logger.Error(fmt.Sprintf("failed rating movie %d", movieID), "err", err)
		return err

	default:
		if r == rating {
			return nil
		}

		query := `UPDATE ratings SET rating=$1, updated_on=$2 WHERE user_id=$3 AND movie_id=$4`
		if _, err := ctx.DB.Exec(query, rating, clk.Now().UTC(), usr, movieID); err != nil {
			ctx.Logger.Errorw(fmt.Sprintf("failed rating movie %d", movieID), "err", err)
			return err
		}
	}

	return nil
}

func UnrateMovie(ctx *bot.Context, usr int64, movie int) (bool, error) {
	var r int
	err := ctx.DB.QueryRow(`SELECT rating FROM ratings WHERE user_id=$1 AND movie_id=$2`, usr, movie).Scan(&r)
	switch {
	case err == sql.ErrNoRows:
		return false, nil

	case err != nil:
		ctx.Logger.Errorw(fmt.Sprintf("failed unrating movie %d", movie), "err", err)
		return false, err

	default:
		query := `DELETE FROM ratings WHERE user_id=$1 AND movie_id=$2`
		if _, err := ctx.DB.Exec(query, usr, movie); err != nil {
			ctx.Logger.Errorw(fmt.Sprintf("failed unrating movie %d", movie), "err", err)
			return false, err
		}
	}

	return true, nil
}

type MovieState int

type Movie struct {
	ID       int
	Title    string
	AltTitle string
	Year     int16
	Rating   float32
}

const (
	MovieStateSeen MovieState = iota
	MovieStateUnseen
	MovieStateAll
)

func listMovies(ctx *bot.Context, usr int64, rows *sql.Rows) ([]*Movie, error) {
	var err error
	movies := []*Movie{}
	for rows.Next() {
		var mv Movie
		var altTitle sql.NullString
		var year sql.NullInt16
		var rating sql.NullFloat64
		if err = rows.Scan(&mv.ID, &mv.Title, &altTitle, &year, &rating); err != nil {
			ctx.Logger.Errorw("couldn't read attributes of a movie", "err", err)
			continue
		}

		if altTitle.Valid {
			mv.AltTitle = altTitle.String
		}

		if year.Valid {
			mv.Year = year.Int16
		} else {
			mv.Year = -1
		}

		if rating.Valid {
			mv.Rating = float32(rating.Float64)
		} else {
			mv.Rating = -1
		}
		
		movies = append(movies, &mv)
	}

	return movies, err
}

func ListSeenMovies(ctx *bot.Context, usr int64) ([]*Movie, error) {
	query := `SELECT m.id, m.title, m.alt_title, m.year, r2.avg_rating
FROM movies m
	JOIN ratings r1 ON m.id=r1.movie_id AND r1.user_id=$1
	JOIN (
		SELECT movie_id, AVG(rating) AS avg_rating
		FROM ratings
		GROUP BY movie_id
	) r2 ON m.id=r2.movie_id
ORDER BY m.title`
	rows, err := ctx.DB.Query(query, usr)
	if err != nil {
		ctx.Logger.Errorw("failed querying seen movies", "err", err)
		return []*Movie{}, nil
	}
	defer rows.Close()

	return listMovies(ctx, usr, rows)
}

func ListUnseenMovies(ctx *bot.Context, usr int64) ([]*Movie, error) {
	query := `SELECT m.id, m.title, m.alt_title, m.year, r1.avg_rating
FROM movies m
	LEFT JOIN (
		SELECT movie_id, AVG(rating) AS avg_rating
		FROM ratings
		GROUP BY movie_id
	) r1 ON m.id=r1.movie_id
	LEFT JOIN (
		SELECT movie_id
		FROM ratings
		WHERE user_id=$1
	) r2 ON m.id=r2.movie_id
WHERE r2.movie_id IS NULL
ORDER BY m.title`
	rows, err := ctx.DB.Query(query, usr)
	if err != nil {
		ctx.Logger.Errorw("failed querying seen movies", "err", err)
		return []*Movie{}, nil
	}
	defer rows.Close()

	return listMovies(ctx, usr, rows)
}

func ListAllMovies(ctx *bot.Context, usr int64) ([]*Movie, error) {
	query := `SELECT m.id, m.title, m.alt_title, m.year, r.avg_rating
FROM movies m
	LEFT JOIN (
		SELECT movie_id, AVG(rating) AS avg_rating
		FROM ratings
		GROUP BY movie_id
	) r ON m.id=r.movie_id
ORDER BY m.title`
	rows, err := ctx.DB.Query(query)
	if err != nil {
		ctx.Logger.Errorw("failed querying all movies", "err", err)
		return []*Movie{}, nil
	}
	defer rows.Close()

	return listMovies(ctx, usr, rows)
}

func ListMyMovies(ctx *bot.Context, usr int64) ([]*Movie, error) {
	query := `SELECT m.id, m.title, m.alt_title, m.year, r.rating
FROM movies m
	LEFT JOIN ratings r ON m.id=r.movie_id AND r.user_id=$1
WHERE m.created_by=$1
ORDER BY m.title`
	rows, err := ctx.DB.Query(query, usr)
	if err != nil {
		ctx.Logger.Errorw("failed querying all movies", "err", err)
		return []*Movie{}, nil
	}
	defer rows.Close()

	return listMovies(ctx, usr, rows)
}

func ListTopMovies(ctx *bot.Context, usr int64) ([]*Movie, error) {
	query := `SELECT m.id, m.title, m.alt_title, m.year, r.avg_rating
FROM movies m
	LEFT JOIN (
		SELECT movie_id, AVG(rating) AS avg_rating
		FROM ratings
		GROUP BY movie_id
	) r ON m.id=r.movie_id
ORDER BY avg_rating DESC NULLS LAST, m.title
LIMIT 10`
	rows, err := ctx.DB.Query(query)
	if err != nil {
		ctx.Logger.Errorw("failed querying top movies", "err", err)
		return []*Movie{}, err
	}
	defer rows.Close()

	return listMovies(ctx, usr, rows)
}

func ListLatestMovies(ctx *bot.Context, usr int64) ([]*Movie, error) {
	query := `SELECT m.id, m.title, m.alt_title, m.year, r.avg_rating
FROM movies m
	LEFT JOIN (
		SELECT movie_id, AVG(rating) AS avg_rating
		FROM ratings
		GROUP BY movie_id
	) r ON m.id=r.movie_id
ORDER BY created_on DESC, m.title
LIMIT 10`
	rows, err := ctx.DB.Query(query)
	if err != nil {
		ctx.Logger.Errorw("failed querying latest movies", "err", err)
		return []*Movie{}, err
	}
	defer rows.Close()

	return listMovies(ctx, usr, rows)
}

func GetMovie(ctx *bot.Context, usr int64, id int) (*Movie, error) {
	query := `SELECT id, title, alt_title, year, NULL FROM movies WHERE id=$1`
	mv, err := getMovie(ctx, usr, query, id)
	if err != nil {
		ctx.Logger.Errorw(fmt.Sprintf("failed getting movie %d", id), "err", err)
		return &Movie{}, err
	}

	return mv, nil
}
