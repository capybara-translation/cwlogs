package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
)

func main() {
	utc := flag.Bool("utc", false, "interpret start/end dates as UTC instead of the local timezone")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(),
			"Usage: %s [--utc] <log_group_name> <start_date: YYYYMMDD> <end_date: YYYYMMDD> [<aws_profile>]\n",
			os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() < 3 || flag.NArg() > 4 {
		flag.Usage()
		os.Exit(2)
	}

	logGroupName := flag.Arg(0)
	startDateStr := flag.Arg(1)
	endDateStr := flag.Arg(2)
	profile := "default"
	if flag.NArg() == 4 {
		profile = flag.Arg(3)
	}

	loc := time.Local
	if *utc {
		loc = time.UTC
	}

	startTime, endTime, err := computeTimeRange(startDateStr, endDateStr, loc)
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx, config.WithSharedConfigProfile(profile))
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
			fmt.Print(aws.ToString(logEvent.Message))
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
