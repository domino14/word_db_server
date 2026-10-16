BEGIN;

-- A study plan is an ordered list of searches (stages) that a user works
-- through a batch at a time, adding the cards to one deck. A deck can have
-- several plans adding to it. Progress isn't
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

CREATE INDEX study_plans_user_id_idx ON wordvault_study_plans (user_id);

COMMIT;
