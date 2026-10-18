package app

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/jpeg"

	"golang.org/x/image/draw"

	"github.com/syudead/vv/internal/domain"
)

// PersonImageStore keeps one fitted face per person.
type PersonImageStore interface {
	PersonImage(ctx context.Context, id int64) (domain.PersonImage, error)
	SetPersonImage(ctx context.Context, id int64, image domain.PersonImage) (domain.Credit, error)
	// AddPersonImage stores image only when the person has no face, reporting whether it did.
	AddPersonImage(ctx context.Context, id int64, image domain.PersonImage) (bool, error)
	DeletePersonImage(ctx context.Context, id int64) (domain.Credit, error)
}

// PersonImages fits uploaded faces under the thumbnail uploads' gate, so the two kinds of upload never decode at once.
type PersonImages struct {
	store   PersonImageStore
	uploads uploadGate
}

// NewPersonImages shares ingest's upload gate.
func NewPersonImages(store PersonImageStore, ingest *Ingest) *PersonImages {
	return &PersonImages{store: store, uploads: ingest.uploads}
}

const personImageJPEGQuality = 85

// UploadPersonImage decodes, fits and stores a face. A body over the byte limit returns
// ThumbnailUploadTooLargeError; any other unacceptable image returns ErrPersonImageInvalid; nothing is written on error.
func (p *PersonImages) UploadPersonImage(ctx context.Context, id int64, data []byte) (domain.Credit, error) {
	fitted, err := p.fit(ctx, data)
	if err != nil {
		return domain.Credit{}, err
	}
	return p.store.SetPersonImage(ctx, id, fitted)
}

// fit runs fitPersonImage under the upload gate.
func (p *PersonImages) fit(ctx context.Context, data []byte) (domain.PersonImage, error) {
	release, err := p.uploads.acquire(ctx)
	if err != nil {
		return domain.PersonImage{}, err
	}
	defer release()
	return fitPersonImage(data)
}

func (p *PersonImages) PersonImage(ctx context.Context, id int64) (domain.PersonImage, error) {
	return p.store.PersonImage(ctx, id)
}

func (p *PersonImages) DeletePersonImage(ctx context.Context, id int64) (domain.Credit, error) {
	return p.store.DeletePersonImage(ctx, id)
}

// fitPersonImage scales the image down to PersonImageMaxSide on its long side, never up, over black for transparency.
func fitPersonImage(data []byte) (domain.PersonImage, error) {
	decoded, err := decodeUpload(data)
	var tooLarge *domain.ThumbnailUploadTooLargeError
	switch {
	case errors.As(err, &tooLarge) && tooLarge.Limit == domain.ThumbnailUploadMaxBytes:
		return domain.PersonImage{}, err
	case err != nil:
		return domain.PersonImage{}, domain.ErrPersonImageInvalid
	}
	bounds := decoded.Bounds()
	width, height := fitWithin(bounds.Dx(), bounds.Dy(), domain.PersonImageMaxSide)
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(canvas, canvas.Bounds(), image.Black, image.Point{}, draw.Src)
	draw.CatmullRom.Scale(canvas, canvas.Bounds(), decoded, bounds, draw.Over, nil)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, canvas, &jpeg.Options{Quality: personImageJPEGQuality}); err != nil {
		return domain.PersonImage{}, err
	}
	return domain.PersonImage{JPEG: buf.Bytes(), Width: width, Height: height}, nil
}

// fitWithin keeps the aspect ratio, rounding the short side and keeping it at least 1 px.
func fitWithin(width, height, maxSide int) (int, int) {
	if width <= maxSide && height <= maxSide {
		return width, height
	}
	if width >= height {
		return maxSide, max(1, (height*maxSide+width/2)/width)
	}
	return max(1, (width*maxSide+height/2)/height), maxSide
}
