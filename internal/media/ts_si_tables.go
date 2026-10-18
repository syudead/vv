package media

import (
	"bytes"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/media/arib"
)

// Assemble SI sections from TS packets; reject truncated sections and CRC mismatches.

// ARIB satellite original_network_id values for Sky PerfecTV, BS, and 110-degree CS.
var japaneseSatelliteNetworks = map[int]bool{0x0001: true, 0x0003: true, 0x0004: true, 0x0006: true, 0x0007: true, 0x000A: true}

var japaneseTerrestrialNetworks = [2]int{0x7880, 0x7FE8}

type siCollector struct {
	sections map[uint16]*sectionBuffer
	// First PAT program_number; -1 before PAT arrives.
	serviceID int
	// First SDT/EIT original_network_id; -1 before either arrives.
	networkID int
	services  map[int][]byte
	events    map[int]*siEventPair
	time      time.Time
}

// Track present/following sections, including empty ones; language detection is per service because
// one PID carries all services' EIT events.
type siEventPair struct {
	seen             [2]bool
	events           [2]*siEvent
	japaneseLanguage bool
	foreignLanguage  bool
}

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

// Reads one 188-byte TS packet.
func (c *siCollector) packet(packet []byte) {
	if packet[1]&0x80 != 0 {
		// Ignore packets with transport_error_indicator.
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

// Decode only the first PAT program's service; omit DVB names rather than treating them as ARIB.
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

// Require a Japanese network ID and exclusively Japanese EIT descriptors for the selected service;
// network IDs alone collide with DVB and ffmpeg-generated TS.
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

// Record the first nonzero program_number.
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
		// Empty sections still count as observed, including absent following events.
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

func (c *siCollector) clock(section []byte) {
	if !c.time.IsZero() || len(section) < 8 {
		return
	}
	c.time = parseSITime(section[3:8])
}

// Stop at truncated descriptors.
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

type sectionBuffer struct {
	data   []byte
	active bool
}

// unitStart payloads begin with pointer_field before the new section.
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

// Stop at stuffing (0xFF) or invalid lengths.
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

// Read sixteen-bit MJD plus BCD time in JST; undefined/all-ones or invalid BCD returns zero time.
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

// Undefined/all-ones or invalid BCD duration returns -1.
func parseSIDuration(field []byte) time.Duration {
	clock, ok := parseBCDClock(field)
	if !ok {
		return -1
	}
	return time.Duration(clock[0])*time.Hour + time.Duration(clock[1])*time.Minute + time.Duration(clock[2])*time.Second
}

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

// MPEG-2 CRC-32 uses polynomial 0x04C11DB7 without reflection.
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

// A complete section including its CRC has a zero remainder.
func crc32MPEG(data []byte) uint32 {
	crc := uint32(0xFFFFFFFF)
	for _, b := range data {
		crc = crc<<8 ^ crc32Table[byte(crc>>24)^b]
	}
	return crc
}
