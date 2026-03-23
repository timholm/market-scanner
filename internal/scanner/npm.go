package scanner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// NpmResult represents a competing package found on npm.
type NpmResult struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Version     string   `json:"version"`
	Keywords    []string `json:"keywords"`
	URL         string   `json:"url"`
	Score       float64  `json:"score"`
}

// NpmSearchResponse is the npm registry search response.
type NpmSearchResponse struct {
	Objects []NpmSearchObject `json:"objects"`
	Total   int               `json:"total"`
}

// NpmSearchObject wraps a single npm search result.
type NpmSearchObject struct {
	Package NpmPackage `json:"package"`
	Score   NpmScore   `json:"score"`
}

// NpmPackage is package metadata from npm search.
type NpmPackage struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Version     string   `json:"version"`
	Keywords    []string `json:"keywords"`
	Links       struct {
		Npm string `json:"npm"`
	} `json:"links"`
}

// NpmScore contains the quality/popularity/maintenance scores.
type NpmScore struct {
	Final  float64 `json:"final"`
	Detail struct {
		Quality     float64 `json:"quality"`
		Popularity  float64 `json:"popularity"`
		Maintenance float64 `json:"maintenance"`
	} `json:"detail"`
}

// SearchNpm queries the npm registry for packages matching the name and problem.
func SearchNpm(ctx context.Context, name, problem string) ([]NpmResult, error) {
	queries := buildNpmQueries(name, problem)

	seen := make(map[string]bool)
	var results []NpmResult

	client := &http.Client{Timeout: 15 * time.Second}

	for _, q := range queries {
		items, err := doNpmSearch(ctx, client, q)
		if err != nil {
			continue
		}
		for _, item := range items {
			if seen[item.Package.Name] {
				continue
			}
			seen[item.Package.Name] = true
			results = append(results, NpmResult{
				Name:        item.Package.Name,
				Description: item.Package.Description,
				Version:     item.Package.Version,
				Keywords:    item.Package.Keywords,
				URL:         item.Package.Links.Npm,
				Score:       item.Score.Final,
			})
		}
	}

	return results, nil
}

func buildNpmQueries(name, problem string) []string {
	var queries []string
	queries = append(queries, name)

	keywords := extractKeywords(problem, 3)
	if len(keywords) > 0 {
		for _, kw := range keywords {
			queries = append(queries, kw)
		}
	}

	return queries
}

func doNpmSearch(ctx context.Context, client *http.Client, query string) ([]NpmSearchObject, error) {
	u := fmt.Sprintf("https://registry.npmjs.org/-/v1/search?text=%s&size=20",
		url.QueryEscape(query))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("npm search: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("npm search %d: %s", resp.StatusCode, string(body))
	}

	var sr NpmSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return nil, fmt.Errorf("decode npm response: %w", err)
	}

	return sr.Objects, nil
}
