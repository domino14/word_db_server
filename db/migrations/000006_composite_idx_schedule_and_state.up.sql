BEGIN;

CREATE INDEX IF NOT EXISTS idx_cards_user_lexicon_scheduled_state ON wordvault_cards (user_id, lexicon_name, next_scheduled, (fsrs_card->'State'));

COMMIT;
