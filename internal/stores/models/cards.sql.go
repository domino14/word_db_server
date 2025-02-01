// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.28.0
// source: cards.sql

package models

import (
	"context"

	"github.com/domino14/word_db_server/internal/stores"
	"github.com/jackc/pgx/v5/pgtype"
	go_fsrs "github.com/open-spaced-repetition/go-fsrs/v3"
)

const addCards = `-- name: AddCards :one
WITH inserted_rows AS (
    INSERT INTO wordvault_cards(
        alphagram, next_scheduled, fsrs_card, user_id, lexicon_name, review_log, deck_id
    )
    SELECT
        unnest($1::TEXT[]),
        unnest($2::TIMESTAMPTZ[]),
        unnest($3::JSONB[]),
        unnest(array_fill($4::BIGINT, array[array_length($1, 1)])),
        unnest(array_fill($5::TEXT, array[array_length($1, 1)])),
        unnest(
            COALESCE(
                $6::JSONB[],
                array_fill('[]'::JSONB, array[array_length($1, 1)])
            )
        ),
        $7::BIGINT
    ON CONFLICT(user_id, lexicon_name, alphagram) DO NOTHING
    RETURNING 1
)
SELECT COUNT(*) FROM inserted_rows
`

type AddCardsParams struct {
	Alphagrams     []string
	NextScheduleds []pgtype.Timestamptz
	FsrsCards      [][]byte
	UserID         int64
	LexiconName    string
	ReviewLogs     [][]byte
	DeckID         pgtype.Int8
}

func (q *Queries) AddCards(ctx context.Context, arg AddCardsParams) (int64, error) {
	row := q.db.QueryRow(ctx, addCards,
		arg.Alphagrams,
		arg.NextScheduleds,
		arg.FsrsCards,
		arg.UserID,
		arg.LexiconName,
		arg.ReviewLogs,
		arg.DeckID,
	)
	var count int64
	err := row.Scan(&count)
	return count, err
}

const addDeck = `-- name: AddDeck :one
INSERT INTO wordvault_decks(user_id, lexicon_name, name)
VALUES ($1, $2, $3)
RETURNING id, user_id, lexicon_name, fsrs_params_override, name
`

type AddDeckParams struct {
	UserID      int64
	LexiconName string
	Name        string
}

func (q *Queries) AddDeck(ctx context.Context, arg AddDeckParams) (WordvaultDeck, error) {
	row := q.db.QueryRow(ctx, addDeck, arg.UserID, arg.LexiconName, arg.Name)
	var i WordvaultDeck
	err := row.Scan(
		&i.ID,
		&i.UserID,
		&i.LexiconName,
		&i.FsrsParamsOverride,
		&i.Name,
	)
	return i, err
}

const bulkUpdateCards = `-- name: BulkUpdateCards :exec
WITH updated_values AS (
  SELECT
    UNNEST($1::TEXT[]) AS alphagram,
    UNNEST($2::TIMESTAMPTZ[]) AS next_scheduled,
    UNNEST($3::JSONB[]) AS fsrs_card,
    UNNEST(array_fill($4::BIGINT, array[array_length($1, 1)])) AS user_id,
    UNNEST(array_fill($5::TEXT, array[array_length($1, 1)])) AS lexicon_name
)
UPDATE wordvault_cards w
SET
  fsrs_card = u.fsrs_card,
  next_scheduled = u.next_scheduled
FROM updated_values u
WHERE
  w.user_id = u.user_id AND
  w.lexicon_name = u.lexicon_name AND
  w.alphagram = u.alphagram
`

type BulkUpdateCardsParams struct {
	Alphagrams     []string
	NextScheduleds []pgtype.Timestamptz
	FsrsCards      [][]byte
	UserID         int64
	LexiconName    string
}

func (q *Queries) BulkUpdateCards(ctx context.Context, arg BulkUpdateCardsParams) error {
	_, err := q.db.Exec(ctx, bulkUpdateCards,
		arg.Alphagrams,
		arg.NextScheduleds,
		arg.FsrsCards,
		arg.UserID,
		arg.LexiconName,
	)
	return err
}

const deleteCards = `-- name: DeleteCards :execrows
DELETE FROM wordvault_cards
WHERE user_id = $1 AND lexicon_name = $2
`

type DeleteCardsParams struct {
	UserID      int64
	LexiconName string
}

func (q *Queries) DeleteCards(ctx context.Context, arg DeleteCardsParams) (int64, error) {
	result, err := q.db.Exec(ctx, deleteCards, arg.UserID, arg.LexiconName)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected(), nil
}

const deleteCardsWithAlphagrams = `-- name: DeleteCardsWithAlphagrams :execrows
DELETE FROM wordvault_cards
WHERE user_id = $1 AND lexicon_name = $2 AND alphagram = ANY($3::text[])
`

type DeleteCardsWithAlphagramsParams struct {
	UserID      int64
	LexiconName string
	Alphagrams  []string
}

func (q *Queries) DeleteCardsWithAlphagrams(ctx context.Context, arg DeleteCardsWithAlphagramsParams) (int64, error) {
	result, err := q.db.Exec(ctx, deleteCardsWithAlphagrams, arg.UserID, arg.LexiconName, arg.Alphagrams)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected(), nil
}

const deleteDeck = `-- name: DeleteDeck :exec
DELETE FROM wordvault_decks
WHERE id = $1
`

func (q *Queries) DeleteDeck(ctx context.Context, id int64) error {
	_, err := q.db.Exec(ctx, deleteDeck, id)
	return err
}

const deleteNewCards = `-- name: DeleteNewCards :execrows
DELETE FROM wordvault_cards
WHERE user_id = $1 AND lexicon_name = $2 AND jsonb_array_length(review_log) = 0
`

type DeleteNewCardsParams struct {
	UserID      int64
	LexiconName string
}

func (q *Queries) DeleteNewCards(ctx context.Context, arg DeleteNewCardsParams) (int64, error) {
	result, err := q.db.Exec(ctx, deleteNewCards, arg.UserID, arg.LexiconName)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected(), nil
}

const editDeck = `-- name: EditDeck :one
UPDATE wordvault_decks
SET name = $2
WHERE id = $1 AND user_id = $3
RETURNING id, user_id, lexicon_name, fsrs_params_override, name
`

type EditDeckParams struct {
	ID     int64
	Name   string
	UserID int64
}

func (q *Queries) EditDeck(ctx context.Context, arg EditDeckParams) (WordvaultDeck, error) {
	row := q.db.QueryRow(ctx, editDeck, arg.ID, arg.Name, arg.UserID)
	var i WordvaultDeck
	err := row.Scan(
		&i.ID,
		&i.UserID,
		&i.LexiconName,
		&i.FsrsParamsOverride,
		&i.Name,
	)
	return i, err
}

const getCard = `-- name: GetCard :one
SELECT next_scheduled, fsrs_card, review_log
FROM wordvault_cards
WHERE user_id = $1 AND lexicon_name = $2 AND alphagram = $3
`

type GetCardParams struct {
	UserID      int64
	LexiconName string
	Alphagram   string
}

type GetCardRow struct {
	NextScheduled pgtype.Timestamptz
	FsrsCard      stores.Card
	ReviewLog     []stores.ReviewLog
}

func (q *Queries) GetCard(ctx context.Context, arg GetCardParams) (GetCardRow, error) {
	row := q.db.QueryRow(ctx, getCard, arg.UserID, arg.LexiconName, arg.Alphagram)
	var i GetCardRow
	err := row.Scan(&i.NextScheduled, &i.FsrsCard, &i.ReviewLog)
	return i, err
}

const getCards = `-- name: GetCards :many
SELECT alphagram, next_scheduled, fsrs_card, review_log, deck_id
FROM wordvault_cards
WHERE user_id = $1 AND lexicon_name = $2 AND alphagram = ANY($3::text[])
`

type GetCardsParams struct {
	UserID      int64
	LexiconName string
	Alphagrams  []string
}

type GetCardsRow struct {
	Alphagram     string
	NextScheduled pgtype.Timestamptz
	FsrsCard      stores.Card
	ReviewLog     []stores.ReviewLog
	DeckID        pgtype.Int8
}

func (q *Queries) GetCards(ctx context.Context, arg GetCardsParams) ([]GetCardsRow, error) {
	rows, err := q.db.Query(ctx, getCards, arg.UserID, arg.LexiconName, arg.Alphagrams)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []GetCardsRow
	for rows.Next() {
		var i GetCardsRow
		if err := rows.Scan(
			&i.Alphagram,
			&i.NextScheduled,
			&i.FsrsCard,
			&i.ReviewLog,
			&i.DeckID,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const getDecks = `-- name: GetDecks :many
SELECT id, user_id, lexicon_name, fsrs_params_override, name
FROM wordvault_decks
WHERE user_id = $1
`

func (q *Queries) GetDecks(ctx context.Context, userID int64) ([]WordvaultDeck, error) {
	rows, err := q.db.Query(ctx, getDecks, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []WordvaultDeck
	for rows.Next() {
		var i WordvaultDeck
		if err := rows.Scan(
			&i.ID,
			&i.UserID,
			&i.LexiconName,
			&i.FsrsParamsOverride,
			&i.Name,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const getNextScheduled = `-- name: GetNextScheduled :many
SELECT alphagram, next_scheduled, fsrs_card, deck_id
FROM wordvault_cards
WHERE user_id = $1 AND lexicon_name = $2 AND next_scheduled <= $3
    AND (($5::bigint IS NULL AND deck_id IS NULL) OR deck_id = $5::bigint)
ORDER BY next_scheduled ASC
LIMIT $4
`

type GetNextScheduledParams struct {
	UserID        int64
	LexiconName   string
	NextScheduled pgtype.Timestamptz
	Limit         int32
	DeckID        pgtype.Int8
}

type GetNextScheduledRow struct {
	Alphagram     string
	NextScheduled pgtype.Timestamptz
	FsrsCard      stores.Card
	DeckID        pgtype.Int8
}

func (q *Queries) GetNextScheduled(ctx context.Context, arg GetNextScheduledParams) ([]GetNextScheduledRow, error) {
	rows, err := q.db.Query(ctx, getNextScheduled,
		arg.UserID,
		arg.LexiconName,
		arg.NextScheduled,
		arg.Limit,
		arg.DeckID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []GetNextScheduledRow
	for rows.Next() {
		var i GetNextScheduledRow
		if err := rows.Scan(
			&i.Alphagram,
			&i.NextScheduled,
			&i.FsrsCard,
			&i.DeckID,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const getNextScheduledBreakdown = `-- name: GetNextScheduledBreakdown :many
WITH scheduled_cards AS (
    SELECT
        CASE WHEN next_scheduled <= $3 THEN '-infinity'::date
        ELSE (next_scheduled AT TIME ZONE $4::text)::date END
        AS scheduled_date
    FROM
        wordvault_cards
    WHERE user_id = $1 AND lexicon_name = $2
)
SELECT
    scheduled_date,
    COUNT(*) AS question_count
FROM
    scheduled_cards
GROUP BY
    scheduled_date
ORDER BY
    scheduled_date
`

type GetNextScheduledBreakdownParams struct {
	UserID      int64
	LexiconName string
	Now         pgtype.Timestamptz
	Tz          string
}

type GetNextScheduledBreakdownRow struct {
	ScheduledDate pgtype.Date
	QuestionCount int64
}

func (q *Queries) GetNextScheduledBreakdown(ctx context.Context, arg GetNextScheduledBreakdownParams) ([]GetNextScheduledBreakdownRow, error) {
	rows, err := q.db.Query(ctx, getNextScheduledBreakdown,
		arg.UserID,
		arg.LexiconName,
		arg.Now,
		arg.Tz,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []GetNextScheduledBreakdownRow
	for rows.Next() {
		var i GetNextScheduledBreakdownRow
		if err := rows.Scan(&i.ScheduledDate, &i.QuestionCount); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const getNumCardsInVault = `-- name: GetNumCardsInVault :many
SELECT lexicon_name, count(*) as card_count FROM wordvault_cards
WHERE user_id = $1
GROUP BY lexicon_name
`

type GetNumCardsInVaultRow struct {
	LexiconName string
	CardCount   int64
}

func (q *Queries) GetNumCardsInVault(ctx context.Context, userID int64) ([]GetNumCardsInVaultRow, error) {
	rows, err := q.db.Query(ctx, getNumCardsInVault, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []GetNumCardsInVaultRow
	for rows.Next() {
		var i GetNumCardsInVaultRow
		if err := rows.Scan(&i.LexiconName, &i.CardCount); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const getOverdueCount = `-- name: GetOverdueCount :one
SELECT
    count(*) from wordvault_cards
WHERE next_scheduled <= $3 AND user_id = $1 AND lexicon_name = $2
`

type GetOverdueCountParams struct {
	UserID      int64
	LexiconName string
	Now         pgtype.Timestamptz
}

func (q *Queries) GetOverdueCount(ctx context.Context, arg GetOverdueCountParams) (int64, error) {
	row := q.db.QueryRow(ctx, getOverdueCount, arg.UserID, arg.LexiconName, arg.Now)
	var count int64
	err := row.Scan(&count)
	return count, err
}

const getSingleNextScheduled = `-- name: GetSingleNextScheduled :one
WITH matching_cards AS (
  SELECT
    alphagram,
    next_scheduled,
    fsrs_card,
    deck_id,
    COUNT(*) OVER () AS total_count -- Window function to get the total count
  FROM wordvault_cards
  WHERE user_id = $1
    AND lexicon_name = $2
    AND next_scheduled <= $3
    AND (($4::BIGINT IS NULL AND deck_id IS NULL) OR $4::BIGINT = deck_id)
  ORDER BY
    -- When short-term scheduling is enabled, we want to de-prioritize
    -- new cards so that you clear your backlog of reviewed cards first.
    CASE WHEN CAST(fsrs_card->'State' AS INTEGER) = 0 THEN FALSE ELSE $5::bool END DESC,
    next_scheduled ASC
)
SELECT alphagram, next_scheduled, fsrs_card, deck_id, total_count FROM matching_cards
LIMIT 1
`

type GetSingleNextScheduledParams struct {
	UserID               int64
	LexiconName          string
	NextScheduled        pgtype.Timestamptz
	DeckID               pgtype.Int8
	IsShortTermScheduler bool
}

type GetSingleNextScheduledRow struct {
	Alphagram     string
	NextScheduled pgtype.Timestamptz
	FsrsCard      []byte
	DeckID        pgtype.Int8
	TotalCount    int64
}

func (q *Queries) GetSingleNextScheduled(ctx context.Context, arg GetSingleNextScheduledParams) (GetSingleNextScheduledRow, error) {
	row := q.db.QueryRow(ctx, getSingleNextScheduled,
		arg.UserID,
		arg.LexiconName,
		arg.NextScheduled,
		arg.DeckID,
		arg.IsShortTermScheduler,
	)
	var i GetSingleNextScheduledRow
	err := row.Scan(
		&i.Alphagram,
		&i.NextScheduled,
		&i.FsrsCard,
		&i.DeckID,
		&i.TotalCount,
	)
	return i, err
}

const loadFsrsParams = `-- name: LoadFsrsParams :one
SELECT params FROM wordvault_params
WHERE user_id = $1
`

func (q *Queries) LoadFsrsParams(ctx context.Context, userID int64) (go_fsrs.Parameters, error) {
	row := q.db.QueryRow(ctx, loadFsrsParams, userID)
	var params go_fsrs.Parameters
	err := row.Scan(&params)
	return params, err
}

const moveCards = `-- name: MoveCards :one
WITH moved_rows AS (
    UPDATE wordvault_cards
    SET deck_id = $3
    WHERE user_id = $1 AND lexicon_name = $2 AND alphagram = ANY($4::text[])
    RETURNING 1
)
SELECT COUNT(*) from moved_rows
`

type MoveCardsParams struct {
	UserID      int64
	LexiconName string
	DeckID      pgtype.Int8
	Alphagrams  []string
}

func (q *Queries) MoveCards(ctx context.Context, arg MoveCardsParams) (int64, error) {
	row := q.db.QueryRow(ctx, moveCards,
		arg.UserID,
		arg.LexiconName,
		arg.DeckID,
		arg.Alphagrams,
	)
	var count int64
	err := row.Scan(&count)
	return count, err
}

const postponementQuery = `-- name: PostponementQuery :many
SELECT alphagram, next_scheduled, fsrs_card
FROM wordvault_cards
WHERE user_id = $1 AND lexicon_name = $2 AND next_scheduled <= $3
AND jsonb_array_length(review_log) > 0
`

type PostponementQueryParams struct {
	UserID        int64
	LexiconName   string
	NextScheduled pgtype.Timestamptz
}

type PostponementQueryRow struct {
	Alphagram     string
	NextScheduled pgtype.Timestamptz
	FsrsCard      stores.Card
}

func (q *Queries) PostponementQuery(ctx context.Context, arg PostponementQueryParams) ([]PostponementQueryRow, error) {
	rows, err := q.db.Query(ctx, postponementQuery, arg.UserID, arg.LexiconName, arg.NextScheduled)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []PostponementQueryRow
	for rows.Next() {
		var i PostponementQueryRow
		if err := rows.Scan(&i.Alphagram, &i.NextScheduled, &i.FsrsCard); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const setFsrsParams = `-- name: SetFsrsParams :exec
INSERT INTO wordvault_params(user_id, params)
VALUES ($1, $2)
ON CONFLICT(user_id) DO UPDATE
SET params = $2
`

type SetFsrsParamsParams struct {
	UserID int64
	Params go_fsrs.Parameters
}

func (q *Queries) SetFsrsParams(ctx context.Context, arg SetFsrsParamsParams) error {
	_, err := q.db.Exec(ctx, setFsrsParams, arg.UserID, arg.Params)
	return err
}

const updateCard = `-- name: UpdateCard :exec
UPDATE wordvault_cards
SET fsrs_card = $1, next_scheduled = $2, review_log = review_log || $6::jsonb
WHERE user_id = $3 AND lexicon_name = $4 AND alphagram = $5
`

type UpdateCardParams struct {
	FsrsCard      stores.Card
	NextScheduled pgtype.Timestamptz
	UserID        int64
	LexiconName   string
	Alphagram     string
	ReviewLogItem []byte
}

func (q *Queries) UpdateCard(ctx context.Context, arg UpdateCardParams) error {
	_, err := q.db.Exec(ctx, updateCard,
		arg.FsrsCard,
		arg.NextScheduled,
		arg.UserID,
		arg.LexiconName,
		arg.Alphagram,
		arg.ReviewLogItem,
	)
	return err
}

const updateCardReplaceLastLog = `-- name: UpdateCardReplaceLastLog :exec
UPDATE wordvault_cards
SET
    fsrs_card = $1,
    next_scheduled = $2,
    review_log = (review_log - (jsonb_array_length(review_log) - 1)) || $6::jsonb
WHERE
    user_id = $3
    AND lexicon_name = $4
    AND alphagram = $5
`

type UpdateCardReplaceLastLogParams struct {
	FsrsCard      stores.Card
	NextScheduled pgtype.Timestamptz
	UserID        int64
	LexiconName   string
	Alphagram     string
	ReviewLogItem []byte
}

func (q *Queries) UpdateCardReplaceLastLog(ctx context.Context, arg UpdateCardReplaceLastLogParams) error {
	_, err := q.db.Exec(ctx, updateCardReplaceLastLog,
		arg.FsrsCard,
		arg.NextScheduled,
		arg.UserID,
		arg.LexiconName,
		arg.Alphagram,
		arg.ReviewLogItem,
	)
	return err
}
