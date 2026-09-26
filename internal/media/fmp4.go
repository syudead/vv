package media

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
)

// ライブ変換の出力（fragmented MP4）の先頭の moov を読み、各 track の edit list から
// 実際の開始位置を得て、時間軸が 0 から始まるよう edit list を書き換える
// （specs/018-live-transcode-seek/research.md R-1）。moof/mdat には触れない。

const (
	boxHeaderSize = 8
	// maxInitSegmentSize は moov までの先頭部分の上限である。empty_moov の moov は
	// sample table を持たないので数 KB に収まる。
	maxInitSegmentSize = 1 << 20
)

// errInvalidInitSegment は出力の先頭が期待した形（ftyp などに続く moov）でないことを表す。
var errInvalidInitSegment = errors.New("ライブ変換の出力の先頭を読めません")

// readInitSegment は r から moov までの box を読み、そのまま返す。moov より前に moof が
// 来たとき、または box の大きさが読めないときは errInvalidInitSegment を返す。moov を
// 読み終える前に出力が終わったら io.EOF か io.ErrUnexpectedEOF を返す。
func readInitSegment(r io.Reader) ([]byte, error) {
	var segment []byte
	for {
		header := make([]byte, boxHeaderSize)
		if _, err := io.ReadFull(r, header); err != nil {
			if errors.Is(err, io.EOF) && len(segment) > 0 {
				return nil, io.ErrUnexpectedEOF
			}
			return nil, err
		}
		size := uint64(binary.BigEndian.Uint32(header[:4]))
		boxType := string(header[4:8])
		headerSize := uint64(boxHeaderSize)
		if size == 1 {
			large := make([]byte, 8)
			if _, err := io.ReadFull(r, large); err != nil {
				return nil, unexpectedEOF(err)
			}
			header = append(header, large...)
			size = binary.BigEndian.Uint64(large)
			headerSize += 8
		}
		if size < headerSize || size > maxInitSegmentSize || uint64(len(segment))+size > maxInitSegmentSize {
			return nil, fmt.Errorf("%w: box %q の大きさ %d", errInvalidInitSegment, boxType, size)
		}
		if boxType == "moof" || boxType == "mdat" {
			return nil, fmt.Errorf("%w: moov より前に %s がある", errInvalidInitSegment, boxType)
		}
		body := make([]byte, size-headerSize)
		if _, err := io.ReadFull(r, body); err != nil {
			return nil, unexpectedEOF(err)
		}
		segment = append(segment, header...)
		segment = append(segment, body...)
		if boxType == "moov" {
			return segment, nil
		}
	}
}

func unexpectedEOF(err error) error {
	if errors.Is(err, io.EOF) {
		return io.ErrUnexpectedEOF
	}
	return err
}

// mp4Box は読み取った box 1 つである。payload は header を除いた中身で、children は
// 書き換えのために分解した容器（moov・trak・edts）の中身である。
type mp4Box struct {
	boxType  string
	payload  []byte
	children []mp4Box
}

func parseBoxes(data []byte) ([]mp4Box, error) {
	var boxes []mp4Box
	for len(data) > 0 {
		if len(data) < boxHeaderSize {
			return nil, fmt.Errorf("%w: box の header が途中で切れている", errInvalidInitSegment)
		}
		size := uint64(binary.BigEndian.Uint32(data[:4]))
		boxType := string(data[4:8])
		headerSize := uint64(boxHeaderSize)
		switch size {
		case 0:
			size = uint64(len(data))
		case 1:
			if len(data) < 16 {
				return nil, fmt.Errorf("%w: box %q の header が途中で切れている", errInvalidInitSegment, boxType)
			}
			size = binary.BigEndian.Uint64(data[8:16])
			headerSize = 16
		}
		if size < headerSize || size > uint64(len(data)) {
			return nil, fmt.Errorf("%w: box %q の大きさ %d", errInvalidInitSegment, boxType, size)
		}
		box := mp4Box{boxType: boxType, payload: data[headerSize:size]}
		if boxType == "moov" || boxType == "trak" || boxType == "edts" {
			children, err := parseBoxes(box.payload)
			if err != nil {
				return nil, err
			}
			box.children = children
		}
		boxes = append(boxes, box)
		data = data[size:]
	}
	return boxes, nil
}

func serializeBoxes(boxes []mp4Box) ([]byte, error) {
	var out []byte
	for _, box := range boxes {
		payload := box.payload
		if box.children != nil {
			serialized, err := serializeBoxes(box.children)
			if err != nil {
				return nil, err
			}
			payload = serialized
		}
		size := uint64(boxHeaderSize) + uint64(len(payload))
		if size > math.MaxUint32 {
			return nil, fmt.Errorf("%w: box %q が大きすぎる", errInvalidInitSegment, box.boxType)
		}
		out = binary.BigEndian.AppendUint32(out, uint32(size))
		out = append(out, box.boxType...)
		out = append(out, payload...)
	}
	return out, nil
}

// editList は elst の中身である。空の edit（media_time が -1）は track の開始を
// 遅らせる区間で、その長さ（movie timescale）が track の開始時刻になる。
type editList struct {
	version uint8
	flags   [3]byte
	entries []editEntry
}

type editEntry struct {
	segmentDuration uint64
	mediaTime       int64
	rate            [4]byte
}

func (e editEntry) empty() bool { return e.mediaTime == -1 }

func parseEditList(payload []byte) (editList, error) {
	if len(payload) < 8 {
		return editList{}, fmt.Errorf("%w: elst が短い", errInvalidInitSegment)
	}
	list := editList{version: payload[0]}
	copy(list.flags[:], payload[1:4])
	count := binary.BigEndian.Uint32(payload[4:8])
	entrySize := 12
	if list.version == 1 {
		entrySize = 20
	} else if list.version != 0 {
		return editList{}, fmt.Errorf("%w: elst の版 %d", errInvalidInitSegment, list.version)
	}
	data := payload[8:]
	if uint64(len(data)) < uint64(count)*uint64(entrySize) {
		return editList{}, fmt.Errorf("%w: elst の entry が足りない", errInvalidInitSegment)
	}
	for range count {
		var entry editEntry
		if list.version == 1 {
			entry.segmentDuration = binary.BigEndian.Uint64(data[0:8])
			entry.mediaTime = int64(binary.BigEndian.Uint64(data[8:16]))
			copy(entry.rate[:], data[16:20])
		} else {
			entry.segmentDuration = uint64(binary.BigEndian.Uint32(data[0:4]))
			entry.mediaTime = int64(int32(binary.BigEndian.Uint32(data[4:8])))
			copy(entry.rate[:], data[8:12])
		}
		list.entries = append(list.entries, entry)
		data = data[entrySize:]
	}
	return list, nil
}

func (l editList) serialize() []byte {
	out := []byte{l.version, l.flags[0], l.flags[1], l.flags[2]}
	out = binary.BigEndian.AppendUint32(out, uint32(len(l.entries)))
	for _, entry := range l.entries {
		if l.version == 1 {
			out = binary.BigEndian.AppendUint64(out, entry.segmentDuration)
			out = binary.BigEndian.AppendUint64(out, uint64(entry.mediaTime))
		} else {
			out = binary.BigEndian.AppendUint32(out, uint32(entry.segmentDuration))
			out = binary.BigEndian.AppendUint32(out, uint32(int32(entry.mediaTime)))
		}
		out = append(out, entry.rate[:]...)
	}
	return out
}

// startDelay は track の開始時刻（movie timescale）で、先頭の空の edit の長さである。
// 空の edit が無い track は 0 から始まる。
func (l editList) startDelay() uint64 {
	if len(l.entries) > 0 && l.entries[0].empty() {
		return l.entries[0].segmentDuration
	}
	return 0
}

// withStartDelay は先頭の空の edit を delay にした edit list を返す。delay が 0 なら
// 空の edit を除く。
func (l editList) withStartDelay(delay uint64) editList {
	entries := l.entries
	if len(entries) > 0 && entries[0].empty() {
		entries = entries[1:]
	}
	if delay > 0 {
		empty := editEntry{segmentDuration: delay, mediaTime: -1, rate: [4]byte{0, 1, 0, 0}}
		entries = append([]editEntry{empty}, entries...)
	}
	l.entries = entries
	if l.version == 0 && delay > math.MaxUint32 {
		l.version = 1
	}
	return l
}

// rebaseEditLists は moov までの先頭部分 segment を読み、最も早く始まる track の開始
// 時刻を実際の開始位置（ミリ秒）として返す。書き換えた先頭部分では、その track が 0 から
// 始まり、他の track は元の差だけ遅れて始まる。すべての track が 0 から始まるときは
// segment をそのまま返す。
func rebaseEditLists(segment []byte) ([]byte, int64, error) {
	boxes, err := parseBoxes(segment)
	if err != nil {
		return nil, 0, err
	}
	moovIndex := -1
	for i, box := range boxes {
		if box.boxType == "moov" {
			moovIndex = i
		}
	}
	if moovIndex < 0 {
		return nil, 0, fmt.Errorf("%w: moov が無い", errInvalidInitSegment)
	}
	moov := &boxes[moovIndex]

	timescale := uint64(0)
	type trackEdit struct {
		trak, edts, elst int
		list             editList
	}
	var tracks []trackEdit
	for i, child := range moov.children {
		switch child.boxType {
		case "mvhd":
			timescale, err = movieTimescale(child.payload)
			if err != nil {
				return nil, 0, err
			}
		case "trak":
			track := trackEdit{trak: i, edts: -1, elst: -1}
			for j, trakChild := range child.children {
				if trakChild.boxType != "edts" {
					continue
				}
				for k, edtsChild := range trakChild.children {
					if edtsChild.boxType == "elst" {
						track.edts, track.elst = j, k
						track.list, err = parseEditList(edtsChild.payload)
						if err != nil {
							return nil, 0, err
						}
					}
				}
			}
			tracks = append(tracks, track)
		}
	}
	if timescale == 0 {
		return nil, 0, fmt.Errorf("%w: mvhd の timescale が無い", errInvalidInitSegment)
	}
	if len(tracks) == 0 {
		return nil, 0, fmt.Errorf("%w: track が無い", errInvalidInitSegment)
	}

	earliest := uint64(math.MaxUint64)
	for _, track := range tracks {
		earliest = min(earliest, track.list.startDelay())
	}
	startMs := int64(math.Round(float64(earliest) * 1000 / float64(timescale)))
	if earliest == 0 {
		return segment, 0, nil
	}
	for _, track := range tracks {
		// earliest が正なら、どの track にも空の edit がある。
		list := track.list.withStartDelay(track.list.startDelay() - earliest)
		trak := &moov.children[track.trak]
		edts := &trak.children[track.edts]
		if len(list.entries) == 0 {
			trak.children = append(trak.children[:track.edts:track.edts], trak.children[track.edts+1:]...)
			continue
		}
		edts.children[track.elst].payload = list.serialize()
	}
	rewritten, err := serializeBoxes(boxes)
	if err != nil {
		return nil, 0, err
	}
	return rewritten, startMs, nil
}

func movieTimescale(payload []byte) (uint64, error) {
	offset := 12
	if len(payload) > 0 && payload[0] == 1 {
		offset = 20
	}
	if len(payload) < offset+4 {
		return 0, fmt.Errorf("%w: mvhd が短い", errInvalidInitSegment)
	}
	return uint64(binary.BigEndian.Uint32(payload[offset : offset+4])), nil
}
