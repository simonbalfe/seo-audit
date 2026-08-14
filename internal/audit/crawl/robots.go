package crawl

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

var auditedAgents = []string{"Googlebot", "Bingbot", "OAI-SearchBot", "GPTBot", "PerplexityBot", "Claude-SearchBot", "ClaudeBot"}

type robotsGroup struct {
	agents []string
	rules  []robotsRule
}

type robotsRule struct {
	allow bool
	path  string
}

func (c *Client) InspectRobots(ctx context.Context, rawURL string) (RobotsReport, error) {
	target, err := normalizeStartURL(rawURL)
	if err != nil {
		return RobotsReport{}, err
	}
	robotsURL := &url.URL{Scheme: target.Scheme, Host: target.Host, Path: "/robots.txt"}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, robotsURL.String(), nil)
	if err != nil {
		return RobotsReport{}, err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	response, err := c.HTTP.Do(req)
	if err != nil {
		return RobotsReport{}, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return RobotsReport{}, err
	}
	report := RobotsReport{URL: robotsURL.String(), StatusCode: response.StatusCode, Body: string(body)}
	groups, sitemaps := parseRobots(string(body))
	report.Sitemaps = sitemaps
	for _, agent := range auditedAgents {
		allowed, rule := evaluateRobots(groups, agent, target.EscapedPath())
		report.Agents = append(report.Agents, AgentAccess{Agent: agent, Allowed: allowed, Rule: rule})
	}
	return report, nil
}

func parseRobots(body string) ([]robotsGroup, []string) {
	var groups []robotsGroup
	var current *robotsGroup
	var sitemaps []string
	scanner := bufio.NewScanner(strings.NewReader(body))
	for scanner.Scan() {
		line := strings.TrimSpace(strings.SplitN(scanner.Text(), "#", 2)[0])
		if line == "" || !strings.Contains(line, ":") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		key := strings.ToLower(strings.TrimSpace(parts[0]))
		value := strings.TrimSpace(parts[1])
		switch key {
		case "user-agent":
			if current == nil || len(current.rules) > 0 {
				groups = append(groups, robotsGroup{})
				current = &groups[len(groups)-1]
			}
			current.agents = append(current.agents, strings.ToLower(value))
		case "allow", "disallow":
			if current != nil && value != "" {
				current.rules = append(current.rules, robotsRule{allow: key == "allow", path: value})
			}
		case "sitemap":
			if value != "" {
				sitemaps = appendUnique(sitemaps, value)
			}
		}
	}
	return groups, sitemaps
}

func evaluateRobots(groups []robotsGroup, agent, path string) (bool, string) {
	if path == "" {
		path = "/"
	}
	agent = strings.ToLower(agent)
	bestAgentLength := -1
	groupMatches := make([]int, len(groups))
	for index, group := range groups {
		groupMatches[index] = -1
		for _, candidate := range group.agents {
			matchLength := -1
			if candidate == "*" {
				matchLength = 0
			} else if strings.Contains(agent, candidate) {
				matchLength = len(candidate)
			}
			if matchLength > groupMatches[index] {
				groupMatches[index] = matchLength
			}
			if matchLength > bestAgentLength {
				bestAgentLength = matchLength
			}
		}
	}
	bestLength := -1
	allowed := true
	ruleText := "no matching rule"
	for index, group := range groups {
		if groupMatches[index] != bestAgentLength {
			continue
		}
		for _, rule := range group.rules {
			prefix := strings.TrimSuffix(rule.path, "$")
			prefix = strings.SplitN(prefix, "*", 2)[0]
			if strings.HasPrefix(path, prefix) && (len(prefix) > bestLength || len(prefix) == bestLength && rule.allow) {
				bestLength = len(prefix)
				allowed = rule.allow
				action := "Disallow"
				if rule.allow {
					action = "Allow"
				}
				ruleText = fmt.Sprintf("%s: %s", action, rule.path)
			}
		}
	}
	return allowed, ruleText
}
