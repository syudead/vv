// Package faceproto is the protocol between mdm and the face helper mdm-face over the helper's standard
// input and output. Each message is a little-endian uint32 length, a kind byte and the kind's payload.
// The helper first writes Hello (or Error when it cannot load its models), then answers each request
// with one reply. See docs/design-docs/face-recognition.md.
package faceproto

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
)

// Version changes whenever a message's layout or meaning changes; the runtime archive carries the helper
// built for one version.
const Version = 1

// AlignedSide is the side of the aligned face an EmbedRequest carries.
const AlignedSide = 112

// maxMessage bounds one message; a 4K frame is about 25 MB.
const maxMessage = 64 << 20

// ErrVersion reports a Hello from a helper built for another protocol version.
var ErrVersion = errors.New("face helper protocol version mismatch")

// Message is one of the types below.
type Message interface {
	kind() byte
	encode(w *bytes.Buffer)
}

// Hello is the helper's first message: the protocol version and the models it loaded.
type Hello struct {
	Version    uint16
	Detector   string
	Recognizer string
	Dim        uint16
}

// DetectRequest asks for every face in one packed BGR frame and its vector.
type DetectRequest struct {
	Width, Height int
	Pix           []byte
}

// EmbedRequest asks for the vector of one face already aligned to AlignedSide, packed BGR.
type EmbedRequest struct {
	Pix []byte
}

// Detection is one face in frame pixels; Landmarks are the eyes, nose and mouth corners, image-left
// first, x then y.
type Detection struct {
	X, Y, W, H float32
	Landmarks  [10]float32
	Score      float32
	Vector     []float32
}

// Detections answers a DetectRequest.
type Detections struct {
	Faces []Detection
}

// Vector answers an EmbedRequest.
type Vector struct {
	Values []float32
}

// Error answers a request the helper could not serve, or replaces Hello when loading failed.
type Error struct {
	Message string
}

const (
	kindHello byte = iota + 1
	kindDetectRequest
	kindEmbedRequest
	kindDetections
	kindVector
	kindError
)

func (Hello) kind() byte         { return kindHello }
func (DetectRequest) kind() byte { return kindDetectRequest }
func (EmbedRequest) kind() byte  { return kindEmbedRequest }
func (Detections) kind() byte    { return kindDetections }
func (Vector) kind() byte        { return kindVector }
func (Error) kind() byte         { return kindError }

// Write sends one message.
func Write(w io.Writer, m Message) error {
	var body bytes.Buffer
	body.WriteByte(m.kind())
	m.encode(&body)
	if body.Len() > maxMessage {
		return fmt.Errorf("face message of %d bytes exceeds %d", body.Len(), maxMessage)
	}
	var head [4]byte
	binary.LittleEndian.PutUint32(head[:], uint32(body.Len()))
	if _, err := w.Write(head[:]); err != nil {
		return err
	}
	_, err := w.Write(body.Bytes())
	return err
}

// Read receives one message. It returns io.EOF when the stream ends between messages, and an error
// wrapping ErrVersion for a Hello of another protocol version.
func Read(r io.Reader) (Message, error) {
	var head [4]byte
	if _, err := io.ReadFull(r, head[:]); err != nil {
		return nil, err
	}
	n := binary.LittleEndian.Uint32(head[:])
	if n == 0 || n > maxMessage {
		return nil, fmt.Errorf("face message length %d out of range", n)
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, fmt.Errorf("face message: %w", io.ErrUnexpectedEOF)
	}
	d := &decoder{b: body[1:]}
	m, err := decodeBody(body[0], d)
	if err != nil {
		return nil, err
	}
	if d.err == nil && len(d.b) > 0 {
		d.err = fmt.Errorf("%d bytes after the message", len(d.b))
	}
	if d.err != nil {
		return nil, fmt.Errorf("face message kind %d: %w", body[0], d.err)
	}
	if h, ok := m.(Hello); ok && h.Version != Version {
		return nil, fmt.Errorf("%w: helper speaks %d, mdm speaks %d", ErrVersion, h.Version, Version)
	}
	return m, nil
}

func decodeBody(kind byte, d *decoder) (Message, error) {
	switch kind {
	case kindHello:
		return Hello{Version: d.u16(), Detector: d.str(), Recognizer: d.str(), Dim: d.u16()}, nil
	case kindDetectRequest:
		w, h := int(d.u32()), int(d.u32())
		return DetectRequest{Width: w, Height: h, Pix: d.bytes(int64(w) * int64(h) * 3)}, nil
	case kindEmbedRequest:
		return EmbedRequest{Pix: d.bytes(AlignedSide * AlignedSide * 3)}, nil
	case kindDetections:
		return decodeDetections(d), nil
	case kindVector:
		return Vector{Values: d.floats()}, nil
	case kindError:
		return Error{Message: d.str()}, nil
	}
	return nil, fmt.Errorf("unknown face message kind %d", kind)
}

func (m Hello) encode(w *bytes.Buffer) {
	putU16(w, m.Version)
	putStr(w, m.Detector)
	putStr(w, m.Recognizer)
	putU16(w, m.Dim)
}

func (m DetectRequest) encode(w *bytes.Buffer) {
	putU32(w, uint32(m.Width))
	putU32(w, uint32(m.Height))
	w.Write(m.Pix)
}

func (m EmbedRequest) encode(w *bytes.Buffer) { w.Write(m.Pix) }

func (m Detections) encode(w *bytes.Buffer) {
	putU32(w, uint32(len(m.Faces)))
	for _, f := range m.Faces {
		for _, v := range [5]float32{f.X, f.Y, f.W, f.H, f.Score} {
			putF32(w, v)
		}
		for _, v := range f.Landmarks {
			putF32(w, v)
		}
		putFloats(w, f.Vector)
	}
}

func decodeDetections(d *decoder) Detections {
	n := d.u32()
	// Each face takes at least 15 floats and a length, so a count beyond the payload is corrupt.
	if uint64(n)*64 > uint64(len(d.b)) {
		d.fail(fmt.Errorf("%d faces in %d bytes", n, len(d.b)))
		return Detections{}
	}
	faces := make([]Detection, n)
	for i := range faces {
		f := &faces[i]
		f.X, f.Y, f.W, f.H, f.Score = d.f32(), d.f32(), d.f32(), d.f32(), d.f32()
		for k := range f.Landmarks {
			f.Landmarks[k] = d.f32()
		}
		f.Vector = d.floats()
	}
	return Detections{Faces: faces}
}

func (m Vector) encode(w *bytes.Buffer) { putFloats(w, m.Values) }

func (m Error) encode(w *bytes.Buffer) { putStr(w, m.Message) }

func putU16(w *bytes.Buffer, v uint16) { w.Write(binary.LittleEndian.AppendUint16(nil, v)) }
func putU32(w *bytes.Buffer, v uint32) { w.Write(binary.LittleEndian.AppendUint32(nil, v)) }
func putF32(w *bytes.Buffer, v float32) {
	putU32(w, math.Float32bits(v))
}

func putStr(w *bytes.Buffer, s string) {
	putU32(w, uint32(len(s)))
	w.WriteString(s)
}

func putFloats(w *bytes.Buffer, v []float32) {
	putU32(w, uint32(len(v)))
	for _, x := range v {
		putF32(w, x)
	}
}

// decoder reads a payload and keeps the first error; reads after it return zero values.
type decoder struct {
	b   []byte
	err error
}

func (d *decoder) fail(err error) {
	if d.err == nil {
		d.err = err
	}
	d.b = nil
}

func (d *decoder) bytes(n int64) []byte {
	if d.err != nil {
		return nil
	}
	if n < 0 || n > int64(len(d.b)) {
		d.fail(fmt.Errorf("need %d bytes, have %d", n, len(d.b)))
		return nil
	}
	out := d.b[:n:n]
	d.b = d.b[n:]
	return out
}

func (d *decoder) u16() uint16 {
	if b := d.bytes(2); b != nil {
		return binary.LittleEndian.Uint16(b)
	}
	return 0
}

func (d *decoder) u32() uint32 {
	if b := d.bytes(4); b != nil {
		return binary.LittleEndian.Uint32(b)
	}
	return 0
}

func (d *decoder) f32() float32 { return math.Float32frombits(d.u32()) }

func (d *decoder) str() string { return string(d.bytes(int64(d.u32()))) }

func (d *decoder) floats() []float32 {
	b := d.bytes(int64(d.u32()) * 4)
	if b == nil {
		return nil
	}
	out := make([]float32, len(b)/4)
	for i := range out {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return out
}
