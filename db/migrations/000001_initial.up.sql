BEGIN;

-- Add foreign key relation later.
CREATE TABLE wordvault_cards(
    user_id BIGINT NOT NULL,
    lexicon_name TEXT NOT NULL,
    alphagram TEXT NOT NULL,
    next_scheduled TIMESTAMPTZ NOT NULL,
    fsrs_card JSONB NOT NULL DEFAULT '{}',
    UNIQUE(user_id, lexicon_name, alphagram));

CREATE INDEX cards_userid_idx ON wordvault_cards USING btree(user_id);
CREATE INDEX scheduled_idx ON wordvault_cards USING btree(next_scheduled);

CREATE TABLE wordvault_params(
    user_id BIGINT NOT NULL,
    params JSONB NOT NULL DEFAULT '{}'
);

CREATE INDEX params_userid_idx ON wordvault_params USING btree(user_id);



COMMIT;