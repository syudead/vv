import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import type { Video } from "../api/client";
import type { components } from "../api/gen/openapi";
import { formatDateTime } from "../i18n";
import { enablePseudoLocale, expectCatalogTextOnly } from "../i18n/pseudo";
import MediaInfoGroups, {
  audioCodecName,
  audioTrackText,
  subtitleCodecName,
  subtitleTrackText,
  videoItems,
} from "./MediaInfoGroups";

type MediaInfo = components["schemas"]["MediaInfo"];

const full: MediaInfo = {
  video: {
    hdr: "hlg",
    frameRate: 29.97,
    bitDepth: 10,
    bitRate: 18_200_000,
    interlaced: true,
  },
  audioTracks: [
    { index: 1, codec: "aac", channelLayout: "5.1", language: "jpn", default: true },
    { index: 2, codec: "aac", channelLayout: "stereo", language: "eng" },
  ],
  subtitleTracks: [{ index: 3, codec: "subrip", language: "jpn" }],
  embedded: {
    recordedAt: "2026-09-27T15:04:00+09:00",
    device: "Apple iPhone 15",
    location: { latitude: 35.6895, longitude: 139.6917, altitude: 40.2 },
    coverArt: true,
  },
  broadcast: { station: "NHK総合", program: "ニュース" },
};

const bare: Video = {
  id: 7,
  title: "a",
  sizeBytes: 1,
  addedAt: "2026-09-20T03:00:00Z",
  updatedAt: "2026-09-21T03:00:00Z",
  fileCreatedAt: "2026-09-19T03:00:00Z",
  fileModifiedAt: "2026-09-19T03:00:00Z",
  playable: true,
  probeState: "done",
  thumbnailState: "done",
  previewState: "done",
  tags: [],
  public: false,
};

function renderMedia(value: Partial<Video>) {
  return render(<MediaInfoGroups video={{ ...bare, ...value }} />);
}

function terms(): string[] {
  return screen.queryAllByRole("term").map((term) => term.textContent ?? "");
}

function value(term: string): HTMLElement {
  const found = screen.getAllByRole("term").find((item) => item.textContent === term);
  const definition = found?.nextElementSibling;
  if (!(definition instanceof HTMLElement)) throw new Error(`no ${term}`);
  return definition;
}

function items(term: string): string[] {
  return within(value(term))
    .getAllByRole("listitem")
    .map((item) => item.textContent ?? "");
}

describe("MediaInfoGroups の文字", () => {
  it("Video のまとまりは HDR の種類・コマ数・深さ・ビットレート・インターレースの順", () => {
    expect(videoItems(full.video!)).toEqual([
      "HLG",
      "29.97 fps",
      "10-bit",
      "18.2 Mbps",
      "Interlaced",
    ]);
    expect(
      videoItems({
        hdr: "dolby_vision",
        frameRate: 30,
        bitRate: 850_400,
        interlaced: false,
        projection: "equirectangular",
        stereo3d: "side by side",
      }),
    ).toEqual(["Dolby Vision", "30 fps", "850 kbps", "360°", "3D (side by side)"]);
    expect(
      videoItems({ hdr: "sdr", frameRate: 24000 / 1001, bitRate: 12_000_000 }),
    ).toEqual(["SDR", "23.98 fps", "12 Mbps"]);
  });

  it("jpn は Japanese と出て、und と言語の無いトラックは言語を出さない", () => {
    expect(audioTrackText({ index: 1, codec: "aac", language: "jpn", channels: 2 })).toBe(
      "AAC · 2 ch · Japanese",
    );
    expect(audioTrackText({ index: 1, codec: "aac", language: "und" })).toBe("AAC");
    expect(subtitleTrackText({ index: 2, language: "und", title: "Signs" })).toBe(
      "Signs",
    );
    expect(
      subtitleTrackText(
        {
          index: 4,
          codec: "hdmv_pgs_subtitle",
          language: "eng",
          default: true,
          forced: true,
          hearingImpaired: true,
        },
        2,
      ),
    ).toBe("2 · PGS · English · Default · Forced · Hearing impaired");
  });

  it("コーデックは表の名前にし、表に無いものは大文字にする", () => {
    expect(audioCodecName("eac3")).toBe("E-AC-3");
    expect(audioCodecName("pcm_s16le")).toBe("PCM");
    expect(audioCodecName("alac")).toBe("ALAC");
    expect(subtitleCodecName("arib_caption")).toBe("ARIB");
    expect(subtitleCodecName("mov_text")).toBe("MP4 text");
    expect(subtitleCodecName("xsub")).toBe("XSUB");
  });
});

describe("MediaInfoGroups", () => {
  it("すべてのまとまりを持つ動画は Video・Audio・Subtitles・Chapters・Container・Embedded・Broadcast の順に出す", () => {
    renderMedia({
      width: 1920,
      height: 1080,
      videoCodec: "h264",
      container: "mkv",
      mediaInfo: {
        ...full,
        chapters: [
          { startMs: 0, endMs: 1 },
          { startMs: 1, endMs: 2 },
          { startMs: 2, endMs: 3 },
        ],
      },
    });
    expect(terms()).toEqual([
      "Video",
      "Audio",
      "Subtitles",
      "Chapters",
      "Container",
      "Embedded",
      "Broadcast",
    ]);
    expect(value("Video").textContent).toBe(
      "1920×1080 · H.264 · HLG · 29.97 fps · 10-bit · 18.2 Mbps · Interlaced",
    );
    expect(items("Audio")).toEqual([
      "1 · AAC · 5.1 · Japanese · Default",
      "2 · AAC · Stereo · English",
    ]);
    expect(items("Subtitles")).toEqual(["SRT · Japanese"]);
    expect(value("Chapters").textContent).toBe("3 chapters");
    expect(value("Container").textContent).toBe("MKV");
    expect(items("Embedded")).toEqual([
      `Recorded${formatDateTime("2026-09-27T15:04:00+09:00")}`,
      "DeviceApple iPhone 15",
      "Location35.6895, 139.6917 · 40 m",
      "Cover art",
    ]);
    expect(items("Broadcast")).toEqual(["StationNHK総合", "Programニュース"]);
  });

  it("mediaInfo が無ければ技術情報の解像度とコーデック・音声のコーデック・コンテナから組む", () => {
    renderMedia({
      width: 1920,
      height: 1080,
      container: "mp4",
      videoCodec: "hevc",
      audioCodec: "eac3",
    });
    expect(terms()).toEqual(["Video", "Audio", "Container"]);
    expect(value("Video").textContent).toBe("1920×1080 · H.265");
    expect(items("Audio")).toEqual(["E-AC-3"]);
    expect(value("Container").textContent).toBe("MP4");
  });

  it("何も分からない動画は一覧を描かず、字幕はクローズドキャプションだけでも出る", () => {
    const { container, unmount } = renderMedia({});
    expect(container.innerHTML).toBe("");
    unmount();
    renderMedia({ mediaInfo: { video: { hdr: "sdr", closedCaptions: true } } });
    expect(items("Subtitles")).toEqual(["Closed captions"]);
  });

  it("probe 済みで字幕も章も無い動画は、Subtitles と Chapters を None と出す", () => {
    renderMedia({ container: "mp4", mediaInfo: { video: { hdr: "sdr" } } });
    expect(terms()).toEqual(["Video", "Subtitles", "Chapters", "Container"]);
    expect(value("Subtitles").textContent).toBe("None");
    expect(value("Chapters").textContent).toBe("None");
  });

  it("読み取り中は読み取り中と出し、失敗なら警告の色で 1 行出す", () => {
    const view = renderMedia({ probeState: "pending" });
    expect(screen.getByText("Reading technical details…")).toBeDefined();
    view.unmount();
    renderMedia({ probeState: "failed", probeError: "moov atom" });
    expect(screen.getByText("Couldn't read the technical details").className).toContain(
      "text-warning",
    );
  });

  it("7 本以上のトラックは 6 本と +{n} more に畳み、開くと 7 本目へフォーカスを移す", () => {
    const tracks = Array.from({ length: 8 }, (_, index) => ({
      index,
      codec: "aac",
      language: "jpn",
    }));
    renderMedia({ mediaInfo: { audioTracks: tracks } });
    expect(items("Audio")).toHaveLength(7);
    fireEvent.click(screen.getByRole("button", { name: "Show all 8 audio tracks" }));
    const shown = items("Audio");
    expect(shown).toHaveLength(9);
    expect(shown[6]).toBe("7 · AAC · Japanese");
    expect(document.activeElement?.textContent).toBe("7 · AAC · Japanese");
    fireEvent.click(screen.getByRole("button", { name: "Show fewer" }));
    expect(items("Audio")).toHaveLength(7);
  });

  it("コメントと説明はラベル付きの段落で出す", () => {
    renderMedia({ mediaInfo: { embedded: { comment: "撮影メモ", title: "題" } } });
    expect(items("Embedded")).toEqual(["Title題"]);
    expect(screen.getByText("撮影メモ").textContent).toBe("Comment撮影メモ");
  });

  it("コメントだけの Embedded は空の一覧を描かず、名前と段落を出す", () => {
    renderMedia({ mediaInfo: { embedded: { comment: "撮影メモ" } } });
    expect(within(value("Embedded")).queryAllByRole("list")).toHaveLength(0);
    expect(screen.getByText("撮影メモ").textContent).toBe("Comment撮影メモ");
  });

  it("疑似ロケールではカタログの文言と書式の出力だけを描く", () => {
    enablePseudoLocale();
    renderMedia({
      mediaInfo: {
        audioTracks: [{ index: 1, codec: "aac", language: "jpn" }],
        embedded: { comment: "撮影メモ" },
      },
    });
    // Codec names are untranslated identifiers, like user content.
    expectCatalogTextOnly(document.body, ["AAC", "撮影メモ"]);
  });
});
