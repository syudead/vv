// mdm-face is the face helper that ships only inside the face runtime archive. It loads onnxruntime,
// YuNet and one recognizer, then answers faceproto requests on standard input and output until the
// input closes. See docs/design-docs/face-recognition.md.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/syudead/vv/internal/faceinfer"
	"github.com/syudead/vv/internal/faceproto"
)

// detectorID names the one detector every recognizer is paired with.
const detectorID = "yunet_2023mar"

type options struct {
	ort, detector, recognizer, model string
	threads                          int
}

func main() {
	var o options
	flag.StringVar(&o.ort, "ort", "", "onnxruntime shared library")
	flag.StringVar(&o.detector, "detector", "", "YuNet 2023mar ONNX file")
	flag.StringVar(&o.recognizer, "recognizer", "", "recognizer ONNX file")
	flag.StringVar(&o.model, "model", faceinfer.SFaceInt8.ID, "recognizer id: sface_int8 or arcface_r50")
	flag.IntVar(&o.threads, "threads", 1, "onnxruntime intra-op threads")
	flag.Parse()
	log.SetPrefix("mdm-face: ")
	log.SetFlags(0)

	in := bufio.NewReaderSize(os.Stdin, 1<<20)
	out := bufio.NewWriterSize(os.Stdout, 1<<16)
	e, hello, err := load(o)
	if err != nil {
		// Report the failure on the protocol too, so mdm can show it instead of a bare exit status.
		if werr := reply(out, faceproto.Error{Message: err.Error()}); werr != nil {
			log.Print(werr)
		}
		log.Fatal(err)
	}
	if err := serve(in, out, e, hello); err != nil {
		log.Fatal(err)
	}
}

func load(o options) (*models, faceproto.Hello, error) {
	m, ok := faceinfer.ModelByID(o.model)
	if !ok {
		return nil, faceproto.Hello{}, fmt.Errorf("unknown recognizer %q", o.model)
	}
	rt, err := faceinfer.Load(o.ort)
	if err != nil {
		return nil, faceproto.Hello{}, err
	}
	detModel, err := os.ReadFile(o.detector)
	if err != nil {
		return nil, faceproto.Hello{}, err
	}
	det, err := rt.NewDetector(detModel, o.threads)
	if err != nil {
		return nil, faceproto.Hello{}, err
	}
	recModel, err := os.ReadFile(o.recognizer)
	if err != nil {
		return nil, faceproto.Hello{}, err
	}
	rec, err := rt.NewRecognizer(m, recModel, o.threads)
	if err != nil {
		return nil, faceproto.Hello{}, err
	}
	hello := faceproto.Hello{Version: faceproto.Version, Detector: detectorID, Recognizer: m.ID, Dim: uint16(m.Dim)}
	return &models{Detector: det, Recognizer: rec}, hello, nil
}

// models joins the loaded detector and recognizer into the engine serve uses.
type models struct {
	*faceinfer.Detector
	*faceinfer.Recognizer
}
