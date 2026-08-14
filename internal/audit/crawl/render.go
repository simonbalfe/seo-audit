package crawl

import (
	"context"
	"errors"
	"os/exec"
	"time"

	"github.com/simonbalfe/seo-audit/internal/audit/browser"
)

func renderHTML(ctx context.Context, target string) (string, error) {
	binary := browser.Binary()
	if binary == "" {
		return "", errors.New("Chrome or Chromium not found")
	}
	renderContext, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	command := exec.CommandContext(
		renderContext,
		binary,
		"--headless=new",
		"--disable-gpu",
		"--disable-extensions",
		"--no-first-run",
		"--no-default-browser-check",
		"--virtual-time-budget=5000",
		"--dump-dom",
		target,
	)
	output, err := command.Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}
