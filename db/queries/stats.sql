-- name: GetDailyProgress :one
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
    user_id = @user_id
    AND ((fsrs_card->>'LastReview')::timestamp AT TIME ZONE 'UTC' AT TIME ZONE sqlc.arg(timezone)::text)::date =
        (sqlc.arg(now)::timestamptz AT TIME ZONE sqlc.arg(timezone)::text)::date;

-- name: GetDailyLeaderboard :many
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
            now() AT TIME ZONE sqlc.arg(timezone)::text)
                  AT TIME ZONE sqlc.arg(timezone)::text
                  AT TIME ZONE 'UTC',
        'YYYY-MM-DD"T"HH24:MI:SS'
    )
GROUP BY
    u.username
ORDER BY
    cards_studied_today DESC;

-- name: GetDailyProgressByDeck :many
SELECT
    deck_id,
    COALESCE(SUM(CASE WHEN jsonb_array_length(review_log) = 1 THEN 1 ELSE 0 END), 0)::int AS new_cards,
    COALESCE(SUM(CASE WHEN jsonb_array_length(review_log) > 1 THEN 1 ELSE 0 END), 0)::int AS reviewed_cards,
    COALESCE(COUNT(*) FILTER (WHERE jsonb_array_length(review_log) = 1 AND (review_log->0->>'Rating')::int = 1), 0)::int AS new_rating_1,
    COALESCE(COUNT(*) FILTER (WHERE jsonb_array_length(review_log) = 1 AND (review_log->0->>'Rating')::int = 2), 0)::int AS new_rating_2,
    COALESCE(COUNT(*) FILTER (WHERE jsonb_array_length(review_log) = 1 AND (review_log->0->>'Rating')::int = 3), 0)::int AS new_rating_3,
    COALESCE(COUNT(*) FILTER (WHERE jsonb_array_length(review_log) = 1 AND (review_log->0->>'Rating')::int = 4), 0)::int AS new_rating_4,
    COALESCE(COUNT(*) FILTER (WHERE jsonb_array_length(review_log) > 1 AND (review_log->-1->>'Rating')::int = 1), 0)::int AS reviewed_rating_1,
    COALESCE(COUNT(*) FILTER (WHERE jsonb_array_length(review_log) > 1 AND (review_log->-1->>'Rating')::int = 2), 0)::int AS reviewed_rating_2,
    COALESCE(COUNT(*) FILTER (WHERE jsonb_array_length(review_log) > 1 AND (review_log->-1->>'Rating')::int = 3), 0)::int AS reviewed_rating_3,
    COALESCE(COUNT(*) FILTER (WHERE jsonb_array_length(review_log) > 1 AND (review_log->-1->>'Rating')::int = 4), 0)::int AS reviewed_rating_4
FROM
    wordvault_cards
WHERE
    user_id = @user_id
    AND ((fsrs_card->>'LastReview')::timestamp AT TIME ZONE 'UTC' AT TIME ZONE sqlc.arg(timezone)::text)::date =
        (sqlc.arg(now)::timestamptz AT TIME ZONE sqlc.arg(timezone)::text)::date
GROUP BY
    deck_id
ORDER BY
    deck_id NULLS FIRST;