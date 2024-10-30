
-- name: GetDailyProgress :one
SELECT
    -- Count of new cards studied today
    SUM(CASE
        WHEN jsonb_array_length(review_log) = 1
        THEN 1
        ELSE 0
    END) AS new_cards,

    -- Count of reviewed cards studied today
    SUM(CASE
        WHEN jsonb_array_length(review_log) > 1
        THEN 1
        ELSE 0
    END) AS reviewed_cards,

    -- Rating breakdown for new cards
    COUNT(*) FILTER (
        WHERE jsonb_array_length(review_log) = 1
              AND (review_log->0->>'Rating')::int = 1
    ) AS new_rating_1,
    COUNT(*) FILTER (
        WHERE jsonb_array_length(review_log) = 1
              AND (review_log->0->>'Rating')::int = 2
    ) AS new_rating_2,
    COUNT(*) FILTER (
        WHERE jsonb_array_length(review_log) = 1
              AND (review_log->0->>'Rating')::int = 3
    ) AS new_rating_3,
    COUNT(*) FILTER (
        WHERE jsonb_array_length(review_log) = 1
              AND (review_log->0->>'Rating')::int = 4
    ) AS new_rating_4,

    -- Rating breakdown for reviewed cards
    COUNT(*) FILTER (
        WHERE jsonb_array_length(review_log) > 1
              AND (review_log->-1->>'Rating')::int = 1
    ) AS reviewed_rating_1,
    COUNT(*) FILTER (
        WHERE jsonb_array_length(review_log) > 1
              AND (review_log->-1->>'Rating')::int = 2
    ) AS reviewed_rating_2,
    COUNT(*) FILTER (
        WHERE jsonb_array_length(review_log) > 1
              AND (review_log->-1->>'Rating')::int = 3
    ) AS reviewed_rating_3,
    COUNT(*) FILTER (
        WHERE jsonb_array_length(review_log) > 1
              AND (review_log->-1->>'Rating')::int = 4
    ) AS reviewed_rating_4
FROM
    wordvault_cards
WHERE
    user_id = @user_id
    AND ((fsrs_card->>'LastReview')::timestamp AT TIME ZONE 'UTC' AT TIME ZONE sqlc.arg(timezone)::text)::date = (sqlc.arg(now)::timestamptz AT TIME ZONE sqlc.arg(timezone)::text)::date;


-- name: GetDailyLeaderboard :many
SELECT
    user_id,
    COUNT(*) AS cards_studied_today
FROM
    wordvault_cards
WHERE
    -- this query needs to be made more efficient; we can add a separate last_review
    -- column and keep it up to date.
    ((fsrs_card->>'LastReview')::timestamp AT TIME ZONE 'UTC' AT TIME ZONE 'America/Los_Angeles')::date =
        (NOW() AT TIME ZONE 'America/Los_Angeles')::date
GROUP BY
    user_id
ORDER BY
    cards_studied_today DESC;