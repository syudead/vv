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

// videoImageSource は生成物の置き場から代表サムネイルとシーク用プレビューのコマを読み、
// app.VideoImageSource を満たす。
type videoImageSource struct {
	artifacts *artifacts.Store
}

// ThumbnailJPEG は内容の代表サムネイルを読む。
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

// SeekFramesJPEG はシーク用プレビューから動画全体に等間隔に n コマを選んで読む。
func (t videoImageSource) SeekFramesJPEG(contentKey string, n int) ([][]byte, error) {
	return t.artifacts.SeekFrames(contentKey, n)
}
