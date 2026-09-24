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
  };
  for (const [name, codecs] of Object.entries(expected)) {
    assertFixture(file(name), codecs);
  }
}

/**
 * generateFolderFixtures はフォルダ画面の検証用のフォルダ構成を作る
 * （specs/011-folder-browser/quickstart.md）。動画はどれも内容を変えた数秒の
 * H.264 で、同じ内容の複製だけが同じ動画になる。
 */
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

/**
 * appendFreeBox は mp4 の末尾に中身の違う free ボックスを足す。再生にも解析にも
 * 影響しないまま末尾の内容が変わるので、同じ映像から別の動画を安く作れる
 * （内容の識別子は先頭と末尾から作る。internal/scanner/content_key.go）。
 */
function appendFreeBox(file, label) {
  const payload = Buffer.from(`vv-search-fixture:${label}`, "utf8");
  const header = Buffer.alloc(8);
  header.writeUInt32BE(payload.length + 8, 0);
  header.write("free", 4, "ascii");
  appendFileSync(file, Buffer.concat([header, payload]));
}

/** SEARCH_FILLERS は一覧を2ページ以上にするための動画の数である。 */
export const SEARCH_FILLERS = 62;

/**
 * generateSearchFixtures は一覧の検索・絞り込み・並べ替えの検証用の動画を作る
 * （specs/013-library-search、親 Issue #195 の受け入れ条件 1〜4・11〜15・22）。
 *
 * - 「京都旅行 2024」「京都旅行 2023」「2024 奈良」は長さ・大きさ・更新日時が
 *   互いに違う（並べ替えの向きで並びが逆になることを確かめる）
 * - 「京都 嵐山」「京都 伏見」は再生位置を残さない（未視聴のまま）京都の動画
 * - 「2話」「10話」は題名の自然順を確かめる
 * - 「長さ不明」は動画として読めないファイルで、長さが入らない
 * - 「clip NN」は 1 ページ（60 件）を越えるための動画
 */
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

  const now = Date.now() / 1000;
  const day = 24 * 60 * 60;
  utimesSync(file("京都旅行 2024.mp4"), now - 3 * day, now - 3 * day);
  utimesSync(file("京都旅行 2023.mp4"), now - 2 * day, now - 2 * day);
  utimesSync(file("2024 奈良.mp4"), now - day, now - day);

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
