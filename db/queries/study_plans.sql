-- name: AddStudyPlan :one
INSERT INTO wordvault_study_plans(user_id, lexicon_name, name, deck_id, batch_unit, batch_size, stages)
VALUES (@user_id, @lexicon_name, @name, NULLIF(sqlc.arg(deck_id)::BIGINT, 0), @batch_unit, @batch_size, @stages)
RETURNING id, user_id, lexicon_name, name, COALESCE(deck_id, 0)::BIGINT AS deck_id, batch_unit, batch_size, stages;

-- name: GetStudyPlans :many
SELECT id, user_id, lexicon_name, name, COALESCE(deck_id, 0)::BIGINT AS deck_id, batch_unit, batch_size, stages
FROM wordvault_study_plans
WHERE user_id = $1
ORDER BY id;

-- name: GetStudyPlan :one
SELECT id, user_id, lexicon_name, name, COALESCE(deck_id, 0)::BIGINT AS deck_id, batch_unit, batch_size, stages
FROM wordvault_study_plans
WHERE id = $1 AND user_id = $2;

-- name: EditStudyPlan :one
UPDATE wordvault_study_plans
SET name = @name, deck_id = NULLIF(sqlc.arg(deck_id)::BIGINT, 0), batch_unit = @batch_unit,
    batch_size = @batch_size, stages = @stages, updated_at = now()
WHERE id = @id AND user_id = @user_id
RETURNING id, user_id, lexicon_name, name, COALESCE(deck_id, 0)::BIGINT AS deck_id, batch_unit, batch_size, stages;

-- name: DeleteStudyPlan :execrows
DELETE FROM wordvault_study_plans
WHERE id = $1 AND user_id = $2;

-- name: GetVaultAlphagrams :many
-- Answered from the (user_id, lexicon_name, alphagram) index alone.
SELECT alphagram
FROM wordvault_cards
WHERE user_id = $1 AND lexicon_name = $2;

-- name: GetVaultAlphagramsInDeck :many
-- Unlike GetVaultAlphagrams, this has to read the card rows.
SELECT alphagram
FROM wordvault_cards
WHERE user_id = $1 AND lexicon_name = $2 AND COALESCE(deck_id, 0) = sqlc.arg(deck_id)::BIGINT;
