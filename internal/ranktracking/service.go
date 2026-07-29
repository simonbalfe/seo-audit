package ranktracking

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type KeywordUpdate struct {
	Config        Config    `json:"config"`
	Added         int       `json:"added,omitempty"`
	Removed       int       `json:"removed,omitempty"`
	TotalKeywords int       `json:"total_keywords"`
	Keywords      []Keyword `json:"keywords"`
}

func Add(ctx context.Context, store Store, config Config, values []string) (KeywordUpdate, error) {
	normalized, err := NormalizeConfig(config)
	if err != nil {
		return KeywordUpdate{}, err
	}
	keywords, err := NormalizeKeywords(values)
	if err != nil {
		return KeywordUpdate{}, err
	}
	savedConfig, err := store.UpsertRankConfig(ctx, normalized)
	if err != nil {
		return KeywordUpdate{}, err
	}
	added, err := store.AddRankKeywords(ctx, savedConfig.ID, keywords, MaxKeywords)
	if err != nil {
		return KeywordUpdate{}, err
	}
	savedKeywords, err := store.ListRankKeywords(ctx, savedConfig.ID)
	if err != nil {
		return KeywordUpdate{}, err
	}
	return KeywordUpdate{
		Config:        savedConfig,
		Added:         added,
		TotalKeywords: len(savedKeywords),
		Keywords:      savedKeywords,
	}, nil
}

func Remove(ctx context.Context, store Store, target, location, language string, values []string) (KeywordUpdate, error) {
	config, err := store.GetRankConfig(ctx, target, location, language)
	if err != nil {
		return KeywordUpdate{}, err
	}
	keywords, err := NormalizeKeywords(values)
	if err != nil {
		return KeywordUpdate{}, err
	}
	removed, err := store.RemoveRankKeywords(ctx, config.ID, keywords)
	if err != nil {
		return KeywordUpdate{}, err
	}
	savedKeywords, err := store.ListRankKeywords(ctx, config.ID)
	if err != nil {
		return KeywordUpdate{}, err
	}
	return KeywordUpdate{
		Config:        config,
		Removed:       removed,
		TotalKeywords: len(savedKeywords),
		Keywords:      savedKeywords,
	}, nil
}

func Check(
	ctx context.Context,
	store Store,
	provider Provider,
	target,
	location,
	language string,
	progress func(string),
) (Report, error) {
	config, err := store.GetRankConfig(ctx, target, location, language)
	if err != nil {
		return Report{}, err
	}
	keywords, err := store.ListRankKeywords(ctx, config.ID)
	if err != nil {
		return Report{}, err
	}
	if len(keywords) == 0 {
		return Report{}, errors.New("rank tracker has no keywords; add keywords first")
	}
	tasks := BuildTasks(keywords, config.Devices)
	run, err := store.StartRankRun(ctx, config.ID, ProviderDataForSEO, len(tasks))
	if err != nil {
		return Report{}, err
	}
	providerReport, providerErr := provider.CheckRanks(ctx, CheckOptions{
		Target:   config.Target,
		Location: config.Location,
		Language: config.Language,
		Depth:    config.SERPDepth,
		Tasks:    tasks,
		Progress: progress,
	})
	run.SuccessfulTasks = providerReport.SuccessfulTasks
	run.LiveCalls = providerReport.LiveCalls
	run.CostUSD = providerReport.CostUSD
	run.ErrorMessage = strings.Join(providerReport.Errors, "; ")
	if providerErr != nil {
		if run.ErrorMessage != "" {
			run.ErrorMessage += "; "
		}
		run.ErrorMessage += providerErr.Error()
	}
	if len(providerReport.Results) == 0 && providerErr != nil {
		if failErr := store.FailRankRun(ctx, run); failErr != nil {
			return Report{}, fmt.Errorf("%v; additionally could not store failed rank check: %w", providerErr, failErr)
		}
		return Report{}, providerErr
	}
	if err := store.FinishRankRun(ctx, run, providerReport.Results); err != nil {
		return Report{}, err
	}
	report, err := store.GetRankReport(ctx, config.ID)
	if err != nil {
		return Report{}, err
	}
	return report, nil
}

func LoadReport(ctx context.Context, store Store, target, location, language string) (Report, error) {
	config, err := store.GetRankConfig(ctx, target, location, language)
	if err != nil {
		return Report{}, err
	}
	return store.GetRankReport(ctx, config.ID)
}
