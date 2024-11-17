package wordvault

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/mattn/go-sqlite3"
	"github.com/open-spaced-repetition/go-fsrs/v3"
	"github.com/rs/zerolog/log"

	"github.com/domino14/word_db_server/internal/auth"
	"github.com/domino14/word_db_server/internal/searchserver"
	"github.com/domino14/word_db_server/internal/stores"
	"github.com/domino14/word_db_server/internal/stores/models"
)

func LeitnerImport(ctx context.Context, searchServer *searchserver.Server, lexicon string, queries *models.Queries, dbPool *pgxpool.Pool,
	sqliteFilename string) (int, []string, error) {

	u := auth.UserFromContext(ctx)
	if u != nil {
		log.Info().Str("username", u.Username).Msg("authenticated-user-importing-cardbox")
	} else {
		return 0, nil, errors.New("leitner-import-needs-authentication")
	}

	now := time.Now()

	// Open the SQLite database
	sqliteDB, err := sql.Open("sqlite3", sqliteFilename)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to open SQLite database: %w", err)
	}
	defer sqliteDB.Close()

	// Begin a transaction in PostgreSQL
	tx, err := dbPool.Begin(ctx)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback(ctx) // Roll back the transaction if it isn't committed

	// Use the transaction with the queries object
	qtx := queries.WithTx(tx)

	// Fetch Leitner cards from SQLite in batches
	rows, err := sqliteDB.QueryContext(ctx, `
        SELECT question, correct, incorrect, streak, last_correct, cardbox, next_scheduled
        FROM questions`)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to fetch questions from SQLite: %w", err)
	}
	defer rows.Close()

	// Initialize batch parameters
	var (
		batchSize     = 1000
		totalInserted = 0
		batch         models.AddCardsParams
		batchCount    int
	)

	unimportedAlphagrams := []string{}

	for rows.Next() {
		var (
			question      string
			correct       int
			incorrect     int
			streak        int
			lastCorrect   sql.NullInt32
			cardbox       sql.NullInt32
			nextScheduled sql.NullInt32
		)

		if err := rows.Scan(&question, &correct, &incorrect, &streak, &lastCorrect, &cardbox, &nextScheduled); err != nil {
			return 0, nil, fmt.Errorf("failed to scan question: %w", err)
		}

		exists, err := searchServer.HasAlphagram(question, lexicon)
		if err != nil {
			return 0, nil, fmt.Errorf("failed to find alphagram: %w", err)
		}
		if !exists {
			log.Info().Str("question", question).Str("lexicon", lexicon).
				Msg("did not import, does not exist in lex")
			unimportedAlphagrams = append(unimportedAlphagrams, question)
			continue
		}
		if !cardbox.Valid {
			// This question was probably not meant to be imported.
			log.Info().Str("question", question).Msg("did not import, cardbox null")
			unimportedAlphagrams = append(unimportedAlphagrams, question)
			continue
		}

		// Convert from Leitner to FSRS (stub conversion)
		fsrsCard, reviewLogs, nextScheduledTs := convertLeitnerToFsrs(
			correct, incorrect, streak, lastCorrect, nextScheduled, cardbox, now)

		cardbts, err := json.Marshal(fsrsCard.Card)
		if err != nil {
			return 0, nil, err
		}
		rlbts, err := json.Marshal(reviewLogs)
		if err != nil {
			return 0, nil, err
		}

		// Add to the batch
		batch.Alphagrams = append(batch.Alphagrams, question)
		batch.NextScheduleds = append(batch.NextScheduleds, nextScheduledTs)
		batch.FsrsCards = append(batch.FsrsCards, cardbts)
		batch.UserID = int64(u.DBID)
		batch.LexiconName = lexicon
		batch.ReviewLogs = append(batch.ReviewLogs, rlbts)

		batchCount++

		// If batch size is reached, execute AddCards and reset the batch
		if batchCount >= batchSize {
			inserted, err := qtx.AddCards(ctx, batch)
			if err != nil {
				return 0, nil, fmt.Errorf("failed to insert batch: %w", err)
			}
			totalInserted += int(inserted)
			batch = models.AddCardsParams{} // Reset the batch
			batchCount = 0
		}
	}

	// Insert any remaining batch
	if batchCount > 0 {
		inserted, err := qtx.AddCards(ctx, batch)
		if err != nil {
			return 0, nil, fmt.Errorf("failed to insert final batch: %w", err)
		}
		totalInserted += int(inserted)
	}

	// Commit the transaction
	if err := tx.Commit(ctx); err != nil {
		return 0, nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return totalInserted, unimportedAlphagrams, nil

}
func convertLeitnerToFsrs(correct, incorrect, streak int, lastCorrect, nextScheduled, cardbox sql.NullInt32,
	now time.Time) (stores.Card, []stores.ReviewLog, pgtype.Timestamptz) {

	lc := lastCorrect.Int32
	if !lastCorrect.Valid {
		lc = 0
	}

	nextScheduledTime := time.Unix(int64(nextScheduled.Int32), 0).UTC()
	if !nextScheduled.Valid || nextScheduled.Int32 == 0 {
		nextScheduledTime = now
	}

	cb := cardbox.Int32
	if !cardbox.Valid {
		cb = -1
	}

	lastCorrectTime := time.Unix(int64(lc), 0).UTC()
	reviewLog := []stores.ReviewLog{{
		ImportLog: &stores.ImportLog{
			ImportedDate:    now,
			NumCorrect:      correct,
			NumIncorrect:    incorrect,
			Streak:          streak,
			LastCorrect:     lastCorrectTime,
			CardboxAtImport: int(cb),
		},
	},
	}
	state := fsrs.Review
	if correct == 0 && incorrect == 0 {
		state = fsrs.New
	}

	// A good proxy for stability is the time it was scheduled for minus the
	// last time the answer was correct. It would be better if we had the last time
	// the question was asked, but we don't. Convert to days.
	var stability float64
	if lc != 0 {
		stability = float64(nextScheduledTime.Sub(lastCorrectTime).Hours() / 24.0)
		if stability < 0 {
			// This question was asked outside of the cardbox schedule. We don't want
			// a negative stability, this breaks the program.
			// Use a smallish number instead.
			stability = 1.0
		} else if cb == 0 && correct > 0 {
			// This card was in cardbox 0, but it has been answered correctly at some point.
			// It's possible that if we import the card the user will answer it correctly,
			// and then it might have a giant interval afterwards. We don't want that.
			// We want something akin to cardbox 1
			// Let's handwave its stability.
			stability = 1.0
		}
	} else {
		stability = float64(0.2) // It has never been correct, use some small number.
	}
	card := stores.Card{
		Card: fsrs.Card{
			Due:        nextScheduledTime,
			Stability:  stability,
			Difficulty: max(min(float64(5+incorrect)-(0.5*float64(correct)), 10), 1),
			Reps:       uint64(correct) + uint64(incorrect),
			Lapses:     uint64(incorrect), // could be this minus 1, but we don't know
			State:      state,
			LastReview: lastCorrectTime, // Not necessarily true, but it's the closest we have.
		},
	}

	nextScheduledTs := toPGTimestamp(nextScheduledTime)

	return card, reviewLog, nextScheduledTs
}
