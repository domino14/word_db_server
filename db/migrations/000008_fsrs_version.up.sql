BEGIN;

-- Which FSRS weights a user is on: 5 (go-fsrs v3 era) or 6 (go-fsrs v4).
-- Existing rows are users from before the upgrade; new rows default to 6.
ALTER TABLE wordvault_params ADD COLUMN fsrs_version SMALLINT NOT NULL DEFAULT 5;
ALTER TABLE wordvault_params ALTER COLUMN fsrs_version SET DEFAULT 6;

-- Existing users who never changed their settings have no params row. Give
-- them one with the defaults the server used for them, pinned to version 5,
-- so that a missing row only ever means a new (version 6) user.
INSERT INTO wordvault_params (user_id, params, fsrs_version)
SELECT DISTINCT c.user_id,
    '{"RequestRetention": 0.9, "MaximumInterval": 1825, "EnableShortTerm": false, "EnableFuzz": true}'::jsonb,
    5
FROM wordvault_cards c
WHERE NOT EXISTS (SELECT 1 FROM wordvault_params p WHERE p.user_id = c.user_id);

COMMIT;
