package scanner

import (
	"testing"
)

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

func TestNameSimilarity(t *testing.T) {
	tests := []struct {
		target   string
		compName string
		compDesc string
		minSim   float64
		maxSim   float64
	}{
		// Exact match should be high
		{"kubectl", "kubernetes/kubectl", "Kubernetes CLI", 0.5, 1.0},
		// Substring match
		{"mesh", "linkerd/linkerd-mesh", "Service mesh", 0.5, 1.0},
		// No relation should be low
		{"quantum-compiler", "facebook/react", "A JavaScript library for building UIs", 0.0, 0.2},
		// Partial word overlap
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

func TestRecommend(t *testing.T) {
	sc := New("", 0.6)

	tests := []struct {
		score    float64
		contains string
	}{
		{0.9, "PROCEED:"},
		{0.7, "PROCEED_WITH_CAUTION:"},
		{0.4, "SKIP:"},
		{0.1, "SKIP:"},
	}

	for _, tt := range tests {
		rec := sc.recommend(tt.score)
		if !containsStr(rec, tt.contains) {
			t.Errorf("recommend(%.1f) = %q, want contains %q", tt.score, rec, tt.contains)
		}
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s[:len(substr)] == substr || containsStr(s[1:], substr))
}
