package api

import (
	"context"
	"errors"
	"testing"
)

func TestJobManagerBoundsCapacityAndReplaysIdempotentJobs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	manager := NewJobManager(ctx, 1, 1)
	block := make(chan struct{})
	first, replayed, err := manager.Submit("audit", "audit-key", func(
		context.Context,
		func(string, string),
	) (any, error) {
		<-block
		return map[string]string{"status": "done"}, nil
	})
	if err != nil || replayed {
		t.Fatalf("submit first job: replayed=%t err=%v", replayed, err)
	}
	replayedJob, replayed, err := manager.Submit("audit", "audit-key", func(
		context.Context,
		func(string, string),
	) (any, error) {
		return nil, errors.New("must not run")
	})
	if err != nil || !replayed || replayedJob.ID != first.ID {
		t.Fatalf("replay job: %#v replayed=%t err=%v", replayedJob, replayed, err)
	}
	if _, _, err := manager.Submit("audit", "other-key", func(
		context.Context,
		func(string, string),
	) (any, error) {
		return nil, nil
	}); !errors.Is(err, ErrJobCapacity) {
		t.Fatalf("capacity error = %v", err)
	}
	close(block)
}
