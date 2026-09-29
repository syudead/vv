import videojs from "video.js";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { SubtitleTrack } from "../api/client";
import { enablePseudoLocale, expectCatalogTextOnly } from "../i18n/pseudo";
import {
  createSubtitleTracks,
  type SubtitlePlayer,
  type SubtitleTracks,
} from "./subtitleTracks";
import { playerDictionary, playerLanguage } from "./VideoPlayer";

/**
 * 字幕ボタンとメニューを、本物の video.js が作る DOM で確かめる（受け入れ条件 2・3・6〜8・12）。
 * 操作バーは VideoPlayer と同じ設定（`subsCapsButton`、`textTrackSettings: false`、独自言語）にする。
 */

const storageKey = "vv.subtitles.v1";
const ja: SubtitleTrack = { file: "movie.ja.srt", label: "ja", format: "srt" };
const en: SubtitleTrack = { file: "movie.en.vtt", label: "en", format: "vtt" };
const plain: SubtitleTrack = { file: "movie.srt", label: "", format: "srt" };

type Player = ReturnType<typeof videojs>;

let player: Player | undefined;
let tracks: SubtitleTracks | undefined;

beforeEach(() => {
  // 字幕ファイルの取得（XHR）は jsdom では届かないので、送らずに止めておく。
  vi.spyOn(XMLHttpRequest.prototype, "send").mockImplementation(() => undefined);
});

afterEach(() => {
  tracks?.dispose();
  tracks = undefined;
  player?.dispose();
  player = undefined;
  window.localStorage.clear();
  vi.restoreAllMocks();
});

function save(value: unknown) {
  window.localStorage.setItem(storageKey, JSON.stringify(value));
}

function saved(): unknown {
  const raw = window.localStorage.getItem(storageKey);
  return raw === null ? null : JSON.parse(raw);
}

function create(list: readonly SubtitleTrack[]) {
  videojs.addLanguage(playerLanguage, playerDictionary());
  const element = document.createElement("video-js");
  document.body.append(element);
  const created = videojs(element, {
    controls: true,
    language: playerLanguage,
    textTrackSettings: false,
    playbackRates: [0.5, 1, 2],
    controlBar: { children: ["subsCapsButton", "playbackRateMenuButton"] },
  });
  player = created;
  const controller = createSubtitleTracks(created as unknown as SubtitlePlayer);
  tracks = controller;
  controller.replace(7, list);
  controller.start();
  const root = created.el();
  const wrapper = root.querySelector<HTMLElement>("div.vjs-subs-caps-button");
  return { player: created, controller, root, wrapper };
}

function textTracks(target: Player): TextTrack[] {
  const list = target.textTracks() as unknown as ArrayLike<TextTrack>;
  return Array.from({ length: list.length }, (_, index) => list[index] as TextTrack);
}

function showing(target: Player): string[] {
  return textTracks(target)
    .filter((track) => track.mode === "showing")
    .map((track) => track.label);
}

function menuItems(wrapper: HTMLElement | null): HTMLElement[] {
  return Array.from(wrapper?.querySelectorAll<HTMLElement>(".vjs-menu-item") ?? []);
}

function itemTexts(wrapper: HTMLElement | null): string[] {
  return menuItems(wrapper).map(
    (item) => item.querySelector(".vjs-menu-item-text")?.textContent ?? "",
  );
}

/** video.js は TextTrackList の change を次の tick にまとめて出す。 */
async function flush() {
  await new Promise((resolve) => window.setTimeout(resolve, 5));
}

describe("字幕ボタンとメニュー", () => {
  it("字幕が無い動画では字幕ボタンを出さない", () => {
    const { wrapper } = create([]);
    expect(wrapper).not.toBeNull();
    expect(wrapper?.classList.contains("vjs-hidden")).toBe(true);
  });

  it("2 件あるとボタンとメニューに ja・en と「オフ」が出て、字幕の設定の項目は無い", () => {
    const { wrapper, root } = create([ja, en]);
    expect(wrapper?.classList.contains("vjs-hidden")).toBe(false);
    expect(itemTexts(wrapper)).toEqual(["Off", "ja", "en"]);
    expect(root.querySelector(".vjs-texttrack-settings")).toBeNull();
    const button = root.querySelector<HTMLElement>("button.vjs-subs-caps-button");
    expect(button?.getAttribute("title")).toBe("Subtitles (C)");
    // 字幕ボタンは再生速度の前に並ぶ。
    const bar = root.querySelector(".vjs-control-bar");
    const order = Array.from(bar?.children ?? []).map((child) =>
      child.classList.contains("vjs-subs-caps-button")
        ? "subtitles"
        : child.classList.contains("vjs-playback-rate")
          ? "rate"
          : "other",
    );
    expect(order.filter((name) => name !== "other")).toEqual(["subtitles", "rate"]);
  });

  it("ラベルの無い字幕はカタログの既定の名前で出す", () => {
    const { wrapper } = create([plain, ja]);
    expect(itemTexts(wrapper)).toEqual(["Off", "Default", "ja"]);
  });

  it("疑似ロケールで、字幕のボタンとメニューの文言がカタログから出る", () => {
    enablePseudoLocale();
    const { root } = create([plain, ja]);
    const bar = root.querySelector<HTMLElement>(".vjs-control-bar");
    if (bar === null) throw new Error("操作バーがありません");
    const button = root.querySelector<HTMLElement>("button.vjs-subs-caps-button");
    expect(button?.getAttribute("title")?.startsWith("⟦")).toBe(true);
    const subtitles = root.querySelector<HTMLElement>("div.vjs-subs-caps-button");
    if (subtitles === null) throw new Error("字幕ボタンがありません");
    expectCatalogTextOnly(subtitles, ["ja"]);
  });
});

describe("字幕の選択と記憶", () => {
  it("保存値が無ければオフで始まる", () => {
    const { player: created } = create([ja, en]);
    expect(showing(created)).toEqual([]);
    expect(saved()).toBeNull();
  });

  it("保存したラベルの字幕があればオンで始まり、無ければオフで保存値を変えない", async () => {
    save({ enabled: true, label: "ja" });
    const first = create([en, ja]);
    expect(showing(first.player)).toEqual(["ja"]);
    await flush();
    expect(saved()).toEqual({ enabled: true, label: "ja" });
    tracks?.dispose();
    player?.dispose();

    const second = create([en]);
    expect(showing(second.player)).toEqual([]);
    await flush();
    expect(saved()).toEqual({ enabled: true, label: "ja" });
  });

  it("メニューで選ぶと保存値が変わり、「オフ」でオフを保存する", async () => {
    const { player: created, wrapper } = create([ja, en]);
    menuItems(wrapper)[2]?.click();
    await flush();
    expect(showing(created)).toEqual(["en"]);
    expect(saved()).toEqual({ enabled: true, label: "en" });

    menuItems(wrapper)[1]?.click();
    await flush();
    expect(showing(created)).toEqual(["ja"]);
    expect(saved()).toEqual({ enabled: true, label: "ja" });

    menuItems(wrapper)[0]?.click();
    await flush();
    expect(showing(created)).toEqual([]);
    expect(saved()).toEqual({ enabled: false, label: "ja" });
  });

  it("別の動画の一覧に替えると、前の動画のトラックを残さず作り直す", () => {
    save({ enabled: true, label: "en" });
    const { player: created, controller, wrapper } = create([ja]);
    expect(showing(created)).toEqual([]);
    controller.replace(8, [en]);
    expect(textTracks(created).map((track) => track.label)).toEqual(["en"]);
    expect(showing(created)).toEqual(["en"]);
    expect(itemTexts(wrapper)).toEqual(["Off", "en"]);
    controller.replace(9, []);
    expect(textTracks(created)).toEqual([]);
    expect(wrapper?.classList.contains("vjs-hidden")).toBe(true);
  });
});

describe("c キーの切り替え", () => {
  it("表示中ならオフにし、オフなら保存済みのラベルの字幕を出す", async () => {
    save({ enabled: true, label: "en" });
    const { player: created, controller } = create([ja, en]);
    expect(showing(created)).toEqual(["en"]);
    controller.toggle();
    expect(showing(created)).toEqual([]);
    expect(saved()).toEqual({ enabled: false, label: "en" });
    controller.toggle();
    expect(showing(created)).toEqual(["en"]);
    expect(saved()).toEqual({ enabled: true, label: "en" });
    await flush();
    expect(saved()).toEqual({ enabled: true, label: "en" });
  });

  it("保存済みのラベルが無ければメニューの最初の字幕を出す", () => {
    save({ enabled: false, label: "fr" });
    const { player: created, controller } = create([plain, ja]);
    controller.toggle();
    expect(showing(created)).toEqual(["Default"]);
    expect(saved()).toEqual({ enabled: true, label: "" });
  });

  it("字幕の無い動画では何もしない", () => {
    save({ enabled: false, label: "ja" });
    const { player: created, controller } = create([]);
    controller.toggle();
    expect(showing(created)).toEqual([]);
    expect(saved()).toEqual({ enabled: false, label: "ja" });
  });
});
