// backfill_daily_studied fills wordvault_daily_studied (migration 000012)
// for the last few leaderboard days from the cards' review logs, so the
// leaderboards are complete right after it's deployed. Run it once the new
// server is up and scoring into the table; it keeps any count that's higher
// than what it finds, so it's safe to run more than once.
//
// Usage: ./backfill_daily_studied [days]   (default 7, which includes today)
package main

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/domino14/word_db_server/config"
	"github.com/domino14/word_db_server/internal/stores/models"
)

func main() {
	cfg := &config.Config{}
	cfg.Load(nil)

	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	if strings.ToLower(cfg.LogLevel) == "debug" {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}

	days := 7
	if len(os.Args) > 1 {
		var err error
		days, err = strconv.Atoi(os.Args[1])
		if err != nil || days < 1 {
			panic("days must be a positive number")
		}
	}

	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		panic(err)
	}
	y, m, d := time.Now().In(loc).Date()
	firstDay := time.Date(y, m, d, 0, 0, 0, 0, time.UTC).AddDate(0, 0, -(days - 1))

	ctx := context.Background()
	dbPool, err := pgxpool.New(ctx, cfg.DBConnUri)
	if err != nil {
		panic(err)
	}
	defer dbPool.Close()

	log.Info().Str("first-day", firstDay.Format("2006-01-02")).Msg("backfilling")
	start := time.Now()
	n, err := models.New(dbPool).BackfillDailyStudied(ctx, pgtype.Date{Time: firstDay, Valid: true})
	if err != nil {
		panic(err)
	}
	log.Info().Int64("rows", n).Dur("took", time.Since(start)).Msg("done backfilling")
}
