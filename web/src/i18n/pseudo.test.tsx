import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import IconButton from "../ui/legacy/IconButton";
import { formatNumber } from "./format";
import { t } from "./messages";
import {
  enablePseudoLocale,
  expectCatalogTextOnly,
  findUncataloguedText,
  resetLocale,
} from "./pseudo";

function CatalogCard({ title, count }: { title: string; count: number }) {
  return (
    <article aria-label={t.folderGrouping.ungrouped(title)}>
      <h2>{title}</h2>
      <p>
        {t.count.videos(count)} · {formatNumber(count)}
      </p>
      <IconButton label={t.common.close} tooltip={false}>
        ×
      </IconButton>
    </article>
  );
}

function LeakyCard({ title }: { title: string }) {
  // カタログを通らない英語を string の変数を経由して描く（型ではすり抜ける経路）。
  const note: string = "Saved to your library";
  return (
    <article>
      <h2>{title}</h2>
      <p>{note}</p>
    </article>
  );
}

function LeakyAttribute() {
  const label: string = "Remove";
  return <button type="button" aria-label={label} title={t.common.close} />;
}

describe("pseudo locale", () => {
  it("marks every catalog sentence and formatted value", () => {
    enablePseudoLocale();
    expect(t.common.close).toBe("⟦Close⟧");
    expect(t.count.videos(2)).toBe("⟦⟦2⟧ videos⟧");
    expect(formatNumber(1200)).toBe("⟦1,200⟧");
    resetLocale();
    expect(t.common.close).toBe("Close");
    expect(formatNumber(1200)).toBe("1,200");
  });

  it("accepts a screen whose text is all from the catalog or the given user data", () => {
    enablePseudoLocale();
    const { container } = render(<CatalogCard title="夏の旅行" count={3} />);
    expect(findUncataloguedText(container, ["夏の旅行"])).toEqual([]);
    expect(() => expectCatalogTextOnly(container, ["夏の旅行"])).not.toThrow();
  });

  it("reports user data that the test did not declare", () => {
    enablePseudoLocale();
    const { container } = render(<CatalogCard title="Trip" count={3} />);
    expect(findUncataloguedText(container)).toEqual([{ where: "text", text: "Trip" }]);
  });

  it("detects English rendered from a string variable outside the catalog", () => {
    enablePseudoLocale();
    const { container } = render(<LeakyCard title="夏の旅行" />);
    expect(findUncataloguedText(container, ["夏の旅行"])).toEqual([
      { where: "text", text: "Saved to your library" },
    ]);
    expect(() => expectCatalogTextOnly(container, ["夏の旅行"])).toThrow(
      /Saved to your library/,
    );
  });

  it("checks aria-label, title, placeholder and alt", () => {
    enablePseudoLocale();
    const { container } = render(<LeakyAttribute />);
    expect(findUncataloguedText(container)).toEqual([
      { where: "aria-label", text: "Remove" },
    ]);
  });
});
