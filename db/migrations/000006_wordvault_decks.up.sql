BEGIN;

-- Schema changes
CREATE TABLE wordvault_decks (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    lexicon_name TEXT NOT NULL,
    fsrs_params_override JSONB DEFAULT null,
    name TEXT NOT NULL,
);

ALTER TABLE wordvault_cards
ADD COLUMN deck_id BIGSERIAL REFERENCES wordvault_decks (id);

-- Indexes for Decks
CREATE INDEX decks_userid_idx ON wordvault_decks USING btree (user_id);

CREATE INDEX decks_userid_lexicon_idx ON wordvault_decks USING btree (user_id, lexicon_name);

-- All card queries will need to be scoped to a deck, so add an index for that
CREATE INDEX wordvault_cards_deckid_idx ON wordvault_cards USING btree (deck_id);

CREATE INDEX wordvault_cards_deckid_scheduled ON wordvault_cards USING btree (deck_id, next_scheduled);

CREATE INDEX wordvault_cards_deckid_last_review_idx on wordvault_cards (deck_id, (fsrs_card - > > 'LastReview'));

-- Replace the (user/lexicon/alphagram) unique index with (user/lexicon/deck/alphagram)
-- So that users can have the same card in multiple decks
CREATE UNIQUE INDEX wordvault_cards_userid_lexicon_deck_alphagram_idx ON wordvault_cards USING btree (user_id, lexicon_name, deck_id, alphagram);

-- remove old unique index
DROP INDEX wordvault_cards_user_id_lexicon_name_alphagram_key;

COMMIT;
