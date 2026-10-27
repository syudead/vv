package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"log/slog"
	"slices"

	"golang.org/x/image/draw"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/faceproto"
)

// People's face images are embedded between videos and checked on the owner's upload; see
// docs/design-docs/face-recognition.md.

// embedNext applies one face waiting for the worker, those named by events first, and reports false when none waits.
func (f *FaceRecognizer) embedNext(run context.Context, settings domain.FaceSettings) (domain.WorkerWait, bool) {
	job, err := f.nextFaceImage(run)
	if errors.Is(err, domain.ErrNoJob) {
		return domain.WorkerNext, false
	}
	if err != nil {
		return f.retryUnless(run, "cannot read a face image", err), true
	}
	f.claimed(job.Recognizer)
	if job.Recognizer != settings.Recognizer || run.Err() != nil {
		// The settings changed after the read; the face is read again under the new ones.
		f.pend(job.PersonID)
		return domain.WorkerNext, true
	}
	return f.embed(run, job), true
}

// nextFaceImage returns the face of a pending person, then any face without a current vector, or ErrNoJob.
func (f *FaceRecognizer) nextFaceImage(ctx context.Context) (domain.FaceImageJob, error) {
	for {
		id, ok := f.popPending()
		if !ok {
			return f.images.StaleFaceImage(ctx)
		}
		job, err := f.images.FaceImage(ctx, id)
		switch {
		case errors.Is(err, domain.ErrNotFound):
			continue
		case err != nil:
			f.pend(id)
		}
		return job, err
	}
}

func (f *FaceRecognizer) popPending() (int64, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.pending) == 0 {
		return 0, false
	}
	// The smallest id keeps the order stable.
	ids := make([]int64, 0, len(f.pending))
	for id := range f.pending {
		ids = append(ids, id)
	}
	id := slices.Min(ids)
	delete(f.pending, id)
	return id, true
}

// embed computes the face's vector unless one is stored, and stores it with the face-image rule's decision. A face
// whose attempt was cancelled, fenced, or hit a helper start or database error stays pending; a merge refused for good
// is not tried again, since retrying cannot change it.
func (f *FaceRecognizer) embed(run context.Context, job domain.FaceImageJob) domain.WorkerWait {
	vector, err := job.Vector, error(nil)
	if !job.Embedded {
		vector, err = f.embedFace(run, f.helperFor(job.Recognizer), job)
	}
	var decision domain.FaceImageDecision
	if err == nil {
		decision, err = f.images.StoreFaceImageVector(run, job, vector)
	}
	switch {
	case err == nil:
		if decision.MergeID != 0 {
			f.logger.Info("merged a provisional person by face", slog.Int64("person", job.PersonID),
				slog.Int64("provisional", decision.MergeID))
		}
		return domain.WorkerNext
	case errors.Is(err, domain.ErrFaceMergeRefused):
		f.logger.Warn("the face-image rule's merge was refused", slog.Int64("person", job.PersonID),
			slog.Any("error", err))
		return domain.WorkerNext
	case run.Err() != nil, errors.Is(err, domain.ErrFaceGenerationChanged), errors.Is(err, domain.ErrFaceHelperStart):
		// Read again under the new settings, or once the runtime is ready again.
		f.pend(job.PersonID)
		return domain.WorkerNext
	}
	f.logger.Error("cannot store a face image vector", slog.Int64("person", job.PersonID), slog.Any("error", err))
	f.pend(job.PersonID)
	return domain.WorkerRetry
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
