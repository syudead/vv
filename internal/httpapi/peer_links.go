package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"maps"
	"slices"

	"github.com/syudead/vv/internal/domain"
)

// readLinks are the links this request reaches, in slot order: none for the peer's own request, which names this
// node's entities only, or for a node that reads none.
func (s *server) readLinks(ctx context.Context) ([]domain.PeerLinkState, error) {
	if s.pairing == nil || isPeerRequest(ctx) {
		return nil, nil
	}
	state, err := s.pairing.State(ctx)
	return state.Links, err
}

// connectedSlots are the slots of the connected links this request reaches, in slot order; a pairing that cannot
// be read reaches none.
func (s *server) connectedSlots(ctx context.Context) []int64 {
	links, err := s.readLinks(ctx)
	if err != nil {
		s.logger.Warn("could not read the pairing state", slog.Any("error", err))
		return nil
	}
	var slots []int64
	for _, link := range links {
		if link.State == domain.PeerConnected {
			slots = append(slots, link.Link.Slot)
		}
	}
	return slots
}

// linksConnected reports whether the link of every slot in slots that this node reads is connected; a slot
// without a link is left to its request, which answers not found.
func (s *server) linksConnected(ctx context.Context, slots []int64) (bool, error) {
	links, err := s.readLinks(ctx)
	if err != nil {
		return false, err
	}
	for _, link := range links {
		if slices.Contains(slots, link.Link.Slot) && link.State != domain.PeerConnected {
			return false, nil
		}
	}
	return true, nil
}

// sortedSlots are the slots of parts in slot order, the order every per-link write takes.
func sortedSlots[T any](parts map[int64]T) []int64 {
	return slices.Sorted(maps.Keys(parts))
}

// linkError is a failure of one link with that link's name, so the answer can name the link that caused it.
type linkError struct {
	name string
	err  error
}

func (e *linkError) Error() string { return e.name + ": " + e.err.Error() }

func (e *linkError) Unwrap() error { return e.err }

// namedLinkError is err with the name of the link in slot, or err alone when this node has no link there.
func (s *server) namedLinkError(ctx context.Context, slot int64, err error) error {
	links, readErr := s.readLinks(ctx)
	if readErr != nil {
		return err
	}
	for _, link := range links {
		if link.Link.Slot == slot {
			return &linkError{name: link.Link.Name, err: err}
		}
	}
	return err
}

// linkErrorName is the name of the link err names, empty when it names none.
func linkErrorName(err error) string {
	var link *linkError
	if errors.As(err, &link) {
		return link.name
	}
	return ""
}
