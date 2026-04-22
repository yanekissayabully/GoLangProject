package models

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
)

// ─── Enums ───────────────────────────────────────────────────────────────────

type CarListingStatus string

const (
	CarStatusAvailable CarListingStatus = "available"
	CarStatusRented    CarListingStatus = "rented"
	CarStatusPending   CarListingStatus = "pending"
	CarStatusPaused    CarListingStatus = "paused"
)

type CarBodyType string

const (
	BodyTypeSedan CarBodyType = "sedan"
	BodyTypeSUV   CarBodyType = "suv"
)

type FuelType string

const (
	FuelTypeGas    FuelType = "gas"
	FuelTypeDiesel FuelType = "diesel"
)

type InsuranceCoverage string

const (
	InsuranceFullCoverage InsuranceCoverage = "full_coverage"
)

// ─── Car model ───────────────────────────────────────────────────────────────

type Car struct {
	ID          uuid.UUID        `json:"id"`
	OwnerID     uuid.UUID        `json:"owner_id"`
	Title       string           `json:"title"`
	Description sql.NullString   `json:"-"`

	Make     string      `json:"make"`
	Model    string      `json:"model"`
	Year     int         `json:"year"`
	BodyType CarBodyType `json:"body_type"`
	FuelType FuelType    `json:"fuel_type"`
	Mileage  int         `json:"mileage"`

	Address      sql.NullString  `json:"-"`
	Neighborhood sql.NullString  `json:"-"`
	Latitude     sql.NullFloat64 `json:"-"`
	Longitude    sql.NullFloat64 `json:"-"`
	Area         sql.NullString  `json:"-"`
	Street       sql.NullString  `json:"-"`
	Block        sql.NullString  `json:"-"`
	Zip          sql.NullString  `json:"-"`

	IsForRent       bool            `json:"is_for_rent"`
	WeeklyRentPrice sql.NullFloat64 `json:"-"`
	IsForSale       bool            `json:"is_for_sale"`
	SalePrice       sql.NullFloat64 `json:"-"`
	Currency        string          `json:"currency"`

	MinYearsLicensed  int               `json:"min_years_licensed"`
	DepositAmount     float64           `json:"deposit_amount"`
	InsuranceCoverage InsuranceCoverage `json:"insurance_coverage"`

	Status   CarListingStatus `json:"status"`
	IsPaused bool             `json:"is_paused"`

	RentedWeeks int     `json:"rented_weeks"`
	TotalEarned float64 `json:"total_earned"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ─── Response types ──────────────────────────────────────────────────────────

type CarResponse struct {
	ID          uuid.UUID        `json:"id"`
	OwnerID     uuid.UUID        `json:"owner_id"`
	Title       string           `json:"title"`
	Description string           `json:"description"`
	Specs       CarSpecsResponse `json:"specs"`
	Location    CarLocResponse   `json:"location"`

	IsForRent       bool     `json:"is_for_rent"`
	WeeklyRentPrice *float64 `json:"weekly_rent_price,omitempty"`
	IsForSale       bool     `json:"is_for_sale"`
	SalePrice       *float64 `json:"sale_price,omitempty"`
	Currency        string   `json:"currency"`

	Requirements CarReqResponse   `json:"requirements"`
	Status       CarListingStatus `json:"status"`
	IsPaused     bool             `json:"is_paused"`
	RentedWeeks  int              `json:"rented_weeks"`
	TotalEarned  float64          `json:"total_earned"`

	Photos    []interface{}    `json:"photos"`
	Documents []interface{}    `json:"documents"`
	Owner     *CarOwnerResp    `json:"owner,omitempty"`

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

type CarLocResponse struct {
	Address      string   `json:"address"`
	Neighborhood string   `json:"neighborhood"`
	Latitude     *float64 `json:"latitude,omitempty"`
	Longitude    *float64 `json:"longitude,omitempty"`
	Area         string   `json:"area"`
	Street       string   `json:"street"`
	Block        string   `json:"block"`
	Zip          string   `json:"zip"`
}

type CarReqResponse struct {
	MinYearsLicensed  int               `json:"min_years_licensed"`
	DepositAmount     float64           `json:"deposit_amount"`
	InsuranceCoverage InsuranceCoverage `json:"insurance_coverage"`
}

type CarOwnerResp struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Rating      float64   `json:"rating"`
	ReviewCount int       `json:"review_count"`
}

// ToResponse converts Car to the API response format.
func (c *Car) ToResponse(owner *User) *CarResponse {
	resp := &CarResponse{
		ID:          c.ID,
		OwnerID:     c.OwnerID,
		Title:       c.Title,
		Specs:       CarSpecsResponse{Make: c.Make, Model: c.Model, Year: c.Year, BodyType: c.BodyType, FuelType: c.FuelType, Mileage: c.Mileage},
		Location:    CarLocResponse{},
		IsForRent:   c.IsForRent,
		IsForSale:   c.IsForSale,
		Currency:    c.Currency,
		Requirements: CarReqResponse{MinYearsLicensed: c.MinYearsLicensed, DepositAmount: c.DepositAmount, InsuranceCoverage: c.InsuranceCoverage},
		Status:      c.Status,
		IsPaused:    c.IsPaused,
		RentedWeeks: c.RentedWeeks,
		TotalEarned: c.TotalEarned,
		Photos:      make([]interface{}, 0),
		Documents:   make([]interface{}, 0),
		CreatedAt:   RFC3339Time(c.CreatedAt),
		UpdatedAt:   RFC3339Time(c.UpdatedAt),
	}

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
		v := c.Latitude.Float64
		resp.Location.Latitude = &v
	}
	if c.Longitude.Valid {
		v := c.Longitude.Float64
		resp.Location.Longitude = &v
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
		v := c.WeeklyRentPrice.Float64
		resp.WeeklyRentPrice = &v
	}
	if c.SalePrice.Valid {
		v := c.SalePrice.Float64
		resp.SalePrice = &v
	}

	if owner != nil {
		resp.Owner = &CarOwnerResp{
			ID:          owner.ID,
			Name:        owner.FullName(),
			Rating:      5.0,
			ReviewCount: 0,
		}
	}

	return resp
}

// ─── Request types ───────────────────────────────────────────────────────────

type CreateCarRequest struct {
	Title       string  `json:"title"`
	Description *string `json:"description,omitempty"`
	Make        string  `json:"make"`
	Model       string  `json:"model"`
	Year        int     `json:"year"`
	BodyType    CarBodyType `json:"body_type"`
	FuelType    FuelType    `json:"fuel_type"`
	Mileage     int         `json:"mileage"`

	Address      *string  `json:"address,omitempty"`
	Neighborhood *string  `json:"neighborhood,omitempty"`
	Latitude     *float64 `json:"latitude,omitempty"`
	Longitude    *float64 `json:"longitude,omitempty"`
	Area         *string  `json:"area,omitempty"`
	Street       *string  `json:"street,omitempty"`
	Block        *string  `json:"block,omitempty"`
	Zip          *string  `json:"zip,omitempty"`

	IsForRent       bool     `json:"is_for_rent"`
	WeeklyRentPrice *float64 `json:"weekly_rent_price,omitempty"`
	IsForSale       bool     `json:"is_for_sale"`
	SalePrice       *float64 `json:"sale_price,omitempty"`

	MinYearsLicensed  *int               `json:"min_years_licensed,omitempty"`
	DepositAmount     *float64           `json:"deposit_amount,omitempty"`
	InsuranceCoverage *InsuranceCoverage `json:"insurance_coverage,omitempty"`
}

type UpdateCarRequest struct {
	Title       *string `json:"title,omitempty"`
	Description *string `json:"description,omitempty"`
	Make        *string      `json:"make,omitempty"`
	Model       *string      `json:"model,omitempty"`
	Year        *int         `json:"year,omitempty"`
	BodyType    *CarBodyType `json:"body_type,omitempty"`
	FuelType    *FuelType    `json:"fuel_type,omitempty"`
	Mileage     *int         `json:"mileage,omitempty"`

	Address      *string  `json:"address,omitempty"`
	Neighborhood *string  `json:"neighborhood,omitempty"`
	Latitude     *float64 `json:"latitude,omitempty"`
	Longitude    *float64 `json:"longitude,omitempty"`
	Area         *string  `json:"area,omitempty"`
	Street       *string  `json:"street,omitempty"`
	Block        *string  `json:"block,omitempty"`
	Zip          *string  `json:"zip,omitempty"`

	IsForRent       *bool    `json:"is_for_rent,omitempty"`
	WeeklyRentPrice *float64 `json:"weekly_rent_price,omitempty"`
	IsForSale       *bool    `json:"is_for_sale,omitempty"`
	SalePrice       *float64 `json:"sale_price,omitempty"`

	MinYearsLicensed  *int               `json:"min_years_licensed,omitempty"`
	DepositAmount     *float64           `json:"deposit_amount,omitempty"`
	InsuranceCoverage *InsuranceCoverage `json:"insurance_coverage,omitempty"`

	Status   *CarListingStatus `json:"status,omitempty"`
	IsPaused *bool             `json:"is_paused,omitempty"`
}
