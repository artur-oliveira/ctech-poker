// Package main provides the operator-only moderation queue CLI. It is not
// mounted by the HTTP API; AWS credentials and direct DynamoDB access are
// required. Only the explicit show command prints free text or evidence.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"gopkg.aoctech.app/poker/api/internal/reports"
)

type moderationStore interface {
	Get(context.Context, string, string) (*reports.Report, error)
	ListByStatus(context.Context, reports.Status, string, int) (reports.Page, error)
	SetStatus(context.Context, string, string, reports.Status, reports.Resolution, string) error
}

func run(ctx context.Context, args []string, store moderationStore, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: moderation <list|show|review|resolve> [flags]")
	}
	switch args[0] {
	case "list":
		flags := flag.NewFlagSet("list", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		statusValue := flags.String("status", string(reports.StatusOpen), "open, reviewing, or resolved")
		cursor := flags.String("cursor", "", "pagination cursor")
		limit := flags.Int("limit", 50, "1-100")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		status := reports.Status(*statusValue)
		if !reports.ValidStatus(status) {
			return errors.New("invalid status")
		}
		page, err := store.ListByStatus(ctx, status, *cursor, *limit)
		if err != nil {
			return err
		}
		summaries := make([]reports.Summary, 0, len(page.Reports))
		for _, report := range page.Reports {
			summaries = append(summaries, report.Summary())
		}
		return json.NewEncoder(out).Encode(map[string]any{"reports": summaries, "next_cursor": page.NextCursor})
	case "show":
		target, key, _, _, err := mutationFlags(args[1:], "show", false)
		if err != nil {
			return err
		}
		report, err := store.Get(ctx, target, key)
		if err != nil {
			return err
		}
		return json.NewEncoder(out).Encode(report)
	case "review":
		target, key, moderator, _, err := mutationFlags(args[1:], "review", true)
		if err != nil {
			return err
		}
		if err := store.SetStatus(ctx, target, key, reports.StatusReviewing, "", moderator); err != nil {
			return err
		}
		return json.NewEncoder(out).Encode(map[string]string{"status": string(reports.StatusReviewing)})
	case "resolve":
		target, key, moderator, resolution, err := mutationFlags(args[1:], "resolve", true)
		if err != nil {
			return err
		}
		if !reports.ValidResolution(resolution) {
			return errors.New("invalid resolution")
		}
		if err := store.SetStatus(ctx, target, key, reports.StatusResolved, resolution, moderator); err != nil {
			return err
		}
		return json.NewEncoder(out).Encode(map[string]string{"status": string(reports.StatusResolved), "resolution": string(resolution)})
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func mutationFlags(args []string, name string, moderatorRequired bool) (string, string, string, reports.Resolution, error) {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	target := flags.String("target", "", "target player id")
	key := flags.String("key", "", "report storage key from list")
	moderator := flags.String("moderator", "", "operator identity")
	resolution := flags.String("resolution", "", "resolution code")
	if err := flags.Parse(args); err != nil {
		return "", "", "", "", err
	}
	if *target == "" || *key == "" || (moderatorRequired && *moderator == "") {
		return "", "", "", "", errors.New("target, key, and moderator (for mutations) are required")
	}
	return *target, *key, *moderator, reports.Resolution(*resolution), nil
}

func main() {
	env := os.Getenv("ENVIRONMENT")
	if env == "" {
		fmt.Fprintln(os.Stderr, "ENVIRONMENT is required")
		os.Exit(2)
	}
	ctx := context.Background()
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	db := dynamodb.NewFromConfig(cfg, func(options *dynamodb.Options) {
		if endpoint := os.Getenv("DYNAMODB_ENDPOINT"); endpoint != "" {
			options.BaseEndpoint = aws.String(endpoint)
		}
	})
	if err := run(ctx, os.Args[1:], reports.NewStore(db, env), os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "moderation:", err)
		os.Exit(1)
	}
}
