package main

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type personJSON struct {
	ID      int           `json:"id"`
	GoodSec float64       `json:"goodSec"`
	Videos  []appearances `json:"videos"`
	Vector  []float32     `json:"vector"`
}

type appearances struct {
	Video string       `json:"video"`
	Spans [][2]float64 `json:"spans"`
}

// indexVideos runs every video, clusters main people across them and writes people.json and people.png.
func indexVideos(m *models, paths []string, out string) error {
	var all []*track
	for _, p := range paths {
		start := time.Now()
		ts, frames, err := indexVideo(m, p)
		if err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
		log.Printf("%s: %.0fs sampled, %d tracks, %.0fs", filepath.Base(p), float64(frames)/sampleFPS, len(ts), time.Since(start).Seconds())
		all = append(all, ts...)
	}
	people := mainPeople(all)
	log.Printf("%d tracks -> %d main people", len(all), len(people))
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	if err := writePeopleJSON(filepath.Join(out, "people.json"), people); err != nil {
		return err
	}
	return writeSheet(filepath.Join(out, "people.png"), people)
}

func writePeopleJSON(path string, people []person) error {
	rows := make([]personJSON, len(people))
	for i, p := range people {
		byVideo := map[string][][2]float64{}
		for _, t := range p.tracks {
			byVideo[t.video] = append(byVideo[t.video], [2]float64{t.times[0], t.times[len(t.times)-1]})
		}
		rows[i] = personJSON{ID: i, GoodSec: p.goodSec, Vector: p.centre}
		for v, spans := range byVideo {
			sort.Slice(spans, func(a, b int) bool { return spans[a][0] < spans[b][0] })
			rows[i].Videos = append(rows[i].Videos, appearances{Video: v, Spans: spans})
		}
		sort.Slice(rows[i].Videos, func(a, b int) bool { return rows[i].Videos[a].Video < rows[i].Videos[b].Video })
	}
	b, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// writeSheet draws one row of up to eight track thumbnails per main person.
func writeSheet(path string, people []person) error {
	const side, cols = 96, 8
	sheet := image.NewRGBA(image.Rect(0, 0, side*cols, side*max(1, len(people))))
	for r, p := range people {
		ts := append([]*track(nil), p.tracks...)
		sort.Slice(ts, func(a, b int) bool { return len(ts[a].times) > len(ts[b].times) })
		for c, t := range ts[:min(cols, len(ts))] {
			for y := range side {
				for x := range side {
					i := (y*side + x) * 3
					sheet.Set(c*side+x, r*side+y, color.RGBA{t.thumb.pix[i+2], t.thumb.pix[i+1], t.thumb.pix[i], 255})
				}
			}
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(f, sheet); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}
