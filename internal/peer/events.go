package peer

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// streamIdleTimeout ends a stream that sent nothing, not even the peer's 30 s keep-alive comment, so a
// connection that died without closing still counts as lost.
const streamIdleTimeout = 75 * time.Second

// maxEventLine bounds one line of the event stream; the forwarded events carry ids only.
const maxEventLine = 1 << 20

// Watch holds the peer's /api/events open with secret. It calls opened once the peer answered 200 and
// changed for each video, group and smartFolder event, with the ids in remote form. It returns when the
// stream ends: ctx's error after cancellation, else the domain's pairing error.
func (c *Client) Watch(
	ctx context.Context, target domain.PeerTarget, secret string, opened func(), changed func(domain.PeerChanged),
) error {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	resp, err := c.send(ctx, target, http.MethodGet, "/api/events", secret, nil, 0)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
		return refusal(resp.StatusCode, body, target.Address)
	}
	opened()
	idle := time.AfterFunc(streamIdleTimeout, func() { cancel(errStreamIdle) })
	defer idle.Stop()
	err = readEvents(resp.Body, func() { idle.Reset(streamIdleTimeout) }, func(name string, data []byte) {
		if event, ok := forwardedEvent(name, data); ok {
			changed(event)
		}
	})
	switch cause := context.Cause(ctx); {
	case errors.Is(cause, errStreamIdle):
		err = cause
	case cause != nil:
		return ctx.Err()
	}
	return &domain.PeerUnreachableError{
		Message: "The event stream of the other VVMDM at " + target.Address + " ended.", Cause: err,
	}
}

var errStreamIdle = errors.New("the event stream sent nothing for too long")

// readEvents parses Server-Sent Events, calling line for every line read and dispatch for each event.
func readEvents(body io.Reader, line func(), dispatch func(name string, data []byte)) error {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 4096), maxEventLine)
	var (
		name    string
		data    []byte
		hasData bool
	)
	for scanner.Scan() {
		line()
		text := scanner.Bytes()
		switch {
		case len(text) == 0:
			if hasData {
				dispatch(name, data)
			}
			name, data, hasData = "", nil, false
		case text[0] == ':':
		default:
			field, value, _ := bytes.Cut(text, []byte(":"))
			value = bytes.TrimPrefix(value, []byte(" "))
			switch string(field) {
			case "event":
				name = string(value)
			case "data":
				if hasData {
					data = append(data, '\n')
				}
				// value points into the scanner's buffer, which the next line overwrites.
				data, hasData = append(data, value...), true
			}
		}
	}
	return scanner.Err()
}

// forwardedEvent turns the peer's change notification into this node's, with remote ids; other events
// and malformed data are dropped.
func forwardedEvent(name string, data []byte) (domain.PeerChanged, bool) {
	switch name {
	case "video":
		var event gen.VideoChanged
		if json.Unmarshal(data, &event) != nil {
			return domain.PeerChanged{}, false
		}
		ids := remoteIDs([]int64{event.Id})
		return domain.PeerChanged{VideoIDs: ids}, len(ids) > 0
	case "group":
		var event gen.GroupChanged
		if json.Unmarshal(data, &event) != nil {
			return domain.PeerChanged{}, false
		}
		ids := remoteIDs(event.Ids)
		return domain.PeerChanged{GroupIDs: ids}, len(ids) > 0
	case "smartFolder":
		var event gen.SmartFolderChanged
		if json.Unmarshal(data, &event) != nil {
			return domain.PeerChanged{}, false
		}
		ids := remoteIDs(event.Ids)
		return domain.PeerChanged{SmartFolderIDs: ids}, len(ids) > 0
	}
	return domain.PeerChanged{}, false
}

// remoteIDs drops ids that are already remote on the peer: they name this node's own entities, and
// forwarding them back would bounce between the two nodes.
func remoteIDs(ids []int64) []int64 {
	var remote []int64
	for _, id := range ids {
		if id < domain.PeerIDOffset {
			remote = append(remote, domain.ToRemoteID(id))
		}
	}
	return remote
}
