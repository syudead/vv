package app

import (
	"image"
	"math"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/faceproto"
)

// faceVideo follows one video's faces frame by frame and keeps a crop of each track's best good face, so a main
// person's crop is ready when the video ends without decoding it again.
type faceVideo struct {
	th      domain.FaceThresholds
	tracker *domain.FaceTracker
	frame   int
	crops   map[domain.FaceSighting]domain.PersonImage
}

func newFaceVideo(th domain.FaceThresholds) *faceVideo {
	return &faceVideo{th: th, tracker: domain.NewFaceTracker(th), crops: map[domain.FaceSighting]domain.PersonImage{}}
}

// add takes the faces of the next frame; pix is that frame, a domain.FaceFrameSide square of packed BGR.
func (v *faceVideo) add(pix []byte, faces []domain.FaceDetection) error {
	v.tracker.Add(faces)
	keep := map[domain.FaceSighting]bool{}
	for _, s := range v.tracker.BestSightings() {
		keep[s] = true
		if _, ok := v.crops[s]; ok || s.Frame != v.frame {
			continue
		}
		crop, err := cropFace(pix, s.Box)
		if err != nil {
			return err
		}
		v.crops[s] = crop
	}
	// A track's earlier best face, and tracks that can no longer count, need no crop.
	for s := range v.crops {
		if !keep[s] {
			delete(v.crops, s)
		}
	}
	v.frame++
	return nil
}

// people returns the main people of the frames added so far, each with its best face's crop.
func (v *faceVideo) people() []domain.FaceAnalysedPerson {
	mains := domain.FindMainPeople(v.th, v.tracker.Tracks(), float64(v.frame)/domain.FaceSampleFPS)
	out := make([]domain.FaceAnalysedPerson, len(mains))
	for i, p := range mains {
		out[i] = domain.FaceAnalysedPerson{
			Spans: p.Spans(), GoodSeconds: p.GoodSeconds, Centroid: p.Centroid, Crop: v.crops[p.Best],
		}
	}
	return out
}

// cropFace cuts the face box doubled around its centre from the frame, clipped to the frame and never enlarged.
func cropFace(pix []byte, box domain.FaceBox) (domain.PersonImage, error) {
	const side = domain.FaceFrameSide
	cx, cy := box.X+box.W/2, box.Y+box.H/2
	clip := func(v float64) int { return int(math.Round(math.Max(0, math.Min(side, v)))) }
	r := image.Rect(clip(cx-box.W), clip(cy-box.H), clip(cx+box.W), clip(cy+box.H))
	if r.Empty() {
		return domain.PersonImage{}, nil
	}
	img := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	for y := range r.Dy() {
		row := ((r.Min.Y+y)*side + r.Min.X) * 3
		for x := range r.Dx() {
			src, dst := row+x*3, img.PixOffset(x, y)
			img.Pix[dst], img.Pix[dst+1], img.Pix[dst+2], img.Pix[dst+3] = pix[src+2], pix[src+1], pix[src], 0xff
		}
	}
	return encodePersonImage(img)
}

// faceDetections converts the helper's faces to the shape the domain rules take.
func faceDetections(d faceproto.Detections) []domain.FaceDetection {
	out := make([]domain.FaceDetection, len(d.Faces))
	for i, f := range d.Faces {
		out[i] = domain.FaceDetection{
			Box:    domain.FaceBox{X: float64(f.X), Y: float64(f.Y), W: float64(f.W), H: float64(f.H)},
			Score:  float64(f.Score),
			Vector: f.Vector,
		}
		for k, l := range f.Landmarks {
			out[i].Landmarks[k] = float64(l)
		}
	}
	return out
}
