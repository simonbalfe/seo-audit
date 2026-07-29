package ranktracking

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"
)

const (
	DefaultDepth       = 100
	MaxKeywords        = 100
	MaxKeywordLength   = 200
	DefaultLocation    = "United Kingdom"
	DefaultLanguage    = "en"
	DefaultDevice      = "desktop"
	ProviderDataForSEO = "DataForSEO"
)

var ErrTrackerNotFound = errors.New("rank tracker not found; add keywords first")

type Config struct {
	ID        int64     `json:"id"`
	Target    string    `json:"target"`
	Location  string    `json:"location"`
	Language  string    `json:"language"`
	Devices   string    `json:"devices"`
	SERPDepth int       `json:"serp_depth"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Keyword struct {
	ID        int64     `json:"id"`
	ConfigID  int64     `json:"config_id"`
	Keyword   string    `json:"keyword"`
	CreatedAt time.Time `json:"created_at"`
}

type Run struct {
	ID              int64      `json:"id"`
	ConfigID        int64      `json:"config_id"`
	Provider        string     `json:"provider"`
	Status          string     `json:"status"`
	RequestedTasks  int        `json:"requested_tasks"`
	SuccessfulTasks int        `json:"successful_tasks"`
	LiveCalls       int        `json:"live_calls"`
	CostUSD         float64    `json:"cost_usd"`
	StartedAt       time.Time  `json:"started_at"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
	ErrorMessage    string     `json:"error_message,omitempty"`
}

type CheckTask struct {
	KeywordID int64  `json:"keyword_id"`
	Keyword   string `json:"keyword"`
	Device    string `json:"device"`
}

type CheckOptions struct {
	Target   string
	Location string
	Language string
	Depth    int
	Tasks    []CheckTask
	Progress func(message string)
}

type Result struct {
	KeywordID    int64    `json:"keyword_id"`
	Keyword      string   `json:"keyword"`
	Device       string   `json:"device"`
	Position     *int     `json:"position,omitempty"`
	RankingURL   string   `json:"ranking_url,omitempty"`
	SERPFeatures []string `json:"serp_features,omitempty"`
}

type ProviderReport struct {
	Source          string    `json:"source"`
	RetrievedAt     time.Time `json:"retrieved_at"`
	RequestedTasks  int       `json:"requested_tasks"`
	SuccessfulTasks int       `json:"successful_tasks"`
	LiveCalls       int       `json:"live_calls"`
	CostUSD         float64   `json:"cost_usd"`
	Results         []Result  `json:"results,omitempty"`
	Errors          []string  `json:"errors,omitempty"`
}

type Row struct {
	Keyword          string   `json:"keyword"`
	Device           string   `json:"device"`
	Observed         bool     `json:"observed"`
	Position         *int     `json:"position,omitempty"`
	PreviousPosition *int     `json:"previous_position,omitempty"`
	PreviousObserved bool     `json:"previous_observed"`
	RankingURL       string   `json:"ranking_url,omitempty"`
	SERPFeatures     []string `json:"serp_features,omitempty"`
	Change           string   `json:"change"`
}

type Summary struct {
	TrackedKeywords int `json:"tracked_keywords"`
	TrackedTasks    int `json:"tracked_tasks"`
	Checked         int `json:"checked"`
	NotChecked      int `json:"not_checked"`
	Ranking         int `json:"ranking"`
	NotRanking      int `json:"not_ranking"`
	Top3            int `json:"top_3"`
	Top10           int `json:"top_10"`
	Improved        int `json:"improved"`
	Declined        int `json:"declined"`
	New             int `json:"new"`
	Lost            int `json:"lost"`
	Stable          int `json:"stable"`
	Uncompared      int `json:"uncompared"`
}

type Report struct {
	Config        Config    `json:"config"`
	Keywords      []Keyword `json:"keywords"`
	LatestRun     *Run      `json:"latest_run,omitempty"`
	PreviousRunID *int64    `json:"previous_run_id,omitempty"`
	Summary       Summary   `json:"summary"`
	Rows          []Row     `json:"rows"`
}

type Store interface {
	UpsertRankConfig(context.Context, Config) (Config, error)
	GetRankConfig(context.Context, string, string, string) (Config, error)
	AddRankKeywords(context.Context, int64, []string, int) (int, error)
	RemoveRankKeywords(context.Context, int64, []string) (int, error)
	ListRankKeywords(context.Context, int64) ([]Keyword, error)
	StartRankRun(context.Context, int64, string, int) (Run, error)
	FinishRankRun(context.Context, Run, []Result) error
	FailRankRun(context.Context, Run) error
	GetRankReport(context.Context, int64) (Report, error)
}

type Provider interface {
	CheckRanks(context.Context, CheckOptions) (ProviderReport, error)
}

func NormalizeConfig(config Config) (Config, error) {
	config.Target = strings.ToLower(strings.TrimSpace(config.Target))
	config.Location = strings.TrimSpace(config.Location)
	config.Language = strings.ToLower(strings.TrimSpace(config.Language))
	config.Devices = strings.ToLower(strings.TrimSpace(config.Devices))
	if config.Location == "" {
		config.Location = DefaultLocation
	}
	if config.Language == "" {
		config.Language = DefaultLanguage
	}
	if config.Devices == "" {
		config.Devices = DefaultDevice
	}
	if config.SERPDepth == 0 {
		config.SERPDepth = DefaultDepth
	}
	if config.Target == "" {
		return Config{}, errors.New("rank tracking target is empty")
	}
	if config.Devices != "desktop" && config.Devices != "mobile" && config.Devices != "both" {
		return Config{}, errors.New("rank tracking device must be desktop, mobile, or both")
	}
	if config.SERPDepth < 10 || config.SERPDepth > 100 || config.SERPDepth%10 != 0 {
		return Config{}, errors.New("rank tracking depth must be a multiple of 10 from 10 to 100")
	}
	return config, nil
}

func NormalizeKeywords(values []string) ([]string, error) {
	seen := make(map[string]bool, len(values))
	keywords := make([]string, 0, len(values))
	for _, value := range values {
		keyword := strings.Join(strings.Fields(value), " ")
		if keyword == "" {
			continue
		}
		if len(keyword) > MaxKeywordLength {
			return nil, errors.New("tracked keyword exceeds 200 characters")
		}
		normalized := strings.ToLower(keyword)
		if seen[normalized] {
			continue
		}
		seen[normalized] = true
		keywords = append(keywords, keyword)
	}
	if len(keywords) == 0 {
		return nil, errors.New("provide at least one non-empty keyword")
	}
	sort.Slice(keywords, func(i, j int) bool {
		return strings.ToLower(keywords[i]) < strings.ToLower(keywords[j])
	})
	return keywords, nil
}

func Devices(value string) []string {
	if value == "both" {
		return []string{"desktop", "mobile"}
	}
	return []string{value}
}

func BuildTasks(keywords []Keyword, devices string) []CheckTask {
	deviceValues := Devices(devices)
	tasks := make([]CheckTask, 0, len(keywords)*len(deviceValues))
	for _, keyword := range keywords {
		for _, device := range deviceValues {
			tasks = append(tasks, CheckTask{
				KeywordID: keyword.ID,
				Keyword:   keyword.Keyword,
				Device:    device,
			})
		}
	}
	return tasks
}

func ClassifyChange(position, previous *int, observed, previousObserved bool) string {
	if !observed {
		return "not-checked"
	}
	if !previousObserved {
		return "uncompared"
	}
	if previous == nil && position != nil {
		return "new"
	}
	if previous != nil && position == nil {
		return "lost"
	}
	if previous == nil {
		return "stable"
	}
	if *position < *previous {
		return "improved"
	}
	if *position > *previous {
		return "declined"
	}
	return "stable"
}
