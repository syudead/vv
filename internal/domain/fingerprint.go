package domain

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"math/bits"
	"slices"
)

// Fingerprints compare frame pHashes by timestamp; bump FingerprintVersion when hashing rules or calibrated thresholds change, and never compare different versions.

const (
	FingerprintVersion   = 1
	FingerprintFrameSize = 32
	// Low-variance frames are excluded because black screens and fades match unrelated videos.
	FingerprintFlatVariance        = 16.0
	FingerprintMinComparableFrames = 3
	// FingerprintMatchMaxDistance is the maximum median Hamming distance across 63 bits.
	FingerprintMatchMaxDistance = 12

	fingerprintLowFrequency = 8
	fingerprintFrameBytes   = 9

	// MaxFingerprintBytes bounds an encoded fingerprint, which hashes the frames of one seek sprite.
	MaxFingerprintBytes = SeekSpriteMaxFrames * fingerprintFrameBytes
)

type FrameHash struct {
	Hash uint64
	Flat bool
}

// Frames[i] covers [i*IntervalMs, (i+1)*IntervalMs).
type Fingerprint struct {
	Version    int
	IntervalMs int64
	Frames     []FrameHash
}

var ErrInvalidFingerprint = errors.New("the fingerprint data is malformed")

// Encode stores each frame as a flat flag byte followed by an eight-byte big-endian hash; version and interval live in separate columns.
func (f Fingerprint) Encode() []byte {
	out := make([]byte, 0, len(f.Frames)*fingerprintFrameBytes)
	for _, frame := range f.Frames {
		var flag byte
		if frame.Flat {
			flag = 1
		}
		out = append(out, flag)
		out = binary.BigEndian.AppendUint64(out, frame.Hash)
	}
	return out
}

func DecodeFingerprint(data []byte, version int, intervalMs int64) (Fingerprint, error) {
	if len(data)%fingerprintFrameBytes != 0 {
		return Fingerprint{}, fmt.Errorf("%w: %d bytes", ErrInvalidFingerprint, len(data))
	}
	frames := make([]FrameHash, 0, len(data)/fingerprintFrameBytes)
	for offset := 0; offset < len(data); offset += fingerprintFrameBytes {
		flag := data[offset]
		if flag > 1 {
			return Fingerprint{}, fmt.Errorf("%w: flag %d", ErrInvalidFingerprint, flag)
		}
		frames = append(frames, FrameHash{
			Hash: binary.BigEndian.Uint64(data[offset+1 : offset+fingerprintFrameBytes]),
			Flat: flag == 1,
		})
	}
	return Fingerprint{Version: version, IntervalMs: intervalMs, Frames: frames}, nil
}

// fingerprintCos[u][x] is cos((2x+1)uπ/64), the eight low-frequency bases of a 32-point DCT-II.
var fingerprintCos = func() (table [fingerprintLowFrequency][FingerprintFrameSize]float64) {
	for u := range fingerprintLowFrequency {
		for x := range FingerprintFrameSize {
			table[u][x] = math.Cos(float64(2*x+1) * float64(u) * math.Pi / float64(2*FingerprintFrameSize))
		}
	}
	return table
}()

// HashFrame thresholds the 63 non-DC coefficients of the low-frequency 8×8 DCT against their median; low-variance frames are marked Flat.
func HashFrame(luma [FingerprintFrameSize][FingerprintFrameSize]uint8) FrameHash {
	var sum, sumSquares float64
	for _, row := range luma {
		for _, value := range row {
			v := float64(value)
			sum += v
			sumSquares += v * v
		}
	}
	count := float64(FingerprintFrameSize * FingerprintFrameSize)
	mean := sum / count
	variance := sumSquares/count - mean*mean

	var rows [FingerprintFrameSize][fingerprintLowFrequency]float64
	for y := range FingerprintFrameSize {
		for v := range fingerprintLowFrequency {
			var total float64
			for x := range FingerprintFrameSize {
				total += float64(luma[y][x]) * fingerprintCos[v][x]
			}
			rows[y][v] = total
		}
	}
	coefficients := make([]float64, 0, fingerprintLowFrequency*fingerprintLowFrequency-1)
	for u := range fingerprintLowFrequency {
		for v := range fingerprintLowFrequency {
			if u == 0 && v == 0 {
				continue
			}
			var total float64
			for y := range FingerprintFrameSize {
				total += rows[y][v] * fingerprintCos[u][y]
			}
			coefficients = append(coefficients, total)
		}
	}
	sorted := slices.Clone(coefficients)
	slices.Sort(sorted)
	median := sorted[len(sorted)/2]

	var hash uint64
	for i, coefficient := range coefficients {
		if coefficient > median {
			hash |= 1 << uint(i)
		}
	}
	return FrameHash{Hash: hash, Flat: variance < FingerprintFlatVariance}
}

// CompareFingerprints pairs nearest frame start times within half the shorter interval, excluding flat frames, invalid versions/intervals, and insufficient pairs; it uses the lower median for even counts.
func CompareFingerprints(a, b Fingerprint) (distance int, ok bool) {
	pairs, ok := ComparableFramePairs(a, b)
	if !ok {
		return 0, false
	}
	distances := make([]int, len(pairs))
	for i, pair := range pairs {
		distances[i] = pair.Distance
	}
	slices.Sort(distances)
	return distances[(len(distances)-1)/2], true
}

type FramePair struct {
	A, B     int
	Distance int
}

// ComparableFramePairs uses CompareFingerprints timing rules and returns non-flat pairs in a-frame order; incompatible or insufficient fingerprints return false.
func ComparableFramePairs(a, b Fingerprint) (pairs []FramePair, ok bool) {
	if a.Version != b.Version || a.IntervalMs <= 0 || b.IntervalMs <= 0 {
		return nil, false
	}
	tolerance := min(a.IntervalMs, b.IntervalMs) / 2
	pairs = make([]FramePair, 0, len(a.Frames))
	for i, frame := range a.Frames {
		start := int64(i) * a.IntervalMs
		j := (start + b.IntervalMs/2) / b.IntervalMs
		if j >= int64(len(b.Frames)) || abs64(j*b.IntervalMs-start) > tolerance {
			continue
		}
		other := b.Frames[j]
		if frame.Flat || other.Flat {
			continue
		}
		pairs = append(pairs, FramePair{A: i, B: int(j), Distance: bits.OnesCount64(frame.Hash ^ other.Hash)})
	}
	if len(pairs) < FingerprintMinComparableFrames {
		return nil, false
	}
	return pairs, true
}

func FingerprintsMatch(a, b Fingerprint) bool {
	distance, ok := CompareFingerprints(a, b)
	return ok && distance <= FingerprintMatchMaxDistance
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
