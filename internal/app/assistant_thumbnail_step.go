package app

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// runCall sends one call of a step; a composite row runs its stages instead.
func (a *Assistant) runCall(ctx context.Context, runner ActionRunner, call domain.AssistantCall) (domain.AssistantCallResult, error) {
	if row, ok := domain.LookupAssistantAction(call.ActionID); ok && row.Operation == "" {
		return a.runThumbnailEdit(ctx, runner, call)
	}
	return runner.Run(ctx, call)
}

// runThumbnailEdit runs an editVideoThumbnail step: it starts an edit session on the video, from the frame at
// positionMs or from the current thumbnail, runs the prompt in ComfyUI, waits for the run to end and applies its
// output. A stage that fails cancels and discards the session, so the thumbnail stays as it was.
func (a *Assistant) runThumbnailEdit(ctx context.Context, runner ActionRunner, call domain.AssistantCall) (domain.AssistantCallResult, error) {
	start := domain.AssistantCall{
		ActionID: call.ActionID, Operation: "startThumbnailEdit", Path: call.Path, Body: map[string]any{"source": "thumbnail"},
	}
	if at, ok := call.Body["positionMs"]; ok && at != nil {
		start.Body = map[string]any{"source": "frame", "positionMs": at}
	}
	started, err := runner.Run(ctx, start)
	if err != nil {
		return started, err
	}
	var session struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(started.Body, &session) != nil || session.ID == "" {
		return domain.AssistantCallResult{}, errors.New("the thumbnail edit session has no id")
	}
	path := map[string]string{"editId": session.ID}
	result, err := a.editThumbnail(ctx, runner, call, path)
	if err != nil {
		cleanup := context.WithoutCancel(ctx)
		for _, operation := range []string{"cancelThumbnailEdit", "discardThumbnailEdit"} {
			_, _ = runner.Run(cleanup, domain.AssistantCall{ActionID: call.ActionID, Operation: operation, Path: path})
		}
	}
	return result, err
}

func (a *Assistant) editThumbnail(
	ctx context.Context, runner ActionRunner, call domain.AssistantCall, path map[string]string,
) (domain.AssistantCallResult, error) {
	run := domain.AssistantCall{
		ActionID: call.ActionID, Operation: "runThumbnailEdit", Path: path,
		Body: map[string]any{"prompt": call.Body["prompt"]},
	}
	if result, err := runner.Run(ctx, run); err != nil {
		return result, err
	}
	if err := a.waitThumbnailEdit(ctx, runner, path["editId"]); err != nil {
		return domain.AssistantCallResult{}, err
	}
	return runner.Run(ctx, domain.AssistantCall{ActionID: call.ActionID, Operation: "applyThumbnailEdit", Path: path})
}

// waitThumbnailEdit reads the session until its run ends; a run that did not succeed is the step's error.
func (a *Assistant) waitThumbnailEdit(ctx context.Context, runner ActionRunner, editID string) error {
	args, _ := json.Marshal(map[string]string{"editId": editID}) // A string map always encodes.
	for {
		read, err := runner.Read(ctx, domain.AssistantRead{Operation: "getThumbnailEdit", Arguments: args})
		if err != nil {
			return err
		}
		var session struct {
			State string `json:"state"`
			Error *struct {
				Reason  string `json:"reason"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(read.Body, &session); err != nil {
			return err
		}
		switch session.State {
		case "succeeded":
			return nil
		case "failed", "cancelled":
			failure := &domain.AssistantCallError{Status: 409, Code: "thumbnail_edit_" + session.State}
			if session.Error != nil {
				failure.Reason, failure.Message = session.Error.Reason, session.Error.Message
			}
			return failure
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(a.editPoll):
		}
	}
}
