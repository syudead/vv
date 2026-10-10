package main

import (
	"errors"
	"fmt"
	"io"

	"github.com/syudead/vv/internal/artifacts"
)

// maxThumbnailBytes は判定に渡す代表サムネイルの大きさの上限である。代表サムネイルは
// 長辺 640px の JPEG なので、通常はずっと小さい。
const maxThumbnailBytes = 8 << 20

// thumbnailSource は生成物の置き場から代表サムネイルの JPEG を読み、app.ThumbnailSource を満たす。
type thumbnailSource struct {
	artifacts *artifacts.Store
}

// ThumbnailJPEG は内容の代表サムネイルを読む。
func (t thumbnailSource) ThumbnailJPEG(contentKey string) ([]byte, error) {
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
