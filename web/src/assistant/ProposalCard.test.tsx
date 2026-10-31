import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { describe, expect, it } from "vitest";

import type { AssistantProposal } from "../api/assistant";
import { createAndAttachProposal, oneStepProposal } from "../testing/assistantProposal";
import type { CardProposal } from "./proposal";
import ProposalCard from "./ProposalCard";

function proposal(extra: Partial<CardProposal> = {}): AssistantProposal {
  return oneStepProposal({
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
  });
}

function renderCard(
  value: AssistantProposal,
  onExecute: (steps: number[]) => void = () => undefined,
) {
  return render(
    <MemoryRouter>
      <ProposalCard
        card={{ proposal: value, view: "live", executing: false }}
        onExecute={onExecute}
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

  it("取得の適用は項目ごとに編集画面の項目名と取得した値を出す", () => {
    renderCard(
      proposal({
        operation: "applyWorkInfoFetch",
        targets: {
          kind: "video",
          count: 1,
          labels: [{ kind: "video", id: 1, name: "a" }],
        },
        change: [
          { argument: "title", value: "取得した題名" },
          { argument: "dates.release", value: "2024-05-01" },
          { argument: "tags", value: ["夏", "海"] },
          { argument: "cover", value: 1 },
        ],
        counts: { total: 1, applied: 0, failed: 0 },
      }),
    );

    expect(screen.getByText("Apply fetched work information")).toBeDefined();
    expect(screen.getByText("Title").nextSibling?.textContent).toBe("取得した題名");
    expect(screen.getByText("Released").nextSibling?.textContent).toBe("2024-05-01");
    expect(screen.getByText("夏")).toBeDefined();
    expect(screen.getByText("Cover").nextSibling?.textContent).toBe("New image");
  });

  it("スキーマ行は id の配列を件数と最初のラベルで、ほかの本文の欄を値で出す", () => {
    renderCard(
      proposal({
        operation: "moveFaceAppearances",
        targets: { kind: "", count: 0, labels: [{ kind: "person", id: 2, name: "甲" }] },
        change: [
          {
            argument: "appearanceIds",
            labels: [{ kind: "faceAppearance", id: 30, name: "動画 a" }],
            value: 12,
          },
          { argument: "targetId", labels: [{ kind: "person", id: 3, name: "乙" }] },
          { argument: "restoreCredits", value: [{ videoId: 5, role: "performer" }] },
        ],
        counts: { total: 1, applied: 0, failed: 0 },
      }),
    );

    expect(screen.getByText("Move faces to “乙”")).toBeDefined();
    expect(screen.getByText("12 face appearances")).toBeDefined();
    expect(screen.getByRole("img", { name: "動画 a" }).getAttribute("src")).toBe(
      "/api/face-appearances/30/crop",
    );
    expect(screen.getByText("videoId: 5, role: performer")).toBeDefined();
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

describe("ProposalCard with steps", () => {
  it("手順を順に件数つきで並べ、後の手順の名前に Created by step 1 を付け、Execute だけを塗る", () => {
    renderCard(createAndAttachProposal());

    expect(screen.getByText("2 steps · 2 selected")).toBeDefined();
    const first = screen.getByText("Create tag “ライブ録画”");
    const second = screen.getByText("Add tags");
    expect(first.compareDocumentPosition(second) & Node.DOCUMENT_POSITION_FOLLOWING).toBe(
      Node.DOCUMENT_POSITION_FOLLOWING,
    );
    expect(screen.getByText("3 videos").className).toContain("text-lg");
    expect(screen.getByText("Created by step 1")).toBeDefined();
    expect(filledButtons()).toEqual(["Execute 2 steps"]);
  });

  it("手順 1 を外すと手順 2 は Needs step 1 になり、Execute は押せない", async () => {
    const executed: number[][] = [];
    const user = userEvent.setup();
    renderCard(createAndAttachProposal(), (steps) => executed.push(steps));

    await user.click(screen.getByRole("checkbox", { name: "Include step 1" }));

    expect(screen.getByText("2 steps · 1 selected")).toBeDefined();
    expect(screen.getByText("Won’t run")).toBeDefined();
    expect(screen.getByText("Needs step 1")).toBeDefined();
    expect(
      screen.getByText("Step 2 needs step 1. Tick step 1, or untick step 2."),
    ).toBeDefined();
    const execute = screen.getByRole("button", { name: "Execute" });
    expect(execute.getAttribute("aria-disabled")).toBe("true");
    await user.click(execute);
    expect(executed).toEqual([]);
  });

  it("戻せない手順に印を付け、それがチェックされているあいだだけ Execute を destructive にする", async () => {
    const user = userEvent.setup();
    renderCard(
      createAndAttachProposal("proposed", [
        {},
        {
          operation: "mergeTag",
          irreversible: true,
          targets: {
            kind: "tag",
            count: 1,
            labels: [{ kind: "tag", id: 9, name: "旅行" }],
          },
          change: [{ argument: "names", labels: [{ kind: "tag", id: 8, name: "旅" }] }],
          impact: { videos: 4 },
          createdBy: [],
        },
      ]),
    );

    expect(screen.getByText("Can’t be undone")).toBeDefined();
    const execute = () => screen.getByRole("button", { name: /^Execute/ });
    expect(execute().getAttribute("data-variant")).toBe("destructive");

    await user.click(screen.getByRole("checkbox", { name: "Include step 2" }));

    expect(execute().getAttribute("data-variant")).toBe("default");
  });
});
