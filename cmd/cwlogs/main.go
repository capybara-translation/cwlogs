package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
)

const (
	formatRaw      = "raw"
	formatWithTime = "with-time"
	formatJSONL    = "jsonl"
)

// version is overridden at build time via `-ldflags "-X main.version=..."`
// (set by GoReleaser). When that override is absent, init() consults
// debug.ReadBuildInfo to pick up the module version embedded by
// `go install ...@vX.Y.Z`. Pseudo versions (commit-hash based, "+dirty", etc.)
// and "(devel)" are deliberately rejected so a local build never surfaces a
// version string that looks like a real release.
var version = "dev"

func init() {
	info, _ := debug.ReadBuildInfo()
	version = resolveVersion(version, info)
}

// resolveVersion picks the effective version string from either the ldflags
// override (ldVersion) or the build info embedded by Go modules. It is split
// out from init() so it can be tested without rebuilding with custom ldflags.
//
// Pseudo versions starting with "v0.0.0-" (Go's auto-generated commit-hash
// based versions, e.g. when installing from a non-tagged commit or a dirty
// tree) are intentionally treated as "dev": surfacing them as if they were
// releases makes bug reports ambiguous about which exact build is running.
func resolveVersion(ldVersion string, info *debug.BuildInfo) string {
	if ldVersion != "dev" {
		return ldVersion
	}
	if info == nil {
		return "dev"
	}
	v := info.Main.Version
	if v == "" || v == "(devel)" || strings.HasPrefix(v, "v0.0.0-") {
		return "dev"
	}
	return v
}

func main() {
	utc := flag.Bool("utc", false, "interpret start/end timestamps as UTC instead of the local timezone")
	profile := flag.String("profile", "", "AWS shared config profile name (default: SDK default resolution, including AWS_PROFILE)")
	region := flag.String("region", "", "AWS region (overrides profile/env default)")
	noNormalizeNewlines := flag.Bool("no-normalize-newlines", false, "disable output normalization (converting \\r\\n and \\r to \\n, and collapsing any trailing run of \\n to a single \\n)")
	format := flag.String("format", formatRaw, "output format: raw | with-time | jsonl")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(),
			"Usage: %s [--utc] [--profile <name>] [--region <region>] [--no-normalize-newlines] [--format <format>] <log_group_name> <start> <end>\n"+
				"  <start>, <end>: YYYYMMDD | YYYY-MM-DD | YYYY-MM-DDTHH:MM:SS\n"+
				"  <format>:       raw (default) | with-time | jsonl\n",
			os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	if *showVersion {
		fmt.Println("cwlogs " + version)
		return
	}

	if flag.NArg() != 3 {
		fmt.Fprintf(flag.CommandLine.Output(), "error: expected 3 positional arguments, got %d\n", flag.NArg())
		flag.Usage()
		os.Exit(2)
	}

	switch *format {
	case formatRaw, formatWithTime, formatJSONL:
	default:
		fmt.Fprintf(flag.CommandLine.Output(), "error: unknown --format value %q (expected raw, with-time, or jsonl)\n", *format)
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
			out, err := formatEvent(aws.ToInt64(logEvent.Timestamp), msg, aws.ToString(logEvent.LogStreamName), *format, loc)
			if err != nil {
				log.Fatal(err)
			}
			fmt.Print(out)
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

// ensureTrailingNewline normalizes the trailing newline run of s so the result
// always ends with exactly one "\n":
//
//   - "foo"        -> "foo\n"
//   - "foo\n"      -> "foo\n"
//   - "foo\n\n\n"  -> "foo\n"  (collapses trailing run, common in build logs
//                                that embed "\n\n" before the next section)
//   - ""           -> "\n"     (blank events stay visible as a blank line)
//   - "\n\n"       -> "\n"
//
// This keeps each CloudWatch event on exactly one line in the CLI output
// while preserving blank events so timestamps and ordering are not lost.
func ensureTrailingNewline(s string) string {
	return strings.TrimRight(s, "\n") + "\n"
}

// formatTimestamp renders a CloudWatch Logs timestamp (Unix milliseconds) as
// ISO 8601 with millisecond precision in loc. The "Z07:00" trailer collapses
// to "Z" for UTC and expands to "+09:00" / "-05:00" / etc. for other zones.
func formatTimestamp(ms int64, loc *time.Location) string {
	return time.UnixMilli(ms).In(loc).Format("2006-01-02T15:04:05.000Z07:00")
}

// formatEvent renders a single log event in the requested output format.
// The message argument is expected to be already-normalized (or raw, if the
// caller chose to skip normalization). Unknown format values return an error
// rather than silently falling back, so the CLI surface stays in sync with
// what main accepts.
func formatEvent(timestampMs int64, message, stream, format string, loc *time.Location) (string, error) {
	switch format {
	case formatRaw:
		return message, nil
	case formatWithTime:
		// The line must terminate with \n so a blank-message event (which
		// carries only a timestamp) still occupies its own line.
		line := formatTimestamp(timestampMs, loc) + "\t" + message
		if !strings.HasSuffix(line, "\n") {
			line += "\n"
		}
		return line, nil
	case formatJSONL:
		// SetEscapeHTML(false) keeps "<", ">", "&" readable in log payloads.
		// json.Marshal would otherwise escape them to < etc.
		var buf strings.Builder
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(struct {
			Timestamp string `json:"timestamp"`
			Stream    string `json:"stream"`
			Message   string `json:"message"`
		}{
			Timestamp: formatTimestamp(timestampMs, loc),
			Stream:    stream,
			Message:   message,
		}); err != nil {
			return "", fmt.Errorf("encode jsonl event: %w", err)
		}
		// json.Encoder always appends a newline, so the result is one event
		// per line as advertised.
		return buf.String(), nil
	default:
		// main pre-validates the --format value, so this branch is unreachable
		// in normal CLI use. It exists for direct callers (tests, future
		// embedding) that bypass the CLI entry point.
		return "", fmt.Errorf("unknown format %q", format)
	}
}
