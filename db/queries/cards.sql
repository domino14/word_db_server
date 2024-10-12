-- name: GetCard :one
SELECT next_scheduled, fsrs_card, review_log
FROM wordvault_cards
WHERE user_id = $1 AND lexicon_name = $2 AND alphagram = $3;

-- name: GetCards :many
SELECT alphagram, next_scheduled, fsrs_card, review_log
FROM wordvault_cards
WHERE user_id = $1 AND lexicon_name = $2 AND alphagram = ANY(@alphagrams::text[]);

-- name: GetNextScheduled :many
SELECT alphagram, next_scheduled, fsrs_card
FROM wordvault_cards
WHERE user_id = $1 AND lexicon_name = $2 AND next_scheduled <= $3
ORDER BY next_scheduled ASC, random()
LIMIT $4;

-- name: GetNumCardsInVault :many
SELECT lexicon_name, count(*) as card_count FROM wordvault_cards
WHERE user_id = $1
GROUP BY lexicon_name;

-- name: UpdateCard :exec
UPDATE wordvault_cards
SET fsrs_card = $1, next_scheduled = $2, review_log = review_log || @review_log_item::jsonb
WHERE user_id = $3 AND lexicon_name = $4 AND alphagram = $5;

-- name: UpdateCardReplaceLastLog :exec
UPDATE wordvault_cards
SET
    fsrs_card = $1,
    next_scheduled = $2,
    review_log = (review_log || @review_log_item::jsonb) - jsonb_array_length(review_log)
WHERE
    user_id = $3
    AND lexicon_name = $4
    AND alphagram = $5;

-- name: LoadParams :one
SELECT params FROM wordvault_params
WHERE user_id = $1;

-- name: SetParams :exec
UPDATE wordvault_params SET params = $1
WHERE user_id = $2;

-- name: AddCards :one
WITH inserted_rows AS (
    INSERT INTO wordvault_cards(alphagram, next_scheduled, fsrs_card, user_id, lexicon_name)
    SELECT unnest(@alphagrams::TEXT[]),
        unnest(@next_scheduleds::TIMESTAMPTZ[]),
        unnest(array_fill(@fsrs_card::JSONB, array[array_length(@alphagrams, 1)])),
        unnest(array_fill(@user_id::BIGINT, array[array_length(@alphagrams, 1)])),
        unnest(array_fill(@lexicon_name::TEXT, array[array_length(@alphagrams, 1)]))
    ON CONFLICT(user_id, lexicon_name, alphagram) DO NOTHING
    RETURNING 1
)
SELECT COUNT(*) FROM inserted_rows;

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

-- name: GetOverdueCount :one
SELECT
    count(*) from wordvault_cards
WHERE next_scheduled <= @now AND user_id = $1 AND lexicon_name = $2;

-- name: PostponementQuery :many
SELECT alphagram, next_scheduled, fsrs_card
FROM wordvault_cards
WHERE user_id = $1 AND lexicon_name = $2 AND next_scheduled <= $3
AND jsonb_array_length(review_log) > 0;

-- name: DeleteCards :exec
DELETE FROM wordvault_cards
WHERE user_id = $1 AND lexicon_name = $2;

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