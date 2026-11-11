package app

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"log/slog"
	"os"
	"runtime"
	"sync"

	"github.com/syudead/vv/internal/domain"
)

// ImageSearchIndex reads the search index under an audience and loads the listed videos it names.
type ImageSearchIndex interface {
	ImageSearchSnapshot(ctx context.Context, audience domain.Audience) (domain.ImageSearchSnapshot, error)
	VideosByIDs(ctx context.Context, audience domain.Audience, ids []int64) ([]domain.Video, error)
}

// SearchUploadArea keeps a video query on disk for ffprobe and ffmpeg only while its request lasts.
type SearchUploadArea interface {
	Create() (*os.File, error)
	Remove(path string) error
}

// ImageSearchMedia hashes queries by the search index's rule. Duration and Frames wrap
// domain.ErrImageSearchUnreadable for a file they cannot read, and end their run when ctx is cancelled.
type ImageSearchMedia struct {
	HashImage func(image.Image) domain.FrameHash
	Duration  func(ctx context.Context, path string) (int64, error)
	Frames    func(ctx context.Context, path string, maxFrames int) ([]domain.FrameHash, error)
}

type ImageSearchOptions struct {
	Index   ImageSearchIndex
	Uploads SearchUploadArea
	Media   ImageSearchMedia
	// ServerDone ends every running search when the server stops, since shutdown does not cancel requests.
	ServerDone <-chan struct{}
	// Nil Logger uses slog's default.
	Logger *slog.Logger
}

// ImageSearch finds the listed videos that contain the footage of one uploaded image or video and keeps nothing it
// was given. See docs/design-docs/image-search.md.
type ImageSearch struct {
	index      ImageSearchIndex
	uploads    SearchUploadArea
	media      ImageSearchMedia
	serverDone <-chan struct{}
	logger     *slog.Logger
	// gate runs one search at a time: each reads the whole index and may decode a long video.
	gate uploadGate
}

func NewImageSearch(opts ImageSearchOptions) *ImageSearch {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	serverDone := opts.ServerDone
	if serverDone == nil {
		serverDone = make(chan struct{})
	}
	return &ImageSearch{
		index: opts.Index, uploads: opts.Uploads, media: opts.Media, serverDone: serverDone, logger: logger,
		gate: newUploadGate(),
	}
}

// Search reads file to its end, then returns onlyFile's error as is so the caller can refuse a request holding more
// than the file; a video stays in the upload area until Search returns, and each search waits for the one before it.
func (s *ImageSearch) Search(
	ctx context.Context, audience domain.Audience, file io.Reader, onlyFile func() error,
) (domain.ImageSearchResult, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		select {
		case <-s.serverDone:
			cancel()
		case <-ctx.Done():
		}
	}()
	head := bufio.NewReader(contextReader{ctx: ctx, r: file})
	// A short file returns fewer bytes with io.EOF, which the readers below see again.
	sniff, err := head.Peek(16)
	if err != nil && !errors.Is(err, io.EOF) {
		return domain.ImageSearchResult{}, fmt.Errorf("cannot receive the file: %w", err)
	}
	if stillImage(sniff) {
		data, err := receiveImage(head)
		if err != nil {
			return domain.ImageSearchResult{}, err
		}
		return s.run(ctx, audience, onlyFile, func(context.Context) (preparedQuery, error) { return s.imageQuery(data) })
	}
	path, err := s.receiveVideo(head)
	if path != "" {
		defer func() {
			if err := s.uploads.Remove(path); err != nil {
				s.logger.Warn("could not delete a search upload", slog.Any("error", err))
			}
		}()
	}
	if err != nil {
		return domain.ImageSearchResult{}, err
	}
	return s.run(ctx, audience, onlyFile, func(ctx context.Context) (preparedQuery, error) {
		return s.videoQuery(ctx, path)
	})
}

// stillImage reports a PNG, JPEG or WebP signature; animated WebP is refused later by the upload decoder.
func stillImage(head []byte) bool {
	return bytes.HasPrefix(head, []byte("\x89PNG\r\n\x1a\n")) || bytes.HasPrefix(head, []byte("\xff\xd8\xff")) ||
		(len(head) >= 12 && bytes.Equal(head[:4], []byte("RIFF")) && bytes.Equal(head[8:12], []byte("WEBP")))
}

// contextReader stops receiving the file once the search ends, so a cancelled upload is removed while the sender
// still sends.
type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (c contextReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}

// preparedQuery is a hashed query: the result's query fields and how it matches the snapshot's contents.
type preparedQuery struct {
	result domain.ImageSearchResult
	match  func(context.Context, []domain.ImageSearchContent) ([]domain.ImageSearchMatch, error)
}

// run refuses a request holding more than the file, then prepares and matches the query under the one-search gate.
func (s *ImageSearch) run(
	ctx context.Context, audience domain.Audience, onlyFile func() error,
	prepare func(context.Context) (preparedQuery, error),
) (domain.ImageSearchResult, error) {
	if err := onlyFile(); err != nil {
		return domain.ImageSearchResult{}, err
	}
	release, err := s.gate.acquire(ctx)
	if err != nil {
		return domain.ImageSearchResult{}, err
	}
	defer release()
	query, err := prepare(ctx)
	if err != nil {
		return domain.ImageSearchResult{}, err
	}
	snapshot, err := s.index.ImageSearchSnapshot(ctx, audience)
	if err != nil {
		return domain.ImageSearchResult{}, err
	}
	matches, err := query.match(ctx, contentsOf(snapshot))
	if err != nil {
		return domain.ImageSearchResult{}, err
	}
	return s.result(ctx, audience, query.result, snapshot, matches)
}

func receiveImage(file io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(file, domain.ImageSearchImageMaxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("cannot receive the file: %w", err)
	}
	if len(data) > domain.ImageSearchImageMaxBytes {
		return nil, &domain.ImageSearchTooLargeError{Limit: domain.ImageSearchImageMaxBytes}
	}
	return data, nil
}

// imageQuery decodes under the thumbnail upload's limits; the caller holds the gate, which bounds decoded memory.
func (s *ImageSearch) imageQuery(data []byte) (preparedQuery, error) {
	decoded, err := decodeUpload(data)
	var tooLarge *domain.ThumbnailUploadTooLargeError
	switch {
	case errors.As(err, &tooLarge):
		return preparedQuery{}, &domain.ImageSearchTooLargeError{Limit: int64(tooLarge.Limit)}
	case err != nil:
		return preparedQuery{}, errors.Join(domain.ErrImageSearchUnreadable, err)
	}
	hash := s.media.HashImage(decoded)
	if hash.Flat {
		return preparedQuery{}, domain.ErrImageSearchUninformative
	}
	return preparedQuery{
		result: domain.ImageSearchResult{Kind: domain.ImageSearchQueryImage},
		match: func(_ context.Context, contents []domain.ImageSearchContent) ([]domain.ImageSearchMatch, error) {
			// Near misses are not refined from the source file yet; only keyframe matches are listed.
			matches, _ := domain.MatchImageQuery(hash.Hash, contents)
			return matches, nil
		},
	}, nil
}

// receiveVideo writes the file to the upload area and returns its path, set whenever a file was created, even with
// an error.
func (s *ImageSearch) receiveVideo(file io.Reader) (string, error) {
	upload, err := s.uploads.Create()
	if err != nil {
		return "", err
	}
	n, copyErr := io.Copy(upload, io.LimitReader(file, domain.ImageSearchVideoMaxBytes+1))
	closeErr := upload.Close()
	switch {
	case copyErr != nil:
		return upload.Name(), fmt.Errorf("cannot receive the file: %w", copyErr)
	case n > domain.ImageSearchVideoMaxBytes:
		return upload.Name(), &domain.ImageSearchTooLargeError{Limit: domain.ImageSearchVideoMaxBytes}
	case closeErr != nil:
		return upload.Name(), fmt.Errorf("cannot save the file: %w", closeErr)
	}
	return upload.Name(), nil
}

// videoQuery samples the query once a second after checking its duration.
func (s *ImageSearch) videoQuery(ctx context.Context, path string) (preparedQuery, error) {
	durationMs, err := s.media.Duration(ctx, path)
	switch {
	case err != nil:
		return preparedQuery{}, err
	case durationMs < domain.ImageSearchVideoMinMs:
		return preparedQuery{}, fmt.Errorf("%w: the video is shorter than a second",
			domain.ErrImageSearchUnreadable)
	case durationMs > domain.ImageSearchVideoMaxMs:
		return preparedQuery{}, &domain.ImageSearchTooLongError{LimitMs: domain.ImageSearchVideoMaxMs}
	}
	frames, err := s.media.Frames(ctx, path, domain.ImageSearchQueryMaxFrames)
	if err != nil {
		return preparedQuery{}, err
	}
	informative := 0
	for _, frame := range frames {
		if !frame.Flat {
			informative++
		}
	}
	if informative < domain.FingerprintMinComparableFrames {
		return preparedQuery{}, domain.ErrImageSearchUninformative
	}
	query := domain.ImageSearchVideoQuery{
		DurationMs: durationMs, IntervalMs: domain.ImageSearchQueryIntervalMs, Frames: frames,
	}
	return preparedQuery{
		result: domain.ImageSearchResult{Kind: domain.ImageSearchQueryVideo, DurationMs: durationMs},
		match: func(ctx context.Context, contents []domain.ImageSearchContent) ([]domain.ImageSearchMatch, error) {
			return matchVideoQuery(ctx, query, contents)
		},
	}, nil
}

func contentsOf(snapshot domain.ImageSearchSnapshot) []domain.ImageSearchContent {
	contents := make([]domain.ImageSearchContent, len(snapshot.Candidates))
	for i, candidate := range snapshot.Candidates {
		contents[i] = candidate.Content
	}
	return contents
}

// matchVideoQuery spreads the contents over the CPUs, each content's match being independent of the others, and
// stops between contents once ctx ends so a cancelled search releases the gate.
func matchVideoQuery(
	ctx context.Context, query domain.ImageSearchVideoQuery, contents []domain.ImageSearchContent,
) ([]domain.ImageSearchMatch, error) {
	matcher := domain.NewVideoQueryMatcher(query)
	found := make([]*domain.ImageSearchMatch, len(contents))
	workers := min(runtime.GOMAXPROCS(0), max(len(contents), 1))
	var wg sync.WaitGroup
	for w := range workers {
		wg.Go(func() {
			for i := w; i < len(contents) && ctx.Err() == nil; i += workers {
				if match, ok := matcher.Match(contents[i]); ok {
					found[i] = &match
				}
			}
		})
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var matches []domain.ImageSearchMatch
	for _, match := range found {
		if match != nil {
			matches = append(matches, *match)
		}
	}
	return matches, nil
}

// result keeps each listed video's best match, so a bundle's members collapse to its representative while the
// match keeps the member it was found in, then ranks them and loads the listed videos.
func (s *ImageSearch) result(
	ctx context.Context, audience domain.Audience, result domain.ImageSearchResult, snapshot domain.ImageSearchSnapshot,
	matches []domain.ImageSearchMatch,
) (domain.ImageSearchResult, error) {
	listedOf := make(map[int64]int64, len(snapshot.Candidates))
	for _, candidate := range snapshot.Candidates {
		listedOf[candidate.Content.VideoID] = candidate.ListedID
	}
	best := map[int64]domain.ImageSearchMatch{}
	for _, match := range matches {
		listed := listedOf[match.VideoID]
		if kept, ok := best[listed]; !ok || domain.CompareImageSearchMatches(match, kept) < 0 {
			best[listed] = match
		}
	}
	collapsed := make([]domain.ImageSearchMatch, 0, len(best))
	listedOfMatch := make(map[int64]int64, len(best))
	for listed, match := range best {
		collapsed = append(collapsed, match)
		listedOfMatch[match.VideoID] = listed
	}
	ranked := domain.RankImageSearchMatches(collapsed)
	ids := make([]int64, len(ranked))
	for i, match := range ranked {
		ids[i] = listedOfMatch[match.VideoID]
	}
	videos, err := s.index.VideosByIDs(ctx, audience, ids)
	if err != nil {
		return domain.ImageSearchResult{}, err
	}
	byID := make(map[int64]domain.Video, len(videos))
	for _, video := range videos {
		byID[video.ID] = video
	}
	result.Items = []domain.ImageSearchItem{}
	for i, match := range ranked {
		// A video removed or hidden since the snapshot is left out.
		if video, ok := byID[ids[i]]; ok {
			result.Items = append(result.Items, domain.ImageSearchItem{Video: video, Match: match})
		}
	}
	result.UnindexedCount = snapshot.UnindexedCount
	return result, nil
}
