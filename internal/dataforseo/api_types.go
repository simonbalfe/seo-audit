package dataforseo

type itemsResult[T any] struct {
	Items []T `json:"items"`
}

type domainOverviewResult struct {
	Items []struct {
		Metrics struct {
			Organic organicMetricItem `json:"organic"`
		} `json:"metrics"`
	} `json:"items"`
}

type organicMetricItem struct {
	Count                    float64 `json:"count"`
	ETV                      float64 `json:"etv"`
	EstimatedPaidTrafficCost float64 `json:"estimated_paid_traffic_cost"`
	Position1                float64 `json:"pos_1"`
	Positions2To3            float64 `json:"pos_2_3"`
	Positions4To10           float64 `json:"pos_4_10"`
	Positions11To20          float64 `json:"pos_11_20"`
	Positions21To30          float64 `json:"pos_21_30"`
	Positions31To40          float64 `json:"pos_31_40"`
	Positions41To50          float64 `json:"pos_41_50"`
	Positions51To60          float64 `json:"pos_51_60"`
	Positions61To70          float64 `json:"pos_61_70"`
	Positions71To80          float64 `json:"pos_71_80"`
	Positions81To90          float64 `json:"pos_81_90"`
	Positions91To100         float64 `json:"pos_91_100"`
	New                      float64 `json:"is_new"`
	Up                       float64 `json:"is_up"`
	Down                     float64 `json:"is_down"`
	Lost                     float64 `json:"is_lost"`
}

type keywordData struct {
	Keyword     string `json:"keyword"`
	KeywordInfo struct {
		SearchVolume     float64  `json:"search_volume"`
		CPC              *float64 `json:"cpc"`
		Competition      *float64 `json:"competition"`
		CompetitionLevel string   `json:"competition_level"`
		LastUpdatedTime  string   `json:"last_updated_time"`
	} `json:"keyword_info"`
	KeywordProperties struct {
		KeywordDifficulty *float64 `json:"keyword_difficulty"`
	} `json:"keyword_properties"`
	SearchIntentInfo struct {
		MainIntent string `json:"main_intent"`
	} `json:"search_intent_info"`
}

type rankedKeywordItem struct {
	KeywordData       keywordData `json:"keyword_data"`
	RankedSERPElement struct {
		LastUpdatedTime string `json:"last_updated_time"`
		SERPItem        struct {
			RankAbsolute float64 `json:"rank_absolute"`
			URL          string  `json:"url"`
			ETV          float64 `json:"etv"`
			RankChanges  struct {
				PreviousRankAbsolute *int `json:"previous_rank_absolute"`
			} `json:"rank_changes"`
		} `json:"serp_item"`
	} `json:"ranked_serp_element"`
}

type competitorItem struct {
	Domain            string  `json:"domain"`
	Intersections     float64 `json:"intersections"`
	AveragePosition   float64 `json:"avg_position"`
	FullDomainMetrics struct {
		Organic organicMetricItem `json:"organic"`
	} `json:"full_domain_metrics"`
}

type backlinkSummaryItem struct {
	Rank                   int    `json:"rank"`
	Backlinks              int    `json:"backlinks"`
	BacklinksSpamScore     int    `json:"backlinks_spam_score"`
	ReferringDomains       int    `json:"referring_domains"`
	ReferringMainDomains   int    `json:"referring_main_domains"`
	ReferringPages         int    `json:"referring_pages"`
	ReferringPagesNofollow int    `json:"referring_pages_nofollow"`
	ReferringIPs           int    `json:"referring_ips"`
	BrokenBacklinks        int    `json:"broken_backlinks"`
	BrokenPages            int    `json:"broken_pages"`
	FirstSeen              string `json:"first_seen"`
	CrawledPages           int    `json:"crawled_pages"`
	Info                   struct {
		TargetSpamScore int `json:"target_spam_score"`
	} `json:"info"`
}

type referringDomainItem struct {
	Domain                 string `json:"domain"`
	Rank                   int    `json:"rank"`
	Backlinks              int    `json:"backlinks"`
	BacklinksSpamScore     int    `json:"backlinks_spam_score"`
	ReferringPages         int    `json:"referring_pages"`
	ReferringPagesNofollow int    `json:"referring_pages_nofollow"`
	FirstSeen              string `json:"first_seen"`
}

type backlinkItem struct {
	URLFrom            string `json:"url_from"`
	DomainFrom         string `json:"domain_from"`
	URLTo              string `json:"url_to"`
	Anchor             string `json:"anchor"`
	Dofollow           bool   `json:"dofollow"`
	Rank               int    `json:"rank"`
	DomainFromRank     int    `json:"domain_from_rank"`
	BacklinkSpamScore  int    `json:"backlink_spam_score"`
	PageFromStatusCode int    `json:"page_from_status_code"`
	URLToStatusCode    int    `json:"url_to_status_code"`
	PageFromTitle      string `json:"page_from_title"`
	PageFromLanguage   string `json:"page_from_language"`
	SemanticLocation   string `json:"semantic_location"`
	FirstSeen          string `json:"first_seen"`
	LastSeen           string `json:"last_seen"`
	IsNew              bool   `json:"is_new"`
	IsLost             bool   `json:"is_lost"`
	IsBroken           bool   `json:"is_broken"`
}
