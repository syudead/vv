package media

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"

	"github.com/syudead/vv/internal/domain"
)

// faceFrameFilter samples at domain.FaceSampleFPS and fits each frame into a domain.FaceFrameSide square, padded
// with black at the right or bottom. ffmpeg applies the display rotation before the filters, so a phone video
// arrives upright and is fitted by its displayed size, never stretched.
var faceFrameFilter = func() string {
	side := strconv.Itoa(domain.FaceFrameSide)
	return "fps=" + strconv.FormatFloat(domain.FaceSampleFPS, 'f', -1, 64) +
		",scale=" + side + ":" + side + ":force_original_aspect_ratio=decrease,pad=" + side + ":" + side
}()

const faceFramesStderrLimit = 4 * 1024

// FaceFrameBytes is the size of one frame FaceFrames passes: a domain.FaceFrameSide square of packed BGR.
const FaceFrameBytes = domain.FaceFrameSide * domain.FaceFrameSide * 3

// FaceFrames decodes the video's first video stream that is not a cover picture and calls each with every
// sampled frame in order. pix is reused after each returns. An error from each stops ffmpeg and is returned as
// is; a cancelled ctx returns ctx.Err().
func FaceFrames(ctx context.Context, videoPath string, each func(pix []byte) error) error {
	run, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := command(run, PriorityLow, thumbnailCommand, faceFrameArgs(videoPath)...)
	// A damaged video can make ffmpeg report an error for every frame.
	stderr := &tailWriter{limit: faceFramesStderrLimit}
	cmd.Stderr = stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("cannot run %s (%s): %w", thumbnailCommand, videoPath, err)
	}
	readErr := readFaceFrames(bufio.NewReaderSize(out, 1<<20), each)
	if readErr != nil {
		// Stop ffmpeg when reading ended early, so Wait does not block on a full pipe.
		cancel()
	}
	waitErr := cmd.Wait()
	if stopped, ok := errors.AsType[*faceFrameStop](readErr); ok {
		return stopped.err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if waitErr != nil {
		return fmt.Errorf("%s failed (%s): %s", thumbnailCommand, videoPath, firstLine([]byte(stderr.String())))
	}
	return readErr
}

// faceFrameStop carries an error returned by the caller's callback.
type faceFrameStop struct{ err error }

func (s *faceFrameStop) Error() string { return s.err.Error() }

// readFaceFrames passes every whole frame to each; a stream that ends inside a frame is an error.
func readFaceFrames(r io.Reader, each func(pix []byte) error) error {
	pix := make([]byte, FaceFrameBytes)
	for {
		_, err := io.ReadFull(r, pix)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("cannot read a face analysis frame: %w", err)
		}
		if err := each(pix); err != nil {
			return &faceFrameStop{err: err}
		}
	}
}

func faceFrameArgs(videoPath string) []string {
	return []string{
		"-nostdin",
		"-v", "error",
		"-i", videoPath,
		"-map", "0:V:0",
		"-an", "-sn",
		"-vf", faceFrameFilter,
		"-f", "rawvideo",
		"-pix_fmt", "bgr24",
		"pipe:1",
	}
}
