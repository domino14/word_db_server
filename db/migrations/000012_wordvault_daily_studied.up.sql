BEGIN;

-- How many cards each user studied each day, for the leaderboards. Days are
-- America/Los_Angeles days. Scoring a card adds to it in the same
-- transaction, once per card per day, so this is the rollup of
-- wordvault_cards review logs without having to scan them.
--
-- Rows from before this migration come from a one-time backfill of recent
-- days (scripts/migrations/backfill_daily_studied), run by hand after deploy.
CREATE TABLE wordvault_daily_studied (
    day DATE NOT NULL,
    user_id BIGINT NOT NULL,
    cards INTEGER NOT NULL DEFAULT 0,
    -- Cards scored from a Wordwalls round of due cards (BatchScoreCards).
    -- Every such score counts, even of a card already studied that day. Not
    -- backfilled.
    wordwalls_cards INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (day, user_id)
);

COMMIT;
