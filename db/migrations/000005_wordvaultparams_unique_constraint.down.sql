BEGIN;

ALTER TABLE wordvault_params
DROP CONSTRAINT wordvault_params_user_id_key;

COMMIT;
