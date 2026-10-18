package main

import (
	"errors"
	"fmt"
	"io"

	"github.com/syudead/vv/internal/artifacts"
)

// 640px JPEG thumbnails normally stay well below this classification input limit.
const maxThumbnailBytes = 8 << 20

type videoImageSource struct {
	artifacts *artifacts.Store
}

func (t videoImageSource) ThumbnailJPEG(contentKey string) ([]byte, error) {
	file, err := t.artifacts.ThumbnailFile(contentKey)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, maxThumbnailBytes+1))
	if err != nil {
		return nil, fmt.Errorf("cannot read the thumbnail: %w", err)
	}
	if len(data) > maxThumbnailBytes {
		return nil, errors.New("the thumbnail is too large")
	}
	return data, nil
}

// SeekFramesJPEG selects n frames evenly across the video.
func (t videoImageSource) SeekFramesJPEG(contentKey string, n int) ([][]byte, error) {
	return t.artifacts.SeekFrames(contentKey, n)
}
