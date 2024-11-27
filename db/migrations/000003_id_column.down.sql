BEGIN;

-- Drop the id column
ALTER TABLE wordvault_cards
DROP COLUMN id;

-- Drop the sequence created for the id column, if it exists
DROP SEQUENCE IF EXISTS wordvault_cards_id_seq;

COMMIT;