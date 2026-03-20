package models

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
)

// CarListingStatus represents the status of a car listing
type CarListingStatus string

const (
	CarStatusAvailable CarListingStatus = "available"
	CarStatusRented    CarListingStatus = "rented"
	CarStatusPending   CarListingStatus = "pending"
	CarStatusPaused    CarListingStatus = "paused"
)

// CarBodyType represents the body type of a car
type CarBodyType string

const (
	BodyTypeSedan       CarBodyType = "sedan"
	BodyTypeSUV         CarBodyType = "suv"
	BodyTypeCoupe       CarBodyType = "coupe"
	BodyTypeHatchback   CarBodyType = "hatchback"
	BodyTypeTruck       CarBodyType = "truck"
	BodyTypeVan         CarBodyType = "van"
	BodyTypeConvertible CarBodyType = "convertible"
	BodyTypeWagon       CarBodyType = "wagon"
)

// FuelType represents the fuel type of a car
type FuelType string

const (
	FuelTypeGas          FuelType = "gas"
	FuelTypeDiesel       FuelType = "diesel"
	FuelTypeElectric     FuelType = "electric"
	FuelTypeHybrid       FuelType = "hybrid"
	FuelTypePlugInHybrid FuelType = "plug_in_hybrid"
)

// InsuranceCoverage represents insurance coverage requirements
type InsuranceCoverage string

const (
	InsuranceLiabilityOnly InsuranceCoverage = "liability_only"
	InsuranceFullCoverage  InsuranceCoverage = "full_coverage"
)

// PhotoSlotType represents the type of photo slot
type PhotoSlotType string

const (
	PhotoSlotCoverFront PhotoSlotType = "cover_front"
	PhotoSlotRight      PhotoSlotType = "right"
	PhotoSlotLeft       PhotoSlotType = "left"
	PhotoSlotBack       PhotoSlotType = "back"
	PhotoSlotDashboard  PhotoSlotType = "dashboard"
)

// CarDocumentType represents the type of car document
type CarDocumentType string

const (
	CarDocInspection   CarDocumentType = "inspection"
	CarDocRegistration CarDocumentType = "registration"
	CarDocPermit       CarDocumentType = "permit"
	CarDocInsurance    CarDocumentType = "insurance"
)

// Car represents a car listing in the system
type Car struct {
	ID          uuid.UUID        `json:"id"`
	OwnerID     uuid.UUID        `json:"owner_id"`
	Title       string           `json:"title"`
	Description sql.NullString   `json:"-"`

	// Specs
	Make     string      `json:"make"`
	Model    string      `json:"model"`
	Year     int         `json:"year"`
	BodyType CarBodyType `json:"body_type"`
	FuelType FuelType    `json:"fuel_type"`
	Mileage  int         `json:"mileage"`

	// Location
	Address      sql.NullString  `json:"-"`
	Neighborhood sql.NullString  `json:"-"`
	Latitude     sql.NullFloat64 `json:"-"`
	Longitude    sql.NullFloat64 `json:"-"`
	Area         sql.NullString  `json:"-"`
	Street       sql.NullString  `json:"-"`
	Block        sql.NullString  `json:"-"`
	Zip          sql.NullString  `json:"-"`

	// Pricing
	IsForRent       bool            `json:"is_for_rent"`
	WeeklyRentPrice sql.NullFloat64 `json:"-"`
	IsForSale       bool            `json:"is_for_sale"`
	SalePrice       sql.NullFloat64 `json:"-"`
	Currency        string          `json:"currency"`

	// Requirements
	MinYearsLicensed  int               `json:"min_years_licensed"`
	DepositAmount     float64           `json:"deposit_amount"`
	InsuranceCoverage InsuranceCoverage `json:"insurance_coverage"`

	// Status
	Status   CarListingStatus `json:"status"`
	IsPaused bool             `json:"is_paused"`

	// Stats
	RentedWeeks int     `json:"rented_weeks"`
	TotalEarned float64 `json:"total_earned"`

	// Timestamps
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CarPhoto represents a photo of a car
type CarPhoto struct {
	ID        uuid.UUID     `json:"id"`
	CarID     uuid.UUID     `json:"car_id"`
	SlotType  PhotoSlotType `json:"slot_type"`
	FilePath  string        `json:"-"`
	FileURL   string        `json:"file_url"`
	FileSize  int           `json:"file_size"`
	MimeType  string        `json:"mime_type"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
}

// CarDocument represents a document associated with a car
type CarDocument struct {
	ID           uuid.UUID       `json:"id"`
	CarID        uuid.UUID       `json:"car_id"`
	DocumentType CarDocumentType `json:"document_type"`
	FileName     string          `json:"file_name"`
	FilePath     string          `json:"-"`
	FileURL      string          `json:"file_url"`
	FileSize     int             `json:"file_size"`
	MimeType     string          `json:"mime_type"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

// CarResponse is the API response format for a car
type CarResponse struct {
	ID          uuid.UUID        `json:"id"`
	OwnerID     uuid.UUID        `json:"owner_id"`
	Title       string           `json:"title"`
	Description string           `json:"description"`

	// Specs
	Specs CarSpecsResponse `json:"specs"`

	// Location
	Location CarLocationResponse `json:"location"`

	// Pricing
	IsForRent       bool     `json:"is_for_rent"`
	WeeklyRentPrice *float64 `json:"weekly_rent_price,omitempty"`
	IsForSale       bool     `json:"is_for_sale"`
	SalePrice       *float64 `json:"sale_price,omitempty"`
	Currency        string   `json:"currency"`

	// Requirements
	Requirements CarRequirementsResponse `json:"requirements"`

	// Status
	Status   CarListingStatus `json:"status"`
	IsPaused bool             `json:"is_paused"`

	// Stats
	RentedWeeks int     `json:"rented_weeks"`
	TotalEarned float64 `json:"total_earned"`

	// Photos and Documents
	Photos    []CarPhotoResponse    `json:"photos"`
	Documents []CarDocumentResponse `json:"documents"`

	// Owner info (for display)
	Owner *CarOwnerResponse `json:"owner,omitempty"`

	// Timestamps
	CreatedAt RFC3339Time `json:"created_at"`
	UpdatedAt RFC3339Time `json:"updated_at"`
}

type CarSpecsResponse struct {
	Make     string      `json:"make"`
	Model    string      `json:"model"`
	Year     int         `json:"year"`
	BodyType CarBodyType `json:"body_type"`
	FuelType FuelType    `json:"fuel_type"`
	Mileage  int         `json:"mileage"`
}

type CarLocationResponse struct {
	Address      string   `json:"address"`
	Neighborhood string   `json:"neighborhood"`
	Latitude     *float64 `json:"latitude,omitempty"`
	Longitude    *float64 `json:"longitude,omitempty"`
	Area         string   `json:"area"`
	Street       string   `json:"street"`
	Block        string   `json:"block"`
	Zip          string   `json:"zip"`
}

type CarRequirementsResponse struct {
	MinYearsLicensed  int               `json:"min_years_licensed"`
	DepositAmount     float64           `json:"deposit_amount"`
	InsuranceCoverage InsuranceCoverage `json:"insurance_coverage"`
}

type CarPhotoResponse struct {
	ID        uuid.UUID     `json:"id"`
	SlotType  PhotoSlotType `json:"slot_type"`
	FileURL   string        `json:"file_url"`
	FileSize  int           `json:"file_size"`
	CreatedAt RFC3339Time   `json:"created_at"`
	UpdatedAt RFC3339Time   `json:"updated_at"`
}

type CarDocumentResponse struct {
	ID           uuid.UUID       `json:"id"`
	DocumentType CarDocumentType `json:"document_type"`
	FileName     string          `json:"file_name"`
	FileURL      string          `json:"file_url"`
	FileSize     int             `json:"file_size"`
	CreatedAt    RFC3339Time     `json:"created_at"`
	UpdatedAt    RFC3339Time     `json:"updated_at"`
}

type CarOwnerResponse struct {
	ID              uuid.UUID `json:"id"`
	Name            string    `json:"name"`
	ProfilePhotoURL *string   `json:"profile_photo_url,omitempty"`
	// Rating and review count would come from a reviews table in the future
	Rating      float64 `json:"rating"`
	ReviewCount int     `json:"review_count"`
}

// ToResponse converts a Car model to CarResponse
func (c *Car) ToResponse(photos []CarPhoto, documents []CarDocument, owner *User) *CarResponse {
	resp := &CarResponse{
		ID:          c.ID,
		OwnerID:     c.OwnerID,
		Title:       c.Title,
		Description: "",
		Specs: CarSpecsResponse{
			Make:     c.Make,
			Model:    c.Model,
			Year:     c.Year,
			BodyType: c.BodyType,
			FuelType: c.FuelType,
			Mileage:  c.Mileage,
		},
		Location: CarLocationResponse{
			Address:      "",
			Neighborhood: "",
		},
		IsForRent: c.IsForRent,
		IsForSale: c.IsForSale,
		Currency:  c.Currency,
		Requirements: CarRequirementsResponse{
			MinYearsLicensed:  c.MinYearsLicensed,
			DepositAmount:     c.DepositAmount,
			InsuranceCoverage: c.InsuranceCoverage,
		},
		Status:      c.Status,
		IsPaused:    c.IsPaused,
		RentedWeeks: c.RentedWeeks,
		TotalEarned: c.TotalEarned,
		Photos:      make([]CarPhotoResponse, 0),
		Documents:   make([]CarDocumentResponse, 0),
		CreatedAt:   RFC3339Time(c.CreatedAt),
		UpdatedAt:   RFC3339Time(c.UpdatedAt),
	}

	// Handle nullable fields
	if c.Description.Valid {
		resp.Description = c.Description.String
	}
	if c.Address.Valid {
		resp.Location.Address = c.Address.String
	}
	if c.Neighborhood.Valid {
		resp.Location.Neighborhood = c.Neighborhood.String
	}
	if c.Latitude.Valid {
		lat := c.Latitude.Float64
		resp.Location.Latitude = &lat
	}
	if c.Longitude.Valid {
		lng := c.Longitude.Float64
		resp.Location.Longitude = &lng
	}
	if c.Area.Valid {
		resp.Location.Area = c.Area.String
	}
	if c.Street.Valid {
		resp.Location.Street = c.Street.String
	}
	if c.Block.Valid {
		resp.Location.Block = c.Block.String
	}
	if c.Zip.Valid {
		resp.Location.Zip = c.Zip.String
	}
	if c.WeeklyRentPrice.Valid {
		price := c.WeeklyRentPrice.Float64
		resp.WeeklyRentPrice = &price
	}
	if c.SalePrice.Valid {
		price := c.SalePrice.Float64
		resp.SalePrice = &price
	}

	// Convert photos
	for _, p := range photos {
		resp.Photos = append(resp.Photos, CarPhotoResponse{
			ID:        p.ID,
			SlotType:  p.SlotType,
			FileURL:   p.FileURL,
			FileSize:  p.FileSize,
			CreatedAt: RFC3339Time(p.CreatedAt),
			UpdatedAt: RFC3339Time(p.UpdatedAt),
		})
	}

	// Convert documents
	for _, d := range documents {
		resp.Documents = append(resp.Documents, CarDocumentResponse{
			ID:           d.ID,
			DocumentType: d.DocumentType,
			FileName:     d.FileName,
			FileURL:      d.FileURL,
			FileSize:     d.FileSize,
			CreatedAt:    RFC3339Time(d.CreatedAt),
			UpdatedAt:    RFC3339Time(d.UpdatedAt),
		})
	}

	// Add owner info if provided
	if owner != nil {
		ownerName := owner.FirstName
		if owner.LastName != "" {
			ownerName += " " + owner.LastName
		}
		resp.Owner = &CarOwnerResponse{
			ID:              owner.ID,
			Name:            ownerName,
			ProfilePhotoURL: owner.ProfilePhotoURL,
			Rating:          5.0, // Default rating for now
			ReviewCount:     0,   // Default review count
		}
	}

	return resp
}

// CreateCarRequest is the request body for creating a car
type CreateCarRequest struct {
	Title       string  `json:"title"`
	Description *string `json:"description,omitempty"`

	// Specs
	Make     string      `json:"make"`
	Model    string      `json:"model"`
	Year     int         `json:"year"`
	BodyType CarBodyType `json:"body_type"`
	FuelType FuelType    `json:"fuel_type"`
	Mileage  int         `json:"mileage"`

	// Location
	Address      *string  `json:"address,omitempty"`
	Neighborhood *string  `json:"neighborhood,omitempty"`
	Latitude     *float64 `json:"latitude,omitempty"`
	Longitude    *float64 `json:"longitude,omitempty"`
	Area         *string  `json:"area,omitempty"`
	Street       *string  `json:"street,omitempty"`
	Block        *string  `json:"block,omitempty"`
	Zip          *string  `json:"zip,omitempty"`

	// Pricing
	IsForRent       bool     `json:"is_for_rent"`
	WeeklyRentPrice *float64 `json:"weekly_rent_price,omitempty"`
	IsForSale       bool     `json:"is_for_sale"`
	SalePrice       *float64 `json:"sale_price,omitempty"`

	// Requirements
	MinYearsLicensed  *int               `json:"min_years_licensed,omitempty"`
	DepositAmount     *float64           `json:"deposit_amount,omitempty"`
	InsuranceCoverage *InsuranceCoverage `json:"insurance_coverage,omitempty"`
}

// UpdateCarRequest is the request body for updating a car
type UpdateCarRequest struct {
	Title       *string `json:"title,omitempty"`
	Description *string `json:"description,omitempty"`

	// Specs
	Make     *string      `json:"make,omitempty"`
	Model    *string      `json:"model,omitempty"`
	Year     *int         `json:"year,omitempty"`
	BodyType *CarBodyType `json:"body_type,omitempty"`
	FuelType *FuelType    `json:"fuel_type,omitempty"`
	Mileage  *int         `json:"mileage,omitempty"`

	// Location
	Address      *string  `json:"address,omitempty"`
	Neighborhood *string  `json:"neighborhood,omitempty"`
	Latitude     *float64 `json:"latitude,omitempty"`
	Longitude    *float64 `json:"longitude,omitempty"`
	Area         *string  `json:"area,omitempty"`
	Street       *string  `json:"street,omitempty"`
	Block        *string  `json:"block,omitempty"`
	Zip          *string  `json:"zip,omitempty"`

	// Pricing
	IsForRent       *bool    `json:"is_for_rent,omitempty"`
	WeeklyRentPrice *float64 `json:"weekly_rent_price,omitempty"`
	IsForSale       *bool    `json:"is_for_sale,omitempty"`
	SalePrice       *float64 `json:"sale_price,omitempty"`

	// Requirements
	MinYearsLicensed  *int               `json:"min_years_licensed,omitempty"`
	DepositAmount     *float64           `json:"deposit_amount,omitempty"`
	InsuranceCoverage *InsuranceCoverage `json:"insurance_coverage,omitempty"`

	// Status
	Status   *CarListingStatus `json:"status,omitempty"`
	IsPaused *bool             `json:"is_paused,omitempty"`
}

// UpdateCarLocationRequest is the request body for updating car location
type UpdateCarLocationRequest struct {
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
	Area      *string  `json:"area,omitempty"`
	Street    *string  `json:"street,omitempty"`
	Block     *string  `json:"block,omitempty"`
	Zip       *string  `json:"zip,omitempty"`
}
