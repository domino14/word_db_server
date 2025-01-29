BEGIN;

-- Schema changes
DROP TABLE wordvault_decks;

ALTER TABLE wordvault_cards
DROP COLUMN deck_id;

-- Add back deleted unique index
ALTER TABLE wordvault_cards ADD CONSTRAINT wordvault_cards_user_id_lexicon_name_alphagram_key UNIQUE (user_id, lexicon_name, alphagram);

COMMIT;
