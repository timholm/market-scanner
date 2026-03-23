package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/timholm/market-scanner/internal/api"
	"github.com/timholm/market-scanner/internal/config"
	"github.com/timholm/market-scanner/internal/db"
	"github.com/timholm/market-scanner/internal/scanner"
)

func main() {
	cfg := config.Load()

	rootCmd := &cobra.Command{
		Use:   "market-scanner",
		Short: "Pre-build competitor scanner for the Claude Code Factory",
		Long: `market-scanner searches GitHub, npm, and PyPI for existing solutions
before the factory builds a product. Prevents wasted effort on already-solved problems.`,
	}

	scanCmd := &cobra.Command{
		Use:   "scan",
		Short: "Scan for competitors of a product",
		RunE: func(cmd *cobra.Command, args []string) error {
			name, _ := cmd.Flags().GetString("name")
			problem, _ := cmd.Flags().GetString("problem")
			jsonOut, _ := cmd.Flags().GetBool("json")

			if name == "" {
				return fmt.Errorf("--name is required")
			}

			sc := scanner.New(cfg.GitHubToken, cfg.NoveltyThreshold)
			result, err := sc.Scan(context.Background(), name, problem)
			if err != nil {
				return fmt.Errorf("scan failed: %w", err)
			}

			// Save to DB if available.
			database, dbErr := db.Open(cfg.DBPath)
			if dbErr == nil {
				defer database.Close()
				_ = database.SaveScan(name, problem, result.NoveltyScore, result.Recommendation, result)
			}

			if jsonOut {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(result)
			}

			fmt.Print(scanner.FormatReport(result))

			// Exit with code 1 if novelty is below threshold (useful for CI).
			if result.NoveltyScore < cfg.NoveltyThreshold {
				os.Exit(1)
			}
			return nil
		},
	}
	scanCmd.Flags().StringP("name", "n", "", "Product name to scan for")
	scanCmd.Flags().StringP("problem", "p", "", "Problem description the product solves")
	scanCmd.Flags().Bool("json", false, "Output as JSON")

	scanQueueCmd := &cobra.Command{
		Use:   "scan-queue",
		Short: "Scan all pending items in the build queue",
		RunE: func(cmd *cobra.Command, args []string) error {
			database, err := db.Open(cfg.DBPath)
			if err != nil {
				return fmt.Errorf("open db: %w", err)
			}
			defer database.Close()

			items, err := database.PendingQueue()
			if err != nil {
				return fmt.Errorf("fetch queue: %w", err)
			}

			if len(items) == 0 {
				fmt.Println("No pending items in build queue.")
				return nil
			}

			sc := scanner.New(cfg.GitHubToken, cfg.NoveltyThreshold)
			skipped, proceeded := 0, 0

			for _, item := range items {
				result, err := sc.Scan(context.Background(), item.Name, item.Problem)
				if err != nil {
					fmt.Fprintf(os.Stderr, "ERROR scanning %s: %v\n", item.Name, err)
					continue
				}

				_ = database.SaveScan(item.Name, item.Problem, result.NoveltyScore, result.Recommendation, result)

				status := "scanned_proceed"
				if result.NoveltyScore < cfg.NoveltyThreshold {
					status = "scanned_skip"
					skipped++
				} else {
					proceeded++
				}
				_ = database.MarkScanned(item.ID, status)

				fmt.Println(scanner.FormatReportCompact(result))
			}

			fmt.Printf("\nQueue scan complete: %d proceed, %d skip, %d total\n",
				proceeded, skipped, len(items))
			return nil
		},
	}

	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the HTTP API server",
		RunE: func(cmd *cobra.Command, args []string) error {
			database, err := db.Open(cfg.DBPath)
			if err != nil {
				return fmt.Errorf("open db: %w", err)
			}
			defer database.Close()

			sc := scanner.New(cfg.GitHubToken, cfg.NoveltyThreshold)
			srv := api.New(cfg, database, sc)

			fmt.Printf("market-scanner API listening on %s\n", cfg.ListenAddr)
			return srv.ListenAndServe()
		},
	}

	rootCmd.AddCommand(scanCmd, scanQueueCmd, serveCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
