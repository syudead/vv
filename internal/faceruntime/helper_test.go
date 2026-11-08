package faceruntime

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/faceproto"
)

// TestHelperRestartsAHelperThatExitedMidRequest: the first helper greets, reads one request and exits without an
// answer; the request fails, and the next one starts a second helper that answers.
func TestHelperRestartsAHelperThatExitedMidRequest(t *testing.T) {
	hello := faceproto.Hello{Version: faceproto.Version, Recognizer: "sface_int8", Dim: 1}
	answer := faceproto.Detections{Faces: []faceproto.Detection{{W: 9, Vector: []float32{1}}}}
	launches := 0
	h := &Helper{
		recognizer: domain.FaceRecognizerSFaceInt8,
		launch: func(HelperCommand) (Process, error) {
			launches++
			exitMidRequest := launches == 1
			return pipeProcess(func(in io.Reader, out io.Writer) {
				if faceproto.Write(out, hello) != nil {
					return
				}
				if _, err := faceproto.Read(in); err != nil || exitMidRequest {
					return
				}
				_ = faceproto.Write(out, answer)
			}), nil
		},
	}
	t.Cleanup(func() { _ = h.Close() })
	frame := faceproto.DetectRequest{Width: 1, Height: 1, Pix: []byte{1, 2, 3}}

	if _, err := h.Detect(context.Background(), frame); !errors.Is(err, ErrHelperExited) {
		t.Fatalf("first Detect error = %v, want ErrHelperExited", err)
	}
	got, err := h.Detect(context.Background(), frame)
	if err != nil {
		t.Fatal(err)
	}
	if launches != 2 || len(got.Faces) != 1 || got.Faces[0].W != 9 {
		t.Fatalf("launches = %d, Detect = %+v; want 2 launches and the second helper's answer", launches, got)
	}
}

// A start that fails before the process exists still closes the parent's pipe ends.
func TestExecLauncherClosesPipesWhenStartFails(t *testing.T) {
	failed := errors.New("start failed")
	var cmd *exec.Cmd
	launch := ExecLauncher(context.Background(), func(ctx context.Context, name string, args ...string) (*exec.Cmd, func() error) {
		cmd = exec.CommandContext(ctx, name, args...)
		return cmd, func() error { return failed }
	})
	if _, err := launch(HelperCommand{Path: "helper"}); !errors.Is(err, failed) {
		t.Fatalf("launch error = %v, want the start error", err)
	}
	child := cmd.Stdout.(*os.File)
	t.Cleanup(func() { _ = child.Close() })
	if _, err := child.Write([]byte{1}); err == nil {
		t.Error("the helper's stdout accepted a write, so the parent end is still open")
	}
}
