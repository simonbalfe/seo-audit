package dashboard

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

var errAuditRunning = errors.New("an audit is already running")

type auditRequest struct {
	PlaceID        string   `json:"place_id"`
	Steps          string   `json:"steps"`
	Limit          int      `json:"limit"`
	TimeoutSeconds int      `json:"timeout_seconds"`
	CheckExternal  bool     `json:"check_external"`
	Performance    bool     `json:"performance"`
	Keywords       []string `json:"keywords"`
}

type auditJob struct {
	ID         string   `json:"id,omitempty"`
	Status     string   `json:"status"`
	Stage      string   `json:"stage,omitempty"`
	Steps      string   `json:"steps,omitempty"`
	PlaceID    string   `json:"place_id,omitempty"`
	Logs       []string `json:"logs"`
	OutputPath string   `json:"output_path,omitempty"`
	Error      string   `json:"error,omitempty"`
	StartedAt  string   `json:"started_at,omitempty"`
	FinishedAt string   `json:"finished_at,omitempty"`
}

type auditRunner struct {
	ctx        context.Context
	executable string
	workdir    string
	mu         sync.RWMutex
	job        auditJob
}

func newAuditRunner(ctx context.Context, executable, workdir string) *auditRunner {
	return &auditRunner{
		ctx:        ctx,
		executable: executable,
		workdir:    workdir,
		job:        auditJob{Status: "idle", Logs: []string{}},
	}
}

func (r *auditRunner) Start(request auditRequest) (auditJob, error) {
	request, err := validAuditRequest(request)
	if err != nil {
		return auditJob{}, err
	}
	r.mu.Lock()
	if r.job.Status == "running" {
		r.mu.Unlock()
		return auditJob{}, errAuditRunning
	}
	r.job = auditJob{
		ID:        strconv.FormatInt(time.Now().UnixNano(), 10),
		Status:    "running",
		Stage:     "queued",
		Steps:     request.Steps,
		PlaceID:   request.PlaceID,
		Logs:      []string{},
		StartedAt: time.Now().UTC().Format(time.RFC3339),
	}
	job := cloneJob(r.job)
	r.mu.Unlock()
	go r.run(request)
	return job, nil
}

func (r *auditRunner) Current() auditJob {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return cloneJob(r.job)
}

func (r *auditRunner) run(request auditRequest) {
	args := []string{
		"audit", request.PlaceID,
		"--steps", request.Steps,
		"--limit", strconv.Itoa(request.Limit),
		"--timeout", strconv.Itoa(request.TimeoutSeconds) + "s",
		"--external=" + strconv.FormatBool(request.CheckExternal),
		"--performance=" + strconv.FormatBool(request.Performance),
		"--debug",
	}
	for _, keyword := range request.Keywords {
		args = append(args, "--keyword", keyword)
	}
	command := exec.CommandContext(r.ctx, r.executable, args...)
	command.Dir = r.workdir
	writer := &auditLogWriter{appendLine: r.appendLog}
	command.Stdout = writer
	command.Stderr = writer
	err := command.Run()
	writer.Close()

	r.mu.Lock()
	defer r.mu.Unlock()
	r.job.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	if err != nil {
		r.job.Status = "failed"
		r.job.Stage = "failed"
		r.job.Error = err.Error()
		return
	}
	r.job.Status = "completed"
	r.job.Stage = "done"
}

func (r *auditRunner) appendLog(line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.job.Logs) == 250 {
		copy(r.job.Logs, r.job.Logs[1:])
		r.job.Logs = r.job.Logs[:249]
	}
	r.job.Logs = append(r.job.Logs, line)
	if stage := progressStage(line); stage != "" {
		r.job.Stage = stage
	}
	if outputPath, ok := strings.CutPrefix(line, "Saved JSON report to "); ok {
		r.job.OutputPath = outputPath
	}
}

func validAuditRequest(request auditRequest) (auditRequest, error) {
	request.PlaceID = strings.TrimSpace(request.PlaceID)
	request.Steps = strings.TrimSpace(request.Steps)
	if request.PlaceID == "" || len(request.PlaceID) > 200 {
		return auditRequest{}, errors.New("place_id is required")
	}
	switch request.Steps {
	case "all", "website", "performance", "visibility", "backlinks", "profile":
	default:
		return auditRequest{}, errors.New("steps is invalid")
	}
	if request.Limit == 0 {
		request.Limit = 50
	}
	if request.Limit < 1 || request.Limit > 5000 {
		return auditRequest{}, errors.New("limit must be from 1 to 5000")
	}
	if request.TimeoutSeconds == 0 {
		request.TimeoutSeconds = 30
	}
	if request.TimeoutSeconds < 1 || request.TimeoutSeconds > 120 {
		return auditRequest{}, errors.New("timeout_seconds must be from 1 to 120")
	}
	if len(request.Keywords) > 20 {
		return auditRequest{}, errors.New("no more than 20 keywords are allowed")
	}
	for index, keyword := range request.Keywords {
		request.Keywords[index] = strings.TrimSpace(keyword)
		if request.Keywords[index] == "" || len(request.Keywords[index]) > 100 {
			return auditRequest{}, fmt.Errorf("keyword %d is invalid", index+1)
		}
	}
	return request, nil
}

func progressStage(line string) string {
	if !strings.HasPrefix(line, "[") {
		return ""
	}
	end := strings.IndexByte(line, ']')
	if end < 2 {
		return ""
	}
	stage := line[1:end]
	switch stage {
	case "places":
		return "profile"
	case "setup", "robots", "sitemaps", "crawl", "analysis", "resources":
		return "website"
	case "market":
		return "visibility"
	case "performance", "visibility", "backlinks", "done":
		return stage
	default:
		return ""
	}
}

func cloneJob(job auditJob) auditJob {
	job.Logs = append([]string{}, job.Logs...)
	return job
}

type auditLogWriter struct {
	mu         sync.Mutex
	buffer     bytes.Buffer
	appendLine func(string)
}

func (w *auditLogWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	written, err := w.buffer.Write(data)
	for {
		line, readErr := w.buffer.ReadString('\n')
		if errors.Is(readErr, io.EOF) {
			w.buffer.WriteString(line)
			break
		}
		if readErr != nil {
			return written, readErr
		}
		w.appendLine(line)
	}
	return written, err
}

func (w *auditLogWriter) Close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.buffer.Len() > 0 {
		w.appendLine(w.buffer.String())
		w.buffer.Reset()
	}
}
