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

// PyPIResult represents a competing package found on PyPI.
type PyPIResult struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Version     string `json:"version"`
	URL         string `json:"url"`
}

// PyPISearchResponse wraps the PyPI JSON API search response.
// Note: PyPI doesn't have an official search API with JSON output,
// so we use the warehouse JSON endpoint for individual packages and
// the simple search endpoint for discovery.
type PyPISearchResponse struct {
	Info struct {
		Name        string `json:"name"`
		Summary     string `json:"summary"`
		Version     string `json:"version"`
		ProjectURL  string `json:"project_url"`
		PackageURL  string `json:"package_url"`
		HomePage    string `json:"home_page"`
		Description string `json:"description"`
	} `json:"info"`
}

// PyPISearchResult from the warehouse search API.
type PyPISearchResult struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Summary string `json:"summary"`
}

// SearchPyPI queries PyPI for packages matching the name and problem.
// We use a two-pronged approach:
//  1. Direct package lookup by name
//  2. Search via the warehouse JSON API
func SearchPyPI(ctx context.Context, name, problem string) ([]PyPIResult, error) {
	seen := make(map[string]bool)
	var results []PyPIResult

	client := &http.Client{Timeout: 15 * time.Second}

	// Try direct package name lookups with variations.
	nameVariations := pypiNameVariations(name)
	for _, n := range nameVariations {
		r, err := lookupPyPIPackage(ctx, client, n)
		if err != nil {
			continue
		}
		if seen[r.Name] {
			continue
		}
		seen[r.Name] = true
		results = append(results, *r)
	}

	// Search via the XML-RPC alternative: use the simple JSON search
	// that's available on warehouse.
	queries := buildPyPIQueries(name, problem)
	for _, q := range queries {
		items, err := searchPyPIWarehouse(ctx, client, q)
		if err != nil {
			continue
		}
		for _, item := range items {
			if seen[item.Name] {
				continue
			}
			seen[item.Name] = true
			results = append(results, item)
		}
	}

	return results, nil
}

func pypiNameVariations(name string) []string {
	base := strings.ToLower(name)
	return []string{
		base,
		strings.ReplaceAll(base, "-", "_"),
		strings.ReplaceAll(base, "_", "-"),
		strings.ReplaceAll(base, " ", "-"),
		strings.ReplaceAll(base, " ", "_"),
		"py" + base,
		"python-" + strings.ReplaceAll(base, " ", "-"),
	}
}

func lookupPyPIPackage(ctx context.Context, client *http.Client, name string) (*PyPIResult, error) {
	u := fmt.Sprintf("https://pypi.org/pypi/%s/json", url.PathEscape(name))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pypi lookup %d", resp.StatusCode)
	}

	var sr PyPISearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return nil, err
	}

	return &PyPIResult{
		Name:        sr.Info.Name,
		Description: sr.Info.Summary,
		Version:     sr.Info.Version,
		URL:         fmt.Sprintf("https://pypi.org/project/%s/", sr.Info.Name),
	}, nil
}

func buildPyPIQueries(name, problem string) []string {
	var queries []string
	queries = append(queries, name)
	kw := extractKeywords(problem, 3)
	if len(kw) > 0 {
		queries = append(queries, strings.Join(kw, "+"))
	}
	return queries
}

// searchPyPIWarehouse uses the PyPI warehouse search page and parses results.
// Since PyPI doesn't have a JSON search API, we use a best-effort approach
// hitting the JSON API for known package patterns.
func searchPyPIWarehouse(ctx context.Context, client *http.Client, query string) ([]PyPIResult, error) {
	// PyPI's search is HTML-only. We'll use a heuristic: search for the
	// query terms as package names directly. For a production system you'd
	// want to scrape or use a third-party index.
	u := fmt.Sprintf("https://pypi.org/search/?q=%s&o=", url.QueryEscape(query))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/html")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pypi search: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pypi search %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return nil, err
	}

	return parsePyPISearchHTML(string(body)), nil
}

// parsePyPISearchHTML does minimal HTML parsing to extract package names
// from PyPI search results. This is intentionally simple and resilient
// to HTML changes -- we just look for the pattern in snippet anchors.
func parsePyPISearchHTML(html string) []PyPIResult {
	var results []PyPIResult

	// Look for: <a class="package-snippet" href="/project/NAME/">
	const marker = `href="/project/`
	idx := 0
	for {
		pos := strings.Index(html[idx:], marker)
		if pos < 0 {
			break
		}
		idx += pos + len(marker)
		end := strings.Index(html[idx:], `/`)
		if end < 0 {
			break
		}
		name := html[idx : idx+end]
		if name == "" {
			idx += end
			continue
		}

		// Try to extract description from the next <p class="package-snippet__description"> element
		desc := ""
		descMarker := `package-snippet__description`
		descPos := strings.Index(html[idx:], descMarker)
		if descPos >= 0 && descPos < 2000 {
			descStart := idx + descPos + len(descMarker)
			gtPos := strings.Index(html[descStart:], ">")
			if gtPos >= 0 {
				descStart += gtPos + 1
				ltPos := strings.Index(html[descStart:], "<")
				if ltPos >= 0 {
					desc = strings.TrimSpace(html[descStart : descStart+ltPos])
				}
			}
		}

		results = append(results, PyPIResult{
			Name:        name,
			Description: desc,
			URL:         fmt.Sprintf("https://pypi.org/project/%s/", name),
		})

		idx += end
		if len(results) >= 20 {
			break
		}
	}

	return results
}
