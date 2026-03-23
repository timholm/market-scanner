package scanner

import (
	"strings"
	"testing"
	"time"
)

func TestFormatReport(t *testing.T) {
	result := &ScanResult{
		Name:         "test-tool",
		Problem:      "Test problem",
		NoveltyScore: 0.85,
		Recommendation: "PROCEED: Highly novel. No significant competition found.",
		ScanDuration: 2 * time.Second,
		GitHub: []GitHubResult{
			{FullName: "user/repo", Stars: 500, Forks: 50, URL: "https://github.com/user/repo", Description: "A test repo"},
		},
		Npm: []NpmResult{
			{Name: "test-pkg", Version: "1.0.0", Score: 0.5, Description: "A test package"},
		},
		PyPI: []PyPIResult{
			{Name: "test-pkg", Version: "1.0.0", Description: "A test package"},
		},
	}

	report := FormatReport(result)

	checks := []string{
		"test-tool",
		"NOVELTY SCORE: 0.85",
		"PROCEED:",
		"user/repo",
		"500 stars",
		"test-pkg@1.0.0",
		"GitHub (1 repos",
		"npm (1 packages",
		"PyPI (1 packages",
	}

	for _, check := range checks {
		if !strings.Contains(report, check) {
			t.Errorf("report missing %q:\n%s", check, report)
		}
	}
}

func TestFormatReportCompact(t *testing.T) {
	result := &ScanResult{
		Name:           "test-tool",
		NoveltyScore:   0.75,
		Recommendation: "PROCEED_WITH_CAUTION",
		GitHub:         make([]GitHubResult, 3),
		Npm:            make([]NpmResult, 5),
		PyPI:           make([]PyPIResult, 2),
	}

	compact := FormatReportCompact(result)
	if !strings.Contains(compact, "0.75") {
		t.Errorf("compact report missing score: %s", compact)
	}
	if !strings.Contains(compact, "gh:3") {
		t.Errorf("compact report missing github count: %s", compact)
	}
	if !strings.Contains(compact, "npm:5") {
		t.Errorf("compact report missing npm count: %s", compact)
	}
}

func TestTruncate(t *testing.T) {
	if truncate("short", 100) != "short" {
		t.Error("truncate should not modify short strings")
	}
	long := "this is a very long string that should be truncated"
	result := truncate(long, 20)
	if len(result) != 20 {
		t.Errorf("truncate length = %d, want 20", len(result))
	}
	if !strings.HasSuffix(result, "...") {
		t.Error("truncate should end with ...")
	}
}
