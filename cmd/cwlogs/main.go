package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
)

func main() {
	utc := flag.Bool("utc", false, "interpret start/end dates as UTC instead of the local timezone")
	profile := flag.String("profile", "", "AWS shared config profile name (default: SDK default resolution, including AWS_PROFILE)")
	region := flag.String("region", "", "AWS region (overrides profile/env default)")
	noNormalizeNewlines := flag.Bool("no-normalize-newlines", false, "disable normalization of \\r\\n and \\r in log messages to \\n")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(),
			"Usage: %s [--utc] [--profile <name>] [--region <region>] [--no-normalize-newlines] <log_group_name> <start_date: YYYYMMDD> <end_date: YYYYMMDD>\n",
			os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() != 3 {
		fmt.Fprintf(flag.CommandLine.Output(), "error: expected 3 positional arguments, got %d\n", flag.NArg())
		flag.Usage()
		os.Exit(2)
	}

	logGroupName := flag.Arg(0)
	startDateStr := flag.Arg(1)
	endDateStr := flag.Arg(2)

	loc := time.Local
	if *utc {
		loc = time.UTC
	}

	startTime, endTime, err := computeTimeRange(startDateStr, endDateStr, loc)
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	var opts []func(*config.LoadOptions) error
	if *profile != "" {
		opts = append(opts, config.WithSharedConfigProfile(*profile))
	}
	if *region != "" {
		opts = append(opts, config.WithRegion(*region))
	}
	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		log.Fatal(err)
	}

	client := cloudwatchlogs.NewFromConfig(cfg)
	var nextToken *string
	for {
		logEventInput := &cloudwatchlogs.FilterLogEventsInput{
			LogGroupName: aws.String(logGroupName),
			StartTime:    aws.Int64(startTime),
			EndTime:      aws.Int64(endTime),
			NextToken:    nextToken,
		}

		logEventsOutput, err := client.FilterLogEvents(ctx, logEventInput)
		if err != nil {
			log.Fatal(err)
		}

		for _, logEvent := range logEventsOutput.Events {
			msg := aws.ToString(logEvent.Message)
			if !*noNormalizeNewlines {
				msg = normalizeNewlines(msg)
			}
			fmt.Print(msg)
		}

		if logEventsOutput.NextToken == nil {
			break
		}

		nextToken = logEventsOutput.NextToken
	}

}

// computeTimeRange parses startStr and endStr as YYYYMMDD in loc and returns
// the inclusive [start, end] range in Unix milliseconds. The end time is the
// last millisecond of the end date in loc (wall-clock 23:59:59.999), computed
// via the next day's 00:00:00 to remain correct across DST transitions.
func computeTimeRange(startStr, endStr string, loc *time.Location) (int64, int64, error) {
	const layout = "20060102"

	startDate, err := time.ParseInLocation(layout, startStr, loc)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid start date format: %w", err)
	}

	endDate, err := time.ParseInLocation(layout, endStr, loc)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid end date format: %w", err)
	}

	startMs := startDate.UnixMilli()
	endMs := time.Date(endDate.Year(), endDate.Month(), endDate.Day()+1, 0, 0, 0, 0, loc).UnixMilli() - 1
	return startMs, endMs, nil
}

// normalizeNewlines converts \r\n and standalone \r in s to \n. The order
// matters: replacing \r first would turn \r\n into \n\n.
func normalizeNewlines(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return s
}
