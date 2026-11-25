import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { TagKind } from "../api/tagKinds";
import { t } from "../i18n";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import TagSelectionBar from "./TagSelectionBar";
import type { KindChoice } from "./TagSetKindMenu";

afterEach(cleanup);

function renderBar(
  busy: boolean,
  onClear = vi.fn(),
  {
    kinds = [],
    sharedKind,
    onSetKind = vi.fn(),
    hasTentative = true,
  }: {
    kinds?: TagKind[] | null;
    sharedKind?: string;
    onSetKind?: (choice: KindChoice) => void;
    hasTentative?: boolean;
  } = {},
) {
  render(
    <TooltipProvider>
      <TagSelectionBar
        count={2}
        hasTentative={hasTentative}
        hasConfirmed
        pending={busy ? "confirm" : null}
        selectAll={{
          loaded: 2,
          matching: 2,
          matchingReason: undefined,
          pending: false,
          onLoaded: vi.fn(),
          onMatching: vi.fn(),
        }}
        onConfirm={vi.fn()}
        onReject={vi.fn()}
        onDelete={vi.fn()}
        onMerge={vi.fn()}
        onClear={onClear}
        kinds={kinds ?? undefined}
        sharedKind={sharedKind}
        onSetKind={onSetKind}
      />
    </TooltipProvider>,
  );
  return onClear;
}

describe("TagSelectionBar", () => {
  it("まとめての操作の送信中は選択の解除を disabled にする", () => {
    renderBar(true);
    const clear = screen.getByRole("button", { name: t.tags.selection.clear });
    expect(clear).toHaveProperty("disabled", true);
  });

  it("送信中でなければ選択の解除を押せる", async () => {
    const onClear = renderBar(false);
    const clear = screen.getByRole("button", { name: t.tags.selection.clear });
    expect(clear).toHaveProperty("disabled", false);
    await userEvent.setup().click(clear);
    expect(onClear).toHaveBeenCalledTimes(1);
  });

  it("操作は Select all、Merge into one tag…、Set kind…、Confirm、More の順に並ぶ", () => {
    renderBar(false);
    const names = screen.getAllByRole("button").map((button) => button.textContent);
    expect(names.slice(1)).toEqual([
      t.tags.selection.selectAll,
      t.tags.selection.mergeInto,
      t.tags.selection.setKind,
      t.tags.selection.confirm,
      t.tags.selection.more,
    ]);
  });

  it("仮のタグを選んでいなければ Confirm と Reject… は理由を添えて押せない", async () => {
    const user = userEvent.setup();
    renderBar(false, vi.fn(), { hasTentative: false });
    const confirm = screen.getByRole("button", { name: t.tags.selection.confirm });
    expect(confirm).toHaveProperty("disabled", true);
    expect(confirm.getAttribute("title")).toBe(t.tags.selection.noTentative);
    await user.click(screen.getByRole("button", { name: t.tags.selection.more }));
    const reject = await screen.findByRole("menuitem", { name: t.tags.selection.reject });
    expect(reject.getAttribute("aria-disabled")).toBe("true");
    const remove = screen.getByRole("menuitem", { name: t.tags.selection.delete });
    expect(remove.hasAttribute("aria-disabled")).toBe(false);
  });

  it("メニューは種類・区切り線・No kind の 1 つの組で、みなが持つ種類に印を付け、選ぶと渡す", async () => {
    const user = userEvent.setup();
    const genre = {
      id: 1,
      name: "Genre",
      cardFirst: false,
      tagCount: 0,
      createdAt: "2026-01-01T00:00:00Z",
    };
    const work = {
      id: 2,
      name: "Work",
      cardFirst: false,
      tagCount: 0,
      createdAt: "2026-01-01T00:00:00Z",
    };
    const onSetKind = vi.fn();
    renderBar(false, vi.fn(), { kinds: [genre, work], sharedKind: "2", onSetKind });
    await user.click(screen.getByRole("button", { name: t.tags.selection.setKind }));
    const items = await screen.findAllByRole("menuitemradio");
    expect(items.map((item) => item.textContent)).toEqual([
      "Genre",
      "Work",
      t.tags.kindFilter.none,
    ]);
    expect(items.map((item) => item.getAttribute("aria-checked"))).toEqual([
      "false",
      "true",
      "false",
    ]);
    expect(screen.getByRole("separator")).toBeDefined();
    await user.click(items[0]!);
    expect(onSetKind).toHaveBeenCalledWith(genre);
  });

  it("種類が無いときは作る場所を伝え、No kind だけを並べる", async () => {
    const user = userEvent.setup();
    const onSetKind = vi.fn();
    renderBar(false, vi.fn(), { kinds: [], sharedKind: undefined, onSetKind });
    await user.click(screen.getByRole("button", { name: t.tags.selection.setKind }));
    expect(await screen.findByText(t.tags.selection.noKinds)).toBeDefined();
    const items = screen.getAllByRole("menuitemradio");
    expect(items.map((item) => item.textContent)).toEqual([t.tags.kindFilter.none]);
    expect(items[0]!.getAttribute("aria-checked")).toBe("false");
    expect(screen.queryByRole("separator")).toBeNull();
    await user.click(items[0]!);
    expect(onSetKind).toHaveBeenCalledWith("none");
  });

  it("種類をまだ読めていなければ「種類が無い」とは言わず、No kind だけを並べる", async () => {
    const user = userEvent.setup();
    renderBar(false, vi.fn(), { kinds: null });
    await user.click(screen.getByRole("button", { name: t.tags.selection.setKind }));
    const items = await screen.findAllByRole("menuitemradio");
    expect(items.map((item) => item.textContent)).toEqual([t.tags.kindFilter.none]);
    expect(screen.queryByText(t.tags.selection.noKinds)).toBeNull();
  });
});
