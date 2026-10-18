import {
  appendFileSync,
  copyFileSync,
  mkdirSync,
  rmSync,
  utimesSync,
  writeFileSync,
} from "node:fs";
import path from "node:path";
import { spawnSync } from "node:child_process";

const commonInput = [
  "-f",
  "lavfi",
  "-i",
  "testsrc2=size=320x180:rate=15:duration=6",
  "-f",
  "lavfi",
  "-i",
  "sine=frequency=660:sample_rate=48000:duration=6",
];

const h264 = [
  "-c:v",
  "libx264",
  "-preset",
  "ultrafast",
  "-pix_fmt",
  "yuv420p",
  "-profile:v",
  "high",
  "-level:v",
  "4.1",
  "-g",
  "30",
];

function run(command, args) {
  const result = spawnSync(command, args, {
    encoding: "utf8",
    stdio: ["ignore", "pipe", "pipe"],
  });
  if (result.error !== undefined) throw result.error;
  if (result.status !== 0) {
    throw new Error(`${command} failed (${String(result.status)}): ${result.stderr}`);
  }
  return result.stdout;
}

function ffmpeg(args) {
  run("ffmpeg", ["-hide_banner", "-loglevel", "error", "-y", ...args]);
}

function probe(file) {
  return JSON.parse(
    run("ffprobe", [
      "-v",
      "error",
      "-show_entries",
      "format=format_name,duration:stream=codec_type,codec_name",
      "-of",
      "json",
      file,
    ]),
  );
}

function assertFixture(file, expected) {
  const metadata = probe(file);
  const video = metadata.streams.find((stream) => stream.codec_type === "video");
  const audio = metadata.streams.find((stream) => stream.codec_type === "audio");
  if (video?.codec_name !== expected.video || audio?.codec_name !== expected.audio) {
    throw new Error(
      `${path.basename(file)} metadata mismatch: ${JSON.stringify({ video, audio })}`,
    );
  }
}

export function generateMediaFixtures(mediaDir) {
  mkdirSync(mediaDir, { recursive: true });
  const file = (name) => path.join(mediaDir, name);

  ffmpeg([
    ...commonInput,
    ...h264,
    "-c:a",
    "aac",
    "-b:a",
    "96k",
    "-ar",
    "48000",
    "-shortest",
    "-movflags",
    "+faststart",
    file("direct.mp4"),
  ]);
  ffmpeg(["-i", file("direct.mp4"), "-c", "copy", file("container-only.mkv")]);
  ffmpeg(["-i", file("direct.mp4"), "-c", "copy", file("container-only-mov.mov")]);
  ffmpeg([
    "-f",
    "lavfi",
    "-i",
    "testsrc2=size=320x180:rate=15:duration=30",
    "-f",
    "lavfi",
    "-i",
    "sine=frequency=550:sample_rate=48000:duration=30",
    ...h264,
    "-c:a",
    "aac",
    "-b:a",
    "96k",
    "-shortest",
    "-movflags",
    "+faststart",
    file("direct-fallback.mp4"),
  ]);
  ffmpeg([
    ...commonInput,
    "-c:v",
    "mpeg4",
    "-q:v",
    "5",
    "-c:a",
    "aac",
    "-b:a",
    "96k",
    "-shortest",
    file("video-only.mp4"),
  ]);
  ffmpeg([
    ...commonInput,
    ...h264,
    "-c:a",
    "flac",
    "-strict",
    "-2",
    "-shortest",
    file("audio-only.mp4"),
  ]);
  ffmpeg([
    ...commonInput,
    "-c:v",
    "mpeg4",
    "-q:v",
    "5",
    "-c:a",
    "pcm_s16le",
    "-shortest",
    file("video-audio.avi"),
  ]);
  ffmpeg([
    "-f",
    "lavfi",
    "-i",
    "testsrc2=size=320x180:rate=15:duration=6",
    "-c:v",
    "mpeg4",
    "-q:v",
    "5",
    "-an",
    file("silent.mkv"),
  ]);
  ffmpeg([
    "-f",
    "lavfi",
    "-i",
    "testsrc2=size=180x320:rate=15:duration=6",
    ...h264,
    "-an",
    file("portrait.mp4"),
  ]);
  ffmpeg([
    "-f",
    "lavfi",
    "-i",
    "color=c=red:size=320x180:rate=15:duration=10",
    "-f",
    "lavfi",
    "-i",
    "color=c=green:size=320x180:rate=15:duration=10",
    "-f",
    "lavfi",
    "-i",
    "color=c=blue:size=320x180:rate=15:duration=10",
    "-f",
    "lavfi",
    "-i",
    "sine=frequency=440:sample_rate=48000:duration=30",
    "-filter_complex",
    "[0:v][1:v][2:v]concat=n=3:v=1:a=0[v]",
    "-map",
    "[v]",
    "-map",
    "3:a",
    ...h264,
    "-g",
    "150",
    "-keyint_min",
    "150",
    "-sc_threshold",
    "0",
    "-c:a",
    "aac",
    "-b:a",
    "96k",
    "-shortest",
    file("long-gop.mkv"),
  ]);
  // Keyframes at 0, 8 and 16 seconds divide red, green and blue scenes, revealing which copied keyframe a seek actually starts from.
  ffmpeg([
    "-f",
    "lavfi",
    "-i",
    "color=c=red:size=320x180:rate=15:duration=8",
    "-f",
    "lavfi",
    "-i",
    "color=c=green:size=320x180:rate=15:duration=8",
    "-f",
    "lavfi",
    "-i",
    "color=c=blue:size=320x180:rate=15:duration=8",
    "-f",
    "lavfi",
    "-i",
    "sine=frequency=330:sample_rate=48000:duration=24",
    "-filter_complex",
    "[0:v][1:v][2:v]concat=n=3:v=1:a=0[v]",
    "-map",
    "[v]",
    "-map",
    "3:a",
    ...h264,
    "-g",
    "120",
    "-keyint_min",
    "120",
    "-sc_threshold",
    "0",
    "-c:a",
    "aac",
    "-b:a",
    "96k",
    "-shortest",
    file("sparse-keyframes.mkv"),
  ]);

  // Use directly playable 1080p media so remembered 480p quality forces transcoding.
  ffmpeg([
    "-f",
    "lavfi",
    "-i",
    "testsrc2=size=1920x1080:rate=15:duration=20",
    "-f",
    "lavfi",
    "-i",
    "sine=frequency=770:sample_rate=48000:duration=20",
    ...h264,
    "-c:a",
    "aac",
    "-b:a",
    "96k",
    "-shortest",
    "-movflags",
    "+faststart",
    file("hd-1080p.mp4"),
  ]);

  const expected = {
    "direct.mp4": { video: "h264", audio: "aac" },
    "direct-fallback.mp4": { video: "h264", audio: "aac" },
    "container-only.mkv": { video: "h264", audio: "aac" },
    "container-only-mov.mov": { video: "h264", audio: "aac" },
    "video-only.mp4": { video: "mpeg4", audio: "aac" },
    "audio-only.mp4": { video: "h264", audio: "flac" },
    "video-audio.avi": { video: "mpeg4", audio: "pcm_s16le" },
    "silent.mkv": { video: "mpeg4", audio: undefined },
    "portrait.mp4": { video: "h264", audio: undefined },
    "long-gop.mkv": { video: "h264", audio: "aac" },
    "sparse-keyframes.mkv": { video: "h264", audio: "aac" },
    "hd-1080p.mp4": { video: "h264", audio: "aac" },
  };
  for (const [name, codecs] of Object.entries(expected)) {
    assertFixture(file(name), codecs);
  }

  // Provide unlabeled and ja sidecar subtitles, with one cue from 0.5 seconds to the end.
  writeFileSync(
    file("direct.srt"),
    "1\r\n00:00:00,500 --> 00:00:06,000\r\nDefault subtitle cue\r\n",
  );
  writeFileSync(
    file("direct.ja.srt"),
    "1\n00:00:00,500 --> 00:00:06,000\n日本語の字幕\n",
  );
  // For a transcode starting at 8 seconds, suppress the 6-7 second cue and show the 9-10 second cue at its original timestamp.
  writeFileSync(
    file("sparse-keyframes.srt"),
    [
      "1\n00:00:06,000 --> 00:00:07,000\nBefore keyframe cue\n",
      "2\n00:00:09,000 --> 00:00:10,000\nAfter keyframe cue\n",
      "3\n00:00:14,000 --> 00:00:15,000\nSeek target cue\n",
    ].join("\n"),
  );
}

/** Use distinct short H.264 clips; only explicit copies share content identity. */
export function generateFolderFixtures(root) {
  let count = 0;
  const make = (relative) => {
    const file = path.join(root, relative);
    mkdirSync(path.dirname(file), { recursive: true });
    count += 1;
    ffmpeg([
      "-f",
      "lavfi",
      "-i",
      "testsrc2=size=320x180:rate=10:duration=2",
      "-vf",
      `hue=h=${String((count * 37) % 360)}`,
      ...h264,
      "-an",
      "-metadata",
      `title=folder-fixture-${String(count)}`,
      file,
    ]);
  };
  const copy = (from, to) => {
    mkdirSync(path.dirname(path.join(root, to)), { recursive: true });
    copyFileSync(path.join(root, from), path.join(root, to));
  };

  make("a/movies/A/x.mp4");
  make("a/movies/A/B/y.mp4");
  make("a/movies/A/B/C/z.mp4");
  make("a/movies/1/w1.mp4");
  make("a/movies/2/w2.mp4");
  make("a/movies/10/w10.mp4");
  for (let index = 1; index <= 5; index += 1)
    make(`a/movies/five/part-${String(index)}.mp4`);
  make("a/movies/only-deeper/inner/d.mp4");
  make("a/movies/100% #1 日本語/s.mp4");
  make("a/movies/dup/same-a.mp4");
  copy("a/movies/dup/same-a.mp4", "a/movies/dup/same-b.mp4");
  copy("a/movies/A/x.mp4", "b/movies/copy-of-x.mp4");
  return count;
}

/** Short silent clips suffice for tag operations; filler videos place the final grid row near the viewport bottom at 1280 x 800. */
export function generateTagsFixtures(root) {
  mkdirSync(root, { recursive: true });
  const file = (name) => path.join(root, name);
  const clip = (name, hue) =>
    ffmpeg([
      "-f",
      "lavfi",
      "-i",
      "testsrc2=size=320x180:rate=10:duration=2",
      "-vf",
      `hue=h=${String(hue)}`,
      ...h264,
      "-an",
      file(name),
    ]);

  clip("タグ動画A.mp4", 20);
  clip("タグ動画B.mp4", 200);
  clip("タグ動画C.mp4", 100);
  clip("タグ動画D.mp4", 320);
  for (let index = 5; index <= TAGS_FIXTURE_VIDEO_COUNT; index += 1) {
    clip(`タグ埋め${String(index).padStart(2, "0")}.mp4`, (index * 37) % 360);
  }
}

/** Distinct short silent clips suffice for work-info tests. */
export const WORK_INFO_VIDEOS = ["作品動画A", "作品動画B", "作品動画C", "作品動画D"];

export function generateWorkInfoFixtures(root) {
  mkdirSync(root, { recursive: true });
  WORK_INFO_VIDEOS.forEach((title, index) =>
    ffmpeg([
      "-f",
      "lavfi",
      "-i",
      "testsrc2=size=320x180:rate=10:duration=2",
      "-vf",
      `hue=h=${String(40 + index * 80)}`,
      ...h264,
      "-an",
      path.join(root, `${title}.mp4`),
    ]),
  );
}

/** Keep the fixture count aligned with videoCount in tags.e2e.ts. */
const TAGS_FIXTURE_VIDEO_COUNT = 16;

/** Append a distinct MP4 free box to change the sampled content key without affecting playback or probing; see internal/scanner/content_key.go. */
function appendFreeBox(file, label) {
  const payload = Buffer.from(`vv-search-fixture:${label}`, "utf8");
  const header = Buffer.alloc(8);
  header.writeUInt32BE(payload.length + 8, 0);
  header.write("free", 4, "ascii");
  appendFileSync(file, Buffer.concat([header, payload]));
}

/** Ensure the library spans at least two pages. */
export const SEARCH_FILLERS = 62;

/** Reset copied fixture mtimes relative to the current run so cached media does not age out of date filters. */
export function prepareSearchFixtures(root) {
  const file = (name) => path.join(root, name);
  const now = Date.now() / 1000;
  const day = 24 * 60 * 60;
  utimesSync(file("京都旅行 2024.mp4"), now - 3 * day, now - 3 * day);
  utimesSync(file("京都旅行 2023.mp4"), now - 2 * day, now - 2 * day);
  utimesSync(file("2024 奈良.mp4"), now - day, now - day);
}

export function generateSearchFixtures(root) {
  mkdirSync(root, { recursive: true });
  const file = (name) => path.join(root, name);
  const clip = (name, seconds, hue) =>
    ffmpeg([
      "-f",
      "lavfi",
      "-i",
      `testsrc2=size=320x180:rate=10:duration=${String(seconds)}`,
      "-f",
      "lavfi",
      "-i",
      `sine=frequency=${String(300 + hue)}:sample_rate=48000:duration=${String(seconds)}`,
      "-vf",
      `hue=h=${String(hue)}`,
      ...h264,
      "-c:a",
      "aac",
      "-b:a",
      "64k",
      "-shortest",
      "-movflags",
      "+faststart",
      file(name),
    ]);

  clip("京都旅行 2024.mp4", 3, 40);
  clip("京都旅行 2023.mp4", 5, 80);
  clip("2024 奈良.mp4", 4, 120);
  clip("base.tmp.mp4", 2, 160);

  const copies = [
    "京都 嵐山.mp4",
    "京都 伏見.mp4",
    "2話.mp4",
    "10話.mp4",
    ...Array.from(
      { length: SEARCH_FILLERS },
      (_, index) => `clip ${String(index + 1).padStart(2, "0")}.mp4`,
    ),
  ];
  for (const name of copies) {
    copyFileSync(file("base.tmp.mp4"), file(name));
    appendFreeBox(file(name), name);
  }
  rmSync(file("base.tmp.mp4"));
  writeFileSync(file("長さ不明.mp4"), "この中身は動画ではない\n".repeat(64));
}

/** Use distinct 40-second clips so short playback remains outside the 15-second completion tail. */
export const HISTORY_VIDEOS = ["履歴の確認A", "履歴の確認B"];

export function generateHistoryFixtures(root) {
  mkdirSync(root, { recursive: true });
  for (const [index, name] of HISTORY_VIDEOS.entries()) {
    ffmpeg([
      "-f",
      "lavfi",
      "-i",
      "testsrc2=size=320x180:rate=15:duration=40",
      "-vf",
      `hue=h=${String(70 + index * 150)}`,
      ...h264,
      "-an",
      "-movflags",
      "+faststart",
      path.join(root, `${name}.mp4`),
    ]);
  }
}

/** Distinct title prefixes isolate reorder, grouping and touch scenarios. Place files directly under the registered folder to avoid automatic groups and vary content to prevent deduplication. */
export const DRAG_ORGANIZE_VIDEOS = [
  "並べ確認の1",
  "並べ確認の2",
  "並べ確認の3",
  "作る確認のA",
  "作る確認のB",
  "タッチ確認の1",
  "タッチ確認の先1",
];

export function generateDragOrganizeFixtures(root) {
  mkdirSync(root, { recursive: true });
  for (const [index, name] of DRAG_ORGANIZE_VIDEOS.entries()) {
    ffmpeg([
      "-f",
      "lavfi",
      "-i",
      "testsrc2=size=320x180:rate=10:duration=2",
      "-vf",
      `hue=h=${String(index * 17)}`,
      ...h264,
      "-an",
      "-movflags",
      "+faststart",
      path.join(root, `${name}.mp4`),
    ]);
  }
}
/**
 * ONBOARDING_VIDEOS は、オンボードの流れ（specs/057-home、子 #1323）で動画のないライブラリに
 * 足すフォルダの動画である（web/e2e/onboarding.e2e.ts）。
 */
export const ONBOARDING_VIDEOS = ["オンボード確認A", "オンボード確認B"];

/**
 * generateOnboardingFixtures は、オンボードで足すフォルダの動画を2本作る。中身で同じ動画と
 * みなされないよう、ほかの組と違う長さにして色合いを変える。
 */
export function generateOnboardingFixtures(root) {
  mkdirSync(root, { recursive: true });
  for (const [index, name] of ONBOARDING_VIDEOS.entries()) {
    ffmpeg([
      "-f",
      "lavfi",
      "-i",
      "testsrc2=size=320x180:rate=8:duration=7",
      "-vf",
      `hue=h=${String(25 + index * 170)}`,
      ...h264,
      "-an",
      "-movflags",
      "+faststart",
      path.join(root, `${name}.mp4`),
    ]);
  }
}

/** HOME_FILLERS is the number of public, unwatched videos in the Home fixture. */
export const HOME_FILLERS = 2;
/** HOME_IN_PROGRESS は、途中まで見て「Continue watching」に出す動画である。 */
export const HOME_IN_PROGRESS = "ホーム確認の途中";

/**
 * generateHomeFixtures は、ホーム（docs/design-docs/home.md）を確かめる動画を作る
 * （web/e2e/home.e2e.ts）。どれもフォルダの直下に置き、グループにしない。
 *
 * - 「ホーム確認 NN」は公開・未視聴の確認用の 2 本。4 秒の同じ映像に中身の違う free ボックスを足す
 * - 「ホーム確認の途中」は途中まで見たことにする 40 秒の 1 本。末尾とみなす 15 秒
 *   （internal/domain/progress.go の CompletionTailMs）より十分長い
 */
export function generateHomeFixtures(root) {
  mkdirSync(root, { recursive: true });
  const base = path.join(root, "base.tmp.mp4");
  ffmpeg([
    "-f",
    "lavfi",
    "-i",
    "testsrc2=size=320x180:rate=9:duration=4",
    "-vf",
    "hue=h=300",
    ...h264,
    "-an",
    "-movflags",
    "+faststart",
    base,
  ]);
  for (let index = 1; index <= HOME_FILLERS; index += 1) {
    const file = path.join(root, `ホーム確認 ${String(index).padStart(2, "0")}.mp4`);
    copyFileSync(base, file);
    appendFreeBox(file, `home:${String(index)}`);
  }
  rmSync(base);
  ffmpeg([
    "-f",
    "lavfi",
    "-i",
    "testsrc2=size=320x180:rate=9:duration=40",
    "-vf",
    "hue=h=200",
    ...h264,
    "-an",
    "-movflags",
    "+faststart",
    path.join(root, `${HOME_IN_PROGRESS}.mp4`),
  ]);
}
