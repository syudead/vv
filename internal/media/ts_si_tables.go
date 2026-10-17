package media

import (
	"bytes"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/media/arib"
)

// TS packet から SI の section を組み立て、PAT・SDT・EIT present/following・TOT/TDT を読む
// 部分。値は section の中の位置で読み、長さが
// 足りない section や CRC の合わない section は捨てる。

// japaneseSatelliteNetworks は ARIB の衛星放送の original_network_id（スカパー！、BS、
// 110 度 CS）である。地上波は japaneseTerrestrialNetworks の範囲にある。
var japaneseSatelliteNetworks = map[int]bool{0x0001: true, 0x0003: true, 0x0004: true, 0x0006: true, 0x0007: true, 0x000A: true}

// japaneseTerrestrialNetworks は地上デジタル放送の original_network_id の範囲である。
var japaneseTerrestrialNetworks = [2]int{0x7880, 0x7FE8}

// siCollector は SI の section から値を集める。
type siCollector struct {
	sections map[uint16]*sectionBuffer
	// serviceID は PAT の最初の番組の program_number。PAT を見るまでは -1。
	serviceID int
	// networkID は最初に見た SDT か EIT の original_network_id。見るまでは -1。
	networkID int
	services  map[int][]byte
	events    map[int]*siEventPair
	time      time.Time
}

// siEventPair は 1 つの service の present（section 0）と following（section 1）である。
// event の無い section も見たものとして数える。japaneseLanguage と foreignLanguage は、
// この service の event に日本語の・日本語でない短形式イベント記述子を見たかである。EIT は
// 同じ PID に TS の全 service の event を載せるので、言語は service ごとに持つ。
type siEventPair struct {
	seen             [2]bool
	events           [2]*siEvent
	japaneseLanguage bool
	foreignLanguage  bool
}

// siEvent は decode する前の EIT の event である。
type siEvent struct {
	name     []byte
	start    time.Time
	duration time.Duration
}

func newSICollector() *siCollector {
	return &siCollector{
		sections: map[uint16]*sectionBuffer{
			pidPAT: {}, pidSDT: {}, pidEIT: {}, pidTime: {},
		},
		serviceID: -1,
		networkID: -1,
		services:  map[int][]byte{},
		events:    map[int]*siEventPair{},
	}
}

// packet は 188 バイトの TS packet 1 つを読む。
func (c *siCollector) packet(packet []byte) {
	if packet[1]&0x80 != 0 {
		// transport_error_indicator の立った packet は読まない。
		return
	}
	pid := uint16(packet[1]&0x1F)<<8 | uint16(packet[2])
	buffer, ok := c.sections[pid]
	if !ok {
		return
	}
	if payload, ok := tsPayload(packet); ok {
		buffer.push(payload, packet[1]&0x40 != 0, c.section)
	}
}

// complete は局名・present と following・時刻がそろったかを返す。
func (c *siCollector) complete() bool {
	if c.serviceID < 0 || c.time.IsZero() {
		return false
	}
	if _, ok := c.services[c.serviceID]; !ok {
		return false
	}
	pair := c.events[c.serviceID]
	return pair != nil && pair.seen[0] && pair.seen[1]
}

// result は集めた値を返す。名前は PAT の最初の番組の service のものだけを decode する。
// ARIB の放送でない（DVB の）録画の名前は decode せず省く。
func (c *siCollector) result() transportStreamSI {
	si := transportStreamSI{Time: c.time}
	if c.serviceID < 0 || !c.isARIB() {
		return si
	}
	if name, ok := c.services[c.serviceID]; ok {
		si.Station = arib.Decode(name)
	}
	if pair := c.events[c.serviceID]; pair != nil {
		si.Present = pair.events[0].broadcastEvent()
		si.Following = pair.events[1].broadcastEvent()
	}
	return si
}

// isARIB は録画が日本の放送（ARIB）かを返す。original_network_id が日本の放送網で、
// PAT の最初の番組の service に日本語の event を見て、日本語でない event を見ていなければ
// ARIB とする。DVB の衛星には日本の衛星と同じ original_network_id を使うものがあり、ffmpeg が
// 作る TS も original_network_id 1 の SDT を持つ（EIT は持たない）ので、event の言語でも
// 確かめる。ほかの service の event の言語は見ない。
func (c *siCollector) isARIB() bool {
	pair := c.events[c.serviceID]
	if pair == nil || pair.foreignLanguage || !pair.japaneseLanguage {
		return false
	}
	return japaneseSatelliteNetworks[c.networkID] ||
		(c.networkID >= japaneseTerrestrialNetworks[0] && c.networkID <= japaneseTerrestrialNetworks[1])
}

func (e *siEvent) broadcastEvent() *domain.BroadcastEvent {
	if e == nil {
		return nil
	}
	return &domain.BroadcastEvent{Name: arib.Decode(e.name), Start: e.start, Duration: e.duration}
}

// section は組み立てた section 1 つを読む。CRC を持つ section は CRC を確かめる。
func (c *siCollector) section(section []byte) {
	if section[1]&0x80 != 0 || section[0] == tableTOT {
		if len(section) < 7 || crc32MPEG(section) != 0 {
			return
		}
	}
	switch section[0] {
	case tablePAT:
		c.pat(section)
	case tableSDT:
		c.sdt(section)
	case tableEIT:
		c.eit(section)
	case tableTDT, tableTOT:
		c.clock(section)
	}
}

// pat は最初の番組（program_number が 0 でないもの）を記録する。
func (c *siCollector) pat(section []byte) {
	if c.serviceID >= 0 || len(section) < 12 {
		return
	}
	for entry := section[8 : len(section)-4]; len(entry) >= 4; entry = entry[4:] {
		if number := int(entry[0])<<8 | int(entry[1]); number != 0 {
			c.serviceID = number
			return
		}
	}
}

// sdt は各 service の service name（サービス記述子）を記録する。
func (c *siCollector) sdt(section []byte) {
	if len(section) < 15 {
		return
	}
	c.noteNetwork(int(section[8])<<8 | int(section[9]))
	for body := section[11 : len(section)-4]; len(body) >= 5; {
		serviceID := int(body[0])<<8 | int(body[1])
		loop := int(body[3]&0x0F)<<8 | int(body[4])
		if 5+loop > len(body) {
			return
		}
		if _, seen := c.services[serviceID]; !seen {
			if name, ok := serviceName(body[5 : 5+loop]); ok {
				c.services[serviceID] = name
			}
		}
		body = body[5+loop:]
	}
}

// serviceName はサービス記述子の service name を返す。
func serviceName(descriptors []byte) ([]byte, bool) {
	var name []byte
	found := false
	forEachDescriptor(descriptors, func(tag byte, body []byte) {
		if found || tag != descriptorService || len(body) < 2 {
			return
		}
		rest := body[2:]
		providerLength := int(body[1])
		if providerLength >= len(rest) {
			return
		}
		nameLength := int(rest[providerLength])
		rest = rest[providerLength+1:]
		if nameLength > len(rest) {
			return
		}
		name, found = bytes.Clone(rest[:nameLength]), true
	})
	return name, found
}

// eit は present（section 0）と following（section 1）の event を記録する。
func (c *siCollector) eit(section []byte) {
	if len(section) < 18 || section[6] > 1 {
		return
	}
	number := int(section[6])
	pair := c.eventPair(int(section[3])<<8 | int(section[4]))
	c.noteNetwork(int(section[10])<<8 | int(section[11]))
	if pair.seen[number] {
		return
	}
	pair.seen[number] = true
	body := section[14 : len(section)-4]
	if len(body) < 12 {
		// event の無い section（following の無い時間帯など）。
		return
	}
	loop := int(body[10]&0x0F)<<8 | int(body[11])
	if 12+loop > len(body) {
		return
	}
	event := &siEvent{start: parseSITime(body[2:7]), duration: parseSIDuration(body[7:10])}
	forEachDescriptor(body[12:12+loop], func(tag byte, descriptor []byte) {
		if tag != descriptorShortEvent || len(descriptor) < 4 || event.name != nil {
			return
		}
		if bytes.EqualFold(descriptor[:3], []byte("jpn")) {
			pair.japaneseLanguage = true
		} else {
			pair.foreignLanguage = true
		}
		if nameLength := int(descriptor[3]); 4+nameLength <= len(descriptor) {
			event.name = bytes.Clone(descriptor[4 : 4+nameLength])
		}
	})
	pair.events[number] = event
}

func (c *siCollector) eventPair(serviceID int) *siEventPair {
	pair, ok := c.events[serviceID]
	if !ok {
		pair = &siEventPair{}
		c.events[serviceID] = pair
	}
	return pair
}

func (c *siCollector) noteNetwork(networkID int) {
	if c.networkID < 0 {
		c.networkID = networkID
	}
}

// clock は最初の TOT か TDT の時刻を記録する。
func (c *siCollector) clock(section []byte) {
	if !c.time.IsZero() || len(section) < 8 {
		return
	}
	c.time = parseSITime(section[3:8])
}

// forEachDescriptor は記述子の並びを 1 つずつ visit に渡す。長さが足りなければそこでやめる。
func forEachDescriptor(descriptors []byte, visit func(tag byte, body []byte)) {
	for len(descriptors) >= 2 {
		length := int(descriptors[1])
		if 2+length > len(descriptors) {
			return
		}
		visit(descriptors[0], descriptors[2:2+length])
		descriptors = descriptors[2+length:]
	}
}

// tsPayload は TS packet の payload を返す。adaptation field を飛ばし、payload の無い
// packet では false を返す。
func tsPayload(packet []byte) ([]byte, bool) {
	control := packet[3] >> 4 & 0x3
	payload := packet[4:]
	if control&0x2 != 0 {
		if len(payload) == 0 || int(payload[0])+1 > len(payload) {
			return nil, false
		}
		payload = payload[int(payload[0])+1:]
	}
	return payload, control&0x1 != 0
}

// sectionBuffer は 1 つの PID の section を packet から組み立てる。
type sectionBuffer struct {
	data   []byte
	active bool
}

// push は packet の payload を足し、そろった section を emit に渡す。unitStart の packet は
// 先頭の pointer_field の後で新しい section が始まる。
func (b *sectionBuffer) push(payload []byte, unitStart bool, emit func([]byte)) {
	if unitStart {
		if len(payload) == 0 || int(payload[0]) >= len(payload) {
			b.reset()
			return
		}
		pointer := int(payload[0])
		payload = payload[1:]
		if b.active {
			b.data = append(b.data, payload[:pointer]...)
			b.drain(emit)
		}
		b.data = append(b.data[:0], payload[pointer:]...)
		b.active = true
	} else if b.active {
		b.data = append(b.data, payload...)
	} else {
		return
	}
	b.drain(emit)
}

// drain はそろった section を先頭から emit に渡す。stuffing（0xFF）か壊れた長さで止める。
func (b *sectionBuffer) drain(emit func([]byte)) {
	for b.active && len(b.data) >= 3 {
		length := 3 + (int(b.data[1]&0x0F)<<8 | int(b.data[2]))
		if b.data[0] == 0xFF || length > maxSectionSize {
			b.reset()
			return
		}
		if len(b.data) < length {
			return
		}
		emit(b.data[:length])
		b.data = b.data[length:]
	}
}

func (b *sectionBuffer) reset() {
	b.data = b.data[:0]
	b.active = false
}

// parseSITime は MJD 16 ビットと BCD の時分秒 24 ビットの時刻（日本標準時）を読む。全ビット
// 1（未定義）や BCD でない値は零値である。
func parseSITime(field []byte) time.Time {
	if bytes.Equal(field, []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF}) {
		return time.Time{}
	}
	clock, ok := parseBCDClock(field[2:5])
	if !ok || clock[0] > 23 || clock[1] > 59 || clock[2] > 59 {
		return time.Time{}
	}
	mjd := int(field[0])<<8 | int(field[1])
	return time.Date(1858, time.November, 17, clock[0], clock[1], clock[2], 0, broadcastZone).AddDate(0, 0, mjd)
}

// parseSIDuration は BCD の時分秒 24 ビットの長さを読む。全ビット 1（未定義）や BCD でない
// 値は -1（未定義）である。
func parseSIDuration(field []byte) time.Duration {
	clock, ok := parseBCDClock(field)
	if !ok {
		return -1
	}
	return time.Duration(clock[0])*time.Hour + time.Duration(clock[1])*time.Minute + time.Duration(clock[2])*time.Second
}

// parseBCDClock は 3 バイトの BCD（時・分・秒）を読む。
func parseBCDClock(field []byte) ([3]int, bool) {
	var clock [3]int
	for i, b := range field[:3] {
		high, low := int(b>>4), int(b&0x0F)
		if high > 9 || low > 9 {
			return clock, false
		}
		clock[i] = high*10 + low
	}
	return clock, true
}

// crc32Table は MPEG-2 の CRC-32（多項式 0x04C11DB7、反転なし）の表である。
var crc32Table = func() [256]uint32 {
	var table [256]uint32
	for i := range table {
		crc := uint32(i) << 24
		for range 8 {
			if crc&0x80000000 != 0 {
				crc = crc<<1 ^ 0x04C11DB7
			} else {
				crc <<= 1
			}
		}
		table[i] = crc
	}
	return table
}()

// crc32MPEG は data の MPEG-2 の CRC-32 を返す。末尾に CRC を含む section では 0 になる。
func crc32MPEG(data []byte) uint32 {
	crc := uint32(0xFFFFFFFF)
	for _, b := range data {
		crc = crc<<8 ^ crc32Table[byte(crc>>24)^b]
	}
	return crc
}
