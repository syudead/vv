import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { MemoryRouter } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { ActivityListItem, ActivityMemberSummary } from "../api/activity";
import { CardList } from "../ui/patterns/card-list";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import { ActivityPipelineCard } from "./ActivityPipelineCard";

const at = "2026-10-18T10:00:00Z";
const now = new Date("2026-10-18T10:05:00Z");
const video = { kind: "video" as const, items: [{ label: "clip.mp4", videoId: 7 }] };

function member(
  id: number,
  extra: Partial<ActivityMemberSummary> = {},
): ActivityMemberSummary {
  return {
    id,
    kind: "import",
    feature: "import",
    subject: video,
    status: "done",
    outcome: "completed",
    decider: "rule",
    retryRun: false,
    undoable: false,
    undone: false,
    replaced: false,
    ...extra,
  };
}

function pipeline(
  extra: Partial<ActivityListItem>,
  members: ActivityMemberSummary[],
): ActivityListItem {
  return {
    id: 1,
    kind: "pipeline",
    feature: "import",
    status: "done",
    outcome: "completed",
    decider: "rule",
    subject: video,
    candidate: false,
    retryRun: false,
    undoable: false,
    undone: false,
    createdAt: at,
    updatedAt: at,
    members,
    ...extra,
  };
}

const processing = pipeline({ status: "running", reason: "processing" }, [
  member(2, {
    status: "running",
    reason: "processing",
    outcome: undefined,
    progress: { stage: "preview", done: 3, total: 5 },
  }),
  member(3, {
    kind: "auto_tag",
    feature: "auto_tagging",
    status: "running",
    reason: "waiting",
    outcome: undefined,
  }),
]);

const failedTagging = pipeline(
  { status: "attention", reason: "failed", outcome: undefined },
  [
    member(2),
    member(3, {
      kind: "auto_tag",
      feature: "auto_tagging",
      status: "attention",
      reason: "failed",
      outcome: undefined,
      retryRun: true,
    }),
  ],
);

/** Keeps the owner's choice as the page does. */
function Harness({ item }: { item: ActivityListItem }) {
  const [chosen, setChosen] = useState<boolean | undefined>(undefined);
  return (
    <MemoryRouter>
      <TooltipProvider>
        <CardList label="Activity log">
          <ActivityPipelineCard
            item={item}
            now={now}
            from="/activity"
            actions={[]}
            chosen={chosen}
            onExpandedChange={setChosen}
            onRetry={vi.fn<() => Promise<void>>()}
            onUndo={vi.fn<() => Promise<void>>()}
          />
        </CardList>
      </TooltipProvider>
    </MemoryRouter>
  );
}

function card(): HTMLElement {
  return document.querySelector<HTMLElement>("[data-slot=card-list-item]")!;
}

function rows(): HTMLElement[] {
  return [...document.querySelectorAll<HTMLElement>("[data-slot=card-list-member]")];
}

function row(name: string): HTMLElement {
  return rows().find((item) => within(item).queryByRole("link", { name }) !== null)!;
}

afterEach(cleanup);

describe("ActivityPipelineCard", () => {
  it("starts expanded while processing, with the import's stage and count", () => {
    render(<Harness item={processing} />);
    expect(card().dataset.tone).toBe("primary");
    expect(
      within(card()).getByText("Import in progress · 0 of 2 tasks done"),
    ).toBeDefined();
    expect(
      screen
        .getByRole("button", { name: "Hide tasks for clip.mp4" })
        .getAttribute("aria-expanded"),
    ).toBe("true");
    expect(rows()).toHaveLength(2);
    const imported = row("Import");
    expect(within(imported).getByText("Preview")).toBeDefined();
    expect(within(imported).getByText("3/5")).toBeDefined();
    expect(within(row("Auto-tagging")).getByText("Waiting")).toBeDefined();
  });

  it.each([
    {
      state: "waiting",
      item: pipeline({ status: "running", reason: "waiting", outcome: undefined }, [
        member(2, { status: "running", reason: "waiting", outcome: undefined }),
      ]),
      summary: "Waiting · 0 of 1 task done",
      tone: "default",
    },
    {
      state: "attention",
      item: failedTagging,
      summary: "Auto-tagging: Failed",
      tone: "destructive",
    },
    {
      state: "done",
      item: pipeline({}, [member(2), member(3, { kind: "auto_tag", tagCount: 2 })]),
      summary: "Finished · 2 tasks",
      tone: "default",
    },
  ])(
    "starts collapsed when $state and says why on the card",
    ({ item, summary, tone }) => {
      render(<Harness item={item} />);
      expect(card().dataset.tone).toBe(tone);
      expect(within(card()).getByText(summary)).toBeDefined();
      expect(
        screen
          .getByRole("button", { name: "Show tasks for clip.mp4" })
          .getAttribute("aria-expanded"),
      ).toBe("false");
      expect(rows()).toHaveLength(0);
    },
  );

  it("lets the owner open a run needing attention to retry the failed task only", async () => {
    const user = userEvent.setup();
    render(<Harness item={failedTagging} />);
    await user.click(screen.getByRole("button", { name: "Show tasks for clip.mp4" }));
    expect(rows()).toHaveLength(2);
    const imported = row("Import");
    expect(within(imported).getByText("Done")).toBeDefined();
    expect(within(imported).queryByRole("button", { name: "Retry" })).toBeNull();
    const tagging = row("Auto-tagging");
    expect(within(tagging).getByText("Failed")).toBeDefined();
    expect(within(tagging).getByRole("button", { name: "Retry" })).toBeDefined();
    expect(within(card()).getAllByRole("button", { name: "Retry" })).toHaveLength(1);

    await user.click(screen.getByRole("button", { name: "Hide tasks for clip.mp4" }));
    expect(rows()).toHaveLength(0);
  });

  it("opens on a matched task, marks it and leaves replaced tasks off the card", () => {
    render(
      <Harness
        item={pipeline({}, [
          member(2, { matched: false }),
          member(3, {
            kind: "auto_tag",
            feature: "auto_tagging",
            status: "attention",
            reason: "failed",
            outcome: undefined,
            replaced: true,
            matched: true,
          }),
          member(4, {
            kind: "auto_tag",
            feature: "auto_tagging",
            tagCount: 1,
            matched: true,
          }),
        ])}
      />,
    );
    expect(rows()).toHaveLength(2);
    expect(within(row("Import")).queryByText("Matches")).toBeNull();
    const tagging = row("Auto-tagging");
    expect(within(tagging).getByText("Matches")).toBeDefined();
    expect(within(tagging).getByText("Added 1 tag")).toBeDefined();
    expect(
      within(tagging).getByRole("link", { name: "Auto-tagging" }).getAttribute("href"),
    ).toBe("/activity/4");
  });
});
