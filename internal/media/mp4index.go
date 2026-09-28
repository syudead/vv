package media

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"slices"
	"sort"

	"github.com/syudead/vv/internal/domain"
)

// MP4／MOV の索引（moov の sample table）から、シーク用スプライトの各コマに使う
// キーフレームの位置と大きさを求める。ffmpeg に時刻シークさせるとコマごとに索引を
// 読み直すので、索引を 1 回だけ読み、必要なキーフレームのバイトだけを取る
// （docs/design-docs/seek-sprite-generation.md）。

const (
	// maxMovieBoxSize は読み込む moov の上限である。数時間の動画でも数十 MB に収まる。
	maxMovieBoxSize = 256 << 20
	// maxTopLevelBoxes は moov を探すときに読み飛ばす最上位の box の数の上限である。
	maxTopLevelBoxes = 1024
	// maxKeyframeSize は 1 枚のキーフレームとして読む大きさの上限である。
	maxKeyframeSize = 64 << 20
	// maxKeyframeTotalSize は 1 枚のスプライトのために読むキーフレームの合計の上限である。
	maxKeyframeTotalSize = 512 << 20
	// maxKeyframes は 1 トラックのキーフレームの数の上限である（1 秒に 1 枚で 12 日分）。
	// 値を取り出すのはキーフレームの分だけなので、確保する量はこれで抑えられる。
	maxKeyframes = 1 << 20
)

// errSeekIndexUnsupported は、入力が索引から抽出できる形（sample table を持つ
// MP4／MOV の H.264／HEVC）でないことを表す。
var errSeekIndexUnsupported = errors.New("keyframes cannot be read from the index for this input")

// keyframeTrack は映像トラックのうち、スプライトの生成に使う部分である。
type keyframeTrack struct {
	// format は ffmpeg の入力形式（h264 か hevc）である。
	format string
	// lengthSize は sample 内の NAL の長さの欄のバイト数である。
	lengthSize int
	// parameterSets はデコーダ設定（avcC／hvcC）にある NAL で、キーフレームの前に置く。
	parameterSets [][]byte
	// displayFilter は表示行列（tkhd）の回転を復号後の画像に施す ffmpeg のフィルタで、
	// 回転しないトラックでは空である。生の映像として復号すると表示行列は失われるので、
	// ffmpeg が容器から読むときの自動回転と同じ向きにここで直す。
	displayFilter string
	// keyframes は表示時刻の昇順に並べたキーフレームである。
	keyframes []keyframe
}

// keyframe は 1 枚のキーフレームの表示時刻（マイクロ秒）とファイル内の位置である。
type keyframe struct {
	presentationUs int64
	offset         int64
	size           int64
}

// readMovieBox はファイルの最上位の box を先頭から順に見て、moov の中身を返す。
// moov の前にある mdat などは header だけを読んで飛ばす。
func readMovieBox(ctx context.Context, r io.ReaderAt, fileSize int64) ([]byte, error) {
	var position int64
	header := make([]byte, 16)
	for range maxTopLevelBoxes {
		if position+boxHeaderSize > fileSize {
			break
		}
		n, err := readAt(ctx, r, header, position)
		if n < boxHeaderSize {
			if err == nil || errors.Is(err, io.EOF) {
				err = io.ErrUnexpectedEOF
			}
			return nil, err
		}
		size := int64(binary.BigEndian.Uint32(header[:4]))
		boxType := string(header[4:8])
		headerSize := int64(boxHeaderSize)
		switch size {
		case 0:
			size = fileSize - position
		case 1:
			if n < 16 {
				return nil, fmt.Errorf("%w: box %q header is truncated", errSeekIndexUnsupported, boxType)
			}
			size = int64(binary.BigEndian.Uint64(header[8:16]))
			headerSize = 16
		}
		if size < headerSize || size > fileSize-position {
			return nil, fmt.Errorf("%w: box %q has size %d", errSeekIndexUnsupported, boxType, size)
		}
		if boxType == "moov" {
			if size-headerSize > maxMovieBoxSize {
				return nil, fmt.Errorf("%w: moov is too large (%d bytes)", errSeekIndexUnsupported, size)
			}
			movie := make([]byte, size-headerSize)
			if _, err := readAt(ctx, r, movie, position+headerSize); err != nil {
				return nil, err
			}
			return movie, nil
		}
		position += size
	}
	return nil, fmt.Errorf("%w: no moov", errSeekIndexUnsupported)
}

// parseKeyframeTrack は moov の中身から、ffmpeg が選ぶのと同じ最初の映像トラックの
// キーフレームを読む。そのトラックが H.264／HEVC でなければ errSeekIndexUnsupported を返す。
func parseKeyframeTrack(movie []byte) (*keyframeTrack, error) {
	boxes, err := parseBoxes(movie)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errSeekIndexUnsupported, err)
	}
	var movieTimescaleValue uint64
	for _, box := range boxes {
		if box.boxType == "mvhd" {
			if movieTimescaleValue, err = movieTimescale(box.payload); err != nil {
				return nil, fmt.Errorf("%w: %w", errSeekIndexUnsupported, err)
			}
		}
	}
	for _, box := range boxes {
		if box.boxType != "trak" {
			continue
		}
		track, err := parseVideoTrak(box.children, movieTimescaleValue)
		if errors.Is(err, errNotVideoTrack) {
			continue
		}
		return track, err
	}
	return nil, fmt.Errorf("%w: no video track", errSeekIndexUnsupported)
}

// errNotVideoTrack は、映像でない、または表紙のような 1 枚だけの画像のトラックを表す。
// ffmpeg の 0:V:0 と同じく、こうしたトラックは飛ばして次を見る。
var errNotVideoTrack = errors.New("not a video track")

func parseVideoTrak(trak []mp4Box, movieTimescaleValue uint64) (*keyframeTrack, error) {
	mdia, err := childPayload(trak, "mdia")
	if err != nil {
		return nil, errNotVideoTrack
	}
	mdiaBoxes, err := parseBoxes(mdia)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errSeekIndexUnsupported, err)
	}
	hdlr, err := childPayload(mdiaBoxes, "hdlr")
	if err != nil || len(hdlr) < 12 || string(hdlr[8:12]) != "vide" {
		return nil, errNotVideoTrack
	}
	mdhd, err := childPayload(mdiaBoxes, "mdhd")
	if err != nil {
		return nil, fmt.Errorf("%w: no mdhd", errSeekIndexUnsupported)
	}
	timescale, err := movieTimescale(mdhd)
	if err != nil || timescale == 0 {
		return nil, fmt.Errorf("%w: cannot read mdhd timescale", errSeekIndexUnsupported)
	}
	stbl, err := nestedPayload(mdiaBoxes, "minf", "stbl")
	if err != nil {
		return nil, fmt.Errorf("%w: no stbl", errSeekIndexUnsupported)
	}
	boxes, err := parseBoxes(stbl)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errSeekIndexUnsupported, err)
	}
	table, err := readSampleTable(boxes)
	if err != nil {
		return nil, err
	}

	track := &keyframeTrack{}
	if err := track.readDecoderConfig(boxes); err != nil {
		// 表紙のような 1 枚だけの画像は、ffmpeg と同じく映像として選ばない。
		if table.count <= 1 {
			return nil, errNotVideoTrack
		}
		return nil, err
	}
	if table.count == 0 {
		return nil, fmt.Errorf("%w: sample table is empty (fragmented MP4 or similar)", errSeekIndexUnsupported)
	}
	if track.displayFilter, err = displayFilter(trak); err != nil {
		return nil, err
	}
	sync, err := syncSamples(boxes, table.count)
	if err != nil {
		return nil, err
	}
	decode, err := table.decodeTimes(sync)
	if err != nil {
		return nil, err
	}
	offsets, err := table.offsets(sync)
	if err != nil {
		return nil, err
	}
	composition := table.compositionOffsets(sync)
	shift, endUs, err := presentationWindow(trak, timescale, movieTimescaleValue)
	if err != nil {
		return nil, err
	}

	track.keyframes = make([]keyframe, len(sync))
	for i, sample := range sync {
		track.keyframes[i] = keyframe{
			presentationUs: ticksToMicroseconds(decode[i]+composition[i]-shift, int64(timescale)),
			offset:         offsets[i],
			size:           table.size(sample),
		}
	}
	sort.SliceStable(track.keyframes, func(i, j int) bool {
		return track.keyframes[i].presentationUs < track.keyframes[j].presentationUs
	})
	// 編集で切り取られて再生されない後ろのキーフレームは選ばない。すべてが範囲の外なら
	// 最初の 1 枚だけを残す。
	if endUs > 0 {
		end := sort.Search(len(track.keyframes), func(i int) bool { return track.keyframes[i].presentationUs >= endUs })
		track.keyframes = track.keyframes[:max(end, 1)]
	}
	return track, nil
}

// displayFilter は tkhd の表示行列を読み、ffmpeg の自動回転と同じ変換のフィルタを返す。
// 行列の a・b・c・d（16.16 固定小数点）が 90 度単位の回転を表すときだけ扱い、
// 反転や拡大縮小を含む行列は errSeekIndexUnsupported にして区間ごとの抽出に任せる。
func displayFilter(trak []mp4Box) (string, error) {
	tkhd, err := childPayload(trak, "tkhd")
	if err != nil {
		return "", nil
	}
	// 版 0 は作成・更新時刻と長さが 4 バイト、版 1 は 8 バイトで、行列はその後ろにある。
	offset := 40
	if len(tkhd) > 0 && tkhd[0] == 1 {
		offset = 52
	}
	if len(tkhd) < offset+36 {
		return "", fmt.Errorf("%w: tkhd is too short", errSeekIndexUnsupported)
	}
	var m [9]int32
	for i := range m {
		m[i] = int32(binary.BigEndian.Uint32(tkhd[offset+4*i:]))
	}
	const one = 1 << 16
	if m[2] != 0 || m[5] != 0 {
		return "", fmt.Errorf("%w: tkhd display matrix contains a projection", errSeekIndexUnsupported)
	}
	switch [4]int32{m[0], m[1], m[3], m[4]} {
	case [4]int32{one, 0, 0, one}:
		return "", nil
	case [4]int32{0, -one, one, 0}:
		return "transpose=cclock", nil
	case [4]int32{0, one, -one, 0}:
		return "transpose=clock", nil
	case [4]int32{-one, 0, 0, -one}:
		return "hflip,vflip", nil
	}
	return "", fmt.Errorf("%w: tkhd display matrix %v", errSeekIndexUnsupported, m)
}

// readDecoderConfig は sample description（stsd）の entry から形式とパラメータセットを
// 読む。entry が複数ある（途中で解像度などが変わる）トラックは扱わない。
func (t *keyframeTrack) readDecoderConfig(table []mp4Box) error {
	stsd, err := childPayload(table, "stsd")
	if err != nil || len(stsd) < 8 {
		return fmt.Errorf("%w: no stsd", errSeekIndexUnsupported)
	}
	if count := binary.BigEndian.Uint32(stsd[4:8]); count != 1 {
		return fmt.Errorf("%w: stsd has %d entries", errSeekIndexUnsupported, count)
	}
	entries, err := parseBoxes(stsd[8:])
	if err != nil || len(entries) == 0 {
		return fmt.Errorf("%w: cannot read stsd entry", errSeekIndexUnsupported)
	}
	entry := entries[0]
	// VisualSampleEntry の固定部分（78 バイト）の後ろに avcC などの box が続く。
	const visualSampleEntrySize = 78
	var configType string
	switch entry.boxType {
	case "avc1", "avc3":
		t.format, configType = "h264", "avcC"
	case "hvc1", "hev1":
		t.format, configType = "hevc", "hvcC"
	default:
		return fmt.Errorf("%w: video format %q", errSeekIndexUnsupported, entry.boxType)
	}
	if len(entry.payload) < visualSampleEntrySize {
		return fmt.Errorf("%w: %s entry is too short", errSeekIndexUnsupported, entry.boxType)
	}
	extensions, err := parseBoxes(entry.payload[visualSampleEntrySize:])
	if err != nil {
		return fmt.Errorf("%w: %w", errSeekIndexUnsupported, err)
	}
	config, err := childPayload(extensions, configType)
	if err != nil {
		return fmt.Errorf("%w: no %s", errSeekIndexUnsupported, configType)
	}
	if t.format == "h264" {
		return t.readAVCConfig(config)
	}
	return t.readHEVCConfig(config)
}

func (t *keyframeTrack) readAVCConfig(config []byte) error {
	if len(config) < 7 {
		return fmt.Errorf("%w: avcC is too short", errSeekIndexUnsupported)
	}
	t.lengthSize = int(config[4]&3) + 1
	rest := config[5:]
	// SPS の数は下位 5 ビット、PPS の数は次の 1 バイトにある。
	for _, mask := range []byte{0x1f, 0xff} {
		if len(rest) < 1 {
			return fmt.Errorf("%w: avcC is truncated", errSeekIndexUnsupported)
		}
		count := int(rest[0] & mask)
		rest = rest[1:]
		for range count {
			var unit []byte
			var err error
			if unit, rest, err = lengthPrefixed(rest); err != nil {
				return err
			}
			t.parameterSets = append(t.parameterSets, unit)
		}
	}
	return nil
}

func (t *keyframeTrack) readHEVCConfig(config []byte) error {
	if len(config) < 23 {
		return fmt.Errorf("%w: hvcC is too short", errSeekIndexUnsupported)
	}
	t.lengthSize = int(config[21]&3) + 1
	arrays := int(config[22])
	rest := config[23:]
	for range arrays {
		if len(rest) < 3 {
			return fmt.Errorf("%w: hvcC is truncated", errSeekIndexUnsupported)
		}
		count := int(binary.BigEndian.Uint16(rest[1:3]))
		rest = rest[3:]
		for range count {
			var unit []byte
			var err error
			if unit, rest, err = lengthPrefixed(rest); err != nil {
				return err
			}
			t.parameterSets = append(t.parameterSets, unit)
		}
	}
	return nil
}

// lengthPrefixed は 2 バイトの長さに続く中身を切り出し、残りと一緒に返す。
func lengthPrefixed(data []byte) ([]byte, []byte, error) {
	if len(data) < 2 {
		return nil, nil, fmt.Errorf("%w: parameter set is truncated", errSeekIndexUnsupported)
	}
	size := int(binary.BigEndian.Uint16(data))
	if len(data) < 2+size {
		return nil, nil, fmt.Errorf("%w: parameter set is truncated", errSeekIndexUnsupported)
	}
	return data[2 : 2+size], data[2+size:], nil
}

// sampleTable は stbl の表で、値はキーフレームの分だけ取り出す。sample の数だけの
// 配列は作らないので、小さな表が大きな sample 数を名乗っても確保は増えない。
type sampleTable struct {
	count     int
	fixedSize int64
	// sizes は stsz の各 sample の大きさの並び（fixedSize が 0 のとき）である。
	sizes []byte
	// timeToSample・compositionToSample・sampleToChunk・chunkOffsets は各 box の entry である。
	timeToSample        [][]byte
	compositionToSample [][]byte
	sampleToChunk       [][]byte
	chunkOffsets        [][]byte
}

func readSampleTable(boxes []mp4Box) (*sampleTable, error) {
	stsz, err := childPayload(boxes, "stsz")
	if err != nil || len(stsz) < 12 {
		return nil, fmt.Errorf("%w: no stsz", errSeekIndexUnsupported)
	}
	table := &sampleTable{
		fixedSize: int64(binary.BigEndian.Uint32(stsz[4:8])),
		count:     int(binary.BigEndian.Uint32(stsz[8:12])),
	}
	if table.fixedSize == 0 {
		if len(stsz)-12 < table.count*4 {
			return nil, fmt.Errorf("%w: stsz has too few entries", errSeekIndexUnsupported)
		}
		table.sizes = stsz[12:]
	}
	if table.timeToSample, err = requiredEntries(boxes, "stts", 8); err != nil {
		return nil, err
	}
	if table.sampleToChunk, err = requiredEntries(boxes, "stsc", 12); err != nil {
		return nil, err
	}
	if table.chunkOffsets, err = requiredEntries(boxes, "stco", 4); err != nil {
		if table.chunkOffsets, err = requiredEntries(boxes, "co64", 8); err != nil {
			return nil, fmt.Errorf("%w: no stco/co64", errSeekIndexUnsupported)
		}
	}
	if ctts, err := childPayload(boxes, "ctts"); err == nil {
		if table.compositionToSample, err = fullBoxEntries(ctts, 8, "ctts"); err != nil {
			return nil, err
		}
	}
	return table, nil
}

func requiredEntries(boxes []mp4Box, boxType string, entrySize int) ([][]byte, error) {
	payload, err := childPayload(boxes, boxType)
	if err != nil {
		return nil, err
	}
	return fullBoxEntries(payload, entrySize, boxType)
}

func (s *sampleTable) size(sample int) int64 {
	if s.fixedSize != 0 {
		return s.fixedSize
	}
	return int64(binary.BigEndian.Uint32(s.sizes[4*sample:]))
}

// decodeTimes は stts から、昇順の samples それぞれの復号時刻（media timescale）を求める。
// stts が stsz より多くの sample を数えていても、ffmpeg と同じく余りは使わない。
func (s *sampleTable) decodeTimes(samples []int) ([]int64, error) {
	times := make([]int64, len(samples))
	next, first := 0, 0
	var start int64
	for _, entry := range s.timeToSample {
		run := int(binary.BigEndian.Uint32(entry[0:4]))
		delta := int64(binary.BigEndian.Uint32(entry[4:8]))
		for next < len(samples) && samples[next] < first+run {
			times[next] = start + int64(samples[next]-first)*delta
			next++
		}
		if first += run; first >= s.count {
			break
		}
		start += int64(run) * delta
	}
	if first < s.count {
		return nil, fmt.Errorf("%w: stts sample count %d is less than stsz count %d", errSeekIndexUnsupported, first, s.count)
	}
	return times, nil
}

// compositionOffsets は ctts から、昇順の samples それぞれの表示時刻と復号時刻の差を
// 求める。ctts が無い、または足りない sample の差は 0 である。版 0 の値も符号付きとして
// 読む（ffmpeg と同じ）。
func (s *sampleTable) compositionOffsets(samples []int) []int64 {
	offsets := make([]int64, len(samples))
	next, first := 0, 0
	for _, entry := range s.compositionToSample {
		run := int(binary.BigEndian.Uint32(entry[0:4]))
		offset := int64(int32(binary.BigEndian.Uint32(entry[4:8])))
		for next < len(samples) && samples[next] < first+run {
			offsets[next] = offset
			next++
		}
		if first += run; first > s.count {
			break
		}
	}
	return offsets
}

// offsets は stsc と stco／co64 から、昇順の samples それぞれのファイル内の位置を求める。
func (s *sampleTable) offsets(samples []int) ([]int64, error) {
	positions := make([]int64, len(samples))
	next, first := 0, 0
	for i, run := range s.sampleToChunk {
		firstChunk := int(binary.BigEndian.Uint32(run[0:4])) - 1
		perChunk := int(binary.BigEndian.Uint32(run[4:8]))
		lastChunk := len(s.chunkOffsets)
		if i+1 < len(s.sampleToChunk) {
			lastChunk = min(lastChunk, int(binary.BigEndian.Uint32(s.sampleToChunk[i+1][0:4]))-1)
		}
		if firstChunk < 0 || firstChunk > lastChunk {
			return nil, fmt.Errorf("%w: stsc chunk number %d", errSeekIndexUnsupported, firstChunk+1)
		}
		for chunk := firstChunk; chunk < lastChunk && first < s.count; chunk++ {
			end := first + min(perChunk, s.count-first)
			for s.fixedSize != 0 && next < len(samples) && samples[next] < end {
				positions[next] = s.chunkOffset(chunk) + int64(samples[next]-first)*s.fixedSize
				next++
			}
			if next < len(samples) && samples[next] < end {
				position := s.chunkOffset(chunk)
				for sample := first; sample < end && next < len(samples); sample++ {
					if samples[next] == sample {
						positions[next] = position
						next++
					}
					position += s.size(sample)
				}
			}
			first = end
		}
	}
	if first != s.count {
		return nil, fmt.Errorf("%w: chunks hold %d samples (expected %d)", errSeekIndexUnsupported, first, s.count)
	}
	return positions, nil
}

func (s *sampleTable) chunkOffset(chunk int) int64 {
	entry := s.chunkOffsets[chunk]
	if len(entry) == 8 {
		return int64(binary.BigEndian.Uint64(entry))
	}
	return int64(binary.BigEndian.Uint32(entry))
}

// syncSamples は stss からキーフレームの sample 番号（0 始まり、昇順）を読む。stss が
// 無いトラックでは、すべての sample がキーフレームである。
func syncSamples(boxes []mp4Box, count int) ([]int, error) {
	stss, err := childPayload(boxes, "stss")
	if err != nil {
		if count > maxKeyframes {
			return nil, fmt.Errorf("%w: %d keyframes", errSeekIndexUnsupported, count)
		}
		all := make([]int, count)
		for i := range all {
			all[i] = i
		}
		return all, nil
	}
	entries, err := fullBoxEntries(stss, 4, "stss")
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 || len(entries) > maxKeyframes {
		return nil, fmt.Errorf("%w: %d keyframes", errSeekIndexUnsupported, len(entries))
	}
	samples := make([]int, 0, len(entries))
	for _, entry := range entries {
		number := int(binary.BigEndian.Uint32(entry))
		if number < 1 || number > count {
			return nil, fmt.Errorf("%w: stss sample number %d", errSeekIndexUnsupported, number)
		}
		samples = append(samples, number-1)
	}
	// 後の表の読み取りは昇順で重複の無い並びを前提にする。
	sort.Ints(samples)
	return slices.Compact(samples), nil
}

// presentationWindow は edit list による時刻のずれ（media timescale）と、再生される
// 範囲の終わりの表示時刻（マイクロ秒、終わりが無ければ 0）を返す。表示時刻は sample の
// 時刻からずれを引いたものになる。空でない edit の media_time から始まり、先頭の空の
// edit の長さだけ遅れ、その edit の長さで終わる。途中を切り取る（空でない edit が
// 複数ある）トラックは扱わない。
func presentationWindow(trak []mp4Box, timescale, movieTimescaleValue uint64) (int64, int64, error) {
	edts, err := childPayload(trak, "edts")
	if err != nil {
		return 0, 0, nil
	}
	edtsBoxes, err := parseBoxes(edts)
	if err != nil {
		return 0, 0, fmt.Errorf("%w: %w", errSeekIndexUnsupported, err)
	}
	elst, err := childPayload(edtsBoxes, "elst")
	if err != nil {
		return 0, 0, nil
	}
	list, err := parseEditList(elst)
	if err != nil {
		return 0, 0, fmt.Errorf("%w: %w", errSeekIndexUnsupported, err)
	}
	var shift int64
	var duration uint64
	edits := 0
	for _, entry := range list.entries {
		if !entry.empty() {
			shift, duration = entry.mediaTime, entry.segmentDuration
			edits++
		}
	}
	if edits > 1 {
		return 0, 0, fmt.Errorf("%w: edit list has %d edits", errSeekIndexUnsupported, edits)
	}
	if delay := list.startDelay(); delay > 0 && movieTimescaleValue > 0 {
		if delay > math.MaxInt64/timescale {
			return 0, 0, fmt.Errorf("%w: edit list start delay %d", errSeekIndexUnsupported, delay)
		}
		shift -= int64(delay * timescale / movieTimescaleValue)
	}
	var endUs int64
	if delay := list.startDelay(); edits == 1 && duration > 0 && movieTimescaleValue > 0 && delay <= math.MaxInt64 && duration <= math.MaxInt64-delay {
		endUs = ticksToMicroseconds(int64(delay+duration), int64(movieTimescaleValue))
	}
	return shift, endUs, nil
}

// fullBoxEntries は version・flags と entry の数に続く、固定長の entry の並びを切り出す。
func fullBoxEntries(payload []byte, entrySize int, boxType string) ([][]byte, error) {
	if len(payload) < 8 {
		return nil, fmt.Errorf("%w: %s is too short", errSeekIndexUnsupported, boxType)
	}
	count := int64(binary.BigEndian.Uint32(payload[4:8]))
	data := payload[8:]
	if int64(len(data)) < count*int64(entrySize) {
		return nil, fmt.Errorf("%w: %s has too few entries", errSeekIndexUnsupported, boxType)
	}
	entries := make([][]byte, count)
	for i := range entries {
		entries[i] = data[i*entrySize : (i+1)*entrySize]
	}
	return entries, nil
}

func childPayload(boxes []mp4Box, boxType string) ([]byte, error) {
	for _, box := range boxes {
		if box.boxType == boxType {
			return box.payload, nil
		}
	}
	return nil, fmt.Errorf("%w: no %s", errSeekIndexUnsupported, boxType)
}

func nestedPayload(boxes []mp4Box, path ...string) ([]byte, error) {
	var payload []byte
	for i, boxType := range path {
		var err error
		if payload, err = childPayload(boxes, boxType); err != nil {
			return nil, err
		}
		if i+1 < len(path) {
			if boxes, err = parseBoxes(payload); err != nil {
				return nil, fmt.Errorf("%w: %w", errSeekIndexUnsupported, err)
			}
		}
	}
	return payload, nil
}

func ticksToMicroseconds(ticks, timescale int64) int64 {
	return ticks/timescale*1_000_000 + ticks%timescale*1_000_000/timescale
}

// pick はスプライトのコマ k ごとに使うキーフレームの番号を返す。コマ k の区間
// [k*IntervalMs, (k+1)*IntervalMs) の中の最初のキーフレームを選ぶ。区間の中に無ければ
// 区間より前の最後のキーフレーム（直前の場面）を、それも無ければ最初のキーフレームを選ぶ。
// 映像が容器より早く終わる末尾の区間も、こうして必ずどれかの場面になる。
func (t *keyframeTrack) pick(layout domain.SeekSpriteLayout) []int {
	picks := make([]int, layout.FrameCount)
	for k := range picks {
		startUs := int64(k) * layout.IntervalMs * 1000
		endUs := startUs + layout.IntervalMs*1000
		i := sort.Search(len(t.keyframes), func(i int) bool { return t.keyframes[i].presentationUs >= startUs })
		switch {
		case i < len(t.keyframes) && t.keyframes[i].presentationUs < endUs:
			picks[k] = i
		case i > 0:
			picks[k] = i - 1
		default:
			picks[k] = 0
		}
	}
	return picks
}

// annexB は 1 枚のキーフレーム（長さの欄で区切った NAL の並び）を、パラメータセットを
// 前に置いた開始コード区切りの並びにする。ffmpeg は -f h264／hevc で読む。
// キーフレームが IDR かどうかも返す。
func (t *keyframeTrack) annexB(sample []byte) ([]byte, bool, error) {
	startCode := []byte{0, 0, 0, 1}
	var out []byte
	for _, unit := range t.parameterSets {
		out = append(out, startCode...)
		out = append(out, unit...)
	}
	idr := false
	for len(sample) > 0 {
		if len(sample) < t.lengthSize {
			return nil, false, errors.New("keyframe NAL length is truncated")
		}
		var size int
		for _, b := range sample[:t.lengthSize] {
			size = size<<8 | int(b)
		}
		sample = sample[t.lengthSize:]
		if size > len(sample) {
			return nil, false, errors.New("keyframe NAL is truncated")
		}
		if size > 0 && t.isIDR(sample[0]) {
			idr = true
		}
		out = append(out, startCode...)
		out = append(out, sample[:size]...)
		sample = sample[size:]
	}
	return out, idr, nil
}

// isIDR は NAL の先頭バイトが IDR のスライス（H.264 は型 5、HEVC は型 19・20）かを返す。
func (t *keyframeTrack) isIDR(header byte) bool {
	if t.format == "h264" {
		return header&0x1f == 5
	}
	nalType := header >> 1 & 0x3f
	return nalType == 19 || nalType == 20
}

// readAt は r.ReadAt を行い、ctx が終わったら読み取りの終わりを待たずに戻る。
// 応答しない置き場で、生成の期限や停止を過ぎてもジョブが止まり続けないようにする。
// 戻った後も読み取りは続きうるので、呼び出し側は p を再利用しない。
func readAt(ctx context.Context, r io.ReaderAt, p []byte, offset int64) (int, error) {
	type result struct {
		n   int
		err error
	}
	done := make(chan result, 1)
	go func() {
		n, err := r.ReadAt(p, offset)
		done <- result{n, err}
	}()
	select {
	case res := <-done:
		return res.n, res.err
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}
