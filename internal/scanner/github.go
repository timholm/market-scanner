package scanner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// GitHubResult represents a competing repository found on GitHub.
type GitHubResult struct {
	FullName    string `json:"full_name"`
	Description string `json:"description"`
	Stars       int    `json:"stars"`
	Forks       int    `json:"forks"`
	Language    string `json:"language"`
	URL         string `json:"url"`
	UpdatedAt   string `json:"updated_at"`
	Topics      []string `json:"topics"`
}

// GitHubSearchResponse is the API response from GitHub search.
type GitHubSearchResponse struct {
	TotalCount int              `json:"total_count"`
	Items      []GitHubRepoItem `json:"items"`
}

// GitHubRepoItem is a single item from GitHub search results.
type GitHubRepoItem struct {
	FullName    string   `json:"full_name"`
	Description string   `json:"description"`
	Stars       int      `json:"stargazers_count"`
	Forks       int      `json:"forks_count"`
	Language    string   `json:"language"`
	HTMLURL     string   `json:"html_url"`
	UpdatedAt   string   `json:"updated_at"`
	Topics      []string `json:"topics"`
}

// SearchGitHub queries the GitHub search API for repositories matching the
// product name and problem description. Only returns repos with > minStars.
func SearchGitHub(ctx context.Context, token, name, problem string, minStars int) ([]GitHubResult, error) {
	queries := buildGitHubQueries(name, problem, minStars)

	seen := make(map[string]bool)
	var results []GitHubResult

	client := &http.Client{Timeout: 15 * time.Second}

	for _, q := range queries {
		items, err := doGitHubSearch(ctx, client, token, q)
		if err != nil {
			// Log but don't fail the whole scan on a single query error.
			continue
		}
		for _, item := range items {
			if seen[item.FullName] {
				continue
			}
			if item.Stars < minStars {
				continue
			}
			seen[item.FullName] = true
			results = append(results, GitHubResult{
				FullName:    item.FullName,
				Description: item.Description,
				Stars:       item.Stars,
				Forks:       item.Forks,
				Language:    item.Language,
				URL:         item.HTMLURL,
				UpdatedAt:   item.UpdatedAt,
				Topics:      item.Topics,
			})
		}
	}

	return results, nil
}

func buildGitHubQueries(name, problem string, minStars int) []string {
	starsFilter := fmt.Sprintf(" stars:>=%d", minStars)
	var queries []string

	// Direct name search
	queries = append(queries, name+starsFilter)

	// Name in topic
	queries = append(queries, fmt.Sprintf("topic:%s%s", strings.ReplaceAll(name, " ", "-"), starsFilter))

	// Keywords from problem description (first 3 significant words)
	keywords := extractKeywords(problem, 3)
	if len(keywords) > 0 {
		queries = append(queries, strings.Join(keywords, " ")+starsFilter)
	}

	return queries
}

func doGitHubSearch(ctx context.Context, client *http.Client, token, query string) ([]GitHubRepoItem, error) {
	u := fmt.Sprintf("https://api.github.com/search/repositories?q=%s&sort=stars&order=desc&per_page=20",
		url.QueryEscape(query))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github search: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("github search %d: %s", resp.StatusCode, string(body))
	}

	var sr GitHubSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return nil, fmt.Errorf("decode github response: %w", err)
	}

	return sr.Items, nil
}
