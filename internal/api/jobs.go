package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"github.com/simonbalfe/seo-audit/internal/protocol"
)

const (
	JobQueued    = protocol.JobQueued
	JobRunning   = protocol.JobRunning
	JobSucceeded = protocol.JobSucceeded
	JobFailed    = protocol.JobFailed
	JobCancelled = protocol.JobCancelled
)

type Job = protocol.Job
type JobEvent = protocol.JobEvent
type JobEvents = protocol.JobEvents

var (
	ErrJobNotFound  = errors.New("job not found")
	ErrJobNotReady  = errors.New("job has not completed")
	ErrJobNoResult  = errors.New("job completed without a result")
	ErrJobIDFailure = errors.New("could not generate job id")
	ErrJobCapacity  = errors.New("API job capacity reached")
)

type JobTask func(context.Context, func(string, string)) (any, error)

type jobRecord struct {
	view   Job
	result any
	cancel context.CancelFunc
}

type JobManager struct {
	ctx         context.Context
	mu          sync.RWMutex
	jobs        map[string]*jobRecord
	idempotency map[string]string
	workers     chan struct{}
	retention   int
}

func NewJobManager(ctx context.Context, workerCount, retention int) *JobManager {
	if workerCount <= 0 {
		workerCount = 4
	}
	if retention <= 0 {
		retention = 100
	}
	return &JobManager{
		ctx:         ctx,
		jobs:        make(map[string]*jobRecord),
		idempotency: make(map[string]string),
		workers:     make(chan struct{}, workerCount),
		retention:   retention,
	}
}

func (m *JobManager) Submit(kind, idempotencyKey string, task JobTask) (Job, bool, error) {
	key := kind + "\x00" + idempotencyKey
	m.mu.Lock()
	if idempotencyKey != "" {
		if existingID := m.idempotency[key]; existingID != "" {
			if existing := m.jobs[existingID]; existing != nil {
				view := cloneJob(existing.view, 0)
				m.mu.Unlock()
				return view, true, nil
			}
			delete(m.idempotency, key)
		}
	}
	m.pruneLocked(m.retention - 1)
	if len(m.jobs) >= m.retention {
		m.mu.Unlock()
		return Job{}, false, ErrJobCapacity
	}
	id, err := newJobID()
	if err != nil {
		m.mu.Unlock()
		return Job{}, false, err
	}
	now := time.Now().UTC()
	ctx, cancel := context.WithCancel(m.ctx)
	record := &jobRecord{
		view: Job{
			ID:        id,
			Kind:      kind,
			Status:    JobQueued,
			CreatedAt: now,
			StatusURL: "/api/v1/jobs/" + id,
			EventsURL: "/api/v1/jobs/" + id + "/events",
			ResultURL: "/api/v1/jobs/" + id + "/result",
		},
		cancel: cancel,
	}
	m.jobs[id] = record
	if idempotencyKey != "" {
		m.idempotency[key] = id
	}
	view := cloneJob(record.view, 0)
	m.mu.Unlock()
	go m.run(ctx, record, task)
	return view, false, nil
}

func (m *JobManager) Get(id string, after int64) (Job, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	record := m.jobs[id]
	if record == nil {
		return Job{}, ErrJobNotFound
	}
	return cloneJob(record.view, after), nil
}

func (m *JobManager) Events(id string, after int64) (JobEvents, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	record := m.jobs[id]
	if record == nil {
		return JobEvents{}, ErrJobNotFound
	}
	events := eventsAfter(record.view.Events, after)
	next := after
	if len(events) > 0 {
		next = events[len(events)-1].Sequence
	}
	return JobEvents{Events: events, NextAfter: next}, nil
}

func (m *JobManager) Result(id string) (any, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	record := m.jobs[id]
	if record == nil {
		return nil, ErrJobNotFound
	}
	if record.view.Status == JobQueued || record.view.Status == JobRunning {
		return nil, ErrJobNotReady
	}
	if record.result == nil {
		return nil, ErrJobNoResult
	}
	return record.result, nil
}

func (m *JobManager) Cancel(id string) (Job, error) {
	m.mu.Lock()
	record := m.jobs[id]
	if record == nil {
		m.mu.Unlock()
		return Job{}, ErrJobNotFound
	}
	if record.view.Status == JobQueued || record.view.Status == JobRunning {
		record.cancel()
	}
	view := cloneJob(record.view, 0)
	m.mu.Unlock()
	return view, nil
}

func (m *JobManager) run(ctx context.Context, record *jobRecord, task JobTask) {
	select {
	case m.workers <- struct{}{}:
	case <-ctx.Done():
		m.finishCancelled(record)
		return
	}
	defer func() { <-m.workers }()

	m.mu.Lock()
	now := time.Now().UTC()
	record.view.Status = JobRunning
	record.view.StartedAt = &now
	m.appendEventLocked(record, "job", "Started "+record.view.Kind)
	m.mu.Unlock()

	emit := func(stage, message string) {
		m.mu.Lock()
		m.appendEventLocked(record, stage, message)
		m.mu.Unlock()
	}
	result, err := task(ctx, emit)

	m.mu.Lock()
	defer m.mu.Unlock()
	completed := time.Now().UTC()
	record.view.CompletedAt = &completed
	record.result = result
	if ctx.Err() != nil {
		record.view.Status = JobCancelled
		record.view.Error = "job cancelled"
		m.appendEventLocked(record, "job", "Cancelled")
		return
	}
	if err != nil {
		record.view.Status = JobFailed
		record.view.Error = err.Error()
		m.appendEventLocked(record, "job", "Failed: "+err.Error())
		return
	}
	record.view.Status = JobSucceeded
	m.appendEventLocked(record, "job", "Completed")
}

func (m *JobManager) finishCancelled(record *jobRecord) {
	m.mu.Lock()
	defer m.mu.Unlock()
	completed := time.Now().UTC()
	record.view.Status = JobCancelled
	record.view.CompletedAt = &completed
	record.view.Error = "job cancelled"
	m.appendEventLocked(record, "job", "Cancelled")
}

func (m *JobManager) appendEventLocked(record *jobRecord, stage, message string) {
	sequence := int64(len(record.view.Events) + 1)
	record.view.Events = append(record.view.Events, JobEvent{
		Sequence: sequence,
		Time:     time.Now().UTC(),
		Stage:    stage,
		Message:  message,
	})
}

func (m *JobManager) pruneLocked(max int) {
	for len(m.jobs) > max {
		var oldestID string
		var oldest time.Time
		for id, record := range m.jobs {
			if record.view.Status == JobQueued || record.view.Status == JobRunning {
				continue
			}
			if oldestID == "" || record.view.CreatedAt.Before(oldest) {
				oldestID = id
				oldest = record.view.CreatedAt
			}
		}
		if oldestID == "" {
			return
		}
		delete(m.jobs, oldestID)
		for key, id := range m.idempotency {
			if id == oldestID {
				delete(m.idempotency, key)
			}
		}
	}
}

func cloneJob(job Job, after int64) Job {
	job.Events = eventsAfter(job.Events, after)
	return job
}

func eventsAfter(events []JobEvent, after int64) []JobEvent {
	result := make([]JobEvent, 0)
	for _, event := range events {
		if event.Sequence > after {
			result = append(result, event)
		}
	}
	return result
}

func newJobID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", ErrJobIDFailure
	}
	return hex.EncodeToString(value), nil
}
