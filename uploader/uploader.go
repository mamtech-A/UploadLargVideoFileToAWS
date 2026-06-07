package uploader

import (
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

const defaultPartSize = 8 * 1024 * 1024 // 8 MB
const defaultMaxRetries = 5

type Config struct {
	Bucket     string
	Key        string
	Region     string
	PartSizeMB int
	MaxRetries int
}

type Uploader struct {
	cfg    Config
	client *s3.Client
}

func New(awsCfg aws.Config, cfg Config) *Uploader {
	if cfg.PartSizeMB <= 0 {
		cfg.PartSizeMB = defaultPartSize / (1024 * 1024)
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = defaultMaxRetries
	}
	return &Uploader{
		cfg:    cfg,
		client: s3.NewFromConfig(awsCfg),
	}
}

func (u *Uploader) Upload(ctx context.Context, filePath string) error {
	partSize := int64(u.cfg.PartSizeMB) * 1024 * 1024

	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat file: %w", err)
	}
	totalSize := fi.Size()
	totalParts := int32(math.Ceil(float64(totalSize) / float64(partSize)))

	// Load or create state
	state, err := loadState(filePath)
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	if state == nil {
		// New upload
		resp, err := u.client.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{
			Bucket: aws.String(u.cfg.Bucket),
			Key:    aws.String(u.cfg.Key),
		})
		if err != nil {
			return fmt.Errorf("create multipart upload: %w", err)
		}
		state = &UploadState{
			UploadID: *resp.UploadId,
			Bucket:   u.cfg.Bucket,
			Key:      u.cfg.Key,
			FilePath: filePath,
			PartSize: partSize,
		}
		if err := saveState(state); err != nil {
			return fmt.Errorf("save state: %w", err)
		}
		fmt.Printf("Started new multipart upload: %s\n", state.UploadID)
	} else {
		fmt.Printf("Resuming upload %s (%d parts already done)\n", state.UploadID, len(state.CompletedParts))
		partSize = state.PartSize
	}

	// Build set of already-uploaded part numbers
	uploaded := make(map[int32]bool)
	for _, p := range state.CompletedParts {
		uploaded[*p.PartNumber] = true
	}

	// Upload missing parts
	for partNum := int32(1); partNum <= totalParts; partNum++ {
		if uploaded[partNum] {
			fmt.Printf("Part %d/%d already uploaded, skipping\n", partNum, totalParts)
			continue
		}

		offset := int64(partNum-1) * partSize
		size := partSize
		if offset+size > totalSize {
			size = totalSize - offset
		}

		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			return fmt.Errorf("seek to part %d: %w", partNum, err)
		}

		buf := make([]byte, size)
		if _, err := io.ReadFull(f, buf); err != nil {
			return fmt.Errorf("read part %d: %w", partNum, err)
		}

		var etag string
		var uploadErr error
		for attempt := 0; attempt <= u.cfg.MaxRetries; attempt++ {
			if attempt > 0 {
				wait := time.Duration(math.Pow(2, float64(attempt-1))) * time.Second
				fmt.Printf("  Retry %d/%d in %s...\n", attempt, u.cfg.MaxRetries, wait)
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(wait):
				}
			}

			result, err := u.client.UploadPart(ctx, &s3.UploadPartInput{
				Bucket:     aws.String(state.Bucket),
				Key:        aws.String(state.Key),
				UploadId:   aws.String(state.UploadID),
				PartNumber: aws.Int32(partNum),
				Body:       newBytesReader(buf),
			})
			if err != nil {
				uploadErr = err
				if ctx.Err() != nil {
					return ctx.Err()
				}
				fmt.Printf("  Part %d failed: %v\n", partNum, err)
				continue
			}
			etag = *result.ETag
			uploadErr = nil
			break
		}
		if uploadErr != nil {
			return fmt.Errorf("upload part %d after %d retries: %w", partNum, u.cfg.MaxRetries, uploadErr)
		}

		state.CompletedParts = append(state.CompletedParts, types.CompletedPart{
			PartNumber: aws.Int32(partNum),
			ETag:       aws.String(etag),
		})
		if err := saveState(state); err != nil {
			return fmt.Errorf("save state after part %d: %w", partNum, err)
		}
		fmt.Printf("Part %d/%d uploaded (%.1f MB)\n", partNum, totalParts, float64(size)/1024/1024)
	}

	// Complete the multipart upload
	_, err = u.client.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket:   aws.String(state.Bucket),
		Key:      aws.String(state.Key),
		UploadId: aws.String(state.UploadID),
		MultipartUpload: &types.CompletedMultipartUpload{
			Parts: state.CompletedParts,
		},
	})
	if err != nil {
		return fmt.Errorf("complete multipart upload: %w", err)
	}

	deleteState(filePath)
	fmt.Printf("Upload complete: s3://%s/%s\n", state.Bucket, state.Key)
	return nil
}

// Abort cleans up the in-progress multipart upload from S3 (call on SIGINT).
func (u *Uploader) Abort(ctx context.Context, filePath string) {
	state, err := loadState(filePath)
	if err != nil || state == nil {
		return
	}
	u.client.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{
		Bucket:   aws.String(state.Bucket),
		Key:      aws.String(state.Key),
		UploadId: aws.String(state.UploadID),
	})
	deleteState(filePath)
	fmt.Println("Multipart upload aborted and cleaned up.")
}
