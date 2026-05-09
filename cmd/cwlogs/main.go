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
	utc := flag.Bool("utc", false, "interpret start/end timestamps as UTC instead of the local timezone")
	profile := flag.String("profile", "", "AWS shared config profile name (default: SDK default resolution, including AWS_PROFILE)")
	region := flag.String("region", "", "AWS region (overrides profile/env default)")
	noNormalizeNewlines := flag.Bool("no-normalize-newlines", false, "disable output normalization (converting \\r\\n and \\r to \\n, and appending a trailing \\n when missing)")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(),
			"Usage: %s [--utc] [--profile <name>] [--region <region>] [--no-normalize-newlines] <log_group_name> <start> <end>\n"+
				"  <start>, <end>: YYYYMMDD | YYYY-MM-DD | YYYY-MM-DDTHH:MM:SS\n",
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
				msg = ensureTrailingNewline(msg)
			}
			fmt.Print(msg)
		}

		if logEventsOutput.NextToken == nil {
			break
		}

		nextToken = logEventsOutput.NextToken
	}

}

// granularity records how precisely the user specified a time, which determines
// how endOfGranularity rounds the end of the range up.
type granularity int

const (
	granularityDay granularity = iota
	granularitySecond
)

// parseFlexibleTime parses s in loc as one of:
//   - YYYYMMDD              (8 chars,  granularityDay)
//   - YYYY-MM-DD            (10 chars, granularityDay)
//   - YYYY-MM-DDTHH:MM:SS   (19 chars, granularitySecond)
//
// Trailing zone designators (Z, +09:00, ...) are not accepted; the global --utc
// flag controls how zoneless input is interpreted. The returned granularity is
// only meaningful when err is nil.
func parseFlexibleTime(s string, loc *time.Location) (time.Time, granularity, error) {
	switch len(s) {
	case 8:
		t, err := time.ParseInLocation("20060102", s, loc)
		return t, granularityDay, err
	case 10:
		t, err := time.ParseInLocation("2006-01-02", s, loc)
		return t, granularityDay, err
	case 19:
		t, err := time.ParseInLocation("2006-01-02T15:04:05", s, loc)
		return t, granularitySecond, err
	default:
		return time.Time{}, 0, fmt.Errorf("unrecognized format %q (expected YYYYMMDD, YYYY-MM-DD, or YYYY-MM-DDTHH:MM:SS)", s)
	}
}

// endOfGranularity returns the first instant *after* the granule t belongs to,
// using wall-clock semantics in loc so DST transitions do not skew the result.
// Subtract 1 ms from the returned UnixMilli value to get the inclusive last
// millisecond of the granule.
func endOfGranularity(t time.Time, g granularity, loc *time.Location) time.Time {
	switch g {
	case granularityDay:
		return time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, loc)
	case granularitySecond:
		return t.Add(time.Second)
	default:
		// Reaching here means a new granularity was added without updating
		// this switch, which would silently produce a range that ends before
		// it begins. Fail loudly instead.
		panic(fmt.Sprintf("endOfGranularity: unhandled granularity %d", g))
	}
}

// computeTimeRange parses startStr and endStr in loc and returns the inclusive
// [start, end] range in Unix milliseconds. start is the first millisecond of
// its granule; end is the last millisecond of its granule. The two arguments
// may use different formats (e.g. day for start, second for end).
func computeTimeRange(startStr, endStr string, loc *time.Location) (int64, int64, error) {
	start, _, err := parseFlexibleTime(startStr, loc)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid start: %w", err)
	}

	end, endGran, err := parseFlexibleTime(endStr, loc)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid end: %w", err)
	}

	startMs := start.UnixMilli()
	endMs := endOfGranularity(end, endGran, loc).UnixMilli() - 1
	if startMs > endMs {
		return 0, 0, fmt.Errorf("start (%s) is after end (%s)", startStr, endStr)
	}
	return startMs, endMs, nil
}

// normalizeNewlines converts \r\n and standalone \r in s to \n. The order
// matters: replacing \r first would turn \r\n into \n\n.
func normalizeNewlines(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return s
}

// ensureTrailingNewline appends "\n" to s if it doesn't already end with one.
// CloudWatch Logs does not guarantee that Message ends with a newline, so this
// keeps each event on its own line in the CLI output. An empty input becomes
// "\n" by design: an empty event is preserved as a visible blank line rather
// than swallowed silently, which matches how it would render in the AWS console.
func ensureTrailingNewline(s string) string {
	if strings.HasSuffix(s, "\n") {
		return s
	}
	return s + "\n"
}
