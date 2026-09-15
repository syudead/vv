import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { describe, expect, it } from "vitest";

import AppShell, { usePublishVideoCount } from "./AppShell";
import { navItems } from "./navigation";

/**
 * 表示のみの要素の規約（FR-005 / FR-006 / FR-013 /
 * contracts/components.md 3.・5.）。
 *
 * 原案には裏側の無い入口が 15 個ある。それらを `<button disabled>` で置くと
 * 「いまは使えない」という誤った意味を持ち、tab 順にも現れて**利用者にできる
 * ことが増えたように見える**（R-503）。ここが見ているのは、その 15 個が
 * 対話要素でも tab 順の停止位置でもないことと、機能する入口が**増えていない**
 * ことである。
 *
 * この検査は目で見ても確かめられない ── 淡く描かれていることと、押せないこと・
 * tab 順に無いことは別である。構図そのものは jsdom では確かめられない（R-511）
 * ので、S3・S7 で人が原案と並べて見る。
 */

/** Publisher は一覧の代わりに件数を公開する（LibraryPage と同じ口を使う）。 */
function Publisher({ count }: { count: number | undefined }) {
  usePublishVideoCount(count);
  return null;
}

/**
 * shell はサイドバーとヘッダーを骨格ごと描く。
 *
 * 部品を単体で描かないのは、件数が骨格の context を通って届くからである
 * （T021）。経路が要るのは NavLink の選択中の判定のためで、`/` で描くと
 * 「すべての動画」と「動画」が選択中になる。
 */
function shell(count: number | undefined) {
  return render(
    <MemoryRouter initialEntries={["/"]}>
      <AppShell>
        <Publisher count={count} />
      </AppShell>
    </MemoryRouter>,
  );
}

/** inertItems は表の中の表示のみの行である。 */
const inertItems = navItems.filter((item) => item.kind === "inert");

/**
 * liveIds は spec が「機能する」と定めた 2 つである（contracts/components.md 1.）。
 *
 * **ここに表から作った値を置いてはならない。** 期待値を `navItems` から導くと、
 * 別の行を `live` に変えたときに期待値も一緒に動き、検査は通ってしまう。
 * FR-013 が求めているのは「利用者が行える操作が増えていないこと」なので、
 * 表とは独立に固定した 2 つと突き合わせる必要がある。
 */
const liveIds = ["all-videos", "tab-videos"];

describe("表示のみの要素は対話要素でない（FR-005 / FR-006）", () => {
  it("表示のみのラベルが button にも link にも現れない", () => {
    shell(undefined);

    const names = [
      ...screen.queryAllByRole("button"),
      ...screen.queryAllByRole("link"),
    ].map((element) => element.textContent ?? "");

    for (const item of inertItems) {
      // 部分一致で見る。対話要素の名前は中身から組み立てられるので、完全一致
      // では件数や添え字が混ざった形を取り逃がす。
      expect(
        names.filter((name) => name.includes(item.label)),
        `${item.id} が対話要素になっている`,
      ).toEqual([]);
    }

    // 設定（C7）も表示のみである。押せる見た目の歯車が 1 つでもあれば、
    // 利用者にできることが増えたということである。
    expect(screen.queryAllByRole("button")).toHaveLength(0);
  });

  it("機能する入口が「すべての動画」と「動画」の 2 つだけである", () => {
    // まず表そのものを見る。これ以外の行が live になったら、それは利用者が
    // 行える操作が増えたということで、FR-013 の違反である。
    expect(
      navItems.filter((item) => item.kind === "live").map((item) => item.id),
    ).toEqual(liveIds);

    const { container } = shell(undefined);

    // 次に、骨格が実際に差し出すリンクがその 2 つとちょうど同じであることを見る。
    // 表が正しくても、描画側が inert な行までリンクにしていたら意味が無い。
    const links = screen.getAllByRole("link");
    expect(links).toHaveLength(liveIds.length);
    for (const id of liveIds) {
      const row = container.querySelector(`[data-nav-id="${id}"]`);
      expect(row?.tagName, `${id} がリンクになっていない`).toBe("A");
    }
  });

  it("表示のみの要素が tab 順に現れない", () => {
    const { container } = shell(undefined);

    // 停止位置になりうるものを数える。live な 2 行以外が増えていたら、
    // 表示のみの要素が対話要素として置かれたということである。
    const focusable = container.querySelectorAll(
      "a[href], button, input, select, textarea, [tabindex]",
    );
    expect(focusable).toHaveLength(liveIds.length);

    // 表示のみの行そのものにも tabIndex を持たせない（R-503）。
    for (const item of inertItems) {
      const row = container.querySelector(`[data-nav-id="${item.id}"]`);
      expect(row, `${item.id} が描かれていない`).not.toBeNull();
      expect(row?.hasAttribute("tabindex")).toBe(false);
    }
  });

  it("ラベルの直後に「（未実装）」が添えられている", () => {
    const { container } = shell(undefined);

    for (const item of inertItems) {
      const row = container.querySelector(`[data-nav-id="${item.id}"]`);
      expect(row?.textContent ?? "", `${item.id} に断りが無い`).toContain("（未実装）");
    }
  });
});

describe("件数は機能する 1 行にだけ出る（FR-005）", () => {
  it("表示のみの要素の近傍に件数が出ない", () => {
    const { container } = shell(42);

    for (const item of inertItems) {
      const row = container.querySelector(`[data-nav-id="${item.id}"]`);
      expect(row?.textContent ?? "", `${item.id} に件数が出ている`).not.toMatch(/\d/);
    }
  });

  it("件数が届いていれば「すべての動画」に出る", () => {
    const { container } = shell(42);

    const row = container.querySelector('[data-nav-id="all-videos"]');
    expect(row?.textContent ?? "").toContain("42");
  });

  it("件数が undefined のあいだは「すべての動画」にも出ない", () => {
    // 未取得（最初の 1 ページを待っている）と取り直しの最中がこの場面である。
    // total は 0 で初期化され前の値も残るので、公開側が undefined に倒している
    // （T021）。0 と「まだ分からない」を同じ見た目にしない。
    const { container } = shell(undefined);

    const row = container.querySelector('[data-nav-id="all-videos"]');
    expect(row?.textContent ?? "").not.toMatch(/\d/);
  });
});
