package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"

	"github.com/syudead/vv/internal/faceinfer"
	"github.com/syudead/vv/internal/faceproto"
)

// engine is the inference serve needs; faceinfer provides it, tests fake it.
type engine interface {
	Detect(img faceinfer.Image) ([]faceinfer.Face, error)
	Embed(img faceinfer.Image, f faceinfer.Face) ([]float32, error)
	EmbedAligned(img faceinfer.Image) ([]float32, error)
}

// serve writes hello, then answers each request until in ends. A request that fails gets an Error
// reply and the helper keeps serving; a broken stream ends it.
func serve(in io.Reader, out *bufio.Writer, e engine, hello faceproto.Hello) error {
	if err := reply(out, hello); err != nil {
		return err
	}
	for {
		req, err := faceproto.Read(in)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		resp, err := answer(e, req)
		if err != nil {
			resp = faceproto.Error{Message: err.Error()}
		}
		if err := reply(out, resp); err != nil {
			return err
		}
	}
}

func answer(e engine, req faceproto.Message) (faceproto.Message, error) {
	switch r := req.(type) {
	case faceproto.DetectRequest:
		return detect(e, faceinfer.Image{W: r.Width, H: r.Height, Pix: r.Pix})
	case faceproto.EmbedRequest:
		v, err := e.EmbedAligned(faceinfer.Image{W: faceproto.AlignedSide, H: faceproto.AlignedSide, Pix: r.Pix})
		return faceproto.Vector{Values: v}, err
	}
	return nil, fmt.Errorf("%T is not a request", req)
}

// detect embeds every detected face; choosing faces is mdm's rule, not the helper's.
func detect(e engine, img faceinfer.Image) (faceproto.Message, error) {
	if img.W == 0 || img.H == 0 {
		return nil, errors.New("empty frame")
	}
	faces, err := e.Detect(img)
	if err != nil {
		return nil, err
	}
	out := faceproto.Detections{Faces: make([]faceproto.Detection, len(faces))}
	for i, f := range faces {
		v, err := e.Embed(img, f)
		if err != nil {
			return nil, err
		}
		d := faceproto.Detection{X: float32(f.X), Y: float32(f.Y), W: float32(f.W), H: float32(f.H), Score: float32(f.Score), Vector: v}
		for k, p := range f.Landmarks {
			d.Landmarks[k] = float32(p)
		}
		out.Faces[i] = d
	}
	return out, nil
}

func reply(out *bufio.Writer, m faceproto.Message) error {
	if err := faceproto.Write(out, m); err != nil {
		return err
	}
	return out.Flush()
}
