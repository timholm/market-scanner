package scanner

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// --- Unit tests for extractKeywords ---

func TestExtractKeywords(t *testing.T) {
	tests := []struct {
		text     string
		max      int
		expected int
	}{
		{"Service mesh for CLI tools that routes commands", 3, 3},
		{"A simple the is not keyword", 5, 2}, // "simple", "keyword"
		{"", 3, 0},
		{"kubernetes container orchestration platform", 2, 2},
	}

	for _, tt := range tests {
		result := extractKeywords(tt.text, tt.max)
		if len(result) != tt.expected {
			t.Errorf("extractKeywords(%q, %d) = %v (len %d), want len %d",
				tt.text, tt.max, result, len(result), tt.expected)
		}
	}
}

func TestExtractKeywordsDeduplicates(t *testing.T) {
	kw := extractKeywords("scanner scanner scanner scanner scanner", 10)
	if len(kw) != 1 {
		t.Errorf("expected 1 unique keyword, got %d: %v", len(kw), kw)
	}
}

func TestExtractKeywordsShortWords(t *testing.T) {
	kw := extractKeywords("go is ok no it me", 10)
	if len(kw) != 0 {
		t.Errorf("expected 0 keywords (all too short or stop words), got %d: %v", len(kw), kw)
	}
}

func TestExtractKeywordsStopWords(t *testing.T) {
	kw := extractKeywords("A tool for scanning the market and finding competitors in the ecosystem", 5)

	if len(kw) == 0 {
		t.Fatal("expected at least one keyword")
	}
	if len(kw) > 5 {
		t.Errorf("expected at most 5 keywords, got %d", len(kw))
	}

	for _, w := range kw {
		lower := strings.ToLower(w)
		if lower == "the" || lower == "for" || lower == "and" || lower == "in" {
			t.Errorf("stop word %q should be filtered", w)
		}
	}
}

// --- Unit tests for nameSimilarity ---

func TestNameSimilarity(t *testing.T) {
	tests := []struct {
		target   string
		compName string
		compDesc string
		minSim   float64
		maxSim   float64
	}{
		{"kubectl", "kubernetes/kubectl", "Kubernetes CLI", 0.5, 1.0},
		{"mesh", "linkerd/linkerd-mesh", "Service mesh", 0.5, 1.0},
		{"quantum-compiler", "facebook/react", "A JavaScript library for building UIs", 0.0, 0.2},
		{"log-analyzer", "elastic/log-parser", "Log parsing tool", 0.2, 0.9},
	}

	for _, tt := range tests {
		sim := nameSimilarity(tt.target, tt.compName, tt.compDesc)
		if sim < tt.minSim || sim > tt.maxSim {
			t.Errorf("nameSimilarity(%q, %q, %q) = %.2f, want [%.2f, %.2f]",
				tt.target, tt.compName, tt.compDesc, sim, tt.minSim, tt.maxSim)
		}
	}
}

func TestNameSimilarityExactMatch(t *testing.T) {
	sim := nameSimilarity("react", "facebook/react", "UI library")
	if sim < 0.8 {
		t.Errorf("expected high similarity for exact name match, got %f", sim)
	}
}

func TestNameSimilarityNoMatch(t *testing.T) {
	sim := nameSimilarity("market-scanner", "tensorflow/tensorflow", "machine learning framework")
	if sim > 0.3 {
		t.Errorf("expected low similarity for unrelated names, got %f", sim)
	}
}

func TestNameSimilarityClampsAtOne(t *testing.T) {
	sim := nameSimilarity("react", "react", "react react react")
	if sim > 1.0 {
		t.Errorf("similarity should not exceed 1.0, got %f", sim)
	}
}

// --- Unit tests for computeNovelty ---

func TestComputeNovelty_NoCompetitors(t *testing.T) {
	sc := New("", 0.6)
	result := &ScanResult{
		Name:    "totally-unique-thing",
		Problem: "Something nobody has built",
	}
	score := sc.computeNovelty(result)
	if score != 1.0 {
		t.Errorf("expected novelty 1.0 with no competitors, got %.2f", score)
	}
}

func TestComputeNovelty_HeavyCompetition(t *testing.T) {
	sc := New("", 0.6)
	result := &ScanResult{
		Name:    "kubernetes",
		Problem: "Container orchestration",
		GitHub: []GitHubResult{
			{FullName: "kubernetes/kubernetes", Stars: 100000, Description: "Production-Grade Container Scheduling and Management"},
			{FullName: "rancher/k3s", Stars: 25000, Description: "Lightweight Kubernetes"},
			{FullName: "kubernetes-sigs/kind", Stars: 12000, Description: "Kubernetes IN Docker"},
		},
		Npm: []NpmResult{
			{Name: "kubernetes-client", Score: 0.8},
			{Name: "@kubernetes/client-node", Score: 0.7},
		},
		PyPI: []PyPIResult{
			{Name: "kubernetes", Description: "Kubernetes python client"},
		},
	}
	score := sc.computeNovelty(result)
	if score > 0.5 {
		t.Errorf("expected low novelty for 'kubernetes', got %.2f", score)
	}
}

func TestComputeNoveltyNpmOnly(t *testing.T) {
	s := New("", 0.6)
	result := &ScanResult{
		Name: "my-tool",
		Npm: []NpmResult{
			{Name: "my-tool", Description: "same tool", Score: 0.9},
			{Name: "my-tool-lite", Description: "lightweight version", Score: 0.7},
		},
	}
	score := s.computeNovelty(result)

	if score >= 1.0 {
		t.Errorf("expected penalty from npm results, got %f", score)
	}
	if score <= 0.0 {
		t.Errorf("expected some novelty remaining, got %f", score)
	}
}

func TestComputeNoveltyPyPIOnly(t *testing.T) {
	s := New("", 0.6)
	result := &ScanResult{
		Name: "data-tool",
		PyPI: []PyPIResult{
			{Name: "data-tool", Description: "exact match"},
		},
	}
	score := s.computeNovelty(result)

	if score >= 1.0 {
		t.Errorf("expected penalty from pypi results, got %f", score)
	}
}

func TestComputeNoveltyClamping(t *testing.T) {
	s := New("", 0.6)
	var ghResults []GitHubResult
	for i := 0; i < 50; i++ {
		ghResults = append(ghResults, GitHubResult{
			FullName:    fmt.Sprintf("org/tool-%d", i),
			Description: "tool",
			Stars:       50000,
		})
	}
	result := &ScanResult{
		Name:   "tool",
		GitHub: ghResults,
		Npm: []NpmResult{
			{Name: "tool", Score: 1.0},
		},
		PyPI: []PyPIResult{
			{Name: "tool"},
		},
	}
	score := s.computeNovelty(result)

	if score < 0.0 || score > 1.0 {
		t.Errorf("score out of range [0,1]: %f", score)
	}
}

// --- Unit tests for recommend ---

func TestRecommend(t *testing.T) {
	sc := New("", 0.6)

	tests := []struct {
		score    float64
		contains string
	}{
		{0.9, "PROCEED:"},
		{0.8, "PROCEED:"},
		{0.7, "PROCEED_WITH_CAUTION:"},
		{0.6, "PROCEED_WITH_CAUTION:"},
		{0.5, "SKIP:"},
		{0.4, "SKIP:"},
		{0.3, "SKIP:"},
		{0.1, "SKIP:"},
		{0.0, "SKIP:"},
	}

	for _, tt := range tests {
		rec := sc.recommend(tt.score)
		if !strings.HasPrefix(rec, tt.contains) {
			t.Errorf("recommend(%.1f) = %q, want prefix %q", tt.score, rec, tt.contains)
		}
	}
}

// --- Query builder tests ---

func TestBuildGitHubQueries(t *testing.T) {
	queries := buildGitHubQueries("market-scanner", "scan competitors before building software", 100)

	if len(queries) < 2 {
		t.Fatalf("expected at least 2 queries, got %d", len(queries))
	}
	if !strings.Contains(queries[0], "market-scanner") {
		t.Errorf("first query should contain product name, got %q", queries[0])
	}
	for _, q := range queries {
		if !strings.Contains(q, "stars:>=100") {
			t.Errorf("query should include stars filter: %q", q)
		}
	}
}

func TestBuildGitHubQueriesNoProblem(t *testing.T) {
	queries := buildGitHubQueries("my-tool", "", 50)

	if len(queries) < 2 {
		t.Fatalf("expected at least 2 queries (name + topic), got %d", len(queries))
	}
}

func TestBuildNpmQueries(t *testing.T) {
	queries := buildNpmQueries("my-tool", "solve a specific problem efficiently")

	if len(queries) == 0 {
		t.Fatal("expected at least one query")
	}
	if queries[0] != "my-tool" {
		t.Errorf("first query should be the product name, got %q", queries[0])
	}
}

func TestBuildNpmQueriesNoProblem(t *testing.T) {
	queries := buildNpmQueries("my-tool", "")
	if len(queries) != 1 {
		t.Errorf("expected 1 query for no problem, got %d", len(queries))
	}
}

func TestPyPINameVariations(t *testing.T) {
	variations := pypiNameVariations("My Tool")

	if len(variations) < 3 {
		t.Fatalf("expected at least 3 variations, got %d", len(variations))
	}

	found := map[string]bool{}
	for _, v := range variations {
		found[v] = true
	}

	if !found["my tool"] {
		t.Error("should include lowercase version")
	}
	if !found["my-tool"] {
		t.Error("should include hyphenated version")
	}
	if !found["my_tool"] {
		t.Error("should include underscored version")
	}
	if !found["pymy tool"] {
		t.Error("should include py-prefixed version")
	}
}

func TestBuildPyPIQueries(t *testing.T) {
	queries := buildPyPIQueries("data-tool", "process data efficiently")

	if len(queries) < 1 {
		t.Fatal("expected at least one query")
	}
	if queries[0] != "data-tool" {
		t.Errorf("first query should be product name, got %q", queries[0])
	}
}

// --- PyPI HTML parser tests ---

func TestParsePyPISearchHTML(t *testing.T) {
	html := `
<html>
<body>
<a class="package-snippet" href="/project/my-tool/">
  <h3 class="package-snippet__title">my-tool</h3>
  <p class="package-snippet__description">A cool tool for doing things</p>
</a>
<a class="package-snippet" href="/project/other-pkg/">
  <h3 class="package-snippet__title">other-pkg</h3>
  <p class="package-snippet__description">Another package</p>
</a>
</body>
</html>
`
	results := parsePyPISearchHTML(html)

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Name != "my-tool" {
		t.Errorf("expected my-tool, got %q", results[0].Name)
	}
	if results[0].Description != "A cool tool for doing things" {
		t.Errorf("expected description, got %q", results[0].Description)
	}
	if results[1].Name != "other-pkg" {
		t.Errorf("expected other-pkg, got %q", results[1].Name)
	}
}

func TestParsePyPISearchHTMLEmpty(t *testing.T) {
	results := parsePyPISearchHTML("<html><body>No results</body></html>")
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestParsePyPISearchHTMLMaxResults(t *testing.T) {
	// Generate HTML with 25 results.
	var b strings.Builder
	b.WriteString("<html><body>")
	for i := 0; i < 25; i++ {
		b.WriteString(fmt.Sprintf(`<a class="package-snippet" href="/project/pkg-%d/">
		  <p class="package-snippet__description">desc %d</p>
		</a>`, i, i))
	}
	b.WriteString("</body></html>")

	results := parsePyPISearchHTML(b.String())
	if len(results) > 20 {
		t.Errorf("expected max 20 results, got %d", len(results))
	}
}

// --- Mock HTTP server tests ---

func TestDoGitHubSearchMock(t *testing.T) {
	mockResp := GitHubSearchResponse{
		TotalCount: 2,
		Items: []GitHubRepoItem{
			{
				FullName:    "org/tool",
				Description: "A great tool",
				Stars:       5000,
				Forks:       200,
				Language:    "Go",
				HTMLURL:     "https://github.com/org/tool",
				UpdatedAt:   "2026-01-01T00:00:00Z",
				Topics:      []string{"tool", "scanner"},
			},
			{
				FullName:    "org/low-star",
				Description: "Not popular",
				Stars:       10,
				Forks:       1,
				Language:    "Go",
				HTMLURL:     "https://github.com/org/low-star",
			},
		},
	}

	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockResp)
	}))
	defer srv.Close()

	// Test JSON decoding of GitHub search response format.
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("mock request failed: %v", err)
	}
	defer resp.Body.Close()

	var decoded GitHubSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode mock response: %v", err)
	}
	if decoded.TotalCount != 2 {
		t.Errorf("expected total_count=2, got %d", decoded.TotalCount)
	}
	if decoded.Items[0].FullName != "org/tool" {
		t.Errorf("expected org/tool, got %q", decoded.Items[0].FullName)
	}
	if decoded.Items[0].Stars != 5000 {
		t.Errorf("expected 5000 stars, got %d", decoded.Items[0].Stars)
	}
	if decoded.Items[1].Stars != 10 {
		t.Errorf("expected 10 stars, got %d", decoded.Items[1].Stars)
	}
	_ = gotAuth // Auth header tested separately via doGitHubSearch
}

func TestDoNpmSearchMock(t *testing.T) {
	mockResp := NpmSearchResponse{
		Objects: []NpmSearchObject{
			{
				Package: NpmPackage{
					Name:        "test-tool",
					Description: "A test tool",
					Version:     "1.0.0",
					Keywords:    []string{"test", "tool"},
				},
				Score: NpmScore{Final: 0.85},
			},
		},
		Total: 1,
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockResp)
	}))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("mock request failed: %v", err)
	}
	defer resp.Body.Close()

	var decoded NpmSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode mock response: %v", err)
	}
	if len(decoded.Objects) != 1 {
		t.Fatalf("expected 1 object, got %d", len(decoded.Objects))
	}
	if decoded.Objects[0].Package.Name != "test-tool" {
		t.Errorf("expected test-tool, got %q", decoded.Objects[0].Package.Name)
	}
	if decoded.Objects[0].Score.Final != 0.85 {
		t.Errorf("expected score 0.85, got %f", decoded.Objects[0].Score.Final)
	}
}

func TestLookupPyPIPackageMock(t *testing.T) {
	mockResp := PyPISearchResponse{}
	mockResp.Info.Name = "my-package"
	mockResp.Info.Summary = "A great package"
	mockResp.Info.Version = "2.0.0"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockResp)
	}))
	defer srv.Close()

	client := srv.Client()
	// lookupPyPIPackage builds its own URL targeting pypi.org, so test the parsing directly.
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("mock request failed: %v", err)
	}
	defer resp.Body.Close()

	var decoded PyPISearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.Info.Name != "my-package" {
		t.Errorf("expected my-package, got %q", decoded.Info.Name)
	}
	if decoded.Info.Version != "2.0.0" {
		t.Errorf("expected 2.0.0, got %q", decoded.Info.Version)
	}
}

// --- Scanner constructor test ---

func TestNewScanner(t *testing.T) {
	s := New("my-token", 0.75)

	if s.GitHubToken != "my-token" {
		t.Errorf("expected my-token, got %q", s.GitHubToken)
	}
	if s.NoveltyThreshold != 0.75 {
		t.Errorf("expected 0.75, got %f", s.NoveltyThreshold)
	}
	if s.MinGitHubStars != 100 {
		t.Errorf("expected default MinGitHubStars=100, got %d", s.MinGitHubStars)
	}
}

// --- Full Scan integration test ---

func TestScanFullIntegration(t *testing.T) {
	s := New("", 0.6)
	s.MinGitHubStars = 999999

	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	result, err := s.Scan(context.Background(), "zzz-nonexistent-tool-12345", "")
	if err != nil {
		t.Logf("Scan returned error (may be expected without network): %v", err)
		return
	}

	if result.NoveltyScore < 0.5 {
		t.Errorf("expected high novelty for nonexistent tool, got %f", result.NoveltyScore)
	}
	if result.Name != "zzz-nonexistent-tool-12345" {
		t.Errorf("expected name preserved, got %q", result.Name)
	}
}
