import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import type { Video } from "../api/client";
import type { components } from "../api/gen/openapi";
import { formatDateTime } from "../i18n";
import { enablePseudoLocale, expectCatalogTextOnly } from "../i18n/pseudo";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import MediaInfoGroups, {
  audioCodecName,
  audioTrackText,
  subtitleCodecName,
  subtitleTrackText,
  videoItems,
} from "./MediaInfoGroups";
import VideoFacts from "./VideoFacts";

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

function groupItems(name: string): string[] {
  return within(screen.getByRole("list", { name }))
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
  it("すべてのまとまりを持つ動画は 5 つを順に出す", () => {
    render(<MediaInfoGroups info={full} />);
    expect(
      screen.getAllByRole("list").map((list) => list.getAttribute("aria-labelledby")),
    ).toHaveLength(5);
    expect(groupItems("Video")).toEqual([
      "HLG",
      "29.97 fps",
      "10-bit",
      "18.2 Mbps",
      "Interlaced",
    ]);
    expect(groupItems("Audio")).toEqual([
      "1 · AAC · 5.1 · Japanese · Default",
      "2 · AAC · Stereo · English",
    ]);
    expect(groupItems("Subtitles")).toEqual(["SRT · Japanese"]);
    expect(groupItems("Embedded")).toEqual([
      `Recorded${formatDateTime("2026-09-27T15:04:00+09:00")}`,
      "DeviceApple iPhone 15",
      "Location35.6895, 139.6917 · 40 m",
      "Cover art",
    ]);
    expect(groupItems("Broadcast")).toEqual(["StationNHK総合", "Programニュース"]);
  });

  it("video だけの動画は Video のまとまり 1 つだけを出す", () => {
    render(<MediaInfoGroups info={{ video: { hdr: "sdr" } }} />);
    expect(screen.getAllByRole("list")).toHaveLength(1);
    expect(groupItems("Video")).toEqual(["SDR"]);
  });

  it("mediaInfo が無ければ何も描かず、字幕はクローズドキャプションだけでも出る", () => {
    const { container, rerender } = render(<MediaInfoGroups info={undefined} />);
    expect(container.innerHTML).toBe("");
    rerender(<MediaInfoGroups info={{ video: { hdr: "sdr", closedCaptions: true } }} />);
    expect(groupItems("Subtitles")).toEqual(["Closed captions"]);
  });

  it("7 本以上のトラックは 6 本と +{n} more に畳み、開くと 7 本目へフォーカスを移す", () => {
    const tracks = Array.from({ length: 8 }, (_, index) => ({
      index,
      codec: "aac",
      language: "jpn",
    }));
    render(<MediaInfoGroups info={{ audioTracks: tracks }} />);
    expect(groupItems("Audio")).toHaveLength(7);
    fireEvent.click(screen.getByRole("button", { name: "Show all 8 audio tracks" }));
    const items = groupItems("Audio");
    expect(items).toHaveLength(9);
    expect(items[6]).toBe("7 · AAC · Japanese");
    expect(document.activeElement?.textContent).toBe("7 · AAC · Japanese");
    fireEvent.click(screen.getByRole("button", { name: "Show fewer" }));
    expect(groupItems("Audio")).toHaveLength(7);
  });

  it("コメントと説明はラベル付きの段落で出す", () => {
    render(<MediaInfoGroups info={{ embedded: { comment: "撮影メモ", title: "題" } }} />);
    expect(groupItems("Embedded")).toEqual(["Title題"]);
    expect(screen.getByText("撮影メモ").textContent).toBe("Comment撮影メモ");
  });

  it("コメントだけの Embedded は空の一覧を描かず、名前と段落を出す", () => {
    render(<MediaInfoGroups info={{ embedded: { comment: "撮影メモ" } }} />);
    expect(screen.queryAllByRole("list")).toHaveLength(0);
    expect(screen.getByText("Embedded")).toBeDefined();
    expect(screen.getByText("撮影メモ").textContent).toBe("Comment撮影メモ");
  });

  it("疑似ロケールではカタログの文言と書式の出力だけを描く", () => {
    enablePseudoLocale();
    render(
      <MediaInfoGroups
        info={{
          audioTracks: [{ index: 1, codec: "aac", language: "jpn" }],
          embedded: { comment: "撮影メモ" },
        }}
      />,
    );
    // Codec names are untranslated identifiers, like user content.
    expectCatalogTextOnly(document.body, ["AAC", "撮影メモ"]);
  });
});

describe("VideoFacts と mediaInfo", () => {
  const video: Video = {
    id: 7,
    virtual: false,
    title: "テスト動画",
    sizeBytes: 84_331_821,
    addedAt: "2026-09-20T03:00:00Z",
    updatedAt: "2026-09-21T03:00:00Z",
    fileCreatedAt: "2026-09-19T03:00:00Z",
    playable: true,
    probeState: "done",
    thumbnailState: "done",
    previewState: "done",
    durationMs: 242_000,
    width: 1920,
    height: 1080,
    container: "mkv",
    videoCodec: "h264",
    audioCodec: "aac",
    tags: [],
    public: false,
  };

  function facts(value: Video) {
    const view = render(
      <TooltipProvider>
        <VideoFacts video={value} />
      </TooltipProvider>,
    );
    const items = groupItems("File details");
    const technical = groupItems("Technical details");
    view.unmount();
    return { items, technical };
  }

  it("Length・Size・Added・Edited・Created と技術情報の行は mediaInfo があっても変わらない", () => {
    const without = facts(video);
    const withInfo = facts({ ...video, mediaInfo: full });
    expect(withInfo).toEqual(without);
    expect(without.items).toHaveLength(5);
  });

  it("まとまりは技術情報の行の後に出る", () => {
    render(
      <TooltipProvider>
        <VideoFacts video={{ ...video, mediaInfo: full }} />
      </TooltipProvider>,
    );
    const technical = screen.getByRole("list", { name: "Technical details" });
    const videoGroup = screen.getByRole("list", { name: "Video" });
    expect(
      technical.compareDocumentPosition(videoGroup) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });
});
