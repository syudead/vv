import videojs from "video.js";
import { afterEach, describe, expect, it, vi } from "vitest";

import { enablePseudoLocale, expectCatalogTextOnly } from "../i18n/pseudo";
import { controlBarMenuOpen } from "./playerControls";
import { qualityOptions, sourceShortSide } from "./quality";
import {
  qualityMenuName,
  qualitySelectEvent,
  setQualityMenu,
  type QualityMenuState,
  type QualitySelection,
} from "./qualityMenu";

/**
 * 画質のメニューを本物の video.js で組み、項目・ボタンの文字・選択の知らせ・開閉の印を
 * 確かめる（specs/027-playback-quality/ui-design.md「Control bar: quality menu」）。
 */
describe("QualityMenuButton", () => {
  let player: ReturnType<typeof videojs> | undefined;

  afterEach(() => {
    player?.dispose();
    player = undefined;
  });

  function create(size: { width?: number; height?: number }, current = "original") {
    const element = document.createElement("video-js");
    document.body.append(element);
    player = videojs(element, {
      controls: true,
      playbackRates: [0.5, 1, 2],
      controlBar: {
        children: ["playToggle", "qualityMenuButton", "playbackRateMenuButton"],
      },
    });
    const state: QualityMenuState = {
      options: qualityOptions(size),
      current: current as QualityMenuState["current"],
      sourceSize: sourceShortSide(size),
    };
    setQualityMenu(player, state);
    const host = player.el();
    const wrapper = host.querySelector<HTMLElement>(".vjs-menu-button.vv-quality");
    if (wrapper === null) throw new Error("画質のボタンがありません");
    return { host, wrapper, player };
  }

  function items(wrapper: HTMLElement) {
    return Array.from(wrapper.querySelectorAll(".vjs-menu-item"), (item) => ({
      text: item.querySelector(".vjs-menu-item-text")?.textContent,
      selected: item.classList.contains("vjs-selected"),
    }));
  }

  function label(wrapper: HTMLElement) {
    return wrapper.querySelector(".vv-quality-value")?.textContent;
  }

  it("再生速度の前に置き、1080p の動画では元の画質と 720p・480p・360p を出して 1080p を出さない", () => {
    const { host, wrapper } = create({ width: 1920, height: 1080 });
    const bar = host.querySelector(".vjs-control-bar");
    const order = Array.from(bar?.children ?? [], (child) =>
      child.classList.contains("vv-quality")
        ? "quality"
        : child.classList.contains("vjs-playback-rate")
          ? "rate"
          : "other",
    );
    expect(order.indexOf("quality")).toBe(order.indexOf("rate") - 1);
    expect(items(wrapper)).toEqual([
      { text: "Original (1080p)", selected: true },
      { text: "720p", selected: false },
      { text: "480p", selected: false },
      { text: "360p", selected: false },
    ]);
    expect(label(wrapper)).toBe("1080p");
    const button = wrapper.querySelector("button");
    expect(button?.getAttribute("title")).toBe("Quality");
    expect(button?.getAttribute("aria-haspopup")).toBe("true");
    expect(wrapper.querySelector(".vv-quality-note")).toBeNull();
  });

  it("360p 以下の動画では選べる画質が無いことを押せない行で示す", () => {
    const { wrapper } = create({ width: 640, height: 360 });
    expect(items(wrapper)).toEqual([{ text: "Original (360p)", selected: true }]);
    const note = wrapper.querySelector(".vv-quality-note");
    expect(note?.textContent).toBe("No smaller sizes for this video");
    expect(note?.classList.contains("vjs-menu-item")).toBe(false);
    expect(note?.getAttribute("tabindex")).toBeNull();
    expect(label(wrapper)).toBe("360p");
  });

  it("寸法の無い動画では Original と Orig を出す", () => {
    const { wrapper } = create({});
    expect(items(wrapper)).toEqual([{ text: "Original", selected: true }]);
    expect(label(wrapper)).toBe("Orig");
    expect(wrapper.querySelector(".vv-quality-note")).not.toBeNull();
  });

  it("項目を選ぶと選んだ画質を知らせ、今の画質を渡すと印とボタンの文字が変わる", () => {
    const { wrapper, player: created } = create({ width: 1920, height: 1080 });
    const selected = vi.fn();
    created.on(qualitySelectEvent, (_event: Event, selection: QualitySelection) =>
      selected(selection.quality),
    );
    const item = Array.from(wrapper.querySelectorAll<HTMLElement>(".vjs-menu-item")).find(
      (candidate) => candidate.textContent?.startsWith("480p"),
    );
    item?.click();
    expect(selected).toHaveBeenCalledWith("480p");

    setQualityMenu(created, {
      options: qualityOptions({ width: 1920, height: 1080 }),
      current: "480p",
      sourceSize: "1080p",
    });
    expect(label(wrapper)).toBe("480p");
    expect(items(wrapper).filter((entry) => entry.selected)).toEqual([
      { text: "480p", selected: true },
    ]);
  });

  it("覚えていた画質で始めた動画は、その画質に印を付けて文字に出す", () => {
    const { wrapper } = create({ width: 1920, height: 1080 }, "720p");
    expect(label(wrapper)).toBe("720p");
    expect(items(wrapper).find((entry) => entry.selected)?.text).toBe("720p");
  });

  it("controlBarMenuOpen が画質のメニューの開閉を見分ける", () => {
    const { host, wrapper, player: created } = create({ width: 1920, height: 1080 });
    const menuButton = created.getChild("ControlBar")?.getChild(qualityMenuName) as
      { pressButton(): void; unpressButton(): void } | undefined;
    if (menuButton === undefined) throw new Error("画質のボタンがありません");
    expect(controlBarMenuOpen(host)).toBe(false);
    menuButton.pressButton();
    expect(controlBarMenuOpen(host)).toBe(true);
    menuButton.unpressButton();
    expect(controlBarMenuOpen(host)).toBe(false);

    const trigger = wrapper.querySelector("button");
    trigger?.dispatchEvent(new MouseEvent("mouseenter"));
    expect(controlBarMenuOpen(host)).toBe(true);
    wrapper.dispatchEvent(new MouseEvent("mouseleave"));
    expect(controlBarMenuOpen(host)).toBe(false);
  });

  it("疑似ロケールで、ボタン・項目・補足の行の文言がカタログから出る", () => {
    enablePseudoLocale();
    const { wrapper } = create({ width: 640, height: 360 });
    const button = wrapper.querySelector("button");
    if (button === null) throw new Error("画質のボタンがありません");
    expect(button.getAttribute("title")?.startsWith("⟦")).toBe(true);
    const value = wrapper.querySelector(".vv-quality-value");
    const note = wrapper.querySelector(".vv-quality-note");
    for (const node of [
      value,
      note,
      ...wrapper.querySelectorAll(".vjs-menu-item-text"),
    ]) {
      if (node === null) throw new Error("文言の要素がありません");
      expectCatalogTextOnly(node);
    }
  });
});
