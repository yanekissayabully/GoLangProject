-- Rollback migration: Remove cars, car_photos, car_documents tables

-- Drop triggers
DROP TRIGGER IF EXISTS trigger_car_photos_updated_at ON car_photos;
DROP TRIGGER IF EXISTS trigger_cars_updated_at ON cars;
DROP FUNCTION IF EXISTS update_cars_updated_at();

-- Drop indexes
DROP INDEX IF EXISTS idx_car_documents_car_id;
DROP INDEX IF EXISTS idx_car_photos_car_id;
DROP INDEX IF EXISTS idx_cars_is_for_sale;
DROP INDEX IF EXISTS idx_cars_is_for_rent;
DROP INDEX IF EXISTS idx_cars_status;
DROP INDEX IF EXISTS idx_cars_owner_id;

-- Drop tables
DROP TABLE IF EXISTS car_documents;
DROP TABLE IF EXISTS car_photos;
DROP TABLE IF EXISTS cars;

-- Drop enums
DROP TYPE IF EXISTS car_document_type;
DROP TYPE IF EXISTS photo_slot_type;
DROP TYPE IF EXISTS insurance_coverage;
DROP TYPE IF EXISTS fuel_type;
DROP TYPE IF EXISTS car_body_type;
DROP TYPE IF EXISTS car_listing_status;
