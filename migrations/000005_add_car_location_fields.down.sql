-- Rollback: Remove extended location fields from cars table

ALTER TABLE cars DROP COLUMN IF EXISTS area;
ALTER TABLE cars DROP COLUMN IF EXISTS street;
ALTER TABLE cars DROP COLUMN IF EXISTS block;
ALTER TABLE cars DROP COLUMN IF EXISTS zip;
