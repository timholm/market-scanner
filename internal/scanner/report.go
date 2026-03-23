package scanner

import (
	"fmt"
	"strings"
)

// FormatReport generates a human-readable competition report from a scan result.
func FormatReport(r *ScanResult) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("=== Market Scan Report: %s ===\n", r.Name))
	b.WriteString(fmt.Sprintf("Problem: %s\n", r.Problem))
	b.WriteString(fmt.Sprintf("Scan Duration: %s\n\n", r.ScanDuration.Round(100*1e6)))

	// Novelty verdict
	b.WriteString(fmt.Sprintf("NOVELTY SCORE: %.2f / 1.00\n", r.NoveltyScore))
	b.WriteString(fmt.Sprintf("RECOMMENDATION: %s\n\n", r.Recommendation))

	// GitHub results
	b.WriteString(fmt.Sprintf("--- GitHub (%d repos with 100+ stars) ---\n", len(r.GitHub)))
	if len(r.GitHub) == 0 {
		b.WriteString("  No significant competitors found.\n")
	}
	for i, gh := range r.GitHub {
		if i >= 10 {
			b.WriteString(fmt.Sprintf("  ... and %d more\n", len(r.GitHub)-10))
			break
		}
		b.WriteString(fmt.Sprintf("  [%d] %s (%d stars, %d forks)\n", i+1, gh.FullName, gh.Stars, gh.Forks))
		if gh.Description != "" {
			b.WriteString(fmt.Sprintf("      %s\n", truncate(gh.Description, 100)))
		}
		b.WriteString(fmt.Sprintf("      %s\n", gh.URL))
	}
	b.WriteString("\n")

	// npm results
	b.WriteString(fmt.Sprintf("--- npm (%d packages) ---\n", len(r.Npm)))
	if len(r.Npm) == 0 {
		b.WriteString("  No competing packages found.\n")
	}
	for i, pkg := range r.Npm {
		if i >= 10 {
			b.WriteString(fmt.Sprintf("  ... and %d more\n", len(r.Npm)-10))
			break
		}
		b.WriteString(fmt.Sprintf("  [%d] %s@%s (score: %.2f)\n", i+1, pkg.Name, pkg.Version, pkg.Score))
		if pkg.Description != "" {
			b.WriteString(fmt.Sprintf("      %s\n", truncate(pkg.Description, 100)))
		}
	}
	b.WriteString("\n")

	// PyPI results
	b.WriteString(fmt.Sprintf("--- PyPI (%d packages) ---\n", len(r.PyPI)))
	if len(r.PyPI) == 0 {
		b.WriteString("  No competing packages found.\n")
	}
	for i, pkg := range r.PyPI {
		if i >= 10 {
			b.WriteString(fmt.Sprintf("  ... and %d more\n", len(r.PyPI)-10))
			break
		}
		b.WriteString(fmt.Sprintf("  [%d] %s", i+1, pkg.Name))
		if pkg.Version != "" {
			b.WriteString(fmt.Sprintf("@%s", pkg.Version))
		}
		b.WriteString("\n")
		if pkg.Description != "" {
			b.WriteString(fmt.Sprintf("      %s\n", truncate(pkg.Description, 100)))
		}
	}
	b.WriteString("\n")

	b.WriteString("=== End Report ===\n")
	return b.String()
}

// FormatReportCompact returns a single-line summary for queue scanning.
func FormatReportCompact(r *ScanResult) string {
	return fmt.Sprintf("[%.2f] %s — %s (gh:%d npm:%d pypi:%d)",
		r.NoveltyScore, r.Name, r.Recommendation,
		len(r.GitHub), len(r.Npm), len(r.PyPI))
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
