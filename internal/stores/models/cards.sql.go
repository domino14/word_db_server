// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.27.0
// source: cards.sql

package models

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	go_fsrs "github.com/open-spaced-repetition/go-fsrs/v3"
)

const addCards = `-- name: AddCards :one
WITH inserted_rows AS (
    INSERT INTO wordvault_cards(alphagram, next_scheduled, fsrs_card, user_id, lexicon_name)
    SELECT unnest($1::TEXT[]),
        unnest($2::TIMESTAMPTZ[]),
        unnest(array_fill($3::JSONB, array[array_length($1, 1)])),
        unnest(array_fill($4::BIGINT, array[array_length($1, 1)])),
        unnest(array_fill($5::TEXT, array[array_length($1, 1)]))
    ON CONFLICT(user_id, lexicon_name, alphagram) DO NOTHING
    RETURNING 1
)
SELECT COUNT(*) FROM inserted_rows
`

type AddCardsParams struct {
	Alphagrams     []string
	NextScheduleds []pgtype.Timestamptz
	FsrsCard       []byte
	UserID         int64
	LexiconName    string
}

func (q *Queries) AddCards(ctx context.Context, arg AddCardsParams) (int64, error) {
	row := q.db.QueryRow(ctx, addCards,
		arg.Alphagrams,
		arg.NextScheduleds,
		arg.FsrsCard,
		arg.UserID,
		arg.LexiconName,
	)
	var count int64
	err := row.Scan(&count)
	return count, err
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
	FsrsCard      go_fsrs.Card
	ReviewLog     []go_fsrs.ReviewLog
}

func (q *Queries) GetCard(ctx context.Context, arg GetCardParams) (GetCardRow, error) {
	row := q.db.QueryRow(ctx, getCard, arg.UserID, arg.LexiconName, arg.Alphagram)
	var i GetCardRow
	err := row.Scan(&i.NextScheduled, &i.FsrsCard, &i.ReviewLog)
	return i, err
}

const getCards = `-- name: GetCards :many
SELECT alphagram, next_scheduled, fsrs_card
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
	FsrsCard      go_fsrs.Card
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

const getNextScheduled = `-- name: GetNextScheduled :many
SELECT alphagram, next_scheduled, fsrs_card
FROM wordvault_cards
WHERE user_id = $1 AND lexicon_name = $2 AND next_scheduled <= $3
LIMIT $4
`

type GetNextScheduledParams struct {
	UserID        int64
	LexiconName   string
	NextScheduled pgtype.Timestamptz
	Limit         int32
}

type GetNextScheduledRow struct {
	Alphagram     string
	NextScheduled pgtype.Timestamptz
	FsrsCard      go_fsrs.Card
}

func (q *Queries) GetNextScheduled(ctx context.Context, arg GetNextScheduledParams) ([]GetNextScheduledRow, error) {
	rows, err := q.db.Query(ctx, getNextScheduled,
		arg.UserID,
		arg.LexiconName,
		arg.NextScheduled,
		arg.Limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []GetNextScheduledRow
	for rows.Next() {
		var i GetNextScheduledRow
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

const loadParams = `-- name: LoadParams :one
SELECT params FROM wordvault_params
WHERE user_id = $1
`

func (q *Queries) LoadParams(ctx context.Context, userID int64) (go_fsrs.Parameters, error) {
	row := q.db.QueryRow(ctx, loadParams, userID)
	var params go_fsrs.Parameters
	err := row.Scan(&params)
	return params, err
}

const setParams = `-- name: SetParams :exec
UPDATE wordvault_params SET params = $1
WHERE user_id = $2
`

type SetParamsParams struct {
	Params go_fsrs.Parameters
	UserID int64
}

func (q *Queries) SetParams(ctx context.Context, arg SetParamsParams) error {
	_, err := q.db.Exec(ctx, setParams, arg.Params, arg.UserID)
	return err
}

const updateCard = `-- name: UpdateCard :exec
UPDATE wordvault_cards
SET fsrs_card = $1, next_scheduled = $2, review_log = review_log || $6::jsonb
WHERE user_id = $3 AND lexicon_name = $4 AND alphagram = $5
`

type UpdateCardParams struct {
	FsrsCard      go_fsrs.Card
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
    review_log = (review_log || $6::jsonb) - jsonb_array_length(review_log)
WHERE
    user_id = $3
    AND lexicon_name = $4
    AND alphagram = $5
`

type UpdateCardReplaceLastLogParams struct {
	FsrsCard      go_fsrs.Card
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
