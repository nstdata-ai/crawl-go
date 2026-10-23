package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	crawl "github.com/nstdata-ai/crawl-go"
)

var errOutput = errors.New("content output failed")

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	client := crawl.NewClient(os.Getenv("NSTDATA_API_TOKEN"))
	if err := run(ctx, client, os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, client *crawl.Client, args []string, out io.Writer) error {
	if len(args) != 2 {
		return errors.New("usage: example scrape|submit-scrape|submit-crawl URL, or resume-scrape|resume-crawl TASK_ID")
	}
	switch args[0] {
	case "scrape":
		result, err := client.SubmitScrapeTaskSync(ctx, &crawl.ScrapeRequestDto{URL: args[1], Formats: []string{crawl.FormatMarkdown}, OnlyMainContent: true})
		if err != nil {
			return err
		}
		return printContent(ctx, result, out)
	case "submit-scrape", "submit-crawl":
		var submitted *crawl.SubmitResponseDto
		var err error
		if args[0] == "submit-scrape" {
			submitted, err = client.SubmitScrapeTask(ctx, &crawl.ScrapeRequestDto{URL: args[1], Formats: []string{crawl.FormatMarkdown}, OnlyMainContent: true})
		} else {
			submitted, err = client.SubmitCrawlTask(ctx, &crawl.CrawlRequestDto{URL: args[1], Formats: []string{crawl.FormatMarkdown}, OnlyMainContent: true, MaxPages: 2, MaxDepth: 1})
		}
		if err != nil {
			return fmt.Errorf("submission failed or outcome is unknown; do not blindly resubmit: %w", err)
		}
		if submitted.Id == "" {
			return errors.New("submission returned no task ID; preserve the response and investigate before resubmitting")
		}
		_, err = fmt.Fprintln(out, "Save this task ID before polling:", submitted.Id)
		return err
	case "resume-scrape":
		for attempt := 0; attempt < 20; attempt++ {
			result, err := client.GetScrapeTaskResult(ctx, args[1])
			if err != nil {
				return err
			}
			switch result.Status {
			case crawl.StatusCompleted, crawl.StatusFailed, crawl.StatusExpired:
				return printContent(ctx, result, out)
			case crawl.StatusProcessing, crawl.StatusScraping:
			default:
				return errors.New("unknown task status; stop and retain the task ID")
			}
			if attempt < 19 {
				if err := wait(ctx); err != nil {
					return err
				}
			}
		}
		return errors.New("poll limit reached; keep the task ID for later continuation")
	case "resume-crawl":
		for attempt := 0; attempt < 20; attempt++ {
			status, err := client.GetCrawlStatus(ctx, args[1])
			if err != nil {
				return err
			}
			switch status.Status {
			case crawl.StatusCompleted, crawl.StatusFailed, crawl.StatusExpired:
				if _, err := fmt.Fprintf(out, "status=%s completed=%d failed=%d pending=%d\n", status.Status, status.Completed, status.Failed, status.Pending); err != nil {
					return err
				}
				err = printPages(ctx, client, args[1], out)
				if err != nil {
					return err
				}
				if status.Status != crawl.StatusCompleted || status.Failed > 0 {
					return errors.New("crawl ended with failures; retain the task ID and partial results")
				}
				return nil
			case crawl.StatusProcessing, crawl.StatusScraping:
			default:
				return errors.New("unknown crawl status; stop and retain the task ID")
			}
			if attempt < 19 {
				if err := wait(ctx); err != nil {
					return err
				}
			}
		}
		return errors.New("poll limit reached; keep the crawl ID for later continuation")
	default:
		return errors.New("unknown command; no request submitted")
	}
}

func wait(ctx context.Context) error {
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func printContent(ctx context.Context, result *crawl.TaskResultResponseDto, out io.Writer) error {
	if result == nil || result.Status != crawl.StatusCompleted || !result.Success || result.Data == nil {
		return errors.New("task did not return successful completed content")
	}
	content, err := result.Data.GetMarkdown(ctx)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(out, content); err != nil {
		return errors.Join(errOutput, err)
	}
	if result.Data.Truncated != nil && *result.Data.Truncated {
		return errors.New("content is marked truncated; retain this partial result")
	}
	return nil
}

func printPages(ctx context.Context, client *crawl.Client, id string, out io.Writer) error {
	cursor := ""
	seen := map[string]bool{}
	failed := false
	for batch := 0; batch < 100; batch++ {
		pages, err := client.ListCrawlPages(ctx, id, crawl.ListCrawlPagesOptions{AfterCursor: cursor, Limit: 20})
		if err != nil {
			return err
		}
		for _, page := range pages.Data {
			if err := printContent(ctx, &crawl.TaskResultResponseDto{Status: page.Status, Success: page.Success, Data: page.Data}, out); err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, errOutput) {
					return err
				}
				failed = true
				if _, writeErr := fmt.Fprintln(out, "Page unavailable, failed or partial; retain its task record."); writeErr != nil {
					return writeErr
				}
			}
		}
		if pages.NextCursor == "" {
			if failed {
				return errors.New("one or more pages failed or were incomplete")
			}
			return nil
		}
		if seen[pages.NextCursor] {
			return errors.New("repeated cursor; stop and keep the crawl ID")
		}
		seen[pages.NextCursor] = true
		cursor = pages.NextCursor
	}
	return errors.New("pagination limit reached; keep the crawl ID and last cursor")
}
