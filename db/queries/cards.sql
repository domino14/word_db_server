-- name: GetCard :one
SELECT next_scheduled, fsrs_card, review_log
FROM wordvault_cards
WHERE user_id = $1 AND lexicon_name = $2 AND alphagram = $3;

-- name: GetCards :many
SELECT alphagram, next_scheduled, fsrs_card, review_log, COALESCE(deck_id, 0) as deck_id
FROM wordvault_cards
WHERE user_id = $1 AND lexicon_name = $2 AND alphagram = ANY(@alphagrams::text[]);

-- name: GetDecks :many
SELECT id, user_id, lexicon_name, fsrs_params_override, name
FROM wordvault_decks
WHERE user_id = $1;

-- name: AddDeck :one
INSERT INTO wordvault_decks(user_id, lexicon_name, name)
VALUES ($1, $2, $3)
RETURNING *;

-- name: EditDeck :one
UPDATE wordvault_decks
SET name = $2
WHERE id = $1 AND user_id = $3
RETURNING *;

-- name: DeleteDeck :exec
DELETE FROM wordvault_decks
WHERE id = $1;

-- name: GetNextScheduled :many
SELECT alphagram, next_scheduled, fsrs_card, COALESCE(deck_id, 0) as deck_id
FROM wordvault_cards
WHERE user_id = $1 AND lexicon_name = $2 AND next_scheduled <= $3
    AND COALESCE(deck_id, 0) = sqlc.arg(deck_id)::bigint
ORDER BY next_scheduled ASC
LIMIT $4;

-- name: GetSingleNextScheduled :one
WITH matching_cards AS (
  SELECT
    alphagram,
    next_scheduled,
    fsrs_card,
    COALESCE(deck_id, 0) as deck_id,
    COUNT(*) OVER () AS total_count -- Window function to get the total count
  FROM wordvault_cards
  WHERE user_id = $1
    AND lexicon_name = $2
    AND next_scheduled <= $3
    AND COALESCE(deck_id, 0) = sqlc.arg(deck_id)::bigint
  ORDER BY
    -- When short-term scheduling is enabled, we want to de-prioritize
    -- new cards so that you clear your backlog of reviewed cards first.
    CASE WHEN CAST(fsrs_card->'State' AS INTEGER) = 0 THEN FALSE ELSE sqlc.arg(is_short_term_scheduler)::bool END DESC,
    next_scheduled ASC
)
SELECT alphagram, next_scheduled, fsrs_card, deck_id, total_count FROM matching_cards
LIMIT 1;

-- name: GetNumCardsInVault :many
SELECT lexicon_name, count(*) as card_count FROM wordvault_cards
WHERE user_id = $1
GROUP BY lexicon_name;

-- name: GetNumCardsInVaultByDeck :many
SELECT
    COALESCE(deck_id, 0) as deck_id,
    COUNT(*) as card_count
FROM
    wordvault_cards
WHERE
    user_id = $1 AND lexicon_name = $2
GROUP BY
    deck_id
ORDER BY
    deck_id NULLS FIRST;

-- name: UpdateCard :exec
UPDATE wordvault_cards
SET fsrs_card = $1, next_scheduled = $2, review_log = review_log || @review_log_item::jsonb
WHERE user_id = $3 AND lexicon_name = $4 AND alphagram = $5;

-- name: UpdateCardReplaceLastLog :exec
UPDATE wordvault_cards
SET
    fsrs_card = $1,
    next_scheduled = $2,
    review_log = (review_log - (jsonb_array_length(review_log) - 1)) || @review_log_item::jsonb
WHERE
    user_id = $3
    AND lexicon_name = $4
    AND alphagram = $5;

-- name: LoadFsrsParams :one
SELECT params FROM wordvault_params
WHERE user_id = $1;

-- name: SetFsrsParams :exec
INSERT INTO wordvault_params(user_id, params)
VALUES ($1, $2)
ON CONFLICT(user_id) DO UPDATE
SET params = $2;

-- name: CountCardsInOtherDecks :one
SELECT COUNT(*)
FROM wordvault_cards
WHERE user_id = $1
    AND lexicon_name = $2
    AND alphagram = ANY(@alphagrams::text[])
    AND COALESCE(deck_id, 0) <> sqlc.arg(deck_id)::BIGINT;

-- name: GetCardsInOtherDecks :many
SELECT id, alphagram, COALESCE(deck_id, 0) as deck_id
FROM wordvault_cards
WHERE user_id = $1
    AND lexicon_name = $2
    AND alphagram = ANY(@alphagrams::text[])
    AND COALESCE(deck_id, 0) <> sqlc.arg(deck_id)::BIGINT
LIMIT $3;

-- name: AddCards :one
WITH inserted_rows AS (
    INSERT INTO wordvault_cards(
        alphagram, next_scheduled, fsrs_card, user_id, lexicon_name, review_log, deck_id
    )
    SELECT
        unnest(@alphagrams::TEXT[]),
        unnest(@next_scheduleds::TIMESTAMPTZ[]),
        unnest(@fsrs_cards::JSONB[]),
        unnest(array_fill(@user_id::BIGINT, array[array_length(@alphagrams, 1)])),
        unnest(array_fill(@lexicon_name::TEXT, array[array_length(@alphagrams, 1)])),
        unnest(
            COALESCE(
                @review_logs::JSONB[],
                array_fill('[]'::JSONB, array[array_length(@alphagrams, 1)])
            )
        ),
        NULLIF(sqlc.arg(deck_id)::BIGINT, 0)
    ON CONFLICT(user_id, lexicon_name, alphagram) DO NOTHING
    RETURNING 1
)
SELECT COUNT(*) FROM inserted_rows;

-- name: MoveCards :execrows
UPDATE wordvault_cards
SET deck_id = NULLIF(sqlc.arg(deck_id)::BIGINT, 0)
WHERE user_id = $1 AND lexicon_name = $2 AND alphagram = ANY(@alphagrams::text[]);

-- name: GetNextScheduledBreakdown :many
WITH scheduled_cards AS (
    SELECT
        CASE WHEN next_scheduled <= @now THEN '-infinity'::date
        ELSE (next_scheduled AT TIME ZONE @tz::text)::date END
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
    scheduled_date;

-- name: GetNextScheduledBreakdownByDeck :many
WITH scheduled_cards AS (
    SELECT
        CASE WHEN next_scheduled <= @now THEN '-infinity'::date
        ELSE (next_scheduled AT TIME ZONE @tz::text)::date END
        AS scheduled_date,
        COALESCE(deck_id, 0) as deck_id
    FROM
        wordvault_cards
    WHERE user_id = $1 AND lexicon_name = $2
),
per_date AS (
    SELECT
        deck_id,
        CASE
            WHEN scheduled_date = '-infinity'::date THEN 'overdue'
            WHEN scheduled_date = 'infinity'::date THEN 'infinity'
            ELSE to_char(scheduled_date, 'YYYY-MM-DD')
        END AS key,
        COUNT(*) AS question_count
    FROM scheduled_cards
    GROUP BY deck_id, scheduled_date
)
SELECT
    deck_id,
    jsonb_object_agg(key, question_count) AS breakdown
FROM per_date
GROUP BY deck_id
ORDER BY deck_id;

-- name: GetOverdueCount :one
SELECT
    count(*) from wordvault_cards
WHERE next_scheduled <= @now AND user_id = $1 AND lexicon_name = $2;

-- name: GetOverdueCountByDeck :many
SELECT
    COALESCE(deck_id, 0) as deck_id, count(*) from wordvault_cards
WHERE next_scheduled <= @now AND user_id = $1 AND lexicon_name = $2
GROUP BY deck_id;

-- name: PostponementQuery :many
SELECT alphagram, next_scheduled, fsrs_card
FROM wordvault_cards
WHERE user_id = $1 AND lexicon_name = $2 AND next_scheduled <= $3
AND jsonb_array_length(review_log) > 0;

-- name: DeleteCards :execrows
DELETE FROM wordvault_cards
WHERE user_id = $1 AND lexicon_name = $2;

-- name: DeleteNewCards :execrows
DELETE FROM wordvault_cards
WHERE user_id = $1 AND lexicon_name = $2 AND jsonb_array_length(review_log) = 0;

-- name: DeleteCardsWithAlphagrams :execrows
DELETE FROM wordvault_cards
WHERE user_id = $1 AND lexicon_name = $2 AND alphagram = ANY(@alphagrams::text[]);

-- name: DeleteCardsFromDeck :execrows
DELETE FROM wordvault_cards
WHERE user_id = $1 AND lexicon_name = $2 AND COALESCE(deck_id, 0) = sqlc.arg(deck_id)::bigint;

-- name: DeleteNewCardsFromDeck :execrows
DELETE FROM wordvault_cards
WHERE user_id = $1 AND lexicon_name = $2 AND jsonb_array_length(review_log) = 0
    AND COALESCE(deck_id, 0) = sqlc.arg(deck_id)::bigint;

-- name: DeleteCardsWithAlphagramsFromDeck :execrows
DELETE FROM wordvault_cards
WHERE user_id = $1 AND lexicon_name = $2 AND alphagram = ANY(@alphagrams::text[])
    AND COALESCE(deck_id, 0) = sqlc.arg(deck_id)::bigint;

-- name: BulkUpdateCards :exec
WITH updated_values AS (
  SELECT
    UNNEST(@alphagrams::TEXT[]) AS alphagram,
    UNNEST(@next_scheduleds::TIMESTAMPTZ[]) AS next_scheduled,
    UNNEST(@fsrs_cards::JSONB[]) AS fsrs_card,
    UNNEST(array_fill(@user_id::BIGINT, array[array_length(@alphagrams, 1)])) AS user_id,
    UNNEST(array_fill(@lexicon_name::TEXT, array[array_length(@alphagrams, 1)])) AS lexicon_name
)
UPDATE wordvault_cards w
SET
  fsrs_card = u.fsrs_card,
  next_scheduled = u.next_scheduled
FROM updated_values u
WHERE
  w.user_id = u.user_id AND
  w.lexicon_name = u.lexicon_name AND
  w.alphagram = u.alphagram;
