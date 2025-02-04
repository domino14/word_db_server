BEGIN;

-- Schema changes
DROP TABLE wordvault_decks;

ALTER TABLE wordvault_cards
DROP COLUMN deck_id;

DROP INDEX decks_unique_name_idx;

COMMIT;
