package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"log/slog"

	"golang.org/x/image/draw"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/faceproto"
)

// People's face images are embedded by face_image jobs and checked on the owner's upload; see
// docs/design-docs/face-recognition.md.

// embedJob embeds the face of the job's person unless a vector of it is stored, and stores it with the face-image
// rule's decision; a person without a face needs nothing.
func (f *FaceRecognizer) embedJob(ctx context.Context, job domain.Job) error {
	run, end := f.begin(ctx)
	defer end()
	image, err := f.images.FaceImage(run, job.PersonID)
	if errors.Is(err, domain.ErrNotFound) {
		return nil
	}
	if err != nil {
		return f.outcome(ctx, run, err)
	}
	if err := f.start(run, image.Recognizer); err != nil {
		return f.outcome(ctx, run, err)
	}
	return f.outcome(ctx, run, f.embed(run, image))
}

// embed computes the face's vector unless one is stored, and stores it; a merge refused for good is logged and not
// tried again, since retrying cannot change it.
func (f *FaceRecognizer) embed(run context.Context, job domain.FaceImageJob) error {
	vector, err := job.Vector, error(nil)
	if !job.Embedded {
		vector, err = f.embedFace(run, f.helperFor(job.Recognizer), job)
	}
	if err != nil {
		return err
	}
	decision, err := f.images.StoreFaceImageVector(run, job, vector)
	switch {
	case errors.Is(err, domain.ErrFaceMergeRefused):
		f.logger.Warn("the face-image rule's merge was refused", slog.Int64("person", job.PersonID),
			slog.Any("error", err))
		return nil
	case err != nil:
		return err
	case decision.MergeID != 0:
		f.logger.Info("merged a provisional person by face", slog.Int64("person", job.PersonID),
			slog.Int64("provisional", decision.MergeID))
	}
	return nil
}

// embedFace returns the vector of the face's only good face, nil when it has none or several or the helper cannot
// read it; it returns an error only when the helper did not start or ctx ended.
func (f *FaceRecognizer) embedFace(ctx context.Context, helper FaceHelper, job domain.FaceImageJob) ([]float32, error) {
	vector, err := faceImageVector(ctx, helper, job.JPEG)
	switch {
	case err == nil:
		return vector, nil
	case ctx.Err() != nil:
		return nil, ctx.Err()
	case errors.Is(err, domain.ErrFaceHelperStart):
		return nil, err
	case !errors.Is(err, domain.ErrPersonImageNoFace) && !errors.Is(err, domain.ErrPersonImageManyFaces):
		// The face is not tried again until it changes, as a video whose helper fails is not.
		f.logger.Warn("cannot embed a face image", slog.Int64("person", job.PersonID), slog.Any("error", err))
	}
	return nil, nil
}

// faceImageVector detects the faces of a stored face's JPEG and returns the vector of its only good face.
func faceImageVector(ctx context.Context, helper FaceHelper, data []byte) ([]float32, error) {
	req, err := faceImageRequest(data)
	if err != nil {
		return nil, err
	}
	faces, err := detectFaces(ctx, helper, req)
	if err != nil {
		return nil, err
	}
	return domain.FaceImageVector(faces)
}

// faceImageRequest decodes a stored face into packed BGR for the helper.
func faceImageRequest(data []byte) (faceproto.DetectRequest, error) {
	decoded, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return faceproto.DetectRequest{}, fmt.Errorf("cannot read the face image: %w", err)
	}
	bounds := decoded.Bounds()
	rgba := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(rgba, rgba.Bounds(), decoded, bounds.Min, draw.Src)
	pix := make([]byte, bounds.Dx()*bounds.Dy()*3)
	for i := range bounds.Dx() * bounds.Dy() {
		pix[3*i], pix[3*i+1], pix[3*i+2] = rgba.Pix[4*i+2], rgba.Pix[4*i+1], rgba.Pix[4*i]
	}
	return faceproto.DetectRequest{Width: bounds.Dx(), Height: bounds.Dy(), Pix: pix}, nil
}
