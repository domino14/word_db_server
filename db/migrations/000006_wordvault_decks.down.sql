BEGIN;

-- Schema changes
DROP TABLE wordvault_decks;

ALTER TABLE wordvault_cards
DROP COLUMN deck_id;

-- Add back deleted unique index
CREATE UNIQUE INDEX wordvault_cards_user_id_lexicon_name_alphagram_key ON wordvault_cards USING btree (user_id, lexicon_name, deck_id, alphagram);

COMMIT;
