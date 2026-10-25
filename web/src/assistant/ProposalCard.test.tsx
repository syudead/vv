import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { describe, expect, it } from "vitest";

import type { AssistantProposal } from "../api/assistant";
import ProposalCard from "./ProposalCard";

function proposal(extra: Partial<AssistantProposal> = {}): AssistantProposal {
  return {
    id: 1,
    status: "proposed",
    request: "これに旅行タグを付けて",
    operation: "updateVideoTags.add",
    irreversible: false,
    targets: {
      kind: "videos",
      count: 3,
      labels: [
        { kind: "video", id: 1, name: "a" },
        { kind: "video", id: 2, name: "b" },
        { kind: "video", id: 3, name: "c" },
      ],
    },
    change: [{ argument: "tagNames", labels: [{ kind: "tag", id: 4, name: "旅行" }] }],
    created: [],
    counts: { total: 3, applied: 0, failed: 0 },
    ...extra,
  };
}

function renderCard(value: AssistantProposal) {
  return render(
    <MemoryRouter>
      <ProposalCard
        card={{ proposal: value, view: "live", executing: false }}
        onExecute={() => undefined}
        onDismiss={() => undefined}
      />
    </MemoryRouter>,
  );
}

function filledButtons(): string[] {
  return screen
    .getAllByRole("button")
    .filter((button) => {
      const variant = button.getAttribute("data-variant");
      return variant === "default" || variant === "destructive";
    })
    .map((button) => button.textContent ?? "");
}

describe("ProposalCard", () => {
  it("出す件数・変更を出し、Execute だけが塗りのボタンで影響の行を説明に持つ", () => {
    renderCard(proposal());

    expect(screen.getByText("Add tags")).toBeDefined();
    const impact = screen.getByText("3 videos");
    expect(screen.getByText("Tags")).toBeDefined();
    expect(screen.getByText("旅行")).toBeDefined();
    expect(screen.queryByText("Can’t be undone")).toBeNull();
    expect(filledButtons()).toEqual(["Execute"]);
    const execute = screen.getByRole("button", { name: "Execute" });
    expect(execute.getAttribute("data-variant")).toBe("default");
    expect(execute.getAttribute("aria-describedby")?.split(" ")).toContain(impact.id);
  });

  it("マージは移る動画の数を impact.videos から出し、戻せない印と destructive の Execute を付ける", () => {
    renderCard(
      proposal({
        operation: "mergeTag",
        irreversible: true,
        targets: {
          kind: "tag",
          count: 1,
          labels: [{ kind: "tag", id: 9, name: "旅行" }],
        },
        change: [{ argument: "names", labels: [{ kind: "tag", id: 8, name: "旅" }] }],
        impact: { videos: 42 },
        counts: { total: 1, applied: 0, failed: 0 },
      }),
    );

    expect(screen.getByText("Merge tags into “旅行”")).toBeDefined();
    expect(screen.getByText("42 videos")).toBeDefined();
    expect(screen.queryByText("1 tag")).toBeNull();
    expect(screen.getByText("Can’t be undone")).toBeDefined();
    expect(screen.getByText("42 videos move to “旅行”")).toBeDefined();
    expect(screen.getByText("Merge from")).toBeDefined();
    expect(filledButtons()).toEqual(["Execute"]);
    expect(
      screen.getByRole("button", { name: "Execute" }).getAttribute("data-variant"),
    ).toBe("destructive");
  });

  it("作られる名前に New の印を付ける", () => {
    renderCard(
      proposal({
        change: [
          {
            argument: "tagNames",
            labels: [
              { kind: "tag", id: 4, name: "旅行" },
              { kind: "tag", name: "温泉" },
            ],
          },
        ],
        created: ["温泉"],
      }),
    );

    expect(screen.getByText("New tag 温泉")).toBeDefined();
    expect(screen.queryByText("New tag 旅行")).toBeNull();
  });

  it("11 件目からは名前の代わりに残りの数を出す", () => {
    const labels = Array.from({ length: 10 }, (_, index) => ({
      kind: "video" as const,
      id: index + 1,
      name: `v${String(index + 1)}`,
    }));
    renderCard(
      proposal({
        targets: { kind: "videos", count: 25, labels },
        counts: { total: 25, applied: 0, failed: 0 },
      }),
    );

    expect(screen.getByText("25 videos")).toBeDefined();
    expect(screen.getByText("+15 more")).toBeDefined();
  });
});
