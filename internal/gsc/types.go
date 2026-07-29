package gsc

import "time"

type Options struct {
	SiteURL  string
	Days     int
	Limit    int
	Progress func(string)
}

type QueryPageMetric struct {
	Query       string  `json:"query"`
	Page        string  `json:"page"`
	Clicks      float64 `json:"clicks"`
	Impressions float64 `json:"impressions"`
	CTR         float64 `json:"ctr"`
	Position    float64 `json:"position"`
}

type QueryOverlap struct {
	Query       string   `json:"query"`
	Pages       []string `json:"pages"`
	Clicks      float64  `json:"clicks"`
	Impressions float64  `json:"impressions"`
}

type Summary struct {
	Rows                int     `json:"rows"`
	ReturnedClicks      float64 `json:"returned_clicks"`
	ReturnedImpressions float64 `json:"returned_impressions"`
	ReturnedCTR         float64 `json:"returned_ctr"`
	WeightedPosition    float64 `json:"weighted_position"`
}

type Report struct {
	Enabled          bool              `json:"enabled"`
	Available        bool              `json:"available"`
	Source           string            `json:"source"`
	SiteURL          string            `json:"site_url"`
	StartDate        string            `json:"start_date"`
	EndDate          string            `json:"end_date"`
	RetrievedAt      time.Time         `json:"retrieved_at"`
	RowLimit         int               `json:"row_limit"`
	AggregationType  string            `json:"aggregation_type,omitempty"`
	Summary          Summary           `json:"summary"`
	QueryPages       []QueryPageMetric `json:"query_pages"`
	StrikingDistance []QueryPageMetric `json:"striking_distance"`
	QueryOverlaps    []QueryOverlap    `json:"query_overlaps"`
}
