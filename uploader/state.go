package uploader

import (
	"encoding/json"
	"os"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type UploadState struct {
	UploadID       string                `json:"upload_id"`
	Bucket         string                `json:"bucket"`
	Key            string                `json:"key"`
	FilePath       string                `json:"file_path"`
	PartSize       int64                 `json:"part_size"`
	CompletedParts []types.CompletedPart `json:"completed_parts"`
}

func stateFilePath(videoPath string) string {
	return videoPath + ".upload-state.json"
}

func loadState(videoPath string) (*UploadState, error) {
	data, err := os.ReadFile(stateFilePath(videoPath))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var s UploadState
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func saveState(s *UploadState) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(stateFilePath(s.FilePath), data, 0644)
}

func deleteState(videoPath string) {
	os.Remove(stateFilePath(videoPath))
}
