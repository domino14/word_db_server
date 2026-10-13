-- With the default scale factors (0.2 and 0.1), autovacuum waits for ~940k
-- dead rows in this 4.7M-row table, so it rarely runs. Until it does, pages
-- aren't marked all-visible and index-only scans (card counts, schedule
-- breakdowns) have to read the table anyway. Vacuum after ~1% churn instead.
-- Only takes a brief SHARE UPDATE EXCLUSIVE lock.
ALTER TABLE wordvault_cards SET (
    autovacuum_vacuum_scale_factor = 0.01,
    autovacuum_analyze_scale_factor = 0.02
);
