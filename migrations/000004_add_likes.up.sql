-- Add likes table for users to save favorite listings
CREATE TABLE IF NOT EXISTS listing_likes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    listing_id UUID NOT NULL REFERENCES cars(id) ON DELETE CASCADE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(user_id, listing_id)
);

-- Create index for fast lookups by user
CREATE INDEX IF NOT EXISTS idx_listing_likes_user_id ON listing_likes(user_id);

-- Create index for fast lookups by listing
CREATE INDEX IF NOT EXISTS idx_listing_likes_listing_id ON listing_likes(listing_id);
