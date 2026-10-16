package media

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/text/encoding/japanese"

	"github.com/syudead/vv/internal/domain"
)

// テストの中で TS（PAT、PMT、ARIB の文字列を持つ SDT と EIT、TOT、その前の padding packet）を
// 組み立て、SI の reader が放送局名と番組名を読むことを確かめる
// （specs/056-media-info/research.md R-4）。

const (
	testServiceID = 0x0400
	testNetworkID = 0x7FE0 // 地上波の original_network_id
	testPMTPID    = 0x0100
	paddingPID    = 0x1FFF
)

// aribKanji は全角の文字列を ARIB の漢字の集合（GL の G0）の符号にする。
func aribKanji(t *testing.T, text string) []byte {
	t.Helper()
	encoded, err := japanese.EUCJP.NewEncoder().String(text)
	if err != nil {
		t.Fatalf("EUC-JP にできない: %v", err)
	}
	out := []byte(encoded)
	for i := range out {
		out[i] &= 0x7F
	}
	return out
}

// longSection は CRC を持つ section を組み立てる。
func longSection(table byte, extension uint16, number byte, body []byte) []byte {
	length := 5 + len(body) + 4
	section := []byte{
		table, 0xB0 | byte(length>>8), byte(length),
		byte(extension >> 8), byte(extension), 0xC1, number, 1,
	}
	section = append(section, body...)
	return appendCRC(section)
}

func appendCRC(section []byte) []byte {
	crc := crc32MPEG(section)
	return append(section, byte(crc>>24), byte(crc>>16), byte(crc>>8), byte(crc))
}

// siClock は日本標準時の時刻を MJD と BCD の 5 バイトにする。
func siClock(at time.Time) []byte {
	at = at.In(broadcastZone)
	base := time.Date(1858, time.November, 17, 0, 0, 0, 0, broadcastZone)
	day := time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, broadcastZone)
	mjd := int(day.Sub(base).Hours() / 24)
	return []byte{byte(mjd >> 8), byte(mjd), bcd(at.Hour()), bcd(at.Minute()), bcd(at.Second())}
}

func bcd(value int) byte {
	return byte(value/10<<4 | value%10)
}

func patSection() []byte {
	return longSection(tablePAT, 1, 0, []byte{
		0x00, 0x00, 0xE0, 0x10, // program 0 は NIT
		byte(testServiceID >> 8), byte(testServiceID & 0xFF), 0xE0 | testPMTPID>>8, testPMTPID & 0xFF,
	})
}

func pmtSection() []byte {
	return longSection(0x02, testServiceID, 0, []byte{0xE1, 0x00, 0xF0, 0x00})
}

func sdtSection(networkID uint16, name []byte) []byte {
	descriptor := append([]byte{descriptorService, byte(3 + len(name)), 0x01, 0, byte(len(name))}, name...)
	body := []byte{byte(networkID >> 8), byte(networkID), 0xFF}
	body = append(body, byte(testServiceID>>8), byte(testServiceID&0xFF), 0xFC,
		0x80|byte(len(descriptor)>>8), byte(len(descriptor)))
	return longSection(tableSDT, 1, 0, append(body, descriptor...))
}

func eitSection(networkID uint16, number byte, language string, name []byte, start time.Time, duration time.Duration) []byte {
	return serviceEITSection(testServiceID, networkID, number, language, name, start, duration)
}

// serviceEITSection は serviceID の service の EIT present/following の section である。
func serviceEITSection(serviceID, networkID uint16, number byte, language string, name []byte, start time.Time, duration time.Duration) []byte {
	descriptor := []byte{descriptorShortEvent, byte(5 + len(name))}
	descriptor = append(descriptor, language...)
	descriptor = append(descriptor, byte(len(name)))
	descriptor = append(descriptor, name...)
	descriptor = append(descriptor, 0)
	seconds := int(duration / time.Second)
	body := []byte{0x00, 0x01, byte(networkID >> 8), byte(networkID), 1, tableEIT}
	body = append(body, 0x00, number)
	body = append(body, siClock(start)...)
	body = append(body, bcd(seconds/3600), bcd(seconds/60%60), bcd(seconds%60))
	body = append(body, 0x80|byte(len(descriptor)>>8), byte(len(descriptor)))
	return longSection(tableEIT, serviceID, number, append(body, descriptor...))
}

func totSection(at time.Time) []byte {
	section := []byte{tableTOT, 0x70, 11}
	section = append(section, siClock(at)...)
	section = append(section, 0xF0, 0x00)
	return appendCRC(section)
}

// sectionPackets は section を pid の TS packet に分ける。最初の packet は pointer_field 0 で始まる。
func sectionPackets(pid uint16, section []byte) []byte {
	var out []byte
	payload := append([]byte{0}, section...)
	for first := true; len(payload) > 0; first = false {
		header := []byte{tsSyncByte, byte(pid >> 8), byte(pid), 0x10}
		if first {
			header[1] |= 0x40
		}
		chunk := payload[:min(len(payload), tsPacketSize-4)]
		payload = payload[len(chunk):]
		packet := append(header, chunk...)
		for len(packet) < tsPacketSize {
			packet = append(packet, 0xFF)
		}
		out = append(out, packet...)
	}
	return out
}

func paddingPackets(count int) []byte {
	var out []byte
	for range count {
		packet := []byte{tsSyncByte, byte(paddingPID >> 8), byte(paddingPID & 0xFF), 0x10}
		out = append(out, append(packet, bytes.Repeat([]byte{0xFF}, tsPacketSize-4)...)...)
	}
	return out
}

var (
	testNewsStart  = time.Date(2026, 10, 16, 20, 0, 0, 0, broadcastZone)
	testDramaStart = time.Date(2026, 10, 16, 21, 0, 0, 0, broadcastZone)
	testRecording  = time.Date(2026, 10, 16, 20, 55, 0, 0, broadcastZone)
)

// broadcastTS は padding の後に PAT・PMT・SDT・EIT present/following・TOT を並べた TS である。
func broadcastTS(t *testing.T, networkID uint16, language string) []byte {
	t.Helper()
	ts := paddingPackets(8)
	ts = append(ts, sectionPackets(pidPAT, patSection())...)
	ts = append(ts, sectionPackets(testPMTPID, pmtSection())...)
	ts = append(ts, sectionPackets(pidSDT, sdtSection(networkID, aribKanji(t, "テスト放送")))...)
	ts = append(ts, sectionPackets(pidEIT, eitSection(networkID, 0, language, aribKanji(t, "ニュース"), testNewsStart, time.Hour))...)
	ts = append(ts, sectionPackets(pidEIT, eitSection(networkID, 1, language, aribKanji(t, "ドラマ"), testDramaStart, 54*time.Minute))...)
	ts = append(ts, sectionPackets(pidTime, totSection(testRecording))...)
	return ts
}

func writeTS(t *testing.T, name string, chunks ...[]byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, bytes.Join(chunks, nil), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReadBroadcastReturnsStationAndProgramme(t *testing.T) {
	path := writeTS(t, "rec.ts", broadcastTS(t, testNetworkID, "jpn"))

	got := readBroadcast(context.Background(), path, (61 * time.Minute).Milliseconds())
	want := &domain.MediaBroadcast{Station: "テスト放送", Program: "ドラマ"}
	if got == nil || *got != *want {
		t.Fatalf("readBroadcast = %+v, want %+v", got, want)
	}
}

func TestScanTransportStreamSIReadsEvents(t *testing.T) {
	si, err := scanTransportStreamSI(context.Background(), bytes.NewReader(broadcastTS(t, testNetworkID, "jpn")))
	if err != nil {
		t.Fatal(err)
	}
	if si.Station != "テスト放送" || !si.Time.Equal(testRecording) {
		t.Errorf("Station = %q, Time = %v", si.Station, si.Time)
	}
	if si.Present == nil || si.Present.Name != "ニュース" || !si.Present.Start.Equal(testNewsStart) || si.Present.Duration != time.Hour {
		t.Errorf("Present = %+v", si.Present)
	}
	if si.Following == nil || si.Following.Name != "ドラマ" || si.Following.Duration != 54*time.Minute {
		t.Errorf("Following = %+v", si.Following)
	}
}

// .m2ts・.mts は各 packet の前に 4 バイトの時刻を持つ。
func TestScanTransportStreamSIReadsM2TSPackets(t *testing.T) {
	ts := broadcastTS(t, testNetworkID, "jpn")
	var m2ts []byte
	for offset := 0; offset < len(ts); offset += tsPacketSize {
		m2ts = append(m2ts, 0x00, 0x00, 0x00, 0x00)
		m2ts = append(m2ts, ts[offset:offset+tsPacketSize]...)
	}
	si, err := scanTransportStreamSI(context.Background(), bytes.NewReader(m2ts))
	if err != nil {
		t.Fatal(err)
	}
	if si.Station != "テスト放送" || si.Following == nil || si.Following.Name != "ドラマ" {
		t.Errorf("si = %+v", si)
	}
}

func TestReadBroadcastIgnoresSIAfterTheReadLimit(t *testing.T) {
	path := writeTS(t, "late.ts", paddingPackets(siReadLimit/tsPacketSize+1), broadcastTS(t, testNetworkID, "jpn"))

	if got := readBroadcast(context.Background(), path, (61 * time.Minute).Milliseconds()); got != nil {
		t.Errorf("先頭 32 MiB に SDT と EIT の無い TS から名前を読んだ: %+v", got)
	}
}

// failAfterReader は読まれたら誤りを返す。名前と時刻がそろった後に読み進めていないことを示す。
type failAfterReader struct{ read *bool }

func (r failAfterReader) Read([]byte) (int, error) {
	*r.read = true
	return 0, errors.New("read past the SI")
}

func TestScanTransportStreamSIStopsWhenComplete(t *testing.T) {
	readPast := false
	reader := io.MultiReader(bytes.NewReader(broadcastTS(t, testNetworkID, "jpn")), failAfterReader{read: &readPast})

	si, err := scanTransportStreamSI(context.Background(), reader)
	if err != nil {
		t.Fatalf("scanTransportStreamSI: %v", err)
	}
	if readPast {
		t.Error("名前と時刻がそろった後も読み進めた")
	}
	if si.Station == "" || si.Time.IsZero() {
		t.Errorf("si = %+v", si)
	}
}

func TestScanTransportStreamSIReturnsReadErrors(t *testing.T) {
	readPast := false
	ts := broadcastTS(t, testNetworkID, "jpn")
	reader := io.MultiReader(bytes.NewReader(ts[:len(ts)-tsPacketSize]), failAfterReader{read: &readPast})

	if _, err := scanTransportStreamSI(context.Background(), reader); err == nil {
		t.Error("読み取りの誤りを返さなかった")
	}
}

// ffmpeg が作る TS は original_network_id 1 の SDT（DVB の文字列）を持ち、EIT を持たない。
// 名前を ARIB として decode せずに省き、前に ARIB の SI を足せば Probe がその名前を読む。
func TestProbeReadsBroadcastOfTransportStream(t *testing.T) {
	requireFFmpeg(t)
	dir := t.TempDir()
	source := filepath.Join(dir, "source.ts")
	if out, err := exec.Command("ffmpeg", "-nostdin", "-v", "error", "-f", "lavfi",
		"-i", "testsrc2=size=320x240:rate=10:duration=10", "-c:v", "mpeg2video",
		"-f", "mpegts", source).CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg: %v: %s", err, out)
	}
	plain, err := Probe(context.Background(), source)
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if plain.Media == nil || plain.Media.Broadcast != nil {
		t.Errorf("ffmpeg の TS の Broadcast = %+v", plain.Media)
	}

	encoded, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	recording := filepath.Join(dir, "rec.m2ts")
	if err := os.WriteFile(recording, append(broadcastTS(t, testNetworkID, "jpn"), encoded...), 0o600); err != nil {
		t.Fatal(err)
	}
	probe, err := Probe(context.Background(), recording)
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	want := domain.MediaBroadcast{Station: "テスト放送", Program: "ニュース"}
	if probe.Media == nil || probe.Media.Broadcast == nil || *probe.Media.Broadcast != want {
		t.Errorf("Broadcast = %+v, want %+v", probe.Media, want)
	}
}

func TestReadBroadcastOmitsDVBNames(t *testing.T) {
	path := writeTS(t, "dvb.ts", broadcastTS(t, 0x2114, "deu"))

	if got := readBroadcast(context.Background(), path, (61 * time.Minute).Milliseconds()); got != nil {
		t.Errorf("DVB の録画の名前を読んだ: %+v", got)
	}
}

func TestReadBroadcastIgnoresLanguageOfOtherServices(t *testing.T) {
	// 同じ TS のほかの service の英語の EIT が、PAT の最初の番組の日本語の名前を消さない。
	const otherService = testServiceID + 1
	other := serviceEITSection(otherService, testNetworkID, 0, "eng", []byte("News"), testNewsStart, time.Hour)
	ts := paddingPackets(1)
	ts = append(ts, sectionPackets(pidEIT, other)...)
	path := writeTS(t, "multi.ts", ts, broadcastTS(t, testNetworkID, "jpn"))

	got := readBroadcast(context.Background(), path, (61 * time.Minute).Milliseconds())
	if got == nil || got.Station != "テスト放送" || got.Program != "ドラマ" {
		t.Errorf("readBroadcast = %+v", got)
	}
}

func TestReadBroadcastOmitsNamesWhenCancelled(t *testing.T) {
	path := writeTS(t, "rec.ts", broadcastTS(t, testNetworkID, "jpn"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if got := readBroadcast(ctx, path, (61 * time.Minute).Milliseconds()); got != nil {
		t.Errorf("期限切れで名前を読んだ: %+v", got)
	}
}

func TestReadBroadcastWithoutTimeOmitsProgramme(t *testing.T) {
	ts := broadcastTS(t, testNetworkID, "jpn")
	// TOT の packet（最後の 1 つ）を padding にする。
	ts = append(ts[:len(ts)-tsPacketSize], paddingPackets(1)...)
	path := writeTS(t, "notime.ts", ts)

	got := readBroadcast(context.Background(), path, (61 * time.Minute).Milliseconds())
	if got == nil || got.Station != "テスト放送" || got.Program != "" {
		t.Errorf("readBroadcast = %+v", got)
	}
}
