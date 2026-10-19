// Package faceinfer runs the face detector and the recognizers through onnxruntime, loaded with purego
// so the helper builds without cgo. Only cmd/mdm-face imports it; see docs/design-docs/face-recognition.md.
package faceinfer

import (
	"errors"
	"fmt"
	"runtime"
	"unsafe"

	"github.com/ebitengine/purego"
)

// ortAPIVersion is the OrtApi version requested from the library. The struct only grows, so the
// indices below hold for every library at or above it.
const ortAPIVersion = 17

// Indices of the OrtApi function pointers used here, counted from onnxruntime_c_api.h.
const (
	fnGetErrorMessage                = 2
	fnCreateEnv                      = 3
	fnCreateSessionFromArray         = 8
	fnRun                            = 9
	fnCreateSessionOptions           = 10
	fnSetSessionLogSeverityLevel     = 22
	fnSetIntraOpNumThreads           = 24
	fnSessionGetInputName            = 36
	fnSessionGetOutputName           = 37
	fnCreateTensorWithDataAsOrtValue = 49
	fnGetTensorMutableData           = 51
	fnGetDimensionsCount             = 61
	fnGetDimensions                  = 62
	fnGetTensorTypeAndShape          = 65
	fnCreateCPUMemoryInfo            = 69
	fnAllocatorFree                  = 76
	fnGetAllocatorWithDefaultOptions = 78
	fnReleaseStatus                  = 93
	fnReleaseMemoryInfo              = 94
	fnReleaseValue                   = 96
	fnReleaseTensorTypeAndShapeInfo  = 99
	fnReleaseSessionOptions          = 100
)

const (
	ortLoggingLevelError = 3
	ortTensorFloat       = 1
)

// Runtime is one loaded onnxruntime library with its environment and CPU memory info.
type Runtime struct {
	api uintptr
	env uintptr
	mem uintptr
}

// tensor is a float32 output copied out of onnxruntime.
type tensor struct {
	shape []int64
	data  []float32
}

// Load opens the onnxruntime shared library at path.
func Load(path string) (*Runtime, error) {
	lib, err := openLibrary(path)
	if err != nil {
		return nil, fmt.Errorf("load %s: %w", path, err)
	}
	getAPIBase, err := librarySymbol(lib, "OrtGetApiBase")
	if err != nil {
		return nil, err
	}
	base, _, _ := purego.SyscallN(getAPIBase)
	api, _, _ := purego.SyscallN(*(*uintptr)(cPointer(base)), ortAPIVersion)
	if api == 0 {
		return nil, fmt.Errorf("%s does not provide OrtApi version %d", path, ortAPIVersion)
	}
	r := &Runtime{api: api}
	var pin runtime.Pinner
	defer pin.Unpin()
	logID := cString("mdm-face")
	if err := r.call(fnCreateEnv, ortLoggingLevelError, pinned(&pin, &logID[0]), pinned(&pin, &r.env)); err != nil {
		return nil, err
	}
	// OrtDeviceAllocator (0) and OrtMemTypeDefault (0) describe plain CPU memory owned by the caller.
	if err := r.call(fnCreateCPUMemoryInfo, 0, 0, pinned(&pin, &r.mem)); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *Runtime) fn(i int) uintptr {
	return *(*uintptr)(unsafe.Add(cPointer(r.api), i*int(unsafe.Sizeof(uintptr(0)))))
}

func (r *Runtime) call(i int, args ...uintptr) error {
	status, _, _ := purego.SyscallN(r.fn(i), args...)
	if status == 0 {
		return nil
	}
	msg, _, _ := purego.SyscallN(r.fn(fnGetErrorMessage), status)
	err := errors.New("onnxruntime: " + goString(msg))
	purego.SyscallN(r.fn(fnReleaseStatus), status)
	return err
}

// ortSession runs one model that takes one float32 tensor.
type ortSession struct {
	r       *Runtime
	h       uintptr
	input   []byte
	outputs []string
	names   [][]byte
}

// newSession creates a session; an empty input name reads the model's first input and output names instead.
func (r *Runtime) newSession(model []byte, threads int, input string, outputs []string) (*ortSession, error) {
	if len(model) == 0 {
		return nil, errors.New("empty model")
	}
	var pin runtime.Pinner
	defer pin.Unpin()
	var opts uintptr
	if err := r.call(fnCreateSessionOptions, pinned(&pin, &opts)); err != nil {
		return nil, err
	}
	defer purego.SyscallN(r.fn(fnReleaseSessionOptions), opts)
	if err := r.call(fnSetIntraOpNumThreads, opts, uintptr(threads)); err != nil {
		return nil, err
	}
	if err := r.call(fnSetSessionLogSeverityLevel, opts, ortLoggingLevelError); err != nil {
		return nil, err
	}
	s := &ortSession{r: r}
	err := r.call(fnCreateSessionFromArray, r.env, pinned(&pin, &model[0]), uintptr(len(model)), opts, pinned(&pin, &s.h))
	if err != nil {
		return nil, err
	}
	if input == "" {
		if input, outputs, err = s.firstNames(); err != nil {
			return nil, err
		}
	}
	s.input, s.outputs = cString(input), outputs
	for _, o := range outputs {
		s.names = append(s.names, cString(o))
	}
	return s, nil
}

// firstNames reads the model's first input and output names; exports of one model name them differently.
func (s *ortSession) firstNames() (string, []string, error) {
	var pin runtime.Pinner
	defer pin.Unpin()
	var alloc uintptr
	if err := s.r.call(fnGetAllocatorWithDefaultOptions, pinned(&pin, &alloc)); err != nil {
		return "", nil, err
	}
	in, err := s.ioName(&pin, fnSessionGetInputName, alloc)
	if err != nil {
		return "", nil, err
	}
	out, err := s.ioName(&pin, fnSessionGetOutputName, alloc)
	if err != nil {
		return "", nil, err
	}
	return in, []string{out}, nil
}

func (s *ortSession) ioName(pin *runtime.Pinner, fn int, alloc uintptr) (string, error) {
	var p uintptr
	if err := s.r.call(fn, s.h, 0, alloc, pinned(pin, &p)); err != nil {
		return "", err
	}
	defer purego.SyscallN(s.r.fn(fnAllocatorFree), alloc, p)
	return goString(p), nil
}

// run feeds one float32 tensor and returns the outputs in the order given to newSession.
func (s *ortSession) run(input []float32, shape []int64) ([]tensor, error) {
	var pin runtime.Pinner
	defer pin.Unpin()
	var in uintptr
	err := s.r.call(fnCreateTensorWithDataAsOrtValue, s.r.mem, pinned(&pin, &input[0]), uintptr(len(input)*4),
		pinned(&pin, &shape[0]), uintptr(len(shape)), ortTensorFloat, pinned(&pin, &in))
	if err != nil {
		return nil, err
	}
	defer purego.SyscallN(s.r.fn(fnReleaseValue), in)
	inName := pinned(&pin, &s.input[0])
	outNames := make([]uintptr, len(s.names))
	for i, n := range s.names {
		outNames[i] = pinned(&pin, &n[0])
	}
	outs := make([]uintptr, len(s.names))
	err = s.r.call(fnRun, s.h, 0, pinned(&pin, &inName), pinned(&pin, &in), 1,
		pinned(&pin, &outNames[0]), uintptr(len(outNames)), pinned(&pin, &outs[0]))
	if err != nil {
		return nil, err
	}
	return readOutputs(outs, s.r.readTensor, func(v uintptr) { purego.SyscallN(s.r.fn(fnReleaseValue), v) })
}

// readOutputs copies every output and releases all of them, including those after a failed read,
// because the helper keeps serving requests after an error.
func readOutputs(outs []uintptr, read func(uintptr) (tensor, error), release func(uintptr)) ([]tensor, error) {
	result := make([]tensor, len(outs))
	var firstErr error
	for i, v := range outs {
		if firstErr == nil {
			result[i], firstErr = read(v)
		}
		release(v)
	}
	if firstErr != nil {
		return nil, firstErr
	}
	return result, nil
}

func (r *Runtime) readTensor(v uintptr) (tensor, error) {
	var pin runtime.Pinner
	defer pin.Unpin()
	var info, dims, data uintptr
	if err := r.call(fnGetTensorTypeAndShape, v, pinned(&pin, &info)); err != nil {
		return tensor{}, err
	}
	defer purego.SyscallN(r.fn(fnReleaseTensorTypeAndShapeInfo), info)
	if err := r.call(fnGetDimensionsCount, info, pinned(&pin, &dims)); err != nil {
		return tensor{}, err
	}
	shape := make([]int64, dims)
	if dims > 0 {
		if err := r.call(fnGetDimensions, info, pinned(&pin, &shape[0]), dims); err != nil {
			return tensor{}, err
		}
	}
	if err := r.call(fnGetTensorMutableData, v, pinned(&pin, &data)); err != nil {
		return tensor{}, err
	}
	n := 1
	for _, d := range shape {
		n *= int(d)
	}
	out := make([]float32, n)
	copy(out, unsafe.Slice((*float32)(cPointer(data)), n))
	return tensor{shape: shape, data: out}, nil
}

// pinned keeps p at a fixed address until the pinner is released and returns that address for C.
func pinned[T any](pin *runtime.Pinner, p *T) uintptr {
	pin.Pin(p)
	return uintptr(unsafe.Pointer(p))
}

// cPointer turns an address returned by onnxruntime into a pointer; the memory stays owned by onnxruntime.
func cPointer(addr uintptr) unsafe.Pointer {
	return unsafe.Add(unsafe.Pointer(nil), addr)
}

func cString(s string) []byte {
	return append([]byte(s), 0)
}

func goString(addr uintptr) string {
	if addr == 0 {
		return ""
	}
	var b []byte
	for p := cPointer(addr); *(*byte)(p) != 0; p = unsafe.Add(p, 1) {
		b = append(b, *(*byte)(p))
	}
	return string(b)
}
