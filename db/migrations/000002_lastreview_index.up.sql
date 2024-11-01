BEGIN;

create index wordvault_cards_last_review_idx on wordvault_cards ((fsrs_card->>'LastReview'));

COMMIT;
