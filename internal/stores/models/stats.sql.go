// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.28.0
// source: stats.sql

package models

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

const getDailyLeaderboard = `-- name: GetDailyLeaderboard :many
SELECT
    u.username,
    COUNT(*) AS cards_studied_today
FROM
    wordvault_cards wc
JOIN
    auth_user u ON wc.user_id = u.id
WHERE
    wc.fsrs_card->>'LastReview' >= to_char(
        date_trunc('day',
            now() AT TIME ZONE $1::text)
                  AT TIME ZONE $1::text
                  AT TIME ZONE 'UTC',
        'YYYY-MM-DD"T"HH24:MI:SS'
    )
GROUP BY
    u.username
ORDER BY
    cards_studied_today DESC
`

type GetDailyLeaderboardRow struct {
	Username          pgtype.Text
	CardsStudiedToday int64
}

func (q *Queries) GetDailyLeaderboard(ctx context.Context, timezone string) ([]GetDailyLeaderboardRow, error) {
	rows, err := q.db.Query(ctx, getDailyLeaderboard, timezone)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []GetDailyLeaderboardRow
	for rows.Next() {
		var i GetDailyLeaderboardRow
		if err := rows.Scan(&i.Username, &i.CardsStudiedToday); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const getDailyProgress = `-- name: GetDailyProgress :one
SELECT
    -- Count of new cards studied today
    COALESCE(SUM(CASE
        WHEN jsonb_array_length(review_log) = 1
        THEN 1
        ELSE 0
    END), 0)::int AS new_cards,

    -- Count of reviewed cards studied today
    COALESCE(SUM(CASE
        WHEN jsonb_array_length(review_log) > 1
        THEN 1
        ELSE 0
    END), 0)::int AS reviewed_cards,

    -- Rating breakdown for new cards
    COALESCE(COUNT(*) FILTER (
        WHERE jsonb_array_length(review_log) = 1
              AND (review_log->0->>'Rating')::int = 1
    ), 0)::int AS new_rating_1,
    COALESCE(COUNT(*) FILTER (
        WHERE jsonb_array_length(review_log) = 1
              AND (review_log->0->>'Rating')::int = 2
    ), 0)::int AS new_rating_2,
    COALESCE(COUNT(*) FILTER (
        WHERE jsonb_array_length(review_log) = 1
              AND (review_log->0->>'Rating')::int = 3
    ), 0)::int AS new_rating_3,
    COALESCE(COUNT(*) FILTER (
        WHERE jsonb_array_length(review_log) = 1
              AND (review_log->0->>'Rating')::int = 4
    ), 0)::int AS new_rating_4,

    -- Rating breakdown for reviewed cards
    COALESCE(COUNT(*) FILTER (
        WHERE jsonb_array_length(review_log) > 1
              AND (review_log->-1->>'Rating')::int = 1
    ), 0)::int AS reviewed_rating_1,
    COALESCE(COUNT(*) FILTER (
        WHERE jsonb_array_length(review_log) > 1
              AND (review_log->-1->>'Rating')::int = 2
    ), 0)::int AS reviewed_rating_2,
    COALESCE(COUNT(*) FILTER (
        WHERE jsonb_array_length(review_log) > 1
              AND (review_log->-1->>'Rating')::int = 3
    ), 0)::int AS reviewed_rating_3,
    COALESCE(COUNT(*) FILTER (
        WHERE jsonb_array_length(review_log) > 1
              AND (review_log->-1->>'Rating')::int = 4
    ), 0)::int AS reviewed_rating_4
FROM
    wordvault_cards
WHERE
    user_id = $1
    AND ((fsrs_card->>'LastReview')::timestamp AT TIME ZONE 'UTC' AT TIME ZONE $2::text)::date =
        ($3::timestamptz AT TIME ZONE $2::text)::date
`

type GetDailyProgressParams struct {
	UserID   int64
	Timezone string
	Now      pgtype.Timestamptz
}

type GetDailyProgressRow struct {
	NewCards        int32
	ReviewedCards   int32
	NewRating1      int32
	NewRating2      int32
	NewRating3      int32
	NewRating4      int32
	ReviewedRating1 int32
	ReviewedRating2 int32
	ReviewedRating3 int32
	ReviewedRating4 int32
}

func (q *Queries) GetDailyProgress(ctx context.Context, arg GetDailyProgressParams) (GetDailyProgressRow, error) {
	row := q.db.QueryRow(ctx, getDailyProgress, arg.UserID, arg.Timezone, arg.Now)
	var i GetDailyProgressRow
	err := row.Scan(
		&i.NewCards,
		&i.ReviewedCards,
		&i.NewRating1,
		&i.NewRating2,
		&i.NewRating3,
		&i.NewRating4,
		&i.ReviewedRating1,
		&i.ReviewedRating2,
		&i.ReviewedRating3,
		&i.ReviewedRating4,
	)
	return i, err
}
