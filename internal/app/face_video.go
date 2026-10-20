package app

import (
	"image"
	"math"
	"slices"

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
	// lit is every part of the frame that a frame so far lit; padding and black bars never are.
	lit image.Rectangle
}

func newFaceVideo(th domain.FaceThresholds) *faceVideo {
	return &faceVideo{th: th, tracker: domain.NewFaceTracker(th), crops: map[domain.FaceSighting]domain.PersonImage{}}
}

// add takes the faces of the next frame; pix is that frame, a domain.FaceFrameSide square of packed BGR.
func (v *faceVideo) add(pix []byte, faces []domain.FaceDetection) error {
	v.lit = v.lit.Union(litBounds(pix))
	picture := v.lit
	if picture.Empty() {
		picture = image.Rect(0, 0, domain.FaceFrameSide, domain.FaceFrameSide)
	}
	faces = slices.Clone(faces)
	for i := range faces {
		faces[i].Cut = !boxRect(faces[i].Box).In(picture)
	}
	v.tracker.Add(faces)
	keep := map[domain.FaceSighting]bool{}
	for _, s := range v.tracker.BestSightings() {
		keep[s] = true
		if _, ok := v.crops[s]; ok || s.Frame != v.frame {
			continue
		}
		crop, err := cropFace(pix, picture, s.Box)
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
			FaceVector: p.FaceVector,
		}
	}
	return out
}

// faceBlackLevel is the brightest sample of a row or column still counted as padding or a black bar.
const faceBlackLevel = 16

// litBounds is the smallest rectangle holding every sample of the frame brighter than faceBlackLevel; empty
// for a frame that is black throughout. One dark frame can be smaller than its picture, so a video takes the
// union over its frames.
func litBounds(pix []byte) image.Rectangle {
	const side = domain.FaceFrameSide
	black := func(x0, y0, x1, y1 int) bool {
		for y := y0; y < y1; y++ {
			for _, b := range pix[(y*side+x0)*3 : (y*side+x1)*3] {
				if b > faceBlackLevel {
					return false
				}
			}
		}
		return true
	}
	r := image.Rect(0, 0, side, side)
	for r.Min.Y < r.Max.Y && black(0, r.Min.Y, side, r.Min.Y+1) {
		r.Min.Y++
	}
	if r.Empty() {
		return image.Rectangle{}
	}
	for black(0, r.Max.Y-1, side, r.Max.Y) {
		r.Max.Y--
	}
	for black(r.Min.X, r.Min.Y, r.Min.X+1, r.Max.Y) {
		r.Min.X++
	}
	for black(r.Max.X-1, r.Min.Y, r.Max.X, r.Max.Y) {
		r.Max.X--
	}
	return r
}

// boxRect is the smallest pixel rectangle holding the box.
func boxRect(b domain.FaceBox) image.Rectangle {
	return image.Rect(int(math.Floor(b.X)), int(math.Floor(b.Y)), int(math.Ceil(b.X+b.W)), int(math.Ceil(b.Y+b.H)))
}

// cropFace cuts the face box doubled around its centre from the picture, never enlarged. A window that reaches
// past the picture's edge is moved back inside it rather than cut, so a face near the edge stays whole, and is
// cut only where it is larger than the picture.
func cropFace(pix []byte, picture image.Rectangle, box domain.FaceBox) (domain.PersonImage, error) {
	const side = domain.FaceFrameSide
	place := func(centre, half float64, lo, hi int) (int, int) {
		size := min(int(math.Round(2*half)), hi-lo)
		start := int(math.Round(centre - half))
		start = max(lo, min(start, hi-size))
		return start, start + size
	}
	x0, x1 := place(box.X+box.W/2, box.W, picture.Min.X, picture.Max.X)
	y0, y1 := place(box.Y+box.H/2, box.H, picture.Min.Y, picture.Max.Y)
	r := image.Rect(x0, y0, x1, y1)
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
