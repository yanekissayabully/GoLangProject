-- Migration: Add cars, car_photos, car_documents tables
-- Created for DriveBai car listing functionality

-- Enum for car listing status
CREATE TYPE car_listing_status AS ENUM ('available', 'rented', 'pending', 'paused');

-- Enum for car body type
CREATE TYPE car_body_type AS ENUM ('sedan', 'suv', 'coupe', 'hatchback', 'truck', 'van', 'convertible', 'wagon');

-- Enum for fuel type
CREATE TYPE fuel_type AS ENUM ('gas', 'diesel', 'electric', 'hybrid', 'plug_in_hybrid');

-- Enum for insurance coverage
CREATE TYPE insurance_coverage AS ENUM ('liability_only', 'full_coverage');

-- Enum for photo slot type
CREATE TYPE photo_slot_type AS ENUM ('cover_front', 'right', 'left', 'back', 'dashboard');

-- Enum for car document type
CREATE TYPE car_document_type AS ENUM ('inspection', 'registration', 'permit', 'insurance');

-- Cars table
CREATE TABLE cars (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- Basic info
    title VARCHAR(255) NOT NULL,
    description TEXT,

    -- Specs
    make VARCHAR(100) NOT NULL,
    model VARCHAR(100) NOT NULL,
    year INTEGER NOT NULL,
    body_type car_body_type NOT NULL DEFAULT 'sedan',
    fuel_type fuel_type NOT NULL DEFAULT 'gas',
    mileage INTEGER NOT NULL DEFAULT 0,

    -- Location
    address VARCHAR(500),
    neighborhood VARCHAR(200),
    latitude DECIMAL(10, 8),
    longitude DECIMAL(11, 8),

    -- Pricing
    is_for_rent BOOLEAN NOT NULL DEFAULT true,
    weekly_rent_price DECIMAL(10, 2),
    is_for_sale BOOLEAN NOT NULL DEFAULT false,
    sale_price DECIMAL(12, 2),
    currency VARCHAR(3) NOT NULL DEFAULT 'USD',

    -- Requirements
    min_years_licensed INTEGER NOT NULL DEFAULT 2,
    deposit_amount DECIMAL(10, 2) NOT NULL DEFAULT 500,
    insurance_coverage insurance_coverage NOT NULL DEFAULT 'full_coverage',

    -- Status
    status car_listing_status NOT NULL DEFAULT 'pending',
    is_paused BOOLEAN NOT NULL DEFAULT false,

    -- Stats
    rented_weeks INTEGER NOT NULL DEFAULT 0,
    total_earned DECIMAL(12, 2) NOT NULL DEFAULT 0,

    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Car photos table
CREATE TABLE car_photos (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    car_id UUID NOT NULL REFERENCES cars(id) ON DELETE CASCADE,
    slot_type photo_slot_type NOT NULL,
    file_path VARCHAR(500) NOT NULL,
    file_url VARCHAR(500) NOT NULL,
    file_size INTEGER NOT NULL DEFAULT 0,
    mime_type VARCHAR(50) NOT NULL DEFAULT 'image/jpeg',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Each car can have only one photo per slot
    UNIQUE(car_id, slot_type)
);

-- Car documents table
CREATE TABLE car_documents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    car_id UUID NOT NULL REFERENCES cars(id) ON DELETE CASCADE,
    document_type car_document_type NOT NULL,
    file_name VARCHAR(255) NOT NULL,
    file_path VARCHAR(500) NOT NULL,
    file_url VARCHAR(500) NOT NULL,
    file_size INTEGER NOT NULL DEFAULT 0,
    mime_type VARCHAR(50) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes for performance
CREATE INDEX idx_cars_owner_id ON cars(owner_id);
CREATE INDEX idx_cars_status ON cars(status);
CREATE INDEX idx_cars_is_for_rent ON cars(is_for_rent) WHERE is_for_rent = true;
CREATE INDEX idx_cars_is_for_sale ON cars(is_for_sale) WHERE is_for_sale = true;
CREATE INDEX idx_car_photos_car_id ON car_photos(car_id);
CREATE INDEX idx_car_documents_car_id ON car_documents(car_id);

-- Trigger to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_cars_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_cars_updated_at
    BEFORE UPDATE ON cars
    FOR EACH ROW
    EXECUTE FUNCTION update_cars_updated_at();

CREATE TRIGGER trigger_car_photos_updated_at
    BEFORE UPDATE ON car_photos
    FOR EACH ROW
    EXECUTE FUNCTION update_cars_updated_at();
