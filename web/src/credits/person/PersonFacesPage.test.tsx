import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import type { FaceCandidate } from "../../api/personFaces";
import {
  installPersonServer,
  person,
  renderPage,
  sent,
  sentBody,
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
    expect(screen.getByRole("tab", { name: /^Faces in videos\s*1$/ })).toBeTruthy();
    await user.click(screen.getByRole("link", { name: "Beach day from 2:10" }));

    expect((await screen.findByTestId("video-screen")).textContent).toBe(
      "/videos/5?t=130 autoplay",
    );
  });

  it("keeps a split face in place with Undo, which merges the new person back", async () => {
    const user = userEvent.setup();
    server.appearances = [
      { appearanceId: 3, videoId: 5, title: "Beach day", spans: [], hasCrop: true },
      { appearanceId: 4, videoId: 6, title: "Mountain", spans: [], hasCrop: true },
    ];
    renderPage("/people/7/faces");

    await user.click(
      await screen.findByRole("button", { name: "Not this person in Beach day" }),
    );

    expect(await screen.findByText("Split into #12")).toBeTruthy();
    expect(sent("POST", "/api/people/7/appearances/3/split")).toHaveLength(1);
    expect(screen.getByRole("img", { name: "Beach day" })).toBeTruthy();
    expect(
      screen.getByRole("button", { name: "Not this person in Mountain" }),
    ).toBeTruthy();

    await user.click(screen.getByRole("button", { name: "Undo the split in Beach day" }));

    expect(
      await screen.findByRole("button", { name: "Not this person in Beach day" }),
    ).toBeTruthy();
    expect(sentBody(sent("POST", "/api/people/7/merge")[0])).toEqual({ sourceIds: [12] });
  });

  it("splits selected faces into one person, and one Undo takes them all back", async () => {
    const user = userEvent.setup();
    server.appearances = [
      { appearanceId: 3, videoId: 5, title: "Beach day", spans: [], hasCrop: true },
      { appearanceId: 4, videoId: 6, title: "Mountain", spans: [], hasCrop: true },
      { appearanceId: 5, videoId: 7, title: "River", spans: [], hasCrop: true },
    ];
    renderPage("/people/7/faces");

    await user.click(await screen.findByRole("button", { name: "Select" }));
    await user.click(screen.getByRole("checkbox", { name: "Select Beach day" }));
    await user.click(screen.getByRole("checkbox", { name: "Select River" }));
    expect(screen.getByText("2 selected")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Not this person" }));

    expect(await screen.findAllByText("Split into #12")).toHaveLength(2);
    expect(sentBody(sent("POST", "/api/people/7/appearances/split")[0])).toEqual({
      appearanceIds: [3, 5],
    });
    expect(screen.queryByText("2 selected")).toBeNull();

    await user.click(screen.getByRole("button", { name: "Undo the split in River" }));

    await waitFor(() => expect(screen.queryByText("Split into #12")).toBeNull());
    expect(sentBody(sent("POST", "/api/people/7/merge")[0])).toEqual({ sourceIds: [12] });
  });

  it("orders the faces least like the person first, with each score, and can switch to newest", async () => {
    const user = userEvent.setup();
    server.appearances = [
      {
        appearanceId: 3,
        videoId: 5,
        title: "Beach day",
        spans: [],
        hasCrop: true,
        similarity: 0.21,
      },
    ];
    renderPage("/people/7/faces");

    expect(await screen.findByLabelText("Beach day, 21% like this person")).toBeTruthy();
    // The grid's reads carry an order; the pair rows' sample of four does not.
    const sorts = () =>
      sent("GET", "/api/people/7/appearances")
        .map((request) => request.url.searchParams.get("sort"))
        .filter((sort) => sort !== null);
    expect(sorts()).toEqual(["leastSimilar"]);

    // jsdom lacks the pointer APIs the Select's popup calls.
    Element.prototype.hasPointerCapture ??= () => false;
    Element.prototype.releasePointerCapture ??= () => undefined;
    Element.prototype.scrollIntoView ??= () => undefined;
    await user.click(screen.getByRole("combobox", { name: "Order" }));
    await user.click(await screen.findByRole("option", { name: "Newest video first" }));

    await waitFor(() => expect(sorts()).toEqual(["leastSimilar", "newest"]));
  });

  function candidate(id: number, name: string): FaceCandidate {
    return { person: { id, name, provisional: name === "" }, similarity: 0.72 };
  }

  it("moves to the merged person's faces when the shown person was merged away", async () => {
    const user = userEvent.setup();
    server.person = person({ name: "", provisional: true });
    server.faceCandidates = [candidate(9, "Alice")];
    server.mergeTarget = person({ id: 9, name: "Alice" });
    renderPage("/people/7/faces");

    await user.click(await screen.findByRole("button", { name: "Same person as Alice" }));

    expect(await screen.findByText('Merged into "Alice"')).toBeTruthy();
    expect(await screen.findByRole("link", { name: "Back to Alice" })).toBeTruthy();
    expect(sent("POST", "/api/people/7/face-candidates/9/confirm")).toHaveLength(1);
  });

  it("keeps a confirmed pair in place and reads nothing again until Refresh", async () => {
    const user = userEvent.setup();
    server.person = person({ imageRevision: 2 });
    server.faceCandidates = [candidate(9, ""), candidate(10, "Mio")];
    server.mergeTarget = person({ videoCount: 2 });
    renderPage("/people/7/faces");

    // The shown person's face stands beside each candidate's.
    const row = await screen.findByRole("group", { name: "Pair with #9" });
    expect(within(row).getByRole("img", { name: "Aoi" }).getAttribute("src")).toBe(
      "/api/people/7/image?v=2",
    );
    const candidateReads = () => sent("GET", "/api/people/7/face-candidates").length;
    const faceReads = () => sent("GET", "/api/people/7/appearances").length;
    const before = { candidates: candidateReads(), faces: faceReads() };

    await user.click(screen.getByRole("button", { name: "Same person as #9" }));

    expect(await screen.findByText("Merged into Aoi")).toBeTruthy();
    expect(screen.getByRole("link", { name: "Open #9" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Same person as Mio" })).toBeTruthy();
    expect(screen.getByRole("link", { name: "Back to Aoi" })).toBeTruthy();
    expect({ candidates: candidateReads(), faces: faceReads() }).toEqual(before);

    await user.click(screen.getByRole("button", { name: "Refresh" }));

    await waitFor(() =>
      expect(screen.queryByRole("link", { name: "Open #9" })).toBeNull(),
    );
    expect(candidateReads()).toBeGreaterThan(before.candidates);
    expect(faceReads()).toBeGreaterThan(before.faces);
  });

  it("undoes Same person by its merge entry and offers the pair again", async () => {
    const user = userEvent.setup();
    server.faceCandidates = [candidate(9, "")];
    server.mergeTarget = person();
    renderPage("/people/7/faces");

    await user.click(await screen.findByRole("button", { name: "Same person as #9" }));
    expect(await screen.findByText("Merged into Aoi")).toBeTruthy();

    await user.click(screen.getByRole("button", { name: "Undo for #9" }));

    expect(await screen.findByRole("button", { name: "Same person as #9" })).toBeTruthy();
    expect(sent("POST", "/api/activity/31/undo")).toHaveLength(1);
    expect(sent("POST", "/api/people/7/face-candidates/9/reopen")).toHaveLength(1);
  });

  it("keeps a dismissed pair with Undo, which reopens it", async () => {
    const user = userEvent.setup();
    server.faceCandidates = [candidate(9, "Alice"), candidate(10, "Mio")];
    renderPage("/people/7/faces");

    await user.click(
      await screen.findByRole("button", { name: "Not the same as Alice" }),
    );

    const row = screen.getByRole("group", { name: "Pair with Alice" });
    expect(await within(row).findByText("Not the same")).toBeTruthy();
    expect(sent("POST", "/api/people/7/face-candidates/9/dismiss")).toHaveLength(1);
    expect(screen.getByRole("link", { name: "Open Alice" })).toBeTruthy();

    // The result survives a visit to the other tab.
    await user.click(screen.getByRole("tab", { name: /^Faces in videos/ }));
    await user.click(screen.getByRole("tab", { name: /^Same person\?/ }));
    await user.click(screen.getByRole("button", { name: "Undo for Alice" }));

    expect(
      await screen.findByRole("button", { name: "Not the same as Alice" }),
    ).toBeTruthy();
    expect(sent("POST", "/api/people/7/face-candidates/9/reopen")).toHaveLength(1);
  });

  it("shows the gone state when the person no longer exists", async () => {
    server.person = null;
    renderPage("/people/7/faces");

    expect(await screen.findByText("This person no longer exists")).toBeTruthy();
  });
});
