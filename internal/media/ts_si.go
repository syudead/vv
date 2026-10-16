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

// TS の録画の SI（PAT・SDT・EIT present/following・TOT/TDT）から放送局名と番組名を読む
// 部分（specs/056-media-info/research.md R-4）。ffprobe の後に、ファイルの先頭
// siReadLimit までを自分の siTimeout の期限で読む。読めなかったものは省くだけで、
// プローブを失敗にしない。

const (
	// siReadLimit は SI を探す範囲（ファイルの先頭から）である。
	siReadLimit = 32 << 20
	// siTimeout は SI の読み取りに与える上限で、ffprobe の probeTimeout とは別に数える。
	siTimeout = 10 * time.Second
	// siContextCheckPackets は期限を確かめる間隔（packet 数）である。
	siContextCheckPackets = 4096
)

// TS の packet と SI の定数（ISO/IEC 13818-1、ARIB STD-B10）。
const (
	tsPacketSize = 188
	tsSyncByte   = 0x47

	pidPAT  = 0x0000
	pidSDT  = 0x0011
	pidEIT  = 0x0012
	pidTime = 0x0014

	tablePAT = 0x00
	tableSDT = 0x42 // 自 TS の SDT
	tableEIT = 0x4E // 自 TS の EIT present/following
	tableTDT = 0x70
	tableTOT = 0x73

	descriptorService    = 0x48
	descriptorShortEvent = 0x4D

	maxSectionSize = 4096
)

// tsUnitSizes は 1 packet が占めるバイト数の候補である。188 は .ts、192 は先頭に 4 バイトの
// 時刻を持つ .m2ts・.mts、204 は誤り訂正の 16 バイトを後ろに持つ TS である。
var tsUnitSizes = []int{tsPacketSize, 192, 204}

// tsSyncCheckPackets は packet の大きさを決めるときに同期バイトを確かめる packet 数である。
const tsSyncCheckPackets = 5

// broadcastZone は ARIB の SI の時刻の時間帯（日本標準時）である。present/following と
// TOT は同じ時間帯なので、比べるだけならどちらでもよいが、時刻として正しく持つ。
var broadcastZone = time.FixedZone("JST", 9*60*60)

// transportStreamSI は SI から読んだ値である。名前は ARIB の文字列を decode したもの。
type transportStreamSI struct {
	Station   string
	Present   *domain.BroadcastEvent
	Following *domain.BroadcastEvent
	// Time は最初に見つけた TOT か TDT の時刻。無ければ零値。
	Time time.Time
}

// readBroadcast は TS の録画の放送局名と番組名を読む。読み取りの誤り・期限切れでは nil を返す。
func readBroadcast(ctx context.Context, path string, durationMs int64) *domain.MediaBroadcast {
	si, err := readTransportStreamSI(ctx, path)
	if err != nil {
		return nil
	}
	return domain.NewMediaBroadcast(si.Station,
		domain.SelectBroadcastProgram(si.Time, durationMs, si.Present, si.Following))
}

// readTransportStreamSI は path の先頭 siReadLimit を siTimeout の期限で読む。
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

// scanTransportStreamSI は r の TS packet から SI を集める。局名・present と following・
// 時刻がそろえばそこで読むのをやめる。r の終わりに達したら、それまでに集めた値を返す。
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

// packetReader は同期バイトに合わせて 188 バイトの TS packet を 1 つずつ返す。
type packetReader struct {
	r *bufio.Reader
	// unit は 1 packet が占めるバイト数。0 なら未定で、次の next で決める。
	unit int
}

// tsSyncWindow は同期を探すときに一度に見るバイト数である。最も大きい packet で
// tsSyncCheckPackets 個先まで確かめられる。
const tsSyncWindow = 204 * (tsSyncCheckPackets + 1)

// next は次の packet を返す。同期が外れたら探し直す。返す slice は次の呼び出しまでしか
// 有効でない。データの終わりでは io.EOF を、読み取りの誤りではその誤りを返す。
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
		// unit が 188 より大きいとき、残りは次の packet の前置き（.m2ts）か後置き（FEC）である。
		// 終わりの packet では Discard が足りなくてもよい（次の Peek が io.EOF を返す）。
		_, _ = p.r.Discard(p.unit)
		return packet, nil
	}
}

// sync は同期バイトが等間隔に続く位置と packet の大きさを探し、その位置まで読み飛ばす。
// データの終わりまで見つからなければ io.EOF を返す。
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

// syncedUnit は data の先頭から同期バイトが等間隔に続く packet の大きさを返す。続く
// tsSyncCheckPackets 個の packet の先頭で確かめる。atEnd（データの終わりまで見えている）
// なら、data に収まる packet の分だけ確かめる。見つからなければ 0。
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

// syncRepeats は data の unit おきの need 個の位置がすべて同期バイトかを返す。
func syncRepeats(data []byte, unit, need int) bool {
	for k := 1; k <= need; k++ {
		if k*unit >= len(data) || data[k*unit] != tsSyncByte {
			return false
		}
	}
	return true
}
