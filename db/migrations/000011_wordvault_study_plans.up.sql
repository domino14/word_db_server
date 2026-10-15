BEGIN;

-- A study plan is an ordered list of searches (stages) that a user works
-- through a batch at a time, adding the cards to one deck. Progress isn't
-- stored: it's whatever of the plan is already in the user's vault.
CREATE TABLE wordvault_study_plans (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    lexicon_name TEXT NOT NULL,
    name TEXT NOT NULL,
    -- NULL is the default deck.
    deck_id BIGINT REFERENCES wordvault_decks(id),
    batch_unit SMALLINT NOT NULL,
    batch_size INTEGER NOT NULL,
    -- JSON array of wordvault.StudyPlanStage in protojson form.
    stages JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- At most one plan per deck, the default deck included.
CREATE UNIQUE INDEX study_plans_user_lexicon_deck_idx
    ON wordvault_study_plans (user_id, lexicon_name, COALESCE(deck_id, 0));

COMMIT;
