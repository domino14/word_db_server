BEGIN;

-- Schema changes
CREATE TABLE wordvault_decks (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    lexicon_name TEXT NOT NULL,
    fsrs_params_override JSONB DEFAULT null,
    name TEXT NOT NULL
);

-- Postgres unique constraints are bad with `null`, so we use 0 to represent the default deck
ALTER TABLE wordvault_cards
ADD COLUMN deck_id BIGINT NOT NULL DEFAULT 0;

-- Indexes for Decks
CREATE INDEX decks_userid_idx ON wordvault_decks USING btree (user_id);

CREATE INDEX decks_userid_lexicon_idx ON wordvault_decks USING btree (user_id, lexicon_name);

-- All card queries will need to be scoped to a deck, so add an index for that
CREATE INDEX wordvault_cards_deckid_idx ON wordvault_cards USING btree (deck_id);

CREATE INDEX wordvault_cards_deckid_scheduled ON wordvault_cards USING btree (deck_id, next_scheduled);

CREATE INDEX wordvault_cards_deckid_last_review_idx on wordvault_cards (deck_id, (fsrs_card ->> 'LastReview'));

COMMIT;
