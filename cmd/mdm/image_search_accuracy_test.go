//go:build accuracy

package main

// The accuracy measurement drives the real image search over generated footage and writes a report; it asserts
// nothing and stays out of CI. Run it with:
//
//	ACCURACY_DIR=/tmp/accuracy go test -tags accuracy -run TestImageSearchAccuracy -v -timeout 2h ./cmd/mdm/
//
// ACCURACY_DIR keeps the generated library between runs; the report is written to ACCURACY_DIR/report.md.

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"math"
	"math/bits"
	"math/rand/v2"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
	"github.com/syudead/vv/internal/media"
)

type accuracyScene struct {
	kind    string
	seed    int
	seconds int
}

type accuracyVideo struct {
	name   string
	scenes []accuracyScene
	gop    []string
	// distractor videos are never indexed; any listed result for their frames is a false positive.
	distractor bool
}

var accuracyGOPDefault = []string{}
var accuracyGOP2s = []string{"-g", "50"}
var accuracyGOP5s = []string{"-g", "125"}
var accuracyGOP10s = []string{"-g", "250", "-keyint_min", "250"}

func accuracyMixed(firstSeed, count int) []accuracyScene {
	kinds := []string{"mandel", "life", "cell", "grad", "kenburns"}
	scenes := make([]accuracyScene, count)
	for i := range scenes {
		seed := firstSeed + i
		scenes[i] = accuracyScene{kind: kinds[(seed*3)%len(kinds)], seed: seed, seconds: 8 + seed*7%8}
	}
	return scenes
}

func accuracyLibrary() []accuracyVideo {
	return []accuracyVideo{
		{name: "L1-zoom-default", scenes: []accuracyScene{{"mandel", 1, 90}}, gop: accuracyGOPDefault},
		{name: "L2-life-2s", scenes: []accuracyScene{{"life", 2, 60}}, gop: accuracyGOP2s},
		{name: "L3-cellauto-5s", scenes: []accuracyScene{{"cell", 3, 60}}, gop: accuracyGOP5s},
		{name: "L4-gradients-default", scenes: []accuracyScene{{"grad", 4, 90}}, gop: accuracyGOPDefault},
		{name: "L5-kenburns-10s", scenes: []accuracyScene{{"kenburns", 5, 120}}, gop: accuracyGOP10s},
		{name: "L6-mixed-default", scenes: accuracyMixed(10, 14), gop: accuracyGOPDefault},
		{name: "L7-mixed-5s", scenes: accuracyMixed(30, 12), gop: accuracyGOP5s},
		{name: "L8-mixed-long-default", scenes: accuracyMixed(50, 26), gop: accuracyGOPDefault},
		{name: "D1-zoom", scenes: []accuracyScene{{"mandel", 101, 60}}, distractor: true},
		{name: "D2-life", scenes: []accuracyScene{{"life", 102, 60}}, distractor: true},
		{name: "D3-mixed", scenes: accuracyMixed(110, 8), distractor: true},
		{name: "D4-kenburns", scenes: []accuracyScene{{"kenburns", 104, 60}}, distractor: true},
	}
}

// accuracyMandelView is where the ray at seed times the golden angle from inside the main cardioid leaves the set, so
// every seed has its own centre on the boundary, which shows detail at any zoom.
func accuracyMandelView(seed int) (x, y string) {
	angle := float64(seed) * 2.399963229728653
	inside := func(r float64) bool {
		cx, cy := -0.25+r*math.Cos(angle), r*math.Sin(angle)
		zx, zy := 0.0, 0.0
		for range 400 {
			zx, zy = zx*zx-zy*zy+cx, 2*zx*zy+cy
			if zx*zx+zy*zy > 4 {
				return false
			}
		}
		return true
	}
	low, high := 0.0, 2.5
	for range 50 {
		if mid := (low + high) / 2; inside(mid) {
			low = mid
		} else {
			high = mid
		}
	}
	format := func(v float64) string { return strconv.FormatFloat(v, 'g', 17, 64) }
	return format(-0.25 + high*math.Cos(angle)), format(high * math.Sin(angle))
}

var accuracyColors = []string{"#ffd060", "#30a0ff", "#e04060", "#60e090", "#c080ff", "#ff8020", "#20c0c0"}

// accuracySource is a lavfi graph of one scene at 640x360, 25 fps; kenburns pans over still, a rendered frame.
func accuracySource(scene accuracyScene, still string) (args []string) {
	s, d := scene.seed, strconv.Itoa(scene.seconds)
	color := func(k int) string { return accuracyColors[(s+k)%len(accuracyColors)] }
	grain := ",noise=alls=6:allf=t,format=yuv420p"
	switch scene.kind {
	case "mandel":
		scale := 0.2 / float64(1+s%3)
		x, y := accuracyMandelView(s)
		return []string{"-f", "lavfi", "-i", fmt.Sprintf(
			"mandelbrot=s=640x360:r=25:maxiter=400:start_x=%s:start_y=%s:start_scale=%g:end_scale=%g:end_pts=%d,"+
				"hue=h=%d%s", x, y, scale, scale/40, scene.seconds*25, s*37%360,
			grain)}
	case "life":
		return []string{"-f", "lavfi", "-i", fmt.Sprintf(
			"life=s=160x90:r=4:mold=25:ratio=0.35:seed=%d:life_color=%s:death_color=#101828:mold_color=%s,"+
				"scale=640:360:flags=bicubic,fps=25,gblur=sigma=1.5%s", s, color(0), color(3), grain)}
	case "cell":
		rules := []int{30, 110, 90, 150, 105}
		return []string{"-f", "lavfi", "-i", fmt.Sprintf(
			"cellauto=s=320x180:r=25:rule=%d:random_fill_ratio=0.5:random_seed=%d:scroll=1,scale=640:360,"+
				"format=yuv420p[c];gradients=s=640x360:r=25:speed=0.02:seed=%d:c0=%s:c1=%s,format=yuv420p[g];"+
				"[c][g]blend=all_mode=multiply,gblur=sigma=1%s", rules[s%len(rules)], s, s, color(1), color(4),
			grain)}
	case "grad":
		return []string{"-f", "lavfi", "-i", fmt.Sprintf(
			"gradients=s=900x900:r=25:speed=0.015:seed=%d:n=5:c0=%s:c1=%s:c2=#ffffff:c3=#000000:c4=%s,"+
				"rotate=a=0.15*t+%d,crop=640:360,drawbox=x='mod(t*60,640)':y=200:w=80:h=80:color=%s:t=fill%s",
			s, color(0), color(2), color(5), s, color(6), grain)}
	case "kenburns":
		return []string{"-loop", "1", "-framerate", "25", "-t", d, "-i", still, "-vf", fmt.Sprintf(
			"zoompan=z='1.4+0.3*sin(on/%d)':x='(iw-iw/zoom)*(0.5+0.45*sin(on/%d))':"+
				"y='(ih-ih/zoom)*(0.5+0.45*cos(on/%d))':d=1:s=640x360:fps=25%s", 170+s*7%60, 230+s*11%70,
			290+s*13%80, grain)}
	}
	panic(scene.kind)
}

func accuracyFFmpegOutput(t *testing.T, args ...string) []byte {
	t.Helper()
	full := append([]string{"-nostdin", "-hide_banner", "-v", "error"}, args...)
	out, err := exec.Command("ffmpeg", full...).Output()
	if err != nil {
		t.Fatalf("ffmpeg %s: %v", strings.Join(args, " "), err)
	}
	return out
}

// buildAccuracyVideo renders each scene losslessly enough, concatenates them with hard cuts and encodes the result
// with libx264 at the video's keyframe interval and the default scenecut.
func buildAccuracyVideo(t *testing.T, dir string, video accuracyVideo) string {
	t.Helper()
	out := filepath.Join(dir, video.name+".mp4")
	if _, err := os.Stat(out); err == nil {
		return out
	}
	var list strings.Builder
	for i, scene := range video.scenes {
		part := filepath.Join(dir, fmt.Sprintf("%s-%02d.mkv", video.name, i))
		still := ""
		if scene.kind == "kenburns" {
			still = filepath.Join(dir, fmt.Sprintf("still-%d.png", scene.seed))
			scale := 0.02 * float64(1+scene.seed%4)
			x, y := accuracyMandelView(scene.seed + 500)
			ffmpegFixture(t, "-f", "lavfi", "-i", fmt.Sprintf(
				"mandelbrot=s=1920x1080:maxiter=600:start_x=%s:start_y=%s:start_scale=%g,hue=h=%d:s=1.4", x, y, scale,
				scene.seed*29%360), "-frames:v", "1", still)
		}
		args := append(accuracySource(scene, still), "-t", strconv.Itoa(scene.seconds), "-r", "25",
			"-c:v", "libx264", "-preset", "ultrafast", "-crf", "12", "-pix_fmt", "yuv420p", part)
		ffmpegFixture(t, args...)
		fmt.Fprintf(&list, "file '%s'\n", part)
	}
	listPath := filepath.Join(dir, video.name+".txt")
	if err := os.WriteFile(listPath, []byte(list.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	args := []string{"-f", "concat", "-safe", "0", "-i", listPath, "-c:v", "libx264", "-preset", "medium",
		"-crf", "23", "-pix_fmt", "yuv420p"}
	args = append(append(args, video.gop...), "-movflags", "+faststart", out+".tmp.mp4")
	ffmpegFixture(t, args...)
	if err := os.Rename(out+".tmp.mp4", out); err != nil {
		t.Fatal(err)
	}
	return out
}

type accuracyRow struct {
	video, variant string
	atMs           int64
	status         int
	rank           int // 1-based rank of the correct video, 0 when it is not listed
	listed, wrong  int
	kind           string
	posErrMs       int64
	// Diagnostics: the query's distance to the nearest index entry of its video, to the frame at its own time
	// decoded as a refinement decodes it, and the time since the preceding index entry.
	dIndex, dExact int
	sinceEntryMs   int64
	// nearestMs is the index entry nearest the query by hash; dRefine is the best distance in the gap the search
	// refines for this video alone, -1 without one; dDense1s and dDense2s are the distance to the nearest frame of a
	// hypothetical index with a frame every 1 s or 2 s.
	nearestMs                   int64
	dRefine, dDense1s, dDense2s int
	note                        string
}

type accuracyLibraryVideo struct {
	accuracyVideo
	path       string
	id         int64
	durationMs int64
	entries    []domain.SearchIndexEntry
	// dense holds the video sampled once a second, as a query video is.
	dense []domain.FrameHash
}

func TestImageSearchAccuracy(t *testing.T) {
	dir := os.Getenv("ACCURACY_DIR")
	if dir == "" {
		dir = t.TempDir()
	}
	ctx := context.Background()
	fixture := newImageSearchFixture(t)
	var library []accuracyLibraryVideo
	for _, video := range accuracyLibrary() {
		built := buildAccuracyVideo(t, dir, video)
		durationMs, err := media.QueryVideoDuration(ctx, built)
		if err != nil {
			t.Fatal(err)
		}
		entry := accuracyLibraryVideo{accuracyVideo: video, path: built, durationMs: durationMs}
		if !video.distractor {
			linked := filepath.Join(fixture.mediaDir, filepath.Base(built))
			if err := os.Link(built, linked); err != nil {
				t.Fatal(err)
			}
			entry.id = fixture.indexVideo(t, linked, durationMs)
			collector := domain.NewSearchIndexCollector(durationMs)
			if err := media.KeyframeHashes(ctx, built, collector.Add); err != nil {
				t.Fatal(err)
			}
			entry.entries = collector.Index().Entries
			if entry.dense, err = media.QueryVideoFrames(ctx, built, domain.ImageSearchQueryMaxFrames); err != nil {
				t.Fatal(err)
			}
		}
		library = append(library, entry)
		t.Logf("%s: %d ms, %d index entries", video.name, durationMs, len(entry.entries))
	}

	m := accuracyRun{t: t, dir: dir, fixture: fixture, library: library}
	for i := range library {
		m.imageVariants(&library[i], uint64(i))
	}
	m.videoVariants()
	report := m.report()
	if err := os.WriteFile(filepath.Join(dir, "report.md"), []byte(report), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Log("\n" + report)
}

type accuracyRun struct {
	t       *testing.T
	dir     string
	fixture imageSearchFixture
	library []accuracyLibraryVideo
	rows    []accuracyRow
}

func (m *accuracyRun) imageVariants(video *accuracyLibraryVideo, seed uint64) {
	rng := rand.New(rand.NewPCG(seed, 74))
	randomTimes := func(n int) []int64 {
		times := make([]int64, n)
		for i := range times {
			times[i] = 500 + rng.Int64N(video.durationMs-1000)
		}
		return times
	}
	if !video.distractor {
		picks := slices.Clone(video.entries)
		rng.Shuffle(len(picks), func(i, j int) { picks[i], picks[j] = picks[j], picks[i] })
		for _, entry := range picks[:min(5, len(picks))] {
			m.imageQuery(video, "a keyframe", entry.TimeMs, "", false)
		}
	}
	n := 10
	if video.distractor {
		n = 15
	}
	for _, at := range randomTimes(n) {
		m.imageQuery(video, "b random", at, "", false)
	}
	if video.distractor {
		return
	}
	for _, at := range randomTimes(5) {
		m.imageQuery(video, "c 50% JPEG q75", at, "scale=iw/2:ih/2", true)
	}
	for _, at := range randomTimes(5) {
		m.imageQuery(video, "d 10% crop", at, "crop=iw*0.9:ih*0.9", false)
	}
	for _, at := range randomTimes(5) {
		m.imageQuery(video, "d letterbox 4:3", at, "pad=iw:iw*3/4:0:(oh-ih)/2:black", false)
	}
	font := "/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf"
	for _, at := range randomTimes(5) {
		m.imageQuery(video, "d text overlay", at, "drawtext=fontfile="+font+
			":text='Screenshot 12\\:34':fontsize=h/16:fontcolor=white:borderw=2:x=w*0.04:y=h*0.88", false)
	}
}

// imageQuery cuts the frame at atMs, applies filter and posts it as PNG, or as JPEG at quality 75.
func (m *accuracyRun) imageQuery(video *accuracyLibraryVideo, variant string, atMs int64, filter string, asJPEG bool) {
	t := m.t
	args := []string{"-ss", fmt.Sprintf("%.3f", float64(atMs)/1000), "-i", video.path, "-frames:v", "1"}
	if filter != "" {
		args = append(args, "-vf", filter)
	}
	frame := accuracyFFmpegOutput(t, append(args, "-f", "image2pipe", "-c:v", "png", "pipe:1")...)
	img, _, err := image.Decode(bytes.NewReader(frame))
	if err != nil {
		t.Fatal(err)
	}
	data, name := frame, "frame.png"
	if asJPEG {
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 75}); err != nil {
			t.Fatal(err)
		}
		data, name = buf.Bytes(), "frame.jpg"
		if img, _, err = image.Decode(bytes.NewReader(data)); err != nil {
			t.Fatal(err)
		}
	}
	row := accuracyRow{video: video.name, variant: variant, atMs: atMs, dIndex: -1, dExact: -1, sinceEntryMs: -1,
		dRefine: -1, dDense1s: -1, dDense2s: -1}
	code, result, body := searchFile(t, m.fixture.handler, name, data)
	row.status = code
	if code != http.StatusOK {
		row.note = strings.TrimSpace(body)
	}
	m.score(&row, video, result, func(match gen.ImageSearchMatch) int64 { return match.PositionMs - atMs })
	for _, item := range result.Items {
		if item.Video.Id == video.id && !video.distractor {
			row.note += fmt.Sprintf("rate %.2f", item.Match.Rate)
		}
	}
	if !video.distractor {
		m.diagnose(&row, video, media.ImageFrameHash(img))
	}
	m.rows = append(m.rows, row)
}

func (m *accuracyRun) diagnose(row *accuracyRow, video *accuracyLibraryVideo, query domain.FrameHash) {
	row.dIndex = 64
	for _, entry := range video.entries {
		row.dIndex = min(row.dIndex, bits.OnesCount64(query.Hash^entry.Hash))
		if entry.TimeMs <= row.atMs {
			row.sinceEntryMs = row.atMs - entry.TimeMs
		}
	}
	frames, err := media.RefineFrameHashes(context.Background(), video.path, row.atMs, row.atMs+250)
	if err == nil && len(frames) > 0 {
		row.dExact = bits.OnesCount64(query.Hash ^ frames[0].Frame.Hash)
	}
	content := domain.ImageSearchContent{VideoID: 1, DurationMs: video.durationMs}
	for _, entry := range video.entries {
		content.Frames = append(content.Frames, domain.ImageSearchFrame(entry))
	}
	_, refinements := domain.MatchImageQuery(query.Hash, []domain.ImageSearchContent{content})
	if len(refinements) == 1 {
		r := refinements[0]
		row.nearestMs = r.StartMs
		refined, err := media.RefineFrameHashes(context.Background(), video.path, r.StartMs, r.EndMs)
		if err == nil {
			row.dRefine = 64
			for _, frame := range refined {
				row.dRefine = min(row.dRefine, bits.OnesCount64(query.Hash^frame.Frame.Hash))
			}
		}
	}
	row.dDense1s, row.dDense2s = 64, 64
	for i, frame := range video.dense {
		d := bits.OnesCount64(query.Hash ^ frame.Hash)
		row.dDense1s = min(row.dDense1s, d)
		if i%2 == 0 {
			row.dDense2s = min(row.dDense2s, d)
		}
	}
}

// score finds the correct video's rank, the wrongly listed videos and the correct match's position error.
func (m *accuracyRun) score(
	row *accuracyRow, video *accuracyLibraryVideo, result gen.ImageSearchResult, posErr func(gen.ImageSearchMatch) int64,
) {
	row.listed = len(result.Items)
	for i, item := range result.Items {
		if !video.distractor && item.Video.Id == video.id {
			row.rank = i + 1
			row.kind = string(item.Match.Kind)
			row.posErrMs = posErr(item.Match)
			continue
		}
		row.wrong++
		for _, other := range m.library {
			if other.id == item.Video.Id && !other.distractor {
				row.note += fmt.Sprintf("wrong %s at %.1f s (%.2f); ", other.name, float64(item.Match.PositionMs)/1000,
					item.Match.Rate)
			}
		}
	}
}

func (m *accuracyRun) videoVariants() {
	for i := range m.library {
		video := &m.library[i]
		query := filepath.Join(m.dir, video.name+"-reencoded.mp4")
		if _, err := os.Stat(query); err != nil {
			ffmpegFixture(m.t, "-i", video.path, "-vf", "scale=320:-2", "-c:v", "libx264", "-preset", "fast",
				"-crf", "32", "-g", "75", "-b:v", "0", query)
		}
		variant := "e whole re-encode 320p CRF 32"
		if video.distractor {
			variant = "e distractor whole video"
		}
		m.videoQuery(video, variant, query, 0)
	}
	for _, excerpt := range []struct {
		video   string
		seconds int64
	}{
		{"L8-mixed-long-default", 10}, {"L8-mixed-long-default", 30}, {"L8-mixed-long-default", 60},
		{"L6-mixed-default", 30}, {"L7-mixed-5s", 20}, {"L5-kenburns-10s", 30}, {"L1-zoom-default", 20},
		{"L4-gradients-default", 30}, {"L2-life-2s", 15}, {"L3-cellauto-5s", 15},
	} {
		video := m.find(excerpt.video)
		startMs := (video.durationMs/2 - excerpt.seconds*500) / 1000 * 1000
		query := filepath.Join(m.dir, fmt.Sprintf("%s-excerpt-%ds.mp4", video.name, excerpt.seconds))
		if _, err := os.Stat(query); err != nil {
			ffmpegFixture(m.t, "-ss", strconv.FormatInt(startMs/1000, 10), "-t", strconv.FormatInt(excerpt.seconds, 10),
				"-i", video.path, "-vf", "scale=480:-2", "-c:v", "libx264", "-preset", "fast", "-crf", "28", query)
		}
		m.videoQuery(video, fmt.Sprintf("f excerpt %d s, 270p CRF 28", excerpt.seconds), query, startMs)
	}
}

func (m *accuracyRun) find(name string) *accuracyLibraryVideo {
	for i := range m.library {
		if m.library[i].name == name {
			return &m.library[i]
		}
	}
	m.t.Fatalf("no library video %s", name)
	return nil
}

// videoQuery posts a query video whose time 0 is startMs of the library video.
func (m *accuracyRun) videoQuery(video *accuracyLibraryVideo, variant, path string, startMs int64) {
	data, err := os.ReadFile(path)
	if err != nil {
		m.t.Fatal(err)
	}
	row := accuracyRow{video: video.name, variant: variant, atMs: startMs, dIndex: -1, dExact: -1, sinceEntryMs: -1,
		dRefine: -1, dDense1s: -1, dDense2s: -1}
	code, result, body := searchFile(m.t, m.fixture.handler, "query.mp4", data)
	row.status = code
	if code != http.StatusOK {
		row.note = strings.TrimSpace(body)
	}
	m.score(&row, video, result, func(match gen.ImageSearchMatch) int64 {
		if match.StartMs == nil || match.QueryStartMs == nil {
			return 0
		}
		return *match.StartMs - *match.QueryStartMs - startMs
	})
	for _, item := range result.Items {
		if item.Video.Id == video.id && !video.distractor {
			row.note += fmt.Sprintf("rate %.2f, section %d-%d ms", item.Match.Rate, deref(item.Match.StartMs),
				deref(item.Match.EndMs))
		}
	}
	m.rows = append(m.rows, row)
}

func deref(v *int64) int64 {
	if v == nil {
		return -1
	}
	return *v
}

func (m *accuracyRun) report() string {
	var b strings.Builder
	type summary struct {
		n, top1, any, wrongQueries, refused int
		posErr                              []int64
	}
	order := []string{}
	byVariant := map[string]*summary{}
	for _, row := range m.rows {
		key := row.variant
		if strings.HasPrefix(row.video, "D") && !strings.HasPrefix(key, "e distractor") {
			key = "distractor image (" + key + ")"
		}
		s := byVariant[key]
		if s == nil {
			s = &summary{}
			byVariant[key] = s
			order = append(order, key)
		}
		s.n++
		if row.rank == 1 {
			s.top1++
		}
		if row.rank > 0 {
			s.any++
			s.posErr = append(s.posErr, abs(row.posErrMs))
		}
		if row.wrong > 0 {
			s.wrongQueries++
		}
		if row.status != http.StatusOK {
			s.refused++
		}
	}
	b.WriteString("| Variant | Queries | Recall@1 | Recall@any | Queries listing a wrong video | Refused | " +
		"Median / max position error (s) |\n| --- | --- | --- | --- | --- | --- | --- |\n")
	for _, key := range order {
		s := byVariant[key]
		slices.Sort(s.posErr)
		errText := "-"
		if len(s.posErr) > 0 {
			errText = fmt.Sprintf("%.1f / %.1f", float64(s.posErr[len(s.posErr)/2])/1000,
				float64(s.posErr[len(s.posErr)-1])/1000)
		}
		fmt.Fprintf(&b, "| %s | %d | %d (%.0f%%) | %d (%.0f%%) | %d | %d | %s |\n", key, s.n, s.top1,
			pct(s.top1, s.n), s.any, pct(s.any, s.n), s.wrongQueries, s.refused, errText)
	}
	b.WriteString("\n## Per query\n\n| Video | Variant | At (s) | Status | Rank | Listed | Wrong | Kind | " +
		"Pos err (s) | d index | d exact | Since entry (s) | Refined from (s) | d refine | d 1 s | d 2 s | Note |\n" +
		"| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |\n")
	for _, r := range m.rows {
		fmt.Fprintf(&b, "| %s | %s | %.1f | %d | %d | %d | %d | %s | %.1f | %d | %d | %.1f | %.1f | %d | %d | %d | %s |\n", r.video,
			r.variant, float64(r.atMs)/1000, r.status, r.rank, r.listed, r.wrong, r.kind, float64(r.posErrMs)/1000,
			r.dIndex, r.dExact, float64(r.sinceEntryMs)/1000, float64(r.nearestMs)/1000, r.dRefine, r.dDense1s, r.dDense2s,
			strings.ReplaceAll(r.note, "|", "/"))
	}
	b.WriteString("\n## Index entries\n\n")
	for _, video := range m.library {
		if video.distractor {
			continue
		}
		times := make([]string, len(video.entries))
		for i, e := range video.entries {
			times[i] = fmt.Sprintf("%.1f", float64(e.TimeMs)/1000)
		}
		fmt.Fprintf(&b, "- %s (%.0f s): %s\n", video.name, float64(video.durationMs)/1000, strings.Join(times, " "))
	}
	return b.String()
}

func pct(a, n int) float64 {
	if n == 0 {
		return 0
	}
	return 100 * float64(a) / float64(n)
}

func abs(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
