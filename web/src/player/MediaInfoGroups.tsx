import { type ReactNode, useId } from "react";

import type { components } from "../api/gen/openapi";
import { formatDateTime, formatLanguage, t, type UiText } from "../i18n";
import { cn } from "../lib/cn";
import { formatCodec } from "./properties";
import { Description, FoldButton, Item, useFold } from "./WorkInfo";

type MediaInfo = components["schemas"]["MediaInfo"];
type MediaInfoVideo = components["schemas"]["MediaInfoVideo"];
type AudioTrack = components["schemas"]["MediaInfoAudioTrack"];
type SubtitleTrack = components["schemas"]["MediaInfoSubtitleTrack"];
type Embedded = components["schemas"]["MediaInfoEmbedded"];
type Broadcast = components["schemas"]["MediaInfoBroadcast"];

/** trackFoldLimit は、トラックの多いまとまりを畳んだときに出しておくトラックの数である。 */
const trackFoldLimit = 6;

/** trackSeparator はトラックの項目の部分をつなぐ文字である。 */
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

/** audioCodecName は音声の `codec_name` を表の名前にする。`pcm_*` は PCM、表に無いものは formatCodec。 */
export function audioCodecName(codec: string): string {
  const key = codec.toLowerCase();
  if (key.startsWith("pcm_")) return "PCM";
  return audioCodecNames[key] ?? formatCodec(codec);
}

/** subtitleCodecName は字幕の `codec_name` を表の名前にする。表に無いものは formatCodec。 */
export function subtitleCodecName(codec: string): string {
  return subtitleCodecNames[codec.toLowerCase()] ?? formatCodec(codec);
}

/** channelLayoutName は音声の並びを `Mono`・`Stereo`・`5.1`・`{n} ch` のように書く。 */
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

/**
 * audioTrackText は音声トラック 1 本の項目の文字である。番号（2 本以上のとき）、コーデック、
 * 並び、言語の名前、題名、Default の順につなぎ、無い部分は飛ばす。
 */
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

/** subtitleTrackText は字幕トラック 1 本の項目の文字である。Default・Forced・Hearing impaired の順に足す。 */
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

/** videoItems は Video のまとまりの項目（HDR の種類、コマ数、深さ、ビットレート、…）である。 */
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

/** groupNameClass はまとまりの名前の形（技術情報の行と同じ小さな大文字）である。 */
const groupNameClass = "text-xs tracking-wider uppercase";

/**
 * Group はまとまり 1 つの行である。ファイルの行と同じ折り返す並びで、先頭に技術情報の行の形の
 * まとまりの名前を置き、その名前で `ul` に名前を付ける。全体を薄い色にする。
 */
function Group({ name, children }: { name: UiText; children: ReactNode }) {
  const id = useId();
  return (
    <ul
      aria-labelledby={id}
      className="flex flex-wrap items-baseline gap-x-4 gap-y-2 text-sm text-muted-foreground sm:gap-x-5"
    >
      <li id={id} lang="en" aria-hidden="true" className={groupNameClass}>
        {name}
      </li>
      {children}
    </ul>
  );
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

/**
 * TrackItems はトラックの項目の並びである。7 本以上あれば先頭の 6 本と「+{n} more」を出し、
 * 押すとすべてを出してフォーカスを 7 本目へ移す（作品情報の長いリストと同じ畳み方）。
 */
function TrackItems({ texts, label }: { texts: readonly string[]; label: UiText }) {
  const fold = useFold<HTMLLIElement>(texts.length, trackFoldLimit);
  const shown = fold.folded ? texts.slice(0, trackFoldLimit) : texts;
  return (
    <>
      {shown.map((text, index) => (
        <li
          // 同じ文字のトラックもあるので、位置を鍵にする（並びは変わらない）。
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
    </>
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

/** embeddedItems は Embedded の行の `label value` の組（値のあるものだけ）である。 */
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

/**
 * EmbeddedGroup は Embedded のまとまりである。行に項目が無く、コメントか説明だけのときは、
 * 空の `ul` を描かず、まとまりの名前を段落の上に見出しの形で置く。
 */
function EmbeddedGroup({ embedded }: { embedded: Embedded }) {
  const m = t.player.mediaInfo;
  const items = embeddedItems(embedded);
  const coverArt = embedded.coverArt === true;
  return (
    <>
      {items.length > 0 || coverArt ? (
        <Group name={m.embedded}>
          {items.map(([label, value]) => (
            <LabelledItem key={label} label={label} value={value} />
          ))}
          {coverArt && <WordItem>{m.coverArt}</WordItem>}
        </Group>
      ) : (
        <p lang="en" className={cn(groupNameClass, "text-muted-foreground")}>
          {m.embedded}
        </p>
      )}
      {embedded.comment !== undefined && (
        <Description text={embedded.comment} tone="muted" label={m.comment} />
      )}
      {embedded.description !== undefined && (
        <Description text={embedded.description} tone="muted" label={m.description} />
      )}
    </>
  );
}

function BroadcastGroup({ broadcast }: { broadcast: Broadcast }) {
  const m = t.player.mediaInfo;
  if (broadcast.station === undefined && broadcast.program === undefined) return null;
  return (
    <Group name={m.broadcast}>
      <LabelledItem label={m.station} value={broadcast.station} />
      <LabelledItem label={m.program} value={broadcast.program} />
    </Group>
  );
}

/**
 * MediaInfoGroups は情報欄の技術情報の行の下の、動画の追加の情報のまとまりである。Video・Audio・Subtitles・
 * Embedded・Broadcast の順に、項目のあるまとまりだけを出す。`mediaInfo` の無い動画では何も
 * 描かない（情報欄は今までどおり）。チャプターはプレーヤーに渡し、ここには出さない。
 */
export default function MediaInfoGroups({ info }: { info: MediaInfo | undefined }) {
  if (info === undefined) return null;
  const m = t.player.mediaInfo;
  const audio = info.audioTracks ?? [];
  const subtitles = info.subtitleTracks ?? [];
  const closedCaptions = info.video?.closedCaptions === true;
  return (
    <>
      {info.video !== undefined && (
        <Group name={m.video}>
          {videoItems(info.video).map((item) => (
            <WordItem key={item}>{item}</WordItem>
          ))}
        </Group>
      )}
      {audio.length > 0 && (
        <Group name={m.audio}>
          <TrackItems texts={numbered(audio, audioTrackText)} label={m.audioTracks} />
        </Group>
      )}
      {(subtitles.length > 0 || closedCaptions) && (
        <Group name={m.subtitles}>
          <TrackItems
            texts={numbered(subtitles, subtitleTrackText)}
            label={m.subtitleTracks}
          />
          {closedCaptions && <WordItem>{m.closedCaptions}</WordItem>}
        </Group>
      )}
      {info.embedded !== undefined && <EmbeddedGroup embedded={info.embedded} />}
      {info.broadcast !== undefined && <BroadcastGroup broadcast={info.broadcast} />}
    </>
  );
}
