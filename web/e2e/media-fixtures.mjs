import { copyFileSync, mkdirSync } from "node:fs";
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
