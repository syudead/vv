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
import { fileURLToPath } from "node:url";

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
  // コピーで途中から始める変換の確かめ（specs/018-live-transcode-seek、#380）。
  // キーフレームは 0・8・16 秒だけで、その区間ごとに赤・緑・青にする。途中へシークすると
  // 変換は直前のキーフレームから映像をコピーして始まり、映っている色で場面がわかる。
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

  // 覚えた画質で直接再生できる動画を変換で始めることの確かめ（specs/027-playback-quality）。
  // 画質の選択肢は短辺で決まるので、480p を選べる 1080p の直接再生できる動画にする。
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

  // 隣に置いた字幕（specs/028-sidecar-subtitles）。ラベルの無い字幕と `ja` の字幕で、
  // どちらも 0.5 秒から動画の終わりまで 1 つの cue を出す。
  writeFileSync(
    file("direct.srt"),
    "1\r\n00:00:00,500 --> 00:00:06,000\r\nDefault subtitle cue\r\n",
  );
  writeFileSync(
    file("direct.ja.srt"),
    "1\n00:00:00,500 --> 00:00:06,000\n日本語の字幕\n",
  );
  // キーフレームが 0・8・16 秒の動画の隣の字幕。ライブ変換が 8 秒から始まったとき、
  // 6〜7 秒の cue は出ず、9〜10 秒の cue が表示の 9〜10 秒台に出るかを見る（受け入れ条件 9）。
  writeFileSync(
    file("sparse-keyframes.srt"),
    [
      "1\n00:00:06,000 --> 00:00:07,000\nBefore keyframe cue\n",
      "2\n00:00:09,000 --> 00:00:10,000\nAfter keyframe cue\n",
      "3\n00:00:14,000 --> 00:00:15,000\nSeek target cue\n",
    ].join("\n"),
  );
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
 * generateScanProgressFixtures は、取り込みの進み具合（specs/024-import-progress）を実際の
 * サーバーで確かめるための、10 本の短い動画を作る（web/e2e/scan-progress.e2e.ts）。
 * 準備（解析・サムネイル・シーク用サムネイル・プレビュー）が一瞬で終わらないよう、
 * 数秒の長さにする。
 */
export const SCAN_PROGRESS_VIDEOS = 10;

export function generateScanProgressFixtures(root) {
  mkdirSync(root, { recursive: true });
  for (let index = 1; index <= SCAN_PROGRESS_VIDEOS; index += 1) {
    const name = `scan-progress-${String(index).padStart(2, "0")}`;
    ffmpeg([
      "-f",
      "lavfi",
      "-i",
      "testsrc2=size=320x180:rate=15:duration=4",
      "-vf",
      `hue=h=${String((index * 29) % 360)}`,
      ...h264,
      "-an",
      "-metadata",
      `title=${name}`,
      path.join(root, `${name}.mp4`),
    ]);
  }
  return SCAN_PROGRESS_VIDEOS;
}

/**
 * generateScanIssueFixtures は、取り込みの問題の一覧（specs/024-import-progress）を実際の
 * サーバーで確かめるための2つのファイルを作る（web/e2e/scan-issues.e2e.ts）。
 *
 * - 読めないファイル: 動画の拡張子を持つ名前付きパイプ。通常のファイルでないので走査は
 *   読めなかったとして記録する。権限を外す方法は root で動く環境では読めてしまうため使わない。
 * - 解析できない動画: 動画の拡張子を持つ、動画でない中身のファイル。登録はされるが、解析の
 *   仕事がやり直しの上限まで失敗する。
 */
export const SCAN_ISSUE_UNREADABLE = "scan-issue-unreadable.mp4";
export const SCAN_ISSUE_BROKEN = "scan-issue-broken.mp4";

export function generateScanIssueFixtures(root) {
  mkdirSync(root, { recursive: true });
  run("mkfifo", [path.join(root, SCAN_ISSUE_UNREADABLE)]);
  writeFileSync(
    path.join(root, SCAN_ISSUE_BROKEN),
    "this is not a video; the probe cannot read any stream from it\n".repeat(64),
  );
}

/**
 * generateFolderSearchFixtures はフォルダ画面・最上位の検索と絞り込みの検証用の
 * フォルダ構成を作る。
 * `root/A/x 京都.mp4`・`root/A/B/y 京都.mp4`・`root/C/z 京都.mp4` に対応する。
 *
 * - `A/x 京都.mp4`・`A/B/y 京都.mp4` は「京都」を含み、A の配下にある
 *   （A の中を「京都」で検索すると当たる）。
 * - `C/z 京都.mp4` も「京都」を含むが、A の配下ではなく別の登録フォルダ直下の
 *   きょうだいフォルダにある（A の中の検索では当たらず、最上位の検索でだけ当たる）。
 * - `A/大阪.mp4` は「京都」を含まない、A 直下のもう1本の動画（絞り込みだけの
 *   ときの件数の変化を確かめる）。
 */
/**
 * generateTagsFixtures は再生画面のタグ（issue 268）、ライブラリのカードの
 * タグ・タグでの絞り込み（issue 269）、選択バーの一括操作・すべて選択
 * （issue 270、受け入れ条件3・4は3本以上の選択を要る）を確かめるための、
 * 短くて軽い動画を作る（web/e2e/tags.e2e.ts）。タグの付け外し自体は
 * 中身を見ないので、短い無音のクリップで足りる。3本目（タグ動画C）は、検索欄が
 * タグ名と題名を別々に扱うことを確かめるための、タグを持たない対照区である
 * （issue 269 の受け入れ条件7）。4本目（タグ動画D）は、選択バーの一括操作が
 * 3本の選択を要る（issue 270）ために足す、C とは別の動画である。
 *
 * 残りの「タグ埋め」の動画は、1280×800 でライブラリの格子の最後の行が画面の
 * 下端に来るよう本数を足すためだけのもので、どの試験も名指しでは使わない
 * （specs/041-tag-overflow-list、下端の近くのカードの「+N」の一覧を確かめる）。
 * 全部の本数は TAGS_FIXTURE_VIDEO_COUNT である。
 */
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

/**
 * WORK_INFO_VIDEOS は generateWorkInfoFixtures が作る動画の題名で、作品情報の流れ
 * （specs/047-video-work-info、子 #1030）を web/e2e/work-info.e2e.ts で確かめる。
 * 作品情報は中身を見ないので、色合いだけを変えた短い無音のクリップで足りる。
 */
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

/**
 * WORK_INFO_PLUGIN_VIDEO は generateWorkInfoPluginFixtures が作る動画の題名で、作品情報プラグインの
 * 取得と反映（specs/049-work-info-plugins、子 #1084）を web/e2e/work-info-plugins.e2e.ts で確かめる。
 */
export const WORK_INFO_PLUGIN_VIDEO = "プラグイン動画";

export function generateWorkInfoPluginFixtures(root) {
  mkdirSync(root, { recursive: true });
  ffmpeg([
    "-f",
    "lavfi",
    "-i",
    "testsrc2=size=320x180:rate=10:duration=2",
    ...h264,
    "-an",
    path.join(root, `${WORK_INFO_PLUGIN_VIDEO}.mp4`),
  ]);
}

/**
 * MEDIA_INFO_VIDEO は generateMediaInfoFixtures が作る動画の題名で、動画ページの追加の情報の
 * まとまり（specs/056-media-info、子 #1252）を web/e2e/media-info.e2e.ts で確かめる。
 * 言語の印の違う音声 2 本と、題名・作者・ジャンルの埋め込みのタグを持つ。
 */
export const MEDIA_INFO_VIDEO = "追加情報動画";

export function generateMediaInfoFixtures(root) {
  mkdirSync(root, { recursive: true });
  ffmpeg([
    "-f",
    "lavfi",
    "-i",
    "testsrc2=size=320x180:rate=10:duration=2",
    "-f",
    "lavfi",
    "-i",
    "sine=frequency=440:sample_rate=48000:duration=2",
    "-f",
    "lavfi",
    "-i",
    "sine=frequency=880:sample_rate=48000:duration=2",
    "-map",
    "0:v",
    "-map",
    "1:a",
    "-map",
    "2:a",
    ...h264,
    "-c:a",
    "aac",
    "-ac",
    "2",
    "-b:a",
    "64k",
    "-metadata:s:a:0",
    "language=jpn",
    "-metadata:s:a:1",
    "language=eng",
    "-metadata:s:a:1",
    "title=Commentary",
    "-disposition:a:0",
    "default",
    "-disposition:a:1",
    "0",
    "-metadata",
    "title=埋め込みの題名",
    "-metadata",
    "artist=E2E Author",
    "-metadata",
    "genre=Documentary",
    "-shortest",
    path.join(root, `${MEDIA_INFO_VIDEO}.mkv`),
  ]);
}

/**
 * CHAPTERS_VIDEO は generateChapterFixtures が作る動画の題名で、プレーヤーのチャプター
 * （specs/056-media-info、子 #1253）を web/e2e/media-info.e2e.ts で確かめる。6 秒の動画に
 * チャプターが 3 つあり、2 つ目はタイトルが無く、3 つ目は動画の終わりを過ぎて終わる
 * （quickstart.md の chapters.mkv）。隣に字幕を 1 つ置く。
 */
export const CHAPTERS_VIDEO = "チャプター動画";

export function generateChapterFixtures(root) {
  mkdirSync(root, { recursive: true });
  // チャプターの定義は取り込みの対象の外（media の root の隣）に置く。
  const metadata = `${root}.chapters.ffmetadata`;
  writeFileSync(
    metadata,
    [
      ";FFMETADATA1",
      "[CHAPTER]",
      "TIMEBASE=1/1000",
      "START=0",
      "END=2000",
      "title=Opening",
      "[CHAPTER]",
      "TIMEBASE=1/1000",
      "START=2000",
      "END=4000",
      "[CHAPTER]",
      "TIMEBASE=1/1000",
      "START=4000",
      "END=9000",
      "title=Ending",
      "",
    ].join("\n"),
  );
  ffmpeg([
    "-f",
    "lavfi",
    "-i",
    "testsrc2=size=320x180:rate=10:duration=6",
    "-f",
    "lavfi",
    "-i",
    "sine=frequency=440:sample_rate=48000:duration=6",
    "-i",
    metadata,
    "-map",
    "0:v",
    "-map",
    "1:a",
    "-map_metadata",
    "2",
    "-map_chapters",
    "2",
    ...h264,
    "-c:a",
    "aac",
    "-b:a",
    "64k",
    "-shortest",
    path.join(root, `${CHAPTERS_VIDEO}.mkv`),
  ]);
  // 隣に置いた字幕。360px の操作バーにチャプター・変換の表示・字幕が並んでも収まるかを見る
  // （ui-design.md「Responsive behaviour」）。
  writeFileSync(
    path.join(root, `${CHAPTERS_VIDEO}.srt`),
    "1\n00:00:00,500 --> 00:00:06,000\nChapter subtitle cue\n",
  );
}

/**
 * generateTagsFixtures が作る動画の本数（タグ動画A〜D と、タグ埋めの動画）。
 * web/e2e/tags.e2e.ts の videoCount と同じ数にする。
 */
const TAGS_FIXTURE_VIDEO_COUNT = 16;

export function generateFolderSearchFixtures(root) {
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
      `hue=h=${String((count * 53) % 360)}`,
      ...h264,
      "-an",
      "-metadata",
      `title=folder-search-fixture-${String(count)}`,
      file,
    ]);
  };

  make("movies/A/x 京都.mp4");
  make("movies/A/B/y 京都.mp4");
  make("movies/A/大阪.mp4");
  make("movies/C/z 京都.mp4");
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

/**
 * generateGuestFixtures は、ゲストの画面（specs/016-single-account-auth、子 #304）を
 * 確かめるための動画を6本と、画面写真の本数を満たす非公開の6本を作る
 * （web/e2e/guest.e2e.ts）。「公開あり」の5本のうち
 * 4本（A・B・E・F）を所有者が公開にし、C と「非公開だけ」の D は非公開のままにする。
 * 再生の途中で操作できるよう 12 秒にし、中身で同じ動画とみなされないよう色合いを変える。
 */
export function generateGuestFixtures(root) {
  const shown = path.join(root, "公開あり");
  const hidden = path.join(root, "非公開だけ");
  mkdirSync(shown, { recursive: true });
  mkdirSync(hidden, { recursive: true });
  const clip = (file, hue) =>
    ffmpeg([
      "-f",
      "lavfi",
      "-i",
      "testsrc2=size=320x180:rate=15:duration=12",
      "-vf",
      `hue=h=${String(hue)}`,
      ...h264,
      "-an",
      file,
    ]);

  clip(path.join(shown, "ゲスト公開A.mp4"), 40);
  clip(path.join(shown, "ゲスト公開B.mp4"), 100);
  clip(path.join(shown, "ゲスト公開E.mp4"), 160);
  clip(path.join(shown, "ゲスト公開F.mp4"), 220);
  clip(path.join(shown, "ゲスト非公開C.mp4"), 280);
  clip(path.join(hidden, "ゲスト非公開D.mp4"), 340);
  // 画面確認に使う、非公開のままの6本。題名に「ゲスト」を含めず、上の6本を題名で
  // 数える確かめに混ざらないようにする。
  for (const [index, name] of ["G", "H", "I", "J", "K", "L"].entries()) {
    clip(path.join(hidden, `確認用${name}.mp4`), 20 + index * 55);
  }
}

/**
 * generateHistoryFixtures は、視聴履歴の流れ（specs/043-watch-history、子 #864）を
 * 確かめるための動画を2本作る（web/e2e/history.e2e.ts）。数秒の再生と一時停止の後も
 * 見終わったことにならず進捗バーが残るよう、末尾とみなす 15 秒（internal/domain/progress.go
 * の CompletionTailMs）より十分長い 40 秒にし、中身で同じ動画とみなされないよう色合いを変える。
 */
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

/**
 * generateThumbnailEditFixtures は、ComfyUI でコマを編集してサムネイルにする流れ
 * （specs/045-comfyui-thumbnails、子 #960）を確かめる動画を1本作る（web/e2e/thumbnail-edit.e2e.ts）。
 * 偽の ComfyUI が返す一色の画像と見分けられるよう、色の混ざった testsrc2 のままにし、
 * ほかの動画と同じ中身とみなされないよう長さと色合いを変える。
 */
export const THUMBNAIL_EDIT_VIDEO = "サムネイル編集の確認";

export function generateThumbnailEditFixtures(root) {
  mkdirSync(root, { recursive: true });
  ffmpeg([
    "-f",
    "lavfi",
    "-i",
    "testsrc2=size=320x180:rate=15:duration=12",
    "-vf",
    "hue=h=200",
    ...h264,
    "-an",
    "-movflags",
    "+faststart",
    path.join(root, `${THUMBNAIL_EDIT_VIDEO}.mp4`),
  ]);
  ffmpeg([
    "-f",
    "lavfi",
    "-i",
    "testsrc=size=320x180:rate=15:duration=4",
    "-vf",
    "hue=h=320",
    ...h264,
    "-an",
    "-movflags",
    "+faststart",
    path.join(root, `${THUMBNAIL_EDIT_NEIGHBOR}.mp4`),
  ]);
}

/**
 * THUMBNAIL_EDIT_NEIGHBOR は generateThumbnailEditFixtures が同じフォルダに置く2本目の動画で、
 * 関連動画にサムネイル編集の動画を出す（specs/046-thumbnail-image-upload、子 #988）。
 * 同じ中身とみなされないよう、元の映像と長さを変える。
 */
export const THUMBNAIL_EDIT_NEIGHBOR = "サムネイル編集の隣の動画";

/**
 * THUMBNAIL_UPLOADS は、手元の画像をサムネイルにする流れ（specs/046-thumbnail-image-upload、
 * 子 #988）でページから上げる画像である。どれも一色で、色は互いにも偽の ComfyUI の出力の色
 * （fake-comfyui.mjs の OUTPUT_COLOR）とも離す。PNG は縦長にして、全体を収めると余白が付き、
 * 枠いっぱいに広げると動画の形に切り抜かれることを確かめる。
 */
export const THUMBNAIL_UPLOADS = {
  png: { file: "thumbnail-upload.png", color: [0, 200, 0], width: 300, height: 400 },
  jpeg: { file: "thumbnail-upload.jpg", color: [0, 90, 255], width: 640, height: 360 },
  webp: { file: "thumbnail-upload.webp", color: [255, 200, 0], width: 480, height: 270 },
};

/**
 * THUMBNAIL_UPLOAD_SPLIT は、上半分と下半分で色の違う縦長の PNG である。枠いっぱいに広げて
 * 動かした範囲だけがサムネイルになることを、どちらの色が出るかで確かめる
 * （docs/design-docs/thumbnail-editing.md「Uploaded images」）。
 */
export const THUMBNAIL_UPLOAD_SPLIT = {
  file: "thumbnail-upload-split.png",
  top: [220, 30, 30],
  bottom: [255, 255, 255],
  width: 300,
  height: 400,
};

/**
 * generateThumbnailUploadFixtures は THUMBNAIL_UPLOADS と THUMBNAIL_UPLOAD_SPLIT の画像を root に置く。PNG と JPEG は
 * ffmpeg で作り、WebP はチェックイン済みの e2e/fixtures/thumbnail-upload.webp を写す。
 * ffmpeg のビルドに libwebp が無いことがあるためである（specs/046-thumbnail-image-upload/plan.md）。
 */
export function generateThumbnailUploadFixtures(root) {
  mkdirSync(root, { recursive: true });
  const solid = ({ color, width, height }) =>
    `color=c=0x${color.map((value) => value.toString(16).padStart(2, "0")).join("")}:s=${String(width)}x${String(height)}`;
  for (const upload of [THUMBNAIL_UPLOADS.png, THUMBNAIL_UPLOADS.jpeg]) {
    ffmpeg([
      "-f",
      "lavfi",
      "-i",
      solid(upload),
      "-frames:v",
      "1",
      ...(upload === THUMBNAIL_UPLOADS.jpeg ? ["-q:v", "2"] : []),
      path.join(root, upload.file),
    ]);
  }
  copyFileSync(
    path.join(
      path.dirname(fileURLToPath(import.meta.url)),
      "fixtures",
      THUMBNAIL_UPLOADS.webp.file,
    ),
    path.join(root, THUMBNAIL_UPLOADS.webp.file),
  );
  const split = THUMBNAIL_UPLOAD_SPLIT;
  const half = { width: split.width, height: split.height / 2 };
  ffmpeg([
    "-f",
    "lavfi",
    "-i",
    solid({ ...half, color: split.top }),
    "-f",
    "lavfi",
    "-i",
    solid({ ...half, color: split.bottom }),
    "-filter_complex",
    "vstack",
    "-frames:v",
    "1",
    path.join(root, split.file),
  ]);
}

/**
 * generatePlaylistsFixtures は、プレイリストの流れ（specs/044-playlists、子 #903）を
 * 確かめるための動画を作る（web/e2e/playlists.e2e.ts）。登録フォルダの直下の 2 本は
 * グループにならない単体の動画で、「プレイリストのグループ」の 3 本はグループになる。
 * 連続再生で何本も最後まで見るので 3 秒と短くし、中身で同じ動画とみなされないよう
 * 色合いを変える。
 */
export const PLAYLIST_VIDEOS = ["プレイリスト動画A", "プレイリスト動画B"];
export const PLAYLIST_GROUP = "プレイリストのグループ";
export const PLAYLIST_GROUP_MEMBERS = [
  "プレイリストのメンバー1",
  "プレイリストのメンバー2",
  "プレイリストのメンバー3",
];

export function generatePlaylistsFixtures(root) {
  const group = path.join(root, PLAYLIST_GROUP);
  mkdirSync(group, { recursive: true });
  const clip = (file, hue) =>
    ffmpeg([
      "-f",
      "lavfi",
      "-i",
      "testsrc2=size=320x180:rate=15:duration=3",
      "-vf",
      `hue=h=${String(hue)}`,
      ...h264,
      "-an",
      "-movflags",
      "+faststart",
      file,
    ]);

  for (const [index, name] of PLAYLIST_VIDEOS.entries()) {
    clip(path.join(root, `${name}.mp4`), 30 + index * 70);
  }
  for (const [index, name] of PLAYLIST_GROUP_MEMBERS.entries()) {
    clip(path.join(group, `${name}.mp4`), 190 + index * 60);
  }
}

/**
 * generateGroupsFixtures は、任意のグループの流れ（specs/048-groups、子 #1059）を確かめる
 * 動画を作る（web/e2e/groups.e2e.ts）。「フォルダ1」〜「フォルダ3」は 1 本ずつなので
 * グループにならず、登録フォルダの直下の 2 本も単体の動画になる。「自動のグループ」と
 * 「解散するグループ」は直下に 2 本以上あるので自動グループになる。`spare` の 1 本は
 * 登録フォルダの外に置き、テストが再スキャンの前に「自動のグループ」へ写す。
 * 中身で同じ動画とみなされないよう、ほかの組と違う長さとコマの速さにし、色合いを変える。
 */
export const GROUPS_SPREAD = [
  ["フォルダ1", "グループ確認の動画1"],
  ["フォルダ2", "グループ確認の動画2"],
  ["フォルダ3", "グループ確認の動画3"],
];
export const GROUPS_SINGLES = ["グループ確認の単体A", "グループ確認の単体B"];
export const GROUPS_AUTO = "自動のグループ";
export const GROUPS_AUTO_MEMBERS = [
  "グループ確認の自動1",
  "グループ確認の自動2",
  "グループ確認の自動3",
];
export const GROUPS_DISSOLVED = "解散するグループ";
export const GROUPS_DISSOLVED_MEMBERS = ["グループ確認の解散1", "グループ確認の解散2"];
export const GROUPS_SPARE = "グループ確認の自動4";

export function generateGroupsFixtures(root, spare) {
  let hue = 0;
  const clip = (dir, name) => {
    mkdirSync(dir, { recursive: true });
    ffmpeg([
      "-f",
      "lavfi",
      "-i",
      "testsrc2=size=320x180:rate=12:duration=3",
      "-vf",
      `hue=h=${String(hue)}`,
      ...h264,
      "-an",
      "-movflags",
      "+faststart",
      path.join(dir, `${name}.mp4`),
    ]);
    hue += 29;
  };

  for (const [folder, name] of GROUPS_SPREAD) clip(path.join(root, folder), name);
  for (const name of GROUPS_SINGLES) clip(root, name);
  for (const name of GROUPS_AUTO_MEMBERS) clip(path.join(root, GROUPS_AUTO), name);
  for (const name of GROUPS_DISSOLVED_MEMBERS) {
    clip(path.join(root, GROUPS_DISSOLVED), name);
  }
  clip(spare, GROUPS_SPARE);
}

/**
 * DRAG_ORGANIZE_VIDEOS は、ドラッグでの整理（specs/050-drag-organize、子 #1109）を確かめる
 * 動画の題名である（web/e2e/drag-organize.e2e.ts）。どれも登録フォルダの直下に置くので、
 * 自動グループにはならない。テストの組ごとに題名の頭を変え、ライブラリの検索でその組だけを
 * 出せるようにする。中身で同じ動画とみなされないよう、ほかの組と違う長さとコマの速さにし、
 * 色合いを変える。
 */
export const DRAG_ORGANIZE_VIDEOS = [
  "並べ確認の1",
  "並べ確認の2",
  "並べ確認の3",
  "作る確認のA",
  "作る確認のB",
  "束ね確認のA",
  "束ね確認のB",
  "合流確認のX1",
  "合流確認のX2",
  "合流確認のY1",
  "入れ子確認のP1",
  "入れ子確認のQ1",
  "移動確認のA1",
  "移動確認のA2",
  "移動確認のB1",
  "選択確認の1",
  "選択確認の2",
  "選択確認の3",
  "選択確認の先1",
  "タッチ確認の1",
  "タッチ確認の先1",
  "取り出し確認のV",
  "取り出し確認のX1",
  "取り出し確認のY1",
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
 * SMART_FOLDER_SINGLES と SMART_FOLDER_GROUPS は、スマートフォルダ（specs/052-smart-folders、
 * 子 #1182）を確かめる動画の題名である（web/e2e/smart-folders.e2e.ts）。単体の 3 本は登録
 * フォルダの直下に置き、二つのフォルダの 2 本ずつは自動グループ（part1・part2）になる。
 * 20 秒にするのは、位置 0 の保存で未視聴に戻せるようにするためである。3 秒の動画は末尾とみなす
 * 15 秒（internal/domain/progress.go の CompletionTailMs）より短く、どの保存でも見終わったことに
 * なり、CI のリトライで未視聴に戻せない。中身で同じ動画とみなされないよう、ほかの組と違う長さと
 * コマの速さにし、色合いを変える。
 */
export const SMART_FOLDER_SINGLES = [
  "スマート確認の猫A",
  "スマート確認の猫B",
  "スマート確認の犬",
];
export const SMART_FOLDER_GROUPS = [
  ["両方に猫のグループ", ["スマート確認の両方1", "スマート確認の両方2"]],
  ["片方に猫のグループ", ["スマート確認の片方1", "スマート確認の片方2"]],
];

export function generateSmartFoldersFixtures(root) {
  let hue = 11;
  const clip = (dir, name) => {
    mkdirSync(dir, { recursive: true });
    ffmpeg([
      "-f",
      "lavfi",
      "-i",
      "testsrc2=size=320x180:rate=6:duration=20",
      "-vf",
      `hue=h=${String(hue)}`,
      ...h264,
      "-an",
      "-movflags",
      "+faststart",
      path.join(dir, `${name}.mp4`),
    ]);
    hue += 41;
  };
  for (const name of SMART_FOLDER_SINGLES) clip(root, name);
  for (const [folder, members] of SMART_FOLDER_GROUPS) {
    for (const name of members) clip(path.join(root, folder), name);
  }
}
