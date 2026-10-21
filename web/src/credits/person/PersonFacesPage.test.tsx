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

  it("reads the rest whole after a split rescored the faces, adding only unseen ones", async () => {
    const user = userEvent.setup();
    server.appearances = [
      { appearanceId: 3, videoId: 5, title: "Beach day", spans: [], hasCrop: true },
      { appearanceId: 4, videoId: 6, title: "Mountain", spans: [], hasCrop: true },
    ];
    server.laterAppearances = [
      { appearanceId: 5, videoId: 7, title: "River", spans: [], hasCrop: true },
    ];
    renderPage("/people/7/faces?tab=faces");

    await user.click(
      await screen.findByRole("button", { name: "Not this person in Beach day" }),
    );
    await screen.findByText("Split into #12");
    await user.click(screen.getByRole("button", { name: "Show more" }));

    expect(await screen.findByRole("img", { name: "River" })).toBeTruthy();
    expect(screen.getAllByRole("img", { name: "Mountain" })).toHaveLength(1);
    const more = sent("GET", "/api/people/7/appearances").at(-1);
    expect(more?.url.searchParams.has("limit")).toBe(false);
    expect(more?.url.searchParams.has("cursor")).toBe(false);
  });

  it("keeps a split face and its Undo when the order changes", async () => {
    const user = userEvent.setup();
    server.appearances = [
      { appearanceId: 3, videoId: 5, title: "Beach day", spans: [], hasCrop: true },
      { appearanceId: 4, videoId: 6, title: "Mountain", spans: [], hasCrop: true },
    ];
    renderPage("/people/7/faces?tab=faces");
    await user.click(
      await screen.findByRole("button", { name: "Not this person in Beach day" }),
    );
    await screen.findByText("Split into #12");
    await user.click(screen.getByRole("button", { name: "Select" }));
    await user.click(screen.getByRole("checkbox", { name: "Select Mountain" }));
    expect(screen.getByText("1 selected")).toBeTruthy();

    // jsdom lacks the pointer APIs the Select's popup calls.
    Element.prototype.hasPointerCapture ??= () => false;
    Element.prototype.releasePointerCapture ??= () => undefined;
    Element.prototype.scrollIntoView ??= () => undefined;
    await user.click(screen.getByRole("combobox", { name: "Order" }));
    await user.click(await screen.findByRole("option", { name: "Newest video first" }));

    // The new order no longer holds the split face, yet its tile and Undo stay, and the selection is cleared.
    await waitFor(() => expect(screen.queryByText("1 selected")).toBeNull());
    expect(screen.getByText("Split into #12")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Select" }));
    expect(
      screen.getByRole("button", { name: "Undo the split in Beach day" }),
    ).toBeTruthy();
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

  it("moves selected faces to a person picked by face, and Undo moves them back", async () => {
    const user = userEvent.setup();
    server.appearances = [
      { appearanceId: 3, videoId: 5, title: "Beach day", spans: [], hasCrop: true },
      { appearanceId: 4, videoId: 6, title: "Mountain", spans: [], hasCrop: true },
    ];
    renderPage("/people/7/faces");

    await user.click(await screen.findByRole("button", { name: "Select" }));
    await user.click(screen.getByRole("checkbox", { name: "Select Beach day" }));
    await user.click(screen.getByRole("button", { name: "Move to person…" }));
    const dialog = await screen.findByRole("dialog", { name: "Move 1 face to…" });
    await user.click(await within(dialog).findByRole("radio", { name: /Mio/ }));
    expect(within(dialog).getByText("6 faces · first in Beach day")).toBeTruthy();
    await user.click(within(dialog).getByRole("button", { name: "Move" }));

    expect(await screen.findByText("Moved to Mio")).toBeTruthy();
    expect(sentBody(sent("POST", "/api/people/7/appearances/move")[0])).toEqual({
      appearanceIds: [3],
      targetId: 9,
      restoreCredits: [],
    });
    expect(
      sent("GET", "/api/people/7/similar-people")[0]?.url.searchParams.getAll(
        "appearanceId",
      ),
    ).toEqual(["3"]);

    // A second move to the same person is its own operation, which the first Undo leaves alone.
    await user.click(screen.getByRole("checkbox", { name: "Select Mountain" }));
    await user.click(screen.getByRole("button", { name: "Move to person…" }));
    const again = await screen.findByRole("dialog", { name: "Move 1 face to…" });
    await user.click(await within(again).findByRole("radio", { name: /Mio/ }));
    await user.click(within(again).getByRole("button", { name: "Move" }));
    expect(
      await screen.findByRole("button", { name: "Undo the move in Mountain" }),
    ).toBeTruthy();

    await user.click(screen.getByRole("button", { name: "Undo the move in Beach day" }));

    await waitFor(() =>
      expect(
        screen.queryByRole("button", { name: "Undo the move in Beach day" }),
      ).toBeNull(),
    );
    // The undo hands back the credit the move took, so the target's own one stays.
    expect(sentBody(sent("POST", "/api/people/9/appearances/move")[0])).toEqual({
      appearanceIds: [3],
      targetId: 7,
      restoreCredits: [{ workKey: "work-5", position: 1 }],
    });
    expect(
      screen.getByRole("button", { name: "Undo the move in Mountain" }),
    ).toBeTruthy();
  });

  it("keeps the move dialog open, saying so, when a face is no longer the person's", async () => {
    const user = userEvent.setup();
    server.moveGone = true;
    server.appearances = [
      { appearanceId: 3, videoId: 5, title: "Beach day", spans: [], hasCrop: true },
    ];
    renderPage("/people/7/faces?tab=faces");

    await user.click(await screen.findByRole("button", { name: "Select" }));
    await user.click(screen.getByRole("checkbox", { name: "Select Beach day" }));
    await user.click(screen.getByRole("button", { name: "Move to person…" }));
    const dialog = await screen.findByRole("dialog", { name: "Move 1 face to…" });
    await user.click(await within(dialog).findByRole("radio", { name: /Mio/ }));
    await user.click(within(dialog).getByRole("button", { name: "Move" }));

    expect(
      await within(dialog).findByText(
        "Some of these faces are no longer this person's. Refresh and try again.",
      ),
    ).toBeTruthy();
    expect(screen.queryByText("Moved to Mio")).toBeNull();
  });

  it("says when the people alike could not be read, with a retry", async () => {
    const user = userEvent.setup();
    server.similarFailures = 1;
    server.appearances = [
      { appearanceId: 3, videoId: 5, title: "Beach day", spans: [], hasCrop: true },
    ];
    renderPage("/people/7/faces?tab=faces");

    await user.click(await screen.findByRole("button", { name: "Select" }));
    await user.click(screen.getByRole("checkbox", { name: "Select Beach day" }));
    await user.click(screen.getByRole("button", { name: "Move to person…" }));
    const dialog = await screen.findByRole("dialog", { name: "Move 1 face to…" });
    expect(
      await within(dialog).findByText("Couldn't look for people who look alike."),
    ).toBeTruthy();
    await user.click(within(dialog).getByRole("button", { name: "Retry" }));

    expect(await within(dialog).findByRole("radio", { name: /Mio/ })).toBeTruthy();
  });

  it("clears the selection when the order changes", async () => {
    const user = userEvent.setup();
    server.appearances = [
      { appearanceId: 3, videoId: 5, title: "Beach day", spans: [], hasCrop: true },
    ];
    renderPage("/people/7/faces?tab=faces");

    await user.click(await screen.findByRole("button", { name: "Select" }));
    await user.click(screen.getByRole("checkbox", { name: "Select Beach day" }));
    expect(screen.getByText("1 selected")).toBeTruthy();
    // jsdom lacks the pointer APIs the Select's popup calls.
    Element.prototype.hasPointerCapture ??= () => false;
    Element.prototype.releasePointerCapture ??= () => undefined;
    Element.prototype.scrollIntoView ??= () => undefined;
    await user.click(screen.getByRole("combobox", { name: "Order" }));
    await user.click(await screen.findByRole("option", { name: "Newest video first" }));

    await waitFor(() => expect(screen.queryByText("1 selected")).toBeNull());
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

  it("leaves a person without a face faceless after undoing a split", async () => {
    const user = userEvent.setup();
    server.appearances = [
      { appearanceId: 3, videoId: 5, title: "Beach day", spans: [], hasCrop: true },
    ];
    // The merge back hands the person the split person's face.
    server.mergeAnswer = { person: person({ imageRevision: 4 }), notFoundIds: [] };
    renderPage("/people/7/faces?tab=faces");

    await user.click(
      await screen.findByRole("button", { name: "Not this person in Beach day" }),
    );
    await user.click(
      await screen.findByRole("button", { name: "Undo the split in Beach day" }),
    );

    expect(
      await screen.findByRole("button", { name: "Not this person in Beach day" }),
    ).toBeTruthy();
    expect(sent("DELETE", "/api/people/7/image")).toHaveLength(1);
  });

  it("reports an Undo whose split person is gone, keeping the split shown", async () => {
    const user = userEvent.setup();
    server.appearances = [
      { appearanceId: 3, videoId: 5, title: "Beach day", spans: [], hasCrop: true },
    ];
    server.mergeAnswer = { person: person(), notFoundIds: [12] };
    renderPage("/people/7/faces?tab=faces");

    await user.click(
      await screen.findByRole("button", { name: "Not this person in Beach day" }),
    );
    await user.click(
      await screen.findByRole("button", { name: "Undo the split in Beach day" }),
    );

    expect(
      await screen.findByText("Couldn't undo the split: This person no longer exists."),
    ).toBeTruthy();
    expect(screen.getByText("Split into #12")).toBeTruthy();
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

    // The merged-away person's Undo comes along in the toast and brings their page back; Sonner captures the pointer.
    Element.prototype.setPointerCapture ??= () => undefined;
    await user.click(screen.getByRole("button", { name: "Undo" }));

    expect(await screen.findByRole("link", { name: "Back to #7" })).toBeTruthy();
    expect(sent("POST", "/api/activity/31/undo")).toHaveLength(1);
    expect(sent("POST", "/api/people/7/face-candidates/9/reopen")).toHaveLength(1);
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
    // The merge gives the faceless shown person #9's face, which the undo takes back.
    server.mergeTarget = person({ imageRevision: 5 });
    renderPage("/people/7/faces");

    await user.click(await screen.findByRole("button", { name: "Same person as #9" }));
    expect(await screen.findByText("Merged into Aoi")).toBeTruthy();
    const pair = screen.getByRole("group", { name: "Pair with #9" });
    expect(within(pair).getByRole("img", { name: "Aoi" })).toBeTruthy();

    await user.click(screen.getByRole("button", { name: "Undo for #9" }));

    expect(await screen.findByRole("button", { name: "Same person as #9" })).toBeTruthy();
    expect(sent("POST", "/api/activity/31/undo")).toHaveLength(1);
    expect(sent("POST", "/api/people/7/face-candidates/9/reopen")).toHaveLength(1);
    await waitFor(() =>
      expect(within(pair).queryByRole("img", { name: "Aoi" })).toBeNull(),
    );
  });

  it("only reopens the pair when Undo is pressed again after the reopen failed", async () => {
    const user = userEvent.setup();
    server.faceCandidates = [candidate(9, "")];
    server.mergeTarget = person();
    server.reopenFailures = 1;
    renderPage("/people/7/faces");

    await user.click(await screen.findByRole("button", { name: "Same person as #9" }));
    await user.click(await screen.findByRole("button", { name: "Undo for #9" }));
    await screen.findByText(/Couldn't undo/);
    await user.click(screen.getByRole("button", { name: "Undo for #9" }));

    expect(await screen.findByRole("button", { name: "Same person as #9" })).toBeTruthy();
    expect(sent("POST", "/api/activity/31/undo")).toHaveLength(1);
    expect(sent("POST", "/api/people/7/face-candidates/9/reopen")).toHaveLength(2);
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
    // A decided pair no longer counts as waiting.
    expect(screen.getByRole("tab", { name: /^Same person\?\s*1$/ })).toBeTruthy();
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

  it("stays on the tab in view when a Refresh changes the pair count", async () => {
    const user = userEvent.setup();
    renderPage("/people/7/faces");
    // With no pairs waiting, the page opens on the faces.
    await waitFor(() =>
      expect(
        screen
          .getByRole("tab", { name: /^Faces in videos/ })
          .getAttribute("aria-selected"),
      ).toBe("true"),
    );

    server.faceCandidates = [candidate(9, "Alice")];
    await user.click(screen.getByRole("button", { name: "Refresh" }));

    expect(await screen.findByRole("tab", { name: /^Same person\?\s*1$/ })).toBeTruthy();
    expect(
      screen.getByRole("tab", { name: /^Faces in videos/ }).getAttribute("aria-selected"),
    ).toBe("true");
  });

  it("says when the pairs could not be read, with a retry", async () => {
    const user = userEvent.setup();
    server.candidateFailures = 1;
    server.faceCandidates = [candidate(9, "Alice")];
    renderPage("/people/7/faces?tab=candidates");

    expect(await screen.findByText(/Couldn't load the pairs/)).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Retry" }));

    expect(await screen.findByRole("group", { name: "Pair with Alice" })).toBeTruthy();
  });

  it("shows the gone state when the person no longer exists", async () => {
    server.person = null;
    renderPage("/people/7/faces");

    expect(await screen.findByText("This person no longer exists")).toBeTruthy();
  });
});
