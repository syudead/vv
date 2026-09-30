import videojs from "video.js";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { SubtitleTrack } from "../api/client";
import { enablePseudoLocale, expectCatalogTextOnly } from "../i18n/pseudo";
import {
  createSubtitleTracks,
  sameSubtitleLabel,
  type SubtitlePlayer,
  type SubtitleTracks,
} from "./subtitleTracks";
import { registerSubtitlesButton, subtitlesButtonName } from "./subtitleMenu";
import { playerDictionary, playerLanguage } from "./VideoPlayer";

/**
 * 字幕ボタンとメニューを、本物の video.js が作る DOM で確かめる（受け入れ条件 2・3・6〜8・12）。
 * 操作バーは VideoPlayer と同じ設定（字幕ボタン、`textTrackSettings: false`、独自言語）にする。
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
  registerSubtitlesButton();
  const element = document.createElement("video-js");
  document.body.append(element);
  const created = videojs(element, {
    controls: true,
    language: playerLanguage,
    textTrackSettings: false,
    playbackRates: [0.5, 1, 2],
    controlBar: { children: [subtitlesButtonName, "playbackRateMenuButton"] },
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

  it("プレイヤーの文言と同じ名前のラベルも、ファイル名のまま出す", () => {
    const labels = ["Mute", "Subtitles", "subtitles off", "constructor"];
    const { wrapper } = create(
      labels.map((label) => ({ file: `movie.${label}.srt`, label, format: "srt" })),
    );
    expect(itemTexts(wrapper)).toEqual(["Off", ...labels]);
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

  it("保存したラベルは、大文字・小文字と Unicode の正規化の違いを同じラベルとみなす", async () => {
    save({ enabled: true, label: "JA" });
    const first = create([en, ja]);
    expect(showing(first.player)).toEqual(["ja"]);
    await flush();
    expect(saved()).toEqual({ enabled: true, label: "JA" });
    tracks?.dispose();
    player?.dispose();

    // 保存値は NFD の「が」、字幕は NFC の「が」。
    save({ enabled: true, label: "\u304b\u3099" });
    const second = create([
      en,
      { file: "movie.\u304c.srt", label: "\u304c", format: "srt" },
    ]);
    expect(showing(second.player)).toEqual(["\u304c"]);
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

describe("ライブ変換の offset に合わせた付け直し", () => {
  function sources(target: Player): string[] {
    const list = target.remoteTextTrackEls() as unknown as ArrayLike<HTMLTrackElement>;
    return Array.from({ length: list.length }, (_, index) => list[index]?.src ?? "");
  }

  it("報告を待つ間はトラックを外し、届いたらその offset の URL で付け直して表示を保つ", async () => {
    save({ enabled: true, label: "en" });
    const { player: created, controller, wrapper } = create([ja, en]);
    // 見る人がメニューで ja を選んでいる（保存値も ja になる）。
    menuItems(wrapper)[1]?.click();
    await flush();
    expect(showing(created)).toEqual(["ja"]);
    expect(saved()).toEqual({ enabled: true, label: "ja" });
    // 保存値だけを en に戻し、付け直しが保存値ではなく直前の表示に従うかを見る。
    save({ enabled: true, label: "en" });

    controller.setOffset(null);
    expect(textTracks(created)).toEqual([]);
    expect(wrapper?.classList.contains("vjs-hidden")).toBe(true);
    await flush();
    expect(saved()).toEqual({ enabled: true, label: "en" });

    controller.setOffset(8000);
    expect(textTracks(created).map((track) => track.label)).toEqual(["ja", "en"]);
    expect(sources(created)).toEqual([
      expect.stringMatching(
        /\/api\/videos\/7\/subtitles\/movie\.ja\.srt\?offsetMs=8000$/,
      ),
      expect.stringMatching(
        /\/api\/videos\/7\/subtitles\/movie\.en\.vtt\?offsetMs=8000$/,
      ),
    ]);
    expect(showing(created)).toEqual(["ja"]);
    await flush();
    expect(saved()).toEqual({ enabled: true, label: "en" });
  });

  it("オフのまま付け直すとオフを保ち、同じ offset では付け直さない", async () => {
    const add = vi.spyOn(
      videojs.getComponent("Player").prototype as unknown as SubtitlePlayer,
      "addRemoteTextTrack",
    );
    const { player: created, controller } = create([ja]);
    expect(add).toHaveBeenCalledTimes(1);
    controller.setOffset(0);
    expect(add).toHaveBeenCalledTimes(1);
    controller.setOffset(null);
    controller.setOffset(64_000);
    expect(add).toHaveBeenCalledTimes(2);
    expect(showing(created)).toEqual([]);
    await flush();
    expect(saved()).toBeNull();
  });

  it("change が届く前に未決になっても、メニューで選んだ字幕を保存して付け直す", async () => {
    const { player: created, controller, wrapper } = create([ja, en]);
    menuItems(wrapper)[2]?.click();
    controller.setOffset(null);
    expect(saved()).toEqual({ enabled: true, label: "en" });
    controller.setOffset(8000);
    expect(showing(created)).toEqual(["en"]);
    await flush();
    expect(saved()).toEqual({ enabled: true, label: "en" });
  });

  it("未決の間に一覧が替わったら、付けるときは保存値で表示を決める", () => {
    save({ enabled: true, label: "en" });
    const { player: created, controller } = create([ja]);
    controller.setOffset(null);
    controller.replace(8, [ja, en]);
    expect(textTracks(created)).toEqual([]);
    controller.setOffset(2000);
    expect(showing(created)).toEqual(["en"]);
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

  it("オンにするとき、保存済みのラベルを大文字・小文字を区別せずに探す", () => {
    save({ enabled: false, label: "EN" });
    const { player: created, controller } = create([ja, en]);
    controller.toggle();
    expect(showing(created)).toEqual(["en"]);
    expect(saved()).toEqual({ enabled: true, label: "en" });
  });

  it("字幕の無い動画では何もしない", () => {
    save({ enabled: false, label: "ja" });
    const { player: created, controller } = create([]);
    controller.toggle();
    expect(showing(created)).toEqual([]);
    expect(saved()).toEqual({ enabled: false, label: "ja" });
  });
});

describe("sameSubtitleLabel", () => {
  it("サーバーと同じく NFC 正規化と文字ごとの case folding で比べる", () => {
    expect(sameSubtitleLabel("ja", "JA")).toBe(true);
    expect(sameSubtitleLabel("en.Forced", "EN.forced")).toBe(true);
    expect(sameSubtitleLabel("\u03a3", "\u03c2")).toBe(true);
    expect(sameSubtitleLabel("\u212a", "k")).toBe(true);
    expect(sameSubtitleLabel("\u304b\u3099", "\u304c")).toBe(true);
    expect(sameSubtitleLabel("ja", "jp")).toBe(false);
    expect(sameSubtitleLabel("", "ja")).toBe(false);
    expect(sameSubtitleLabel("", "")).toBe(true);
  });
});
