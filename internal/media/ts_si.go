package media

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// Read bounded TS SI after ffprobe with an independent deadline; missing broadcast metadata must
// not fail the probe.

const (
	siReadLimit = 32 << 20
	// Independent of ffprobe's timeout.
	siTimeout             = 10 * time.Second
	siContextCheckPackets = 4096
)

// ISO/IEC 13818-1 and ARIB STD-B10 constants.
const (
	tsPacketSize = 188
	tsSyncByte   = 0x47

	pidPAT  = 0x0000
	pidSDT  = 0x0011
	pidEIT  = 0x0012
	pidTime = 0x0014

	tablePAT = 0x00
	tableSDT = 0x42 // SDT for the current transport stream.
	tableEIT = 0x4E // Present/following EIT for the current transport stream.
	tableTDT = 0x70
	tableTOT = 0x73

	descriptorService    = 0x48
	descriptorShortEvent = 0x4D

	maxSectionSize = 4096
)

// TS packets occupy 188 bytes; M2TS adds a four-byte prefix, and FEC TS adds a sixteen-byte suffix.
var tsUnitSizes = []int{tsPacketSize, 192, 204}

const tsSyncCheckPackets = 5

// ARIB timestamps use Japan Standard Time, including present/following and TOT.
var broadcastZone = time.FixedZone("JST", 9*60*60)

// Names contain decoded ARIB strings.
type transportStreamSI struct {
	Station   string
	Present   *domain.BroadcastEvent
	Following *domain.BroadcastEvent
	// First TOT/TDT timestamp; zero when absent.
	Time time.Time
}

// readBroadcast returns nil on read failure or timeout.
func readBroadcast(ctx context.Context, path string, durationMs int64) *domain.MediaBroadcast {
	si, err := readTransportStreamSI(ctx, path)
	if err != nil {
		return nil
	}
	return domain.NewMediaBroadcast(si.Station,
		domain.SelectBroadcastProgram(si.Time, durationMs, si.Present, si.Following))
}

func readTransportStreamSI(ctx context.Context, path string) (transportStreamSI, error) {
	ctx, cancel := context.WithTimeout(ctx, siTimeout)
	defer cancel()
	file, err := os.Open(path)
	if err != nil {
		return transportStreamSI{}, fmt.Errorf("could not open the TS file (%s): %w", path, err)
	}
	defer func() { _ = file.Close() }()
	return scanTransportStreamSI(ctx, io.LimitReader(file, siReadLimit))
}

// Stop once service name, both events, and time are available; EOF returns partial results.
func scanTransportStreamSI(ctx context.Context, r io.Reader) (transportStreamSI, error) {
	packets := &packetReader{r: bufio.NewReaderSize(r, 1<<16)}
	collector := newSICollector()
	for count := 0; !collector.complete(); count++ {
		if count%siContextCheckPackets == 0 {
			if err := ctx.Err(); err != nil {
				return transportStreamSI{}, err
			}
		}
		packet, err := packets.next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return transportStreamSI{}, err
		}
		collector.packet(packet)
	}
	return collector.result(), nil
}

type packetReader struct {
	r *bufio.Reader
	// Zero means packet size is not yet detected.
	unit int
}

// The window checks enough sync bytes even for the largest packet size.
const tsSyncWindow = 204 * (tsSyncCheckPackets + 1)

// Returned slices remain valid only until the next call; lost synchronization triggers a rescan.
func (p *packetReader) next() ([]byte, error) {
	for {
		if p.unit == 0 {
			if err := p.sync(); err != nil {
				return nil, err
			}
		}
		head, err := p.r.Peek(tsPacketSize)
		if err != nil {
			return nil, err
		}
		if head[0] != tsSyncByte {
			p.unit = 0
			continue
		}
		packet := head[:tsPacketSize:tsPacketSize]
		// Extra bytes are the next M2TS prefix or current FEC suffix; an incomplete final discard is
		// harmless because the next Peek returns EOF.
		_, _ = p.r.Discard(p.unit)
		return packet, nil
	}
}

func (p *packetReader) sync() error {
	for {
		window, err := p.r.Peek(tsSyncWindow)
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		atEnd := err != nil
		if len(window) < tsPacketSize {
			return io.EOF
		}
		limit := len(window) - tsSyncWindow + tsUnitSizes[len(tsUnitSizes)-1]
		if atEnd {
			limit = len(window) - tsPacketSize + 1
		}
		for start := range max(limit, 0) {
			if window[start] != tsSyncByte {
				continue
			}
			if unit := syncedUnit(window[start:], atEnd); unit > 0 {
				p.unit = unit
				_, err := p.r.Discard(start)
				return err
			}
		}
		if atEnd {
			return io.EOF
		}
		if _, err := p.r.Discard(limit); err != nil {
			return err
		}
	}
}

// At EOF, check only packets available in data; zero means no recognized packet size.
func syncedUnit(data []byte, atEnd bool) int {
	for _, unit := range tsUnitSizes {
		need := tsSyncCheckPackets
		if atEnd {
			need = min(need, (len(data)-tsPacketSize)/unit)
		}
		if syncRepeats(data, unit, need) {
			return unit
		}
	}
	return 0
}

func syncRepeats(data []byte, unit, need int) bool {
	for k := 1; k <= need; k++ {
		if k*unit >= len(data) || data[k*unit] != tsSyncByte {
			return false
		}
	}
	return true
}
