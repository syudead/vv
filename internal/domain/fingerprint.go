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
	FingerprintVersion   = 2
	FingerprintFrameSize = 32
	// Low-variance frames are excluded because black screens and fades match unrelated videos.
	FingerprintFlatVariance        = 16.0
	FingerprintMinComparableFrames = 3
	// FingerprintMatchMaxDistance is the maximum median Hamming distance across 63 bits.
	FingerprintMatchMaxDistance = 12

	fingerprintLowFrequency = 8
	fingerprintFrameBytes   = 9
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

// CompareFingerprints is the lower median distance of ComparableFramePairs.
func CompareFingerprints(a, b Fingerprint) (distance int, ok bool) {
	pairs, ok := ComparableFramePairs(a, b)
	if !ok {
		return 0, false
	}
	return medianDistance(pairs), true
}

type FramePair struct {
	A, B     int
	Distance int
}

// Start offsets are searched up to the duration difference plus this slack, in steps of the coarser interval, and
// never beyond fingerprintMaxShift steps.
const (
	fingerprintShiftSlack = 3
	fingerprintMaxShift   = 60
)

// ComparableFramePairs pairs frames by time at the start offset whose median distance is lowest, preferring the
// smallest offset on a tie, and returns non-flat pairs in a-frame order; incompatible or insufficient fingerprints
// return false.
func ComparableFramePairs(a, b Fingerprint) (pairs []FramePair, ok bool) {
	if a.Version != b.Version || a.IntervalMs <= 0 || b.IntervalMs <= 0 {
		return nil, false
	}
	step := max(a.IntervalMs, b.IntervalMs)
	lengthDifference := abs64(int64(len(a.Frames))*a.IntervalMs - int64(len(b.Frames))*b.IntervalMs)
	maxShift := min(lengthDifference/step+fingerprintShiftSlack, fingerprintMaxShift)
	bestMedian, bestOffset := -1, int64(0)
	for shift := int64(0); shift <= maxShift; shift++ {
		for _, sign := range []int64{1, -1} {
			if shift == 0 && sign < 0 {
				continue
			}
			offset := sign * shift * step
			var histogram [65]int
			count := 0
			eachFramePairAt(a, b, offset, func(pair FramePair) {
				histogram[pair.Distance]++
				count++
			})
			if count < FingerprintMinComparableFrames {
				continue
			}
			if median := histogramMedian(histogram, count); bestMedian < 0 || median < bestMedian {
				bestMedian, bestOffset = median, offset
			}
		}
	}
	if bestMedian < 0 {
		return nil, false
	}
	eachFramePairAt(a, b, bestOffset, func(pair FramePair) { pairs = append(pairs, pair) })
	return pairs, true
}

// eachFramePairAt pairs each a frame with the b frame offsetMs later in b's timeline, within half the shorter
// interval, skipping flat frames.
func eachFramePairAt(a, b Fingerprint, offsetMs int64, yield func(FramePair)) {
	tolerance := min(a.IntervalMs, b.IntervalMs) / 2
	for i, frame := range a.Frames {
		start := int64(i)*a.IntervalMs + offsetMs
		if start < 0 {
			continue
		}
		j := (start + b.IntervalMs/2) / b.IntervalMs
		if j >= int64(len(b.Frames)) || abs64(j*b.IntervalMs-start) > tolerance {
			continue
		}
		other := b.Frames[j]
		if frame.Flat || other.Flat {
			continue
		}
		yield(FramePair{A: i, B: int(j), Distance: bits.OnesCount64(frame.Hash ^ other.Hash)})
	}
}

// histogramMedian is the lower median of count distances, counted by value.
func histogramMedian(histogram [65]int, count int) int {
	seen := 0
	for distance, n := range histogram {
		seen += n
		if seen > (count-1)/2 {
			return distance
		}
	}
	return len(histogram) - 1
}

func medianDistance(pairs []FramePair) int {
	distances := make([]int, len(pairs))
	for i, pair := range pairs {
		distances[i] = pair.Distance
	}
	slices.Sort(distances)
	return distances[(len(distances)-1)/2]
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
