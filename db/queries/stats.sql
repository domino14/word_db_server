-- name: GetDailyProgress :one
-- Each card reviewed today counts once, by its first review today, so that
-- in short-term mode, relearning a missed card doesn't turn it into a
-- success (or a new card into a reviewed one). A card is new if that first
-- review was its first ever. Further reviews of the same card today are
-- counted as repeats.
WITH today AS (
    SELECT
        first_today.ord,
        first_today.rating,
        first_today.num_reviews
    FROM
        wordvault_cards c
    CROSS JOIN LATERAL (
        SELECT
            MIN(e.ord) AS ord,
            (ARRAY_AGG((e.entry->>'Rating')::int ORDER BY e.ord))[1] AS rating,
            COUNT(*) AS num_reviews
        FROM
            jsonb_array_elements(c.review_log) WITH ORDINALITY AS e(entry, ord)
        WHERE
            -- Review times are UTC RFC3339 ending in Z, like LastReview.
            e.entry->>'Review' >= sqlc.arg(day_start)::text
            AND e.entry->>'Review' < sqlc.arg(day_end)::text
            -- Skip Cardbox import entries (rating 0).
            AND (e.entry->>'Rating')::int BETWEEN 1 AND 4
    ) first_today
    WHERE
        c.user_id = @user_id
        -- LastReview is always UTC RFC3339 ending in Z, so comparing it as
        -- text against the day's bounds (see lastReviewDayBounds) lets this
        -- use an index on LastReview.
        AND c.fsrs_card->>'LastReview' >= sqlc.arg(day_start)::text
        AND c.fsrs_card->>'LastReview' < sqlc.arg(day_end)::text
        AND first_today.ord IS NOT NULL
)
SELECT
    COUNT(*) FILTER (WHERE ord = 1)::int AS new_cards,
    COUNT(*) FILTER (WHERE ord > 1)::int AS reviewed_cards,
    COUNT(*) FILTER (WHERE ord = 1 AND rating = 1)::int AS new_rating_1,
    COUNT(*) FILTER (WHERE ord = 1 AND rating = 2)::int AS new_rating_2,
    COUNT(*) FILTER (WHERE ord = 1 AND rating = 3)::int AS new_rating_3,
    COUNT(*) FILTER (WHERE ord = 1 AND rating = 4)::int AS new_rating_4,
    COUNT(*) FILTER (WHERE ord > 1 AND rating = 1)::int AS reviewed_rating_1,
    COUNT(*) FILTER (WHERE ord > 1 AND rating = 2)::int AS reviewed_rating_2,
    COUNT(*) FILTER (WHERE ord > 1 AND rating = 3)::int AS reviewed_rating_3,
    COUNT(*) FILTER (WHERE ord > 1 AND rating = 4)::int AS reviewed_rating_4,
    COALESCE(SUM(num_reviews - 1), 0)::int AS repeats
FROM
    today;

-- name: AddDailyStudied :exec
-- Adds to a user's studied counts for a leaderboard day.
INSERT INTO wordvault_daily_studied (day, user_id, cards, wordwalls_cards)
VALUES (@day, @user_id, @cards, @wordwalls_cards)
ON CONFLICT (day, user_id) DO UPDATE
SET
    cards = wordvault_daily_studied.cards + EXCLUDED.cards,
    wordwalls_cards = wordvault_daily_studied.wordwalls_cards + EXCLUDED.wordwalls_cards;

-- name: GetLeaderboard :many
-- Cards studied per user per day, from first_day through last_day.
SELECT
    u.username,
    s.day,
    s.cards
FROM
    wordvault_daily_studied s
JOIN
    auth_user u ON s.user_id = u.id
WHERE
    s.day BETWEEN @first_day AND @last_day
    AND s.cards > 0;

-- name: BackfillDailyStudied :execrows
-- Recomputes the studied counts for every leaderboard day from first_day on,
-- from the review logs: each card counts once a day it has a review. Only
-- cards last reviewed since first_day can have one, so they're the only
-- cards whose logs need reading. Existing counts are kept if higher, since
-- scoring adds to them while this runs.
INSERT INTO wordvault_daily_studied (day, user_id, cards)
SELECT
    day,
    user_id,
    COUNT(*)
FROM (
    SELECT DISTINCT
        c.user_id,
        c.lexicon_name,
        c.alphagram,
        ((e.entry->>'Review')::timestamptz AT TIME ZONE 'America/Los_Angeles')::date AS day
    FROM
        wordvault_cards c
    CROSS JOIN LATERAL
        jsonb_array_elements(c.review_log) AS e(entry)
    WHERE
        -- Review times are UTC RFC3339 ending in Z, like LastReview, so
        -- they compare as text against the UTC start of first_day.
        c.fsrs_card->>'LastReview' >= to_char(
            sqlc.arg(first_day)::date::timestamp AT TIME ZONE 'America/Los_Angeles' AT TIME ZONE 'UTC',
            'YYYY-MM-DD"T"HH24:MI:SS')
        AND e.entry->>'Review' >= to_char(
            sqlc.arg(first_day)::date::timestamp AT TIME ZONE 'America/Los_Angeles' AT TIME ZONE 'UTC',
            'YYYY-MM-DD"T"HH24:MI:SS')
        -- Skip Cardbox import entries (rating 0).
        AND (e.entry->>'Rating')::int BETWEEN 1 AND 4
) reviewed
GROUP BY
    day,
    user_id
ON CONFLICT (day, user_id) DO UPDATE
SET
    cards = GREATEST(wordvault_daily_studied.cards, EXCLUDED.cards);

-- name: GetDailyProgressByDeck :many
-- Each card reviewed today counts once, by its first review today, so that
-- in short-term mode, relearning a missed card doesn't turn it into a
-- success (or a new card into a reviewed one). A card is new if that first
-- review was its first ever. Further reviews of the same card today are
-- counted as repeats.
WITH today AS (
    SELECT
        c.deck_id,
        first_today.ord,
        first_today.rating,
        first_today.num_reviews
    FROM
        wordvault_cards c
    CROSS JOIN LATERAL (
        SELECT
            MIN(e.ord) AS ord,
            (ARRAY_AGG((e.entry->>'Rating')::int ORDER BY e.ord))[1] AS rating,
            COUNT(*) AS num_reviews
        FROM
            jsonb_array_elements(c.review_log) WITH ORDINALITY AS e(entry, ord)
        WHERE
            -- Review times are UTC RFC3339 ending in Z, like LastReview.
            e.entry->>'Review' >= sqlc.arg(day_start)::text
            AND e.entry->>'Review' < sqlc.arg(day_end)::text
            -- Skip Cardbox import entries (rating 0).
            AND (e.entry->>'Rating')::int BETWEEN 1 AND 4
    ) first_today
    WHERE
        c.user_id = @user_id
        -- LastReview is always UTC RFC3339 ending in Z, so comparing it as
        -- text against the day's bounds (see lastReviewDayBounds) lets this
        -- use an index on LastReview.
        AND c.fsrs_card->>'LastReview' >= sqlc.arg(day_start)::text
        AND c.fsrs_card->>'LastReview' < sqlc.arg(day_end)::text
        AND first_today.ord IS NOT NULL
)
SELECT
    deck_id,
    COUNT(*) FILTER (WHERE ord = 1)::int AS new_cards,
    COUNT(*) FILTER (WHERE ord > 1)::int AS reviewed_cards,
    COUNT(*) FILTER (WHERE ord = 1 AND rating = 1)::int AS new_rating_1,
    COUNT(*) FILTER (WHERE ord = 1 AND rating = 2)::int AS new_rating_2,
    COUNT(*) FILTER (WHERE ord = 1 AND rating = 3)::int AS new_rating_3,
    COUNT(*) FILTER (WHERE ord = 1 AND rating = 4)::int AS new_rating_4,
    COUNT(*) FILTER (WHERE ord > 1 AND rating = 1)::int AS reviewed_rating_1,
    COUNT(*) FILTER (WHERE ord > 1 AND rating = 2)::int AS reviewed_rating_2,
    COUNT(*) FILTER (WHERE ord > 1 AND rating = 3)::int AS reviewed_rating_3,
    COUNT(*) FILTER (WHERE ord > 1 AND rating = 4)::int AS reviewed_rating_4,
    COALESCE(SUM(num_reviews - 1), 0)::int AS repeats
FROM
    today
GROUP BY
    deck_id
ORDER BY
    deck_id NULLS FIRST;
