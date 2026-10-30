package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/syudead/vv/internal/domain"
)

// videosByIDs reads the videos that ids name: this node's from the library, and the peer's, which carry
// domain.PeerIDOffset, through the runner, which forwards the read as the screens do. A missing local id is left
// out; a peer's video the runner cannot read is the failure the model receives.
func (a *Assistant) videosByIDs(ctx context.Context, runner ActionRunner, ids []int64) ([]domain.Video, string, error) {
	local := slices.DeleteFunc(slices.Clone(ids), domain.IsRemoteID)
	var videos []domain.Video
	if len(local) > 0 {
		if a.videos == nil {
			return nil, "", errors.New("the assistant cannot read videos: it is not configured")
		}
		found, err := a.videos.VideosByIDs(ctx, domain.AudienceOwner, local)
		if err != nil {
			return nil, "", fmt.Errorf("cannot read the videos: %w", err)
		}
		videos = found
	}
	for _, id := range ids {
		if !domain.IsRemoteID(id) {
			continue
		}
		v, failure, err := remoteVideo(ctx, runner, id)
		if failure != "" || err != nil {
			return nil, failure, err
		}
		videos = append(videos, v)
	}
	return videos, "", nil
}

// remoteVideo reads the peer's video by its remote id; its plan key holds that id, since it has no user key here.
func remoteVideo(ctx context.Context, runner ActionRunner, id int64) (domain.Video, string, error) {
	if runner == nil {
		return domain.Video{}, fmt.Sprintf("the video %d of the other VVMDM cannot be read here", id), nil
	}
	args, _ := json.Marshal(map[string]int64{"id": id}) // A map of integers always encodes.
	result, err := runner.Read(ctx, domain.AssistantRead{Operation: "getVideo", Arguments: args})
	var failed *domain.AssistantCallError
	if errors.As(err, &failed) {
		return domain.Video{}, fmt.Sprintf("the video %d of the other VVMDM cannot be read: %s", id, failed.Message), nil
	}
	if err != nil {
		return domain.Video{}, "", fmt.Errorf("cannot read the video %d of the other VVMDM: %w", id, err)
	}
	var video struct {
		Title string `json:"title"`
	}
	if json.Unmarshal(result.Body, &video) != nil {
		return domain.Video{}, fmt.Sprintf("the other VVMDM answered the video %d in an unexpected form", id), nil
	}
	return domain.Video{ID: id, UserKey: domain.AssistantRemoteVideoKey(id), Title: video.Title}, "", nil
}

// remoteVideoRefusal is what the model receives for a row the screens do not offer for the peer's videos.
func remoteVideoRefusal(actionID string) string {
	return fmt.Sprintf("%s is not available for a video of the other VVMDM; tell the owner it can only be done "+
		"on the VVMDM that holds the video", domain.AssistantModelToolName(actionID))
}

func videoLabel(v domain.Video) domain.AssistantLabel {
	return domain.AssistantLabel{Kind: domain.AssistantLabelVideo, ID: v.ID, Key: v.UserKey, Name: v.Title}
}

// screenRemoteVideos finds, among the peer's videos the screen shows or selects, every one titled name, since the
// title search reads this node's library only. It reads at most assistantClarifyLimit of them, and a peer that
// cannot be read leaves the name to the title search.
func (t *assistantTurn) screenRemoteVideos(ctx context.Context, name string) ([]domain.Video, error) {
	c := t.input.Context
	var ids []int64
	if c.Route == domain.AssistantRouteVideo && domain.IsRemoteID(c.RouteID) {
		ids = append(ids, c.RouteID)
	}
	for _, id := range c.SelectedVideoIDs {
		if domain.IsRemoteID(id) && !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	videos, failure, err := t.a.videosByIDs(ctx, t.runner, ids[:min(len(ids), assistantClarifyLimit)])
	if failure != "" || err != nil {
		return nil, err
	}
	key := domain.FoldForMatch(name)
	return slices.DeleteFunc(videos, func(v domain.Video) bool { return domain.FoldForMatch(v.Title) != key }), nil
}

// assistantRemoteFailed is the step error's code for a bulk edit that answered 2xx while the peer's part failed;
// the reason is the peer's.
const assistantRemoteFailed = "remote_failed"

// remotePartFailure reads the peer's part of a bulk edit's answer, which reports its failure inside a 2xx body:
// how many of the call's count the peer did not change, and the step's error, nil when the part applied or the
// answer has none.
func remotePartFailure(body []byte, count int) (int, *domain.AssistantProposalError) {
	var answer struct {
		Remote *struct {
			FailedVideoIDs []int64 `json:"failedVideoIds"`
			Reason         string  `json:"reason"`
		} `json:"remote"`
	}
	if json.Unmarshal(body, &answer) != nil || answer.Remote == nil || len(answer.Remote.FailedVideoIDs) == 0 {
		return 0, nil
	}
	return min(len(answer.Remote.FailedVideoIDs), count), &domain.AssistantProposalError{
		Code: assistantRemoteFailed, Reason: answer.Remote.Reason,
		Message: "the other VVMDM did not apply its part of the change",
	}
}
