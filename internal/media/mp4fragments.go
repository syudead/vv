package media

import (
	"encoding/binary"
	"fmt"
)

// A fragmented MP4 keeps an empty sample table in moov and describes its samples in moof boxes, each followed by
// its mdat. See docs/design-docs/seek-sprite-generation.md.

// movieFragment is one top-level moof; offset is where the box starts in the file.
type movieFragment struct {
	offset  int64
	payload []byte
}

// fragmentSource holds the moof boxes and the moov boxes whose mvex supplies per-track sample defaults.
type fragmentSource struct {
	fragments []movieFragment
	movie     []mp4Box
}

const (
	tfhdBaseDataOffset    = 0x000001
	tfhdDescriptionIndex  = 0x000002
	tfhdDefaultDuration   = 0x000008
	tfhdDefaultSize       = 0x000010
	tfhdDefaultFlags      = 0x000020
	trunDataOffset        = 0x000001
	trunFirstSampleFlags  = 0x000004
	trunSampleDuration    = 0x000100
	trunSampleSize        = 0x000200
	trunSampleFlags       = 0x000400
	trunCompositionOffset = 0x000800
	sampleIsNonSyncSample = 0x010000
)

// fragmented reports whether a moov declares movie fragments.
func fragmented(movie []byte) bool {
	boxes, err := parseBoxes(movie)
	if err != nil {
		return false
	}
	_, err = childPayload(boxes, "mvex")
	return err == nil
}

// sampleDefaults are the trex values a tfhd may override.
type sampleDefaults struct {
	duration, size, flags uint32
}

// keyframes collects the sync samples of the trak's track from every fragment, in decode order.
func (f fragmentSource) keyframes(trak []mp4Box) ([]keyframeSample, error) {
	if len(f.fragments) == 0 {
		return nil, fmt.Errorf("%w: sample table is empty and there are no movie fragments", errSeekIndexUnsupported)
	}
	trackID, err := trackID(trak)
	if err != nil {
		return nil, err
	}
	defaults := f.trackDefaults(trackID)
	reader := fragmentReader{trackID: trackID, trex: defaults}
	for _, fragment := range f.fragments {
		if err := reader.readFragment(fragment); err != nil {
			return nil, err
		}
	}
	if len(reader.keyframes) == 0 {
		return nil, fmt.Errorf("%w: movie fragments hold no keyframe of track %d", errSeekIndexUnsupported, trackID)
	}
	return reader.keyframes, nil
}

func trackID(trak []mp4Box) (uint32, error) {
	tkhd, err := childPayload(trak, "tkhd")
	if err != nil {
		return 0, err
	}
	offset := 12
	if len(tkhd) > 0 && tkhd[0] == 1 {
		offset = 20
	}
	if len(tkhd) < offset+4 {
		return 0, fmt.Errorf("%w: tkhd is too short", errSeekIndexUnsupported)
	}
	return binary.BigEndian.Uint32(tkhd[offset:]), nil
}

// trackDefaults reads the track's trex; a track without one has zero defaults, which each fragment must override.
func (f fragmentSource) trackDefaults(trackID uint32) sampleDefaults {
	mvex, err := childPayload(f.movie, "mvex")
	if err != nil {
		return sampleDefaults{}
	}
	boxes, err := parseBoxes(mvex)
	if err != nil {
		return sampleDefaults{}
	}
	for _, box := range boxes {
		trex := box.payload
		if box.boxType != "trex" || len(trex) < 24 || binary.BigEndian.Uint32(trex[4:]) != trackID {
			continue
		}
		return sampleDefaults{
			duration: binary.BigEndian.Uint32(trex[12:]),
			size:     binary.BigEndian.Uint32(trex[16:]),
			flags:    binary.BigEndian.Uint32(trex[20:]),
		}
	}
	return sampleDefaults{}
}

// fragmentReader carries the decode time across fragments that lack a tfdt.
type fragmentReader struct {
	trackID   uint32
	trex      sampleDefaults
	decode    int64
	keyframes []keyframeSample
}

func (r *fragmentReader) readFragment(fragment movieFragment) error {
	boxes, err := parseBoxes(fragment.payload)
	if err != nil {
		return fmt.Errorf("%w: moof at %d: %w", errSeekIndexUnsupported, fragment.offset, err)
	}
	for _, box := range boxes {
		if box.boxType != "traf" {
			continue
		}
		traf, err := parseBoxes(box.payload)
		if err != nil {
			return fmt.Errorf("%w: traf at %d: %w", errSeekIndexUnsupported, fragment.offset, err)
		}
		if err := r.readTrackFragment(traf, fragment.offset); err != nil {
			return err
		}
	}
	return nil
}

// trackFragment is the parsed tfhd: the data base and the defaults its runs fall back to.
type trackFragment struct {
	trackID  uint32
	base     int64
	defaults sampleDefaults
}

// readTrackFragment treats every data offset as relative to the moof start unless tfhd gives a base, as
// default-base-is-moof requires and as writers such as ffmpeg and MP4Box produce.
func (r *fragmentReader) readTrackFragment(traf []mp4Box, moofOffset int64) error {
	tfhd, err := childPayload(traf, "tfhd")
	if err != nil {
		return err
	}
	header, err := parseTrackFragmentHeader(tfhd, moofOffset, r.trex)
	if err != nil || header.trackID != r.trackID {
		return err
	}
	if tfdt, err := childPayload(traf, "tfdt"); err == nil {
		if r.decode, err = baseMediaDecodeTime(tfdt); err != nil {
			return err
		}
	}
	position := header.base
	for _, box := range traf {
		if box.boxType != "trun" {
			continue
		}
		if position, err = r.readRun(box.payload, header, position); err != nil {
			return err
		}
	}
	return nil
}

func parseTrackFragmentHeader(tfhd []byte, moofOffset int64, trex sampleDefaults) (trackFragment, error) {
	if len(tfhd) < 8 {
		return trackFragment{}, fmt.Errorf("%w: tfhd is too short", errSeekIndexUnsupported)
	}
	flags := binary.BigEndian.Uint32(tfhd) & 0xffffff
	header := trackFragment{trackID: binary.BigEndian.Uint32(tfhd[4:]), base: moofOffset, defaults: trex}
	rest := tfhd[8:]
	for _, field := range []struct {
		flag  uint32
		width int
		apply func([]byte)
	}{
		{tfhdBaseDataOffset, 8, func(b []byte) { header.base = int64(binary.BigEndian.Uint64(b)) }},
		{tfhdDescriptionIndex, 4, func([]byte) {}},
		{tfhdDefaultDuration, 4, func(b []byte) { header.defaults.duration = binary.BigEndian.Uint32(b) }},
		{tfhdDefaultSize, 4, func(b []byte) { header.defaults.size = binary.BigEndian.Uint32(b) }},
		{tfhdDefaultFlags, 4, func(b []byte) { header.defaults.flags = binary.BigEndian.Uint32(b) }},
	} {
		if flags&field.flag == 0 {
			continue
		}
		if len(rest) < field.width {
			return trackFragment{}, fmt.Errorf("%w: tfhd is truncated", errSeekIndexUnsupported)
		}
		field.apply(rest)
		rest = rest[field.width:]
	}
	return header, nil
}

func baseMediaDecodeTime(tfdt []byte) (int64, error) {
	if len(tfdt) >= 12 && tfdt[0] == 1 {
		return int64(binary.BigEndian.Uint64(tfdt[4:])), nil
	}
	if len(tfdt) < 8 {
		return 0, fmt.Errorf("%w: tfdt is too short", errSeekIndexUnsupported)
	}
	return int64(binary.BigEndian.Uint32(tfdt[4:])), nil
}

// readRun records the run's sync samples and returns where the next run's data starts when it gives no offset.
func (r *fragmentReader) readRun(trun []byte, header trackFragment, position int64) (int64, error) {
	if len(trun) < 8 {
		return 0, fmt.Errorf("%w: trun is too short", errSeekIndexUnsupported)
	}
	flags := binary.BigEndian.Uint32(trun) & 0xffffff
	count := int(binary.BigEndian.Uint32(trun[4:]))
	rest := trun[8:]
	if flags&trunDataOffset != 0 {
		if len(rest) < 4 {
			return 0, fmt.Errorf("%w: trun is truncated", errSeekIndexUnsupported)
		}
		position = header.base + int64(int32(binary.BigEndian.Uint32(rest)))
		rest = rest[4:]
	}
	firstFlags, hasFirstFlags := header.defaults.flags, false
	if flags&trunFirstSampleFlags != 0 {
		if len(rest) < 4 {
			return 0, fmt.Errorf("%w: trun is truncated", errSeekIndexUnsupported)
		}
		firstFlags, hasFirstFlags = binary.BigEndian.Uint32(rest), true
		rest = rest[4:]
	}
	entry := runEntryLayout(flags)
	if count < 0 || len(rest) < count*entry.size {
		return 0, fmt.Errorf("%w: trun has too few entries", errSeekIndexUnsupported)
	}
	for i := range count {
		sample := entry.read(rest[i*entry.size:], header.defaults)
		if i == 0 && hasFirstFlags && flags&trunSampleFlags == 0 {
			sample.flags = firstFlags
		}
		if sample.flags&sampleIsNonSyncSample == 0 {
			if len(r.keyframes) >= maxKeyframes {
				return 0, fmt.Errorf("%w: more than %d keyframes", errSeekIndexUnsupported, maxKeyframes)
			}
			r.keyframes = append(r.keyframes, keyframeSample{
				decode: r.decode, composition: sample.composition, offset: position, size: int64(sample.size),
			})
		}
		position += int64(sample.size)
		r.decode += int64(sample.duration)
	}
	return position, nil
}

// runEntry holds the byte offset of each optional per-sample trun field, or -1 when the field is absent.
type runEntry struct {
	size                                  int
	duration, sampleSize, flags, ctOffset int
}

type runSample struct {
	duration, size, flags uint32
	composition           int64
}

func runEntryLayout(flags uint32) runEntry {
	entry := runEntry{duration: -1, sampleSize: -1, flags: -1, ctOffset: -1}
	for _, field := range []struct {
		flag   uint32
		offset *int
	}{
		{trunSampleDuration, &entry.duration},
		{trunSampleSize, &entry.sampleSize},
		{trunSampleFlags, &entry.flags},
		{trunCompositionOffset, &entry.ctOffset},
	} {
		if flags&field.flag != 0 {
			*field.offset = entry.size
			entry.size += 4
		}
	}
	return entry
}

// read treats composition offsets as signed in both trun versions, as ffmpeg does.
func (e runEntry) read(data []byte, defaults sampleDefaults) runSample {
	sample := runSample{duration: defaults.duration, size: defaults.size, flags: defaults.flags}
	if e.duration >= 0 {
		sample.duration = binary.BigEndian.Uint32(data[e.duration:])
	}
	if e.sampleSize >= 0 {
		sample.size = binary.BigEndian.Uint32(data[e.sampleSize:])
	}
	if e.flags >= 0 {
		sample.flags = binary.BigEndian.Uint32(data[e.flags:])
	}
	if e.ctOffset >= 0 {
		sample.composition = int64(int32(binary.BigEndian.Uint32(data[e.ctOffset:])))
	}
	return sample
}
