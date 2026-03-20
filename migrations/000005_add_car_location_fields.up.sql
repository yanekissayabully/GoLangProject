-- Migration: Add extended location fields to cars table
-- Adds area, street, block, zip for detailed car location

ALTER TABLE cars ADD COLUMN IF NOT EXISTS area VARCHAR(200);
ALTER TABLE cars ADD COLUMN IF NOT EXISTS street VARCHAR(300);
ALTER TABLE cars ADD COLUMN IF NOT EXISTS block VARCHAR(50);
ALTER TABLE cars ADD COLUMN IF NOT EXISTS zip VARCHAR(20);

-- Backfill: copy neighborhood into area for existing rows that have neighborhood set
UPDATE cars SET area = neighborhood WHERE neighborhood IS NOT NULL AND neighborhood != '' AND area IS NULL;
