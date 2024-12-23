BEGIN;

CREATE INDEX IF NOT EXISTS idx_cards_user_lexicon_scheduled
  ON wordvault_cards (user_id, lexicon_name, next_scheduled);

COMMIT;