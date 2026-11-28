package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// A change by name whose read of this node failed answers 500 naming that read, and leaves any other error to the
// local write's answer.
func TestVocabularyChangeErrorNamesTheFailedRead(t *testing.T) {
	cause := errors.New("disk")
	tests := []struct {
		step domain.VocabularyStep
		want string
	}{
		{step: domain.VocabularyReadLinks, want: "Could not read the pairing."},
		{step: domain.VocabularyLoadTag, want: "Could not load the tag."},
		{step: domain.VocabularyLookUpTagName, want: "Could not look up the tag name."},
		{step: domain.VocabularyReadCreditNames, want: "Could not read the names of people and organizations."},
	}
	srv := &server{logger: discardLogger()}
	for _, tc := range tests {
		t.Run(string(tc.step), func(t *testing.T) {
			rec := httptest.NewRecorder()
			err := fmt.Errorf("rename: %w", &domain.VocabularyStepError{Step: tc.step, Err: cause})

			srv.writeVocabularyChangeError(rec, err, func(http.ResponseWriter, error) { t.Error("fallback answered") })

			got := decodeErrorBody(t, rec.Body.Bytes())
			if rec.Code != http.StatusInternalServerError || got.Code != gen.ErrorCode(codeInternal) || got.Message != tc.want {
				t.Errorf("status %d, body %+v; want 500 %q", rec.Code, got, tc.want)
			}
		})
	}
	t.Run("the local write", func(t *testing.T) {
		var answered error
		srv.writeVocabularyChangeError(httptest.NewRecorder(), cause, func(_ http.ResponseWriter, err error) { answered = err })
		if !errors.Is(answered, cause) {
			t.Errorf("fallback got %v, want %v", answered, cause)
		}
	})
}

type unreadablePairing struct{ Pairing }

func (unreadablePairing) State(context.Context) (domain.PeerState, error) {
	return domain.PeerState{}, errors.New("pairing store down")
}

// The rename handler carries the use case's failed read to its answer and renames nothing.
func TestRenameTagWithAnUnreadablePairingSaysSo(t *testing.T) {
	tags := &fakeTags{}
	srv := &server{logger: discardLogger(), tags: tags, vocabulary: testVocabulary(tags, nil, nil), pairing: unreadablePairing{}}
	r := httptest.NewRequest(http.MethodPatch, "/", strings.NewReader(`{"name":"ねこ"}`))
	r.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.RenameTag(rec, r, 3)

	got := decodeErrorBody(t, rec.Body.Bytes())
	if rec.Code != http.StatusInternalServerError || got.Message != "Could not read the pairing." || tags.operation != "" {
		t.Errorf("status %d, body %+v, store operation %q; want 500 naming the pairing and no rename",
			rec.Code, got, tags.operation)
	}
}

// A server cannot be built without the vocabulary use case, which owns the tag-name lock.
func TestNewServerRequiresTheVocabularyUseCase(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("NewServer built a server without Vocabulary")
		}
	}()
	NewServer(Options{Assets: emptyAssets{}, Auth: ownerAuth{}})
}
