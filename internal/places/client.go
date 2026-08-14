// Package places audits public Google Maps listings through the Places API.
package places

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/simonbalfe/seo-audit/internal/report"
)

const baseURL = "https://places.googleapis.com"

type Client struct {
	HTTP    *http.Client
	BaseURL string
	APIKey  string
}

type place struct {
	ID                     string             `json:"id"`
	DisplayName            text               `json:"displayName"`
	PrimaryTypeDisplayName text               `json:"primaryTypeDisplayName"`
	FormattedAddress       string             `json:"formattedAddress"`
	AddressComponents      []addressComponent `json:"addressComponents"`
	InternationalPhone     string             `json:"internationalPhoneNumber"`
	Website                string             `json:"websiteUri"`
	GoogleMapsURL          string             `json:"googleMapsUri"`
	BusinessStatus         string             `json:"businessStatus"`
	Rating                 *float64           `json:"rating"`
	UserRatingCount        *int               `json:"userRatingCount"`
	Photos                 []json.RawMessage  `json:"photos"`
	Location               struct {
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
	} `json:"location"`
	RegularOpeningHours struct {
		WeekdayDescriptions []string `json:"weekdayDescriptions"`
	} `json:"regularOpeningHours"`
}

type addressComponent struct {
	LongText string   `json:"longText"`
	Types    []string `json:"types"`
}

type text struct {
	Text string `json:"text"`
}

func NewClient() (*Client, error) {
	key := strings.TrimSpace(os.Getenv("GOOGLE_MAPS_API_KEY"))
	if key == "" {
		return nil, errors.New("Google Places API is not configured; set GOOGLE_MAPS_API_KEY")
	}
	return NewClientWithAPIKey(key), nil
}

func NewClientWithAPIKey(key string) *Client {
	return &Client{
		HTTP:    &http.Client{Timeout: 30 * time.Second},
		BaseURL: baseURL,
		APIKey:  key,
	}
}

// AuditPlace audits a public Places listing by its Google Place ID.
func (c *Client) AuditPlace(ctx context.Context, placeID string) (report.GBPAuditReport, error) {
	placeID = strings.TrimSpace(placeID)
	if placeID == "" {
		return report.GBPAuditReport{}, errors.New("Google Place ID is required")
	}
	details, err := c.details(ctx, placeID)
	if err != nil {
		return report.GBPAuditReport{}, err
	}
	return buildReport(placeID, details), nil
}

func (c *Client) details(ctx context.Context, placeID string) (place, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.BaseURL, "/")+"/v1/places/"+url.PathEscape(placeID), nil)
	if err != nil {
		return place{}, err
	}
	request.Header.Set("X-Goog-Api-Key", c.APIKey)
	request.Header.Set("X-Goog-FieldMask", strings.Join([]string{
		"id", "displayName", "primaryTypeDisplayName", "formattedAddress", "addressComponents", "internationalPhoneNumber",
		"websiteUri", "googleMapsUri", "businessStatus", "rating", "userRatingCount", "photos", "location", "regularOpeningHours",
	}, ","))
	var response place
	if err := c.do(request, &response); err != nil {
		return place{}, fmt.Errorf("get Google Place details: %w", err)
	}
	return response, nil
}

func (c *Client) do(request *http.Request, output any) error {
	response, err := c.HTTP.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("Google Places API returned %s: %s", response.Status, strings.TrimSpace(string(body)))
	}
	if err := json.Unmarshal(body, output); err != nil {
		return fmt.Errorf("decode Google Places response: %w", err)
	}
	return nil
}

func buildReport(query string, item place) report.GBPAuditReport {
	result := report.GBPAuditReport{
		Query:          query,
		PlaceID:        item.ID,
		Name:           item.DisplayName.Text,
		Category:       item.PrimaryTypeDisplayName.Text,
		Address:        item.FormattedAddress,
		Market:         market(item.AddressComponents),
		Country:        country(item.AddressComponents),
		Phone:          item.InternationalPhone,
		Website:        item.Website,
		GoogleMapsURL:  item.GoogleMapsURL,
		BusinessStatus: item.BusinessStatus,
		Latitude:       item.Location.Latitude,
		Longitude:      item.Location.Longitude,
		Hours:          item.RegularOpeningHours.WeekdayDescriptions,
		Findings:       make([]report.GBPFinding, 0),
	}
	if item.Rating != nil {
		result.Rating = *item.Rating
	}
	if item.UserRatingCount != nil {
		result.ReviewCount = *item.UserRatingCount
	}
	if item.Photos != nil {
		result.PhotoCount = len(item.Photos)
	}
	add := func(priority, check, evidence, fix string) {
		result.Findings = append(result.Findings, report.GBPFinding{Priority: priority, Check: check, Evidence: evidence, Fix: fix})
	}
	if item.BusinessStatus != "OPERATIONAL" {
		add("high", "Business status", item.BusinessStatus, "Confirm the listing status and correct it if the business is operating.")
	}
	if result.Category == "" {
		add("medium", "Missing primary category", "no public primary category", "Choose the most specific primary business category.")
	}
	if result.Address == "" {
		add("medium", "Missing address", "no public address", "Add the correct address or service area.")
	}
	if result.Phone == "" {
		add("medium", "Missing phone", "no public phone number", "Add a customer contact number.")
	}
	if result.Website == "" {
		add("medium", "Missing website", "no public website", "Link the canonical business website.")
	}
	if len(result.Hours) == 0 {
		add("medium", "Missing hours", "no public opening hours", "Add current regular opening hours.")
	}
	if item.UserRatingCount != nil && result.ReviewCount == 0 {
		add("low", "No reviews", "no public reviews", "Invite genuine customers to leave reviews.")
	}
	if item.Photos != nil && result.PhotoCount == 0 {
		add("low", "No photos", "no public photos", "Add representative business, team, and service photos.")
	}
	return result
}

func country(components []addressComponent) string {
	return addressPart(components, "country")
}

func market(components []addressComponent) string {
	locality := addressPart(components, "postal_town", "locality", "administrative_area_level_2", "administrative_area_level_1")
	countryName := country(components)
	if locality == "" {
		return countryName
	}
	if countryName == "" || locality == countryName {
		return locality
	}
	return locality + ", " + countryName
}

func addressPart(components []addressComponent, types ...string) string {
	for _, wanted := range types {
		for _, component := range components {
			for _, componentType := range component.Types {
				if componentType == wanted {
					return component.LongText
				}
			}
		}
	}
	return ""
}
