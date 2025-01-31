BEGIN;

-- Schema changes
DROP TABLE wordvault_decks;

ALTER TABLE wordvault_cards
DROP COLUMN deck_id;

COMMIT;
