package wasmplugin

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// runPeople has the probe return one performer per image URL, named p0, p1, ….
func runPeople(t *testing.T, rt *Runtime, images ...string) (domain.WorkInfoRunResult, error) {
	t.Helper()
	people := make([]map[string]string, len(images))
	for i, image := range images {
		people[i] = map[string]string{"role": "performer", "name": fmt.Sprintf("p%d", i), "image": image}
	}
	encoded, err := json.Marshal(people)
	if err != nil {
		t.Fatal(err)
	}
	req := fetchMode("people")
	req.Settings = map[string]string{"people": string(encoded)}
	got, _, err := rt.Run(context.Background(), probeDigest, probeModule(t), req, domain.WorkInfoRunNetwork{}, nil)
	return got, err
}

func TestPersonImagesKeepTheFirstSevenInResultOrder(t *testing.T) {
	site := newTestSite(t)
	rt := newNetTestRuntime(t, site)
	images := make([]string, 10)
	for i := range images {
		images[i] = fmt.Sprintf("http://site.test/face/%d", i)
	}

	got, err := runPeople(t, rt, images...)
	if err != nil || len(got.WorkInfo.People) != 10 || len(got.PersonImages) != 10 {
		t.Fatalf("Run(people) = %d people, %d images, %v; want 10 and 10", len(got.WorkInfo.People), len(got.PersonImages), err)
	}
	for i, image := range got.PersonImages {
		want := ""
		if i < domain.WorkInfoPersonImagesMaxCount {
			want = fmt.Sprintf("face %d", i)
		}
		if string(image) != want {
			t.Errorf("PersonImages[%d] = %q, want %q", i, image, want)
		}
	}
	site.mu.Lock()
	defer site.mu.Unlock()
	if len(site.hosts) != domain.WorkInfoPersonImagesMaxCount {
		t.Fatalf("site saw %d requests, want %d", len(site.hosts), domain.WorkInfoPersonImagesMaxCount)
	}
}

func TestPersonImageThatCannotBeFetchedIsDroppedAlone(t *testing.T) {
	site := newTestSite(t)
	rt := newNetTestRuntime(t, site)

	got, err := runPeople(t, rt, "http://site.test/face/0", "http://site.test/missing", "http://site.test/face/2")
	if err != nil || len(got.PersonImages) != 3 {
		t.Fatalf("Run(people) = %d images, %v; want 3 entries", len(got.PersonImages), err)
	}
	if string(got.PersonImages[0]) != "face 0" || got.PersonImages[1] != nil || string(got.PersonImages[2]) != "face 2" {
		t.Fatalf("PersonImages = %q, want the two faces around a dropped one", got.PersonImages)
	}
}

func TestPersonImageOnARefusedAddressFailsTheRun(t *testing.T) {
	site := newTestSite(t)
	rt := newNetTestRuntime(t, site)

	_, err := runPeople(t, rt, "http://site.test/face/0", "http://lan.test/face.png")
	wantFailure(t, err, domain.WorkInfoRunBlockedAddress)
	if host := runErrorOf(err).Host; host != "lan.test" {
		t.Fatalf("host = %q, want lan.test", host)
	}
}
