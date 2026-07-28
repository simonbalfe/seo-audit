package audit

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"time"
)

func renderHTML(ctx context.Context, target string) (string, error) {
	binary := chromeBinary()
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

func chromeBinary() string {
	candidates := []string{
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"/Applications/Chromium.app/Contents/MacOS/Chromium",
		"google-chrome",
		"chromium",
		"chromium-browser",
	}
	for _, candidate := range candidates {
		if candidate[0] == '/' {
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate
			}
			continue
		}
		if resolved, err := exec.LookPath(candidate); err == nil {
			return resolved
		}
	}
	return ""
}
