package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
)

func main() {
	if len(os.Args) < 4 || len(os.Args) > 5 {
		log.Fatalf("Usage: %s <log_group_name> <start_date: YYYYMMDD> <end_date: YYYYMMDD> [<aws_profile>]", os.Args[0])
	}
	profile := "default"
	if len(os.Args) == 5 {
		profile = os.Args[4]
	}

	logGroupName := os.Args[1]
	startDateStr := os.Args[2]
	endDateStr := os.Args[3]

	// Define string date format
	const layout = "20060102"

	// Parse date string with the system's local timezone
	startDate, err := time.ParseInLocation(layout, startDateStr, time.Local)
	if err != nil {
		log.Fatalf("Invalid start date format: %v", err)
	}

	endDate, err := time.ParseInLocation(layout, endDateStr, time.Local)
	if err != nil {
		log.Fatalf("Invalid end date format: %v", err)
	}

	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx, config.WithSharedConfigProfile(profile))
	if err != nil {
		log.Fatal(err)
	}

	client := cloudwatchlogs.NewFromConfig(cfg)
	startTime := startDate.UnixMilli()
	// Set endDate at 23:59:59
	endTime := endDate.Add(23*time.Hour + 59*time.Minute + 59*time.Second).UnixMilli()
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
