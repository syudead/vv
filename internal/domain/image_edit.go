package domain

import "fmt"

type ThumbnailEditState string

const (
	ThumbnailEditReady  ThumbnailEditState = "ready"
	ThumbnailEditQueued ThumbnailEditState = "queued"
	// ThumbnailEditPreparing is reported only by Comfy Cloud.
	ThumbnailEditPreparing ThumbnailEditState = "preparing"
	ThumbnailEditRunning   ThumbnailEditState = "running"
	ThumbnailEditSucceeded ThumbnailEditState = "succeeded"
	ThumbnailEditFailed    ThumbnailEditState = "failed"
	ThumbnailEditCancelled ThumbnailEditState = "cancelled"
)

func (s ThumbnailEditState) Terminal() bool {
	return s == ThumbnailEditSucceeded || s == ThumbnailEditFailed || s == ThumbnailEditCancelled
}

type ImageEditFailure string

const (
	// ImageEditUnreachable includes connection/DNS failures and HTTP 5xx.
	ImageEditUnreachable  ImageEditFailure = "unreachable"
	ImageEditUnauthorized ImageEditFailure = "unauthorized"
	// ImageEditInsufficientCredits includes HTTP 402 and provider errors naming credits.
	ImageEditInsufficientCredits ImageEditFailure = "insufficient_credits"
	ImageEditMissingModels       ImageEditFailure = "missing_models"
	ImageEditWorkflowRejected    ImageEditFailure = "workflow_rejected"
	ImageEditRunFailed           ImageEditFailure = "run_failed"
	// ImageEditNoOutput means success without an image in vv:output.
	ImageEditNoOutput ImageEditFailure = "no_output"
	// ImageEditTimedOut is enforced by the edit session after ten minutes.
	ImageEditTimedOut ImageEditFailure = "timed_out"
)

// ImageEditError.Message contains a provider explanation or local diagnosis, never the raw response body, and may be empty.
type ImageEditError struct {
	Reason  ImageEditFailure
	Message string
}

func (e *ImageEditError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("the image edit failed (%s)", e.Reason)
	}
	return fmt.Sprintf("the image edit failed (%s): %s", e.Reason, e.Message)
}

type ImageEditInput struct {
	Frame []byte
	// Mask is a frame-sized PNG with opaque white selected pixels and black elsewhere; nil makes the adapter send an all-white mask.
	Mask []byte
	// A nil Reference selects a workflow without a reference image.
	Reference []byte
	// An empty Instruction with a painted mask selects the adapter's default instruction.
	Instruction string
}

// ImageEditJob retains the submission-time Connection even if settings change; ID and Output are provider identifiers.
type ImageEditJob struct {
	Connection ComfyUISettings
	ID         string
	Output     string
}

// ImageEditProgress is optional provider-reported step progress and does not determine state.
type ImageEditProgress struct {
	Value int
	Max   int
}
