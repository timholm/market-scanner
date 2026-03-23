package scanner

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"
)

// Scanner orchestrates searches across GitHub, npm, and PyPI.
type Scanner struct {
	GitHubToken      string
	NoveltyThreshold float64
	MinGitHubStars   int
}

// New creates a scanner with the given configuration.
func New(githubToken string, noveltyThreshold float64) *Scanner {
	return &Scanner{
		GitHubToken:      githubToken,
		NoveltyThreshold: noveltyThreshold,
		MinGitHubStars:   100,
	}
}

// ScanResult contains the full output of a market scan.
type ScanResult struct {
	Name           string         `json:"name"`
	Problem        string         `json:"problem"`
	GitHub         []GitHubResult `json:"github"`
	Npm            []NpmResult    `json:"npm"`
	PyPI           []PyPIResult   `json:"pypi"`
	NoveltyScore   float64        `json:"novelty_score"`
	Recommendation string         `json:"recommendation"`
	ScanDuration   time.Duration  `json:"scan_duration"`
}

// Scan runs all searches and computes a novelty score.
func (s *Scanner) Scan(ctx context.Context, name, problem string) (*ScanResult, error) {
	start := time.Now()
	result := &ScanResult{
		Name:    name,
		Problem: problem,
	}

	type ghRes struct {
		items []GitHubResult
		err   error
	}
	type npmRes struct {
		items []NpmResult
		err   error
	}
	type pypiRes struct {
		items []PyPIResult
		err   error
	}

	ghCh := make(chan ghRes, 1)
	npmCh := make(chan npmRes, 1)
	pypiCh := make(chan pypiRes, 1)

	// Run all three searches concurrently.
	go func() {
		items, err := SearchGitHub(ctx, s.GitHubToken, name, problem, s.MinGitHubStars)
		ghCh <- ghRes{items, err}
	}()
	go func() {
		items, err := SearchNpm(ctx, name, problem)
		npmCh <- npmRes{items, err}
	}()
	go func() {
		items, err := SearchPyPI(ctx, name, problem)
		pypiCh <- pypiRes{items, err}
	}()

	// Collect results.
	gh := <-ghCh
	npm := <-npmCh
	pypi := <-pypiCh

	if gh.err != nil {
		return nil, fmt.Errorf("github search failed: %w", gh.err)
	}
	result.GitHub = gh.items

	// npm and pypi failures are non-fatal; we just note them.
	if npm.err == nil {
		result.Npm = npm.items
	}
	if pypi.err == nil {
		result.PyPI = pypi.items
	}

	// Compute novelty score.
	result.NoveltyScore = s.computeNovelty(result)
	result.Recommendation = s.recommend(result.NoveltyScore)
	result.ScanDuration = time.Since(start)

	return result, nil
}

// computeNovelty returns a score from 0.0 (no novelty, tons of competitors)
// to 1.0 (completely novel, nothing found).
//
// Scoring factors:
//   - Number of high-star GitHub repos (most impactful)
//   - Name similarity of top GitHub results
//   - Number of npm packages with high relevance scores
//   - Number of PyPI packages found
func (s *Scanner) computeNovelty(r *ScanResult) float64 {
	score := 1.0

	// GitHub impact: each high-star repo reduces novelty significantly.
	if len(r.GitHub) > 0 {
		// Penalty per repo, scaled by star count.
		ghPenalty := 0.0
		for _, gh := range r.GitHub {
			// A repo with 10k+ stars is a massive signal.
			starWeight := math.Min(float64(gh.Stars)/10000.0, 1.0)
			nameSim := nameSimilarity(r.Name, gh.FullName, gh.Description)
			ghPenalty += (0.05 + 0.15*starWeight) * nameSim
		}
		score -= math.Min(ghPenalty, 0.6) // Cap GitHub penalty at 0.6
	}

	// npm impact: each highly-scored package reduces novelty.
	if len(r.Npm) > 0 {
		npmPenalty := 0.0
		for _, pkg := range r.Npm {
			nameSim := nameSimilarity(r.Name, pkg.Name, pkg.Description)
			npmPenalty += 0.03 * pkg.Score * nameSim
		}
		score -= math.Min(npmPenalty, 0.25)
	}

	// PyPI impact.
	if len(r.PyPI) > 0 {
		pypiPenalty := 0.0
		for _, pkg := range r.PyPI {
			nameSim := nameSimilarity(r.Name, pkg.Name, pkg.Description)
			pypiPenalty += 0.02 * nameSim
		}
		score -= math.Min(pypiPenalty, 0.15)
	}

	return math.Max(0.0, math.Min(1.0, score))
}

// recommend returns a human-readable recommendation based on the novelty score.
func (s *Scanner) recommend(novelty float64) string {
	switch {
	case novelty >= 0.8:
		return "PROCEED: Highly novel. No significant competition found."
	case novelty >= s.NoveltyThreshold:
		return "PROCEED_WITH_CAUTION: Some competitors exist but differentiation is possible."
	case novelty >= 0.3:
		return "SKIP: Significant competition exists. Consider a different angle or skip."
	default:
		return "SKIP: Market is saturated. This problem is well-solved."
	}
}

// nameSimilarity returns a value 0-1 indicating how similar a competitor
// name/description is to the target product name.
func nameSimilarity(targetName, competitorName, competitorDesc string) float64 {
	target := strings.ToLower(targetName)
	comp := strings.ToLower(competitorName)
	desc := strings.ToLower(competitorDesc)

	sim := 0.0

	// Exact substring match in name is very strong signal.
	if strings.Contains(comp, target) || strings.Contains(target, comp) {
		sim += 0.8
	}

	// Check word overlap.
	targetWords := strings.Fields(strings.ReplaceAll(target, "-", " "))
	compWords := strings.Fields(strings.ReplaceAll(comp, "-", " "))
	compWords = append(compWords, strings.Fields(strings.ReplaceAll(desc, "-", " "))...)

	compSet := make(map[string]bool)
	for _, w := range compWords {
		compSet[w] = true
	}

	overlap := 0
	for _, w := range targetWords {
		if compSet[w] {
			overlap++
		}
	}
	if len(targetWords) > 0 {
		sim += 0.4 * float64(overlap) / float64(len(targetWords))
	}

	return math.Min(sim, 1.0)
}

// extractKeywords pulls the most significant words from a problem description,
// filtering out common stop words.
func extractKeywords(text string, max int) []string {
	stopWords := map[string]bool{
		"a": true, "an": true, "the": true, "is": true, "are": true,
		"was": true, "were": true, "be": true, "been": true, "being": true,
		"have": true, "has": true, "had": true, "do": true, "does": true,
		"did": true, "will": true, "would": true, "could": true, "should": true,
		"may": true, "might": true, "shall": true, "can": true,
		"for": true, "and": true, "nor": true, "but": true, "or": true,
		"yet": true, "so": true, "in": true, "on": true, "at": true,
		"to": true, "from": true, "by": true, "with": true, "of": true,
		"that": true, "this": true, "it": true, "as": true, "if": true,
		"not": true, "no": true, "all": true, "each": true, "every": true,
		"both": true, "few": true, "more": true, "most": true, "other": true,
		"some": true, "such": true, "than": true, "too": true, "very": true,
		"just": true, "because": true, "when": true, "where": true, "which": true,
		"who": true, "what": true, "how": true, "about": true, "into": true,
		"through": true, "during": true, "before": true, "after": true,
		"above": true, "below": true, "between": true, "under": true,
	}

	words := strings.Fields(strings.ToLower(text))
	var keywords []string
	seen := make(map[string]bool)

	for _, w := range words {
		// Clean punctuation.
		w = strings.Trim(w, ".,;:!?\"'()[]{}/-")
		if len(w) < 3 || stopWords[w] || seen[w] {
			continue
		}
		seen[w] = true
		keywords = append(keywords, w)
		if len(keywords) >= max {
			break
		}
	}

	return keywords
}
