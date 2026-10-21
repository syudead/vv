import { fireEvent, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import type { FaceCandidate } from "../../api/personFaces";
import {
  installPersonServer,
  person,
  renderPage,
  sent,
  server,
} from "../../testing/personPage";

installPersonServer();

describe("PersonFacesPage", () => {
  it("opens the video at a span's start, playing, from Faces in videos", async () => {
    const user = userEvent.setup();
    server.appearances = [
      {
        appearanceId: 3,
        videoId: 5,
        title: "Beach day",
        spans: [
          { start: 12.4, end: 100 },
          { start: 130, end: 150 },
        ],
        hasCrop: true,
      },
    ];
    renderPage("/people/7/faces");

    const crop = await screen.findByRole("img", { name: "Beach day" });
    expect(crop.getAttribute("src")).toBe("/api/face-appearances/3/crop");
    await user.click(screen.getByRole("link", { name: "Beach day from 2:10" }));

    expect((await screen.findByTestId("video-screen")).textContent).toBe(
      "/videos/5?t=130 autoplay",
    );
  });

  it("splits a face into a new person from Faces in videos and removes its row", async () => {
    const user = userEvent.setup();
    server.appearances = [
      { appearanceId: 3, videoId: 5, title: "Beach day", spans: [], hasCrop: true },
      { appearanceId: 4, videoId: 6, title: "Mountain", spans: [], hasCrop: true },
    ];
    renderPage("/people/7/faces");

    await user.click(
      await screen.findByRole("button", { name: "Not this person in Beach day" }),
    );

    expect(await screen.findByText("Split into a new unnamed person")).toBeTruthy();
    expect(sent("POST", "/api/people/7/appearances/3/split")).toHaveLength(1);
    expect(
      screen.queryByRole("button", { name: "Not this person in Beach day" }),
    ).toBeNull();
    expect(
      screen.getByRole("button", { name: "Not this person in Mountain" }),
    ).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Open" }));
    await waitFor(() => expect(sent("GET", "/api/people/12").length).toBeGreaterThan(0));
  });

  it("reads the next page of Faces in videos when a split leaves no row shown", async () => {
    const user = userEvent.setup();
    server.appearances = [
      { appearanceId: 3, videoId: 5, title: "Beach day", spans: [], hasCrop: true },
    ];
    server.laterAppearances = [
      { appearanceId: 4, videoId: 6, title: "Mountain", spans: [], hasCrop: true },
    ];
    renderPage("/people/7/faces");

    await user.click(
      await screen.findByRole("button", { name: "Not this person in Beach day" }),
    );

    expect(
      await screen.findByRole("button", { name: "Not this person in Mountain" }),
    ).toBeTruthy();
    expect(
      sent("GET", "/api/people/7/appearances").filter(
        (request) => request.url.searchParams.get("cursor") === "page-2",
      ),
    ).toHaveLength(1);
  });

  function candidate(id: number, name: string): FaceCandidate {
    return { person: { id, name, provisional: name === "" }, similarity: 0.72 };
  }

  it("shows the shown person's face beside the candidate's for comparison", async () => {
    server.person = person({ imageRevision: 2 });
    server.faceCandidates = [
      {
        person: { id: 9, name: "Alice", provisional: false, imageRevision: 4 },
        similarity: 0.72,
      },
    ];
    renderPage("/people/7/faces");

    const shown = await screen.findByRole("img", { name: "Aoi" });
    expect(shown.getAttribute("src")).toBe("/api/people/7/image?v=2");
    expect(screen.getByRole("img", { name: "Alice" }).getAttribute("src")).toBe(
      "/api/people/9/image?v=4",
    );
  });

  it("moves to the merged person's faces when the shown person was merged away", async () => {
    const user = userEvent.setup();
    server.person = person({ name: "", provisional: true });
    server.faceCandidates = [candidate(9, "Alice")];
    server.mergeTarget = person({ id: 9, name: "Alice" });
    renderPage("/people/7/faces");

    expect(await screen.findByText("72% similar")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Same person as Alice" }));

    expect(await screen.findByText('Merged into "Alice"')).toBeTruthy();
    expect(await screen.findByRole("link", { name: "Back to Alice" })).toBeTruthy();
    expect(sent("POST", "/api/people/7/face-candidates/9/confirm")).toHaveLength(1);
  });

  it("keeps the page and reads the faces again when the shown person gains the other", async () => {
    const user = userEvent.setup();
    server.faceCandidates = [candidate(9, "")];
    server.mergeTarget = person({ videoCount: 2 });
    renderPage("/people/7/faces");
    await screen.findByText("No faces found in videos yet.");
    const faceReads = sent("GET", "/api/people/7/appearances").length;

    await user.click(
      await screen.findByRole("button", { name: "Same person as Unnamed person" }),
    );

    expect(await screen.findByText('Merged "Unnamed person"')).toBeTruthy();
    expect(screen.getByRole("link", { name: "Back to Aoi" })).toBeTruthy();
    expect(screen.queryByRole("heading", { name: "Same person?" })).toBeNull();
    await waitFor(() =>
      expect(sent("GET", "/api/people/7/appearances").length).toBeGreaterThan(faceReads),
    );
  });

  it("removes a dismissed candidate without leaving the page", async () => {
    const user = userEvent.setup();
    server.faceCandidates = [candidate(9, "Alice"), candidate(10, "Mio")];
    renderPage("/people/7/faces");

    await user.click(
      await screen.findByRole("button", { name: "Not the same as Alice" }),
    );

    await waitFor(() =>
      expect(screen.queryByRole("link", { name: "Open Alice" })).toBeNull(),
    );
    expect(screen.getByRole("link", { name: "Open Mio" })).toBeTruthy();
    expect(screen.getByRole("link", { name: "Back to Aoi" })).toBeTruthy();
    expect(sent("POST", "/api/people/7/face-candidates/9/dismiss")).toHaveLength(1);
  });

  it("shows the gone state when the person no longer exists", async () => {
    server.person = null;
    renderPage("/people/7/faces");

    expect(await screen.findByText("This person no longer exists")).toBeTruthy();
  });
});
