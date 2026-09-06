-- Signed difference = recorded − expected (gas card minus maint+OneStep).
-- abs_diff stays |difference|. HOLD rows leave both NULL (never invent).

ALTER TABLE mileage_ledger ADD COLUMN difference INTEGER;
