import type { ReactNode } from "react";

import type { Video } from "../api/client";
import type { components } from "../api/gen/openapi";
import { formatDateTime, formatLanguage, t, type UiText } from "../i18n";
import { formatResolution } from "../lib/format";
import { type Fact, FactList } from "../ui/patterns/fact-list";
import { formatCodec, technicalSummary } from "./properties";
import { Description, FoldButton, Item, useFold } from "./WorkInfo";

type MediaInfo = components["schemas"]["MediaInfo"];
type MediaInfoVideo = components["schemas"]["MediaInfoVideo"];
type AudioTrack = components["schemas"]["MediaInfoAudioTrack"];
type SubtitleTrack = components["schemas"]["MediaInfoSubtitleTrack"];
type Embedded = components["schemas"]["MediaInfoEmbedded"];
type Broadcast = components["schemas"]["MediaInfoBroadcast"];

const trackFoldLimit = 6;

const trackSeparator = " · ";

const audioCodecNames: Record<string, string> = {
  aac: "AAC",
  ac3: "AC-3",
  eac3: "E-AC-3",
  dts: "DTS",
  truehd: "TrueHD",
  flac: "FLAC",
  opus: "Opus",
  mp3: "MP3",
  mp2: "MP2",
  vorbis: "Vorbis",
};

const subtitleCodecNames: Record<string, string> = {
  subrip: "SRT",
  ass: "ASS",
  ssa: "SSA",
  hdmv_pgs_subtitle: "PGS",
  dvd_subtitle: "DVD",
  webvtt: "WebVTT",
  mov_text: "MP4 text",
  arib_caption: "ARIB",
  dvb_subtitle: "DVB",
  eia_608: "EIA-608",
};

/** Normalize pcm_* to PCM; use formatCodec for unknown codecs. */
export function audioCodecName(codec: string): string {
  const key = codec.toLowerCase();
  if (key.startsWith("pcm_")) return "PCM";
  return audioCodecNames[key] ?? formatCodec(codec);
}

export function subtitleCodecName(codec: string): string {
  return subtitleCodecNames[codec.toLowerCase()] ?? formatCodec(codec);
}

function channelLayoutName(track: AudioTrack): string | undefined {
  const m = t.player.mediaInfo;
  const layout = track.channelLayout?.trim();
  if (layout !== undefined && layout !== "") {
    if (layout.toLowerCase() === "mono") return m.mono;
    if (layout.toLowerCase() === "stereo") return m.stereo;
    return layout;
  }
  return track.channels === undefined ? undefined : m.channels(track.channels);
}

function present(parts: readonly (string | undefined | false)[]): string {
  return parts
    .filter((part): part is string => typeof part === "string" && part !== "")
    .join(trackSeparator);
}

function language(code: string | undefined): string | undefined {
  return code === undefined ? undefined : (formatLanguage(code) ?? undefined);
}

export function audioTrackText(track: AudioTrack, number?: number): string {
  const m = t.player.mediaInfo;
  return present([
    number !== undefined && m.trackNumber(number),
    track.codec === undefined ? undefined : audioCodecName(track.codec),
    channelLayoutName(track),
    language(track.language),
    track.title,
    track.default === true && m.default,
  ]);
}

export function subtitleTrackText(track: SubtitleTrack, number?: number): string {
  const m = t.player.mediaInfo;
  return present([
    number !== undefined && m.trackNumber(number),
    track.codec === undefined ? undefined : subtitleCodecName(track.codec),
    language(track.language),
    track.title,
    track.default === true && m.default,
    track.forced === true && m.forced,
    track.hearingImpaired === true && m.hearingImpaired,
  ]);
}

export function videoItems(video: MediaInfoVideo): string[] {
  const m = t.player.mediaInfo;
  return [
    m.hdr[video.hdr],
    video.frameRate === undefined ? undefined : m.frameRate(video.frameRate),
    video.bitDepth === undefined ? undefined : m.bitDepth(video.bitDepth),
    video.bitRate === undefined ? undefined : m.bitRate(video.bitRate),
    video.interlaced === true ? m.interlaced : undefined,
    video.projection === undefined ? undefined : m.spherical,
    video.stereo3d === undefined ? undefined : m.stereo3d(video.stereo3d),
  ].filter((item): item is UiText => item !== undefined);
}

function WordItem({ children }: { children: ReactNode }) {
  return <li className="whitespace-nowrap tabular-nums">{children}</li>;
}

function LabelledItem({ label, value }: { label: UiText; value: string | undefined }) {
  if (value === undefined || value === "") return null;
  return (
    <Item label={label}>
      <span className="min-w-0 tabular-nums wrap-anywhere">{value}</span>
    </Item>
  );
}

/** One track per line; more than six fold to six, and expanding focuses the seventh. */
function TrackList({
  texts,
  label,
  extra,
}: {
  texts: readonly string[];
  label: UiText;
  extra?: ReactNode;
}) {
  const fold = useFold<HTMLLIElement>(texts.length, trackFoldLimit);
  const shown = fold.folded ? texts.slice(0, trackFoldLimit) : texts;
  return (
    <ul className="flex flex-col items-start gap-1">
      {shown.map((text, index) => (
        <li
          // Track labels may repeat; their stable order makes indices suitable keys.
          key={index}
          ref={index === trackFoldLimit ? fold.nextRef : undefined}
          tabIndex={index === trackFoldLimit ? -1 : undefined}
          className="min-w-0 tabular-nums wrap-anywhere"
        >
          {text}
        </li>
      ))}
      {fold.foldable && (
        <li>
          <FoldButton
            folded={fold.folded}
            count={texts.length}
            limit={trackFoldLimit}
            label={label}
            onToggle={fold.toggle}
          />
        </li>
      )}
      {extra}
    </ul>
  );
}

function numbered<T>(tracks: readonly T[], text: (track: T, number?: number) => string) {
  return tracks.map((track, index) =>
    text(track, tracks.length >= 2 ? index + 1 : undefined),
  );
}

function locationText(location: NonNullable<Embedded["location"]>): string {
  const m = t.player.mediaInfo;
  const coordinates = m.coordinates(location.latitude, location.longitude);
  return location.altitude === undefined
    ? coordinates
    : `${coordinates}${trackSeparator}${m.altitude(location.altitude)}`;
}

function embeddedItems(embedded: Embedded): [UiText, string][] {
  const m = t.player.mediaInfo;
  const items: [UiText, string | undefined][] = [
    [m.title, embedded.title],
    [m.author, embedded.author],
    [m.genre, embedded.genre],
    [m.date, embedded.date],
    [
      m.recorded,
      embedded.recordedAt === undefined ? undefined : formatDateTime(embedded.recordedAt),
    ],
    [m.device, embedded.device],
    [
      m.location,
      embedded.location === undefined ? undefined : locationText(embedded.location),
    ],
  ];
  return items.filter((item): item is [UiText, string] => (item[1] ?? "") !== "");
}

const itemRow = "flex flex-wrap items-baseline gap-x-4 gap-y-1";

/** Avoid an empty list when only a comment or description is present. */
function EmbeddedValue({ embedded }: { embedded: Embedded }) {
  const m = t.player.mediaInfo;
  const items = embeddedItems(embedded);
  const coverArt = embedded.coverArt === true;
  return (
    <div className="flex flex-col gap-1">
      {(items.length > 0 || coverArt) && (
        <ul className={itemRow}>
          {items.map(([label, value]) => (
            <LabelledItem key={label} label={label} value={value} />
          ))}
          {coverArt && <WordItem>{m.coverArt}</WordItem>}
        </ul>
      )}
      {embedded.comment !== undefined && (
        <Description text={embedded.comment} tone="muted" label={m.comment} />
      )}
      {embedded.description !== undefined && (
        <Description text={embedded.description} tone="muted" label={m.description} />
      )}
    </div>
  );
}

function embeddedFact(embedded: Embedded | undefined): Fact | null {
  if (embedded === undefined) return null;
  const empty =
    embeddedItems(embedded).length === 0 &&
    embedded.coverArt !== true &&
    embedded.comment === undefined &&
    embedded.description === undefined;
  if (empty) return null;
  return {
    id: "embedded",
    term: t.player.mediaInfo.embedded,
    value: <EmbeddedValue embedded={embedded} />,
  };
}

function broadcastFact(broadcast: Broadcast | undefined): Fact | null {
  if (broadcast?.station === undefined && broadcast?.program === undefined) return null;
  const m = t.player.mediaInfo;
  return {
    id: "broadcast",
    term: m.broadcast,
    value: (
      <ul className={itemRow}>
        <LabelledItem label={m.station} value={broadcast.station} />
        <LabelledItem label={m.program} value={broadcast.program} />
      </ul>
    ),
  };
}

/** The probe's resolution and codec, then the media information's marks. */
function videoFact(video: Video): Fact | null {
  const info = video.mediaInfo?.video;
  const text = present([
    formatResolution(video),
    video.videoCodec === undefined ? undefined : formatCodec(video.videoCodec),
    ...(info === undefined ? [] : videoItems(info)),
  ]);
  if (text === "") return null;
  return {
    id: "video",
    term: t.player.mediaInfo.video,
    value: <span className="tabular-nums">{text}</span>,
  };
}

/** Without media information the probe's codec stands for the audio. */
function audioFact(video: Video): Fact | null {
  const m = t.player.mediaInfo;
  const info = video.mediaInfo;
  const fallback =
    video.audioCodec === undefined ? [] : [audioCodecName(video.audioCodec)];
  const texts =
    info === undefined ? fallback : numbered(info.audioTracks ?? [], audioTrackText);
  if (texts.length === 0) return null;
  return {
    id: "audio",
    term: m.audio,
    value: <TrackList texts={texts} label={m.audioTracks} />,
  };
}

/** A probed group with nothing in it, read as `None` rather than left out. */
function noneFact(id: string, term: UiText): Fact {
  return {
    id,
    term,
    value: <span className="text-muted-foreground">{t.player.tabs.none}</span>,
  };
}

function subtitleFact(info: MediaInfo | undefined): Fact | null {
  const m = t.player.mediaInfo;
  const subtitles = info?.subtitleTracks ?? [];
  const closedCaptions = info?.video?.closedCaptions === true;
  if (subtitles.length === 0 && !closedCaptions) {
    return info === undefined ? null : noneFact("subtitles", m.subtitles);
  }
  return {
    id: "subtitles",
    term: m.subtitles,
    value: (
      <TrackList
        texts={numbered(subtitles, subtitleTrackText)}
        label={m.subtitleTracks}
        extra={closedCaptions && <WordItem>{m.closedCaptions}</WordItem>}
      />
    ),
  };
}

function chapterFact(info: MediaInfo | undefined): Fact | null {
  const count = info?.chapters?.length ?? 0;
  const text = t.player.tabs;
  if (count === 0) return info === undefined ? null : noneFact("chapters", text.chapters);
  return { id: "chapters", term: text.chapters, value: text.chapterCount(count) };
}

function containerFact(video: Video): Fact | null {
  const container = video.container?.toUpperCase() ?? "";
  if (container === "") return null;
  return { id: "container", term: t.player.tabs.container, value: container };
}

/** The media facts in the panel's order; a group without content is left out. */
export function mediaFacts(video: Video): Fact[] {
  const info = video.mediaInfo;
  return [
    videoFact(video),
    audioFact(video),
    subtitleFact(info),
    chapterFact(info),
    containerFact(video),
    embeddedFact(info?.embedded),
    broadcastFact(info?.broadcast),
  ].filter((fact): fact is Fact => fact !== null);
}

/** The Media section's content: the facts once the probe is done, otherwise why there are none. */
export default function MediaInfoGroups({ video }: { video: Video }) {
  const summary = technicalSummary(video);
  if (summary.kind === "pending") {
    return (
      <p className="text-sm text-muted-foreground">{t.player.facts.technicalPending}</p>
    );
  }
  if (summary.kind === "failed") {
    return <p className="text-sm text-warning">{t.player.facts.technicalFailed}</p>;
  }
  const facts = mediaFacts(video);
  if (facts.length === 0) return null;
  return <FactList facts={facts} />;
}
