package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/mamtech/uploadvideo/uploader"
)

func main() {
	file := flag.String("file", "", "Path to the video file (required)")
	bucket := flag.String("bucket", "", "S3 bucket name (required)")
	key := flag.String("key", "", "S3 object key (required)")
	region := flag.String("region", "", "AWS region (overrides AWS_REGION env var)")
	partSizeMB := flag.Int("part-size-mb", 8, "Size of each upload part in MB (min 5)")
	maxRetries := flag.Int("retries", 5, "Max retries per part on network error")
	flag.Parse()

	if *file == "" || *bucket == "" || *key == "" {
		fmt.Fprintln(os.Stderr, "Error: -file, -bucket, and -key are required")
		flag.Usage()
		os.Exit(1)
	}

	if *partSizeMB < 5 {
		fmt.Fprintln(os.Stderr, "Error: -part-size-mb must be at least 5 (AWS minimum)")
		os.Exit(1)
	}

	var opts []func(*config.LoadOptions) error
	if *region != "" {
		opts = append(opts, config.WithRegion(*region))
	}

	awsCfg, err := config.LoadDefaultConfig(context.Background(), opts...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load AWS config: %v\n", err)
		os.Exit(1)
	}

	u := uploader.New(awsCfg, uploader.Config{
		Bucket:     *bucket,
		Key:        *key,
		PartSizeMB: *partSizeMB,
		MaxRetries: *maxRetries,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// On SIGINT/SIGTERM: cancel the context so in-flight part retries stop,
	// but do NOT abort the multipart upload — state file is preserved so the
	// next run can resume.  Pass --abort flag explicitly if you want cleanup.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		fmt.Println("\nInterrupted — progress saved. Re-run the same command to resume.")
		cancel()
	}()

	if err := u.Upload(ctx, *file); err != nil {
		fmt.Fprintf(os.Stderr, "Upload failed: %v\n", err)
		os.Exit(1)
	}
}
