package app

import (
	"bytes"
	"context"
	"errors"
	"image/jpeg"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// personImageStoreFunc records the stored face; reads are not used by the upload.
type personImageStoreFunc func(id int64, image domain.PersonImage) (domain.Credit, error)

func (f personImageStoreFunc) PersonImage(context.Context, int64) (domain.PersonImage, error) {
	return domain.PersonImage{}, errors.New("not used")
}

func (f personImageStoreFunc) SetPersonImage(_ context.Context, id int64, image domain.PersonImage) (domain.Credit, error) {
	return f(id, image)
}

func (f personImageStoreFunc) AddPersonImage(context.Context, int64, domain.PersonImage) (bool, error) {
	return false, errors.New("not used")
}

func (f personImageStoreFunc) DeletePersonImage(context.Context, int64) (domain.Credit, error) {
	return domain.Credit{}, errors.New("not used")
}

func TestUploadPersonImageFitsTheLongSideTo800(t *testing.T) {
	var stored domain.PersonImage
	images := &PersonImages{uploads: newUploadGate(), store: personImageStoreFunc(func(id int64, image domain.PersonImage) (domain.Credit, error) {
		if id != 7 {
			t.Errorf("id = %d", id)
		}
		stored = image
		return domain.Credit{ID: id, ImageRevision: 1}, nil
	})}
	if _, err := images.UploadPersonImage(context.Background(), 7, testPNG(t, 2000, 3000)); err != nil {
		t.Fatal(err)
	}
	config, err := jpeg.DecodeConfig(bytes.NewReader(stored.JPEG))
	if err != nil {
		t.Fatalf("保存した画像が JPEG でない: %v", err)
	}
	if config.Width != 533 || config.Height != 800 || stored.Width != 533 || stored.Height != 800 {
		t.Errorf("JPEG %d×%d, 記録 %d×%d, want 533×800", config.Width, config.Height, stored.Width, stored.Height)
	}
}

func TestUploadPersonImageRefusesAnimatedWebPWithoutStoring(t *testing.T) {
	images := &PersonImages{uploads: newUploadGate(), store: personImageStoreFunc(func(int64, domain.PersonImage) (domain.Credit, error) {
		t.Error("アニメーションの WebP を保存した")
		return domain.Credit{}, nil
	})}
	if _, err := images.UploadPersonImage(context.Background(), 7, animatedWebPImage()); !errors.Is(err, domain.ErrPersonImageInvalid) {
		t.Fatalf("err = %v, want ErrPersonImageInvalid", err)
	}
}
