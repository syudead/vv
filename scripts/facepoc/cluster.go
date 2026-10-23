package main

import "sort"

// Main-person cut-off: a cluster needs this much good-quality face time and this many tracks.
const (
	clusterSim  = 0.45
	mainMinSec  = 8.0
	mainMinTrks = 2
)

// person is one cluster of tracks that passed the main-person cut-off.
type person struct {
	tracks  []*track
	goodSec float64
	centre  []float32
}

// averageLinkage merges the two clusters with the highest mean pairwise similarity until no pair
// reaches minSim, and returns clusters as index lists.
func averageLinkage(vecs [][]float32, minSim float64) [][]int {
	clusters := make([][]int, len(vecs))
	for i := range vecs {
		clusters[i] = []int{i}
	}
	sim := make([][]float64, len(vecs))
	for i := range vecs {
		sim[i] = make([]float64, len(vecs))
		for j := range vecs {
			sim[i][j] = dot(vecs[i], vecs[j])
		}
	}
	for {
		bi, bj, best := -1, -1, minSim
		for i := range clusters {
			for j := i + 1; j < len(clusters); j++ {
				if s := meanSim(sim, clusters[i], clusters[j]); s >= best {
					bi, bj, best = i, j, s
				}
			}
		}
		if bi < 0 {
			return clusters
		}
		clusters[bi] = append(clusters[bi], clusters[bj]...)
		clusters = append(clusters[:bj], clusters[bj+1:]...)
	}
}

func meanSim(sim [][]float64, a, b []int) float64 {
	var s float64
	for _, i := range a {
		for _, j := range b {
			s += sim[i][j]
		}
	}
	return s / float64(len(a)*len(b))
}

// mainPeople clusters the tracks that have a vector and keeps clusters with enough good face time,
// longest first.
func mainPeople(tracks []*track) []person {
	var ts []*track
	var vecs [][]float32
	for _, t := range tracks {
		if v := t.vector(); v != nil {
			ts, vecs = append(ts, t), append(vecs, v)
		}
	}
	var people []person
	for _, c := range averageLinkage(vecs, clusterSim) {
		p := person{centre: make([]float32, sfaceDim)}
		for _, i := range c {
			p.tracks = append(p.tracks, ts[i])
			p.goodSec += float64(ts[i].goodCount()) / sampleFPS
			for k, x := range vecs[i] {
				p.centre[k] += x
			}
		}
		p.centre = normalize(p.centre)
		if p.goodSec >= mainMinSec && len(p.tracks) >= mainMinTrks {
			people = append(people, p)
		}
	}
	sort.Slice(people, func(i, j int) bool { return people[i].goodSec > people[j].goodSec })
	return people
}
