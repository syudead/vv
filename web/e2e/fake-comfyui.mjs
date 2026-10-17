// fake-comfyui.mjs は e2e が vv の接続先にする、自前の ComfyUI の偽物である
// （specs/045-comfyui-thumbnails、子 #960）。run-e2e.mjs が別のプロセスで起動し、本物の
// サーバーと internal/comfyui のアダプタがこれに要求を送る。
//
// 答えるのは contracts/workflow.md の自前の ComfyUI のルートだけである。
//
// - GET /system_stats、GET /object_info: ComfyUI であることと、vv が埋め込んだ 2 つの
//   ワークフローのノードの型とモデルファイルが揃っていること。ノードとファイルは
//   internal/comfyui/workflows/ のファイルから読むので、ワークフローが変わっても揃ったままである。
// - POST /upload/image、POST /prompt: 上げた画像の名前と送られたプロンプトを記録する。
// - GET /queue、GET /history/{id}: 実行は待ち行列で少し待ち、実行中を経て、決まった一色の
//   画像を出力して終わる。合成を有効にしたときは、ワークフローの合成（contracts/workflow.md）の
//   ように、上げたコマの大きさで、マスクの白い所だけその色にし、ほかはコマのままにする。
// - GET /view: 実行の出力の画像。
// - POST /interrupt、POST /queue（delete）: 実行を止める。
//
// WebSocket の /ws は持たない。アダプタは接続できなければ進み具合を出さないだけである。
//
// テストが読み書きする /__fake/* は ComfyUI のルートではない。
// - GET /__fake/prompts: 送られたプロンプトと、そのときまでに上げた画像の名前。
// - PUT /__fake/timing: 待ち行列と実行中の長さ（ミリ秒）を変える。
// - PUT /__fake/composite: マスクを通した合成の有無（{ enabled }）を変える。既定は無効で、
//   出力は塗った範囲によらず一色の画像である（specs/045 の流れのテストはそれを前提にする）。
//
// 標準入力が閉じたら（起動した run-e2e.mjs が終わったら）終わる。

import { randomUUID } from "node:crypto";
import { readFileSync } from "node:fs";
import { createServer } from "node:http";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { deflateSync, inflateSync } from "node:zlib";

const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..", "..");
const workflowDir = path.join(repoRoot, "internal", "comfyui", "workflows");
const workflowFiles = ["thumbnail-edit.json", "thumbnail-edit-reference.json"];

/** FAKE_COMFYUI_PORT は偽物が待ち受けるポートである。thumbnail-edit.e2e.ts もこの値を使う。 */
export const FAKE_COMFYUI_PORT = 18188;

/** OUTPUT_COLOR は出力の画像の色（RGB）である。テストはサムネイルがこの一色かを確かめる。 */
export const OUTPUT_COLOR = [255, 0, 255];

const OUTPUT_WIDTH = 320;
const OUTPUT_HEIGHT = 180;

// ローダーのノードの型と、モデルファイルの名前を受ける入力（internal/comfyui/workflows.go）。
const modelLoaders = {
  UNETLoader: "unet_name",
  CLIPLoader: "clip_name",
  VAELoader: "vae_name",
};

/** objectInfo はワークフローのすべてのノードの型と、ローダーの選択肢に揃ったファイルを返す。 */
function objectInfo() {
  const info = {};
  for (const file of workflowFiles) {
    const workflow = JSON.parse(readFileSync(path.join(workflowDir, file), "utf8"));
    for (const node of Object.values(workflow)) {
      info[node.class_type] ??= { input: { required: {} } };
      const input = modelLoaders[node.class_type];
      if (input === undefined) continue;
      const required = info[node.class_type].input.required;
      const options = required[input]?.[0] ?? [];
      if (!options.includes(node.inputs[input])) options.push(node.inputs[input]);
      required[input] = [options, {}];
    }
  }
  return info;
}

const crcTable = Array.from({ length: 256 }, (_, n) => {
  let c = n;
  for (let k = 0; k < 8; k += 1) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1;
  return c >>> 0;
});

function crc32(buffer) {
  let c = 0xffffffff;
  for (const byte of buffer) c = crcTable[(c ^ byte) & 0xff] ^ (c >>> 8);
  return (c ^ 0xffffffff) >>> 0;
}

function pngChunk(type, data) {
  const length = Buffer.alloc(4);
  length.writeUInt32BE(data.length);
  const body = Buffer.concat([Buffer.from(type, "ascii"), data]);
  const crc = Buffer.alloc(4);
  crc.writeUInt32BE(crc32(body));
  return Buffer.concat([length, body, crc]);
}

/** encodePNG は width×height の RGB の画素 pixels（1 画素 3 バイト）を PNG にする。 */
function encodePNG(width, height, pixels) {
  const header = Buffer.alloc(13);
  header.writeUInt32BE(width, 0);
  header.writeUInt32BE(height, 4);
  header[8] = 8; // ビット深度
  header[9] = 2; // RGB
  const stride = width * 3;
  const raw = Buffer.alloc((1 + stride) * height);
  for (let y = 0; y < height; y += 1) {
    pixels.copy(raw, y * (1 + stride) + 1, y * stride, (y + 1) * stride);
  }
  return Buffer.concat([
    Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]),
    pngChunk("IHDR", header),
    pngChunk("IDAT", deflateSync(raw)),
    pngChunk("IEND", Buffer.alloc(0)),
  ]);
}

/** solidPNG は width×height の一色 color（RGB）の PNG を作る。 */
function solidPNG(width, height, color) {
  const pixels = Buffer.alloc(width * height * 3);
  for (let i = 0; i < width * height; i += 1) pixels.set(color, i * 3);
  return encodePNG(width, height, pixels);
}

// PNG の色の型ごとの、1 画素の標本の数。
const pngChannels = { 0: 1, 2: 3, 3: 1, 4: 2, 6: 4 };

/** paeth は PNG のフィルタ 4 の予測である。 */
function paeth(a, b, c) {
  const p = a + b - c;
  const pa = Math.abs(p - a);
  const pb = Math.abs(p - b);
  const pc = Math.abs(p - c);
  if (pa <= pb && pa <= pc) return a;
  return pb <= pc ? b : c;
}

/** unfilter は PNG の行ごとのフィルタを外し、行を連ねたバイト列を返す。 */
function unfilter(data, height, stride, bpp) {
  const out = Buffer.alloc(height * stride);
  for (let y = 0; y < height; y += 1) {
    const filter = data[y * (stride + 1)];
    const row = y * (stride + 1) + 1;
    for (let x = 0; x < stride; x += 1) {
      const a = x >= bpp ? out[y * stride + x - bpp] : 0;
      const b = y > 0 ? out[(y - 1) * stride + x] : 0;
      const c = x >= bpp && y > 0 ? out[(y - 1) * stride + x - bpp] : 0;
      const predict = [0, a, b, (a + b) >> 1, paeth(a, b, c)][filter];
      if (predict === undefined) throw new Error(`unknown PNG filter ${String(filter)}`);
      out[y * stride + x] = (data[row + x] + predict) & 0xff;
    }
  }
  return out;
}

/**
 * decodePNG は vv が上げるコマとマスク（8 か 16 ビット、インターレース無し）を読み、
 * 幅・高さと RGB の画素（1 画素 3 バイト）を返す。
 */
function decodePNG(buffer) {
  let offset = 8;
  let header;
  let palette = Buffer.alloc(0);
  const data = [];
  while (offset < buffer.length) {
    const length = buffer.readUInt32BE(offset);
    const type = buffer.toString("ascii", offset + 4, offset + 8);
    const body = buffer.subarray(offset + 8, offset + 8 + length);
    if (type === "IHDR") header = body;
    if (type === "PLTE") palette = body;
    if (type === "IDAT") data.push(body);
    offset += 12 + length;
  }
  if (header === undefined) throw new Error("PNG without IHDR");
  const width = header.readUInt32BE(0);
  const height = header.readUInt32BE(4);
  const depth = header[8];
  const colorType = header[9];
  const channels = pngChannels[colorType];
  if (channels === undefined || (depth !== 8 && depth !== 16) || header[12] !== 0) {
    throw new Error(`unsupported PNG: depth ${String(depth)}, type ${String(colorType)}`);
  }
  const sample = depth / 8;
  const bpp = channels * sample;
  const raw = unfilter(inflateSync(Buffer.concat(data)), height, width * bpp, bpp);
  const pixels = Buffer.alloc(width * height * 3);
  for (let i = 0; i < width * height; i += 1) {
    const at = (channel) => raw[i * bpp + channel * sample];
    let rgb;
    if (colorType === 3) rgb = [...palette.subarray(at(0) * 3, at(0) * 3 + 3)];
    else if (channels <= 2) rgb = [at(0), at(0), at(0)];
    else rgb = [at(0), at(1), at(2)];
    pixels.set(rgb, i * 3);
  }
  return { width, height, pixels };
}

/**
 * compositePNG はコマ frame の大きさで、マスク mask の赤が半分より明るい所を OUTPUT_COLOR に、
 * ほかをコマのままにした PNG を返す。マスクの大きさが違えば、近い画素を読む。
 */
function compositePNG(frame, mask) {
  const source = decodePNG(frame);
  const painted = decodePNG(mask);
  const pixels = Buffer.from(source.pixels);
  for (let y = 0; y < source.height; y += 1) {
    const my = Math.floor((y * painted.height) / source.height);
    for (let x = 0; x < source.width; x += 1) {
      const mx = Math.floor((x * painted.width) / source.width);
      if (painted.pixels[(my * painted.width + mx) * 3] > 127) {
        pixels.set(OUTPUT_COLOR, (y * source.width + x) * 3);
      }
    }
  }
  return encodePNG(source.width, source.height, pixels);
}

const solidOutput = solidPNG(OUTPUT_WIDTH, OUTPUT_HEIGHT, OUTPUT_COLOR);

const timing = { queuedMs: 600, runningMs: 2500 };
let composite = false;
/** uploads は上げた画像の名前で、上げた順に並ぶ。 */
const uploads = [];
/** uploadedImages は上げた画像の名前ごとの中身である。 */
const uploadedImages = new Map();
/** outputs は出力の画像の名前ごとの中身である。 */
const outputs = new Map();
/** prompts は送られたプロンプトで、送られた順に並ぶ。 */
const prompts = [];
/** runs は実行の id ごとの、始めた時刻・出力のノードと画像の名前・止めたか。 */
const runs = new Map();
let number = 0;

/** phase は実行 run の今の段階（queued・running・done・stopped）を返す。 */
function phase(run) {
  if (run.stopped) return "stopped";
  const elapsed = Date.now() - run.submittedAt;
  if (elapsed < run.queuedMs) return "queued";
  if (elapsed < run.queuedMs + run.runningMs) return "running";
  return "done";
}

function queueItem(id, run) {
  return [run.number, id, {}, {}, [run.output]];
}

function historyEntry(run) {
  if (run.stopped) {
    return {
      outputs: {},
      status: {
        status_str: "error",
        completed: false,
        messages: [["execution_interrupted", { prompt_id: run.id }]],
      },
    };
  }
  return {
    outputs: {
      [run.output]: {
        images: [{ filename: run.outputName, subfolder: "", type: "output" }],
      },
    },
    status: { status_str: "success", completed: true, messages: [] },
  };
}

function send(response, status, body, type = "application/json") {
  const data =
    type === "application/json" ? Buffer.from(JSON.stringify(body)) : Buffer.from(body);
  response.writeHead(status, { "Content-Type": type, "Content-Length": data.length });
  response.end(data);
}

async function readBody(request) {
  const chunks = [];
  for await (const chunk of request) chunks.push(chunk);
  return Buffer.concat(chunks);
}

async function readJSON(request) {
  const body = await readBody(request);
  return body.length === 0 ? {} : JSON.parse(body.toString("utf8"));
}

/**
 * imagePart は multipart の本文 body から、name="image" の部分のファイル名と中身を返す。
 * 無ければ null を返す。
 */
function imagePart(contentType, body) {
  const boundary = /boundary=(?:"([^"]+)"|([^;]+))/.exec(contentType ?? "");
  if (boundary === null) return null;
  const delimiter = Buffer.from(`--${boundary[1] ?? boundary[2]}`);
  let start = body.indexOf(delimiter);
  while (start !== -1) {
    const next = body.indexOf(delimiter, start + delimiter.length);
    if (next === -1) return null;
    const part = body.subarray(start + delimiter.length, next);
    const headerEnd = part.indexOf("\r\n\r\n");
    const headers = part.subarray(0, Math.max(headerEnd, 0)).toString("latin1");
    const match = /name="image"; filename="([^"]+)"/.exec(headers);
    if (headerEnd !== -1 && match !== null) {
      return {
        name: Buffer.from(match[1], "latin1").toString("utf8"),
        // 部分の終わりの CRLF は区切りの一部である。
        content: Buffer.from(part.subarray(headerEnd + 4, part.length - 2)),
      };
    }
    start = next;
  }
  return null;
}

/** inputImage はプロンプトの、_meta.title が title のノードが読む、上げた画像を返す。 */
function inputImage(prompt, title) {
  const node = Object.values(prompt ?? {}).find(
    (candidate) => candidate?._meta?.title === title,
  );
  const name = node?.inputs?.image;
  return typeof name === "string" ? uploadedImages.get(name) : undefined;
}

/** runOutput はプロンプトの実行が出す画像を返す。合成が無効なら一色の画像である。 */
function runOutput(prompt) {
  if (!composite) return solidOutput;
  const frame = inputImage(prompt, "vv:frame");
  const mask = inputImage(prompt, "vv:mask");
  if (frame === undefined || mask === undefined) return solidOutput;
  return compositePNG(frame, mask);
}

/** stop は実行 id を止める。待ち行列にあるか実行中のときだけ効く。 */
function stop(id) {
  const run = runs.get(id);
  if (run === undefined) return;
  const current = phase(run);
  if (current === "queued" || current === "running") run.stopped = true;
}

async function handle(request, response) {
  const url = new URL(request.url ?? "/", "http://127.0.0.1");
  const route = `${request.method ?? "GET"} ${url.pathname}`;

  if (route === "GET /system_stats") {
    send(response, 200, {
      system: { os: "posix", python_version: "3.12.0", comfyui_version: "fake" },
      devices: [{ name: "fake", type: "cpu" }],
    });
    return;
  }
  if (route === "GET /object_info") {
    send(response, 200, objectInfo());
    return;
  }
  if (route === "POST /upload/image") {
    const image = imagePart(request.headers["content-type"], await readBody(request));
    if (image === null) {
      send(response, 400, { error: "no image" });
      return;
    }
    uploads.push(image.name);
    uploadedImages.set(image.name, image.content);
    send(response, 200, { name: image.name, subfolder: "", type: "input" });
    return;
  }
  if (route === "POST /prompt") {
    const { prompt, client_id: clientId } = await readJSON(request);
    const output = Object.entries(prompt ?? {}).find(
      ([, node]) => node?._meta?.title === "vv:output",
    )?.[0];
    if (output === undefined) {
      send(response, 400, {
        error: { type: "prompt_no_outputs", message: "Prompt has no outputs" },
        node_errors: {},
      });
      return;
    }
    const id = randomUUID();
    number += 1;
    const outputName = `vv-fake-output-${String(number)}.png`;
    outputs.set(outputName, runOutput(prompt));
    runs.set(id, {
      id,
      number,
      output,
      outputName,
      submittedAt: Date.now(),
      queuedMs: timing.queuedMs,
      runningMs: timing.runningMs,
      stopped: false,
    });
    prompts.push({ id, clientId, prompt, uploads: [...uploads] });
    send(response, 200, { prompt_id: id, number, node_errors: {} });
    return;
  }
  if (route === "GET /queue") {
    const running = [];
    const pending = [];
    for (const [id, run] of runs) {
      const current = phase(run);
      if (current === "running") running.push(queueItem(id, run));
      if (current === "queued") pending.push(queueItem(id, run));
    }
    send(response, 200, { queue_running: running, queue_pending: pending });
    return;
  }
  if (route === "POST /queue") {
    const body = await readJSON(request);
    for (const id of body.delete ?? []) {
      const run = runs.get(id);
      if (run !== undefined && phase(run) === "queued") run.stopped = true;
    }
    send(response, 200, {});
    return;
  }
  if (route === "POST /interrupt") {
    const body = await readJSON(request);
    if (typeof body.prompt_id === "string") stop(body.prompt_id);
    send(response, 200, {});
    return;
  }
  if (request.method === "GET" && url.pathname.startsWith("/history/")) {
    const id = decodeURIComponent(url.pathname.slice("/history/".length));
    const run = runs.get(id);
    const current = run === undefined ? undefined : phase(run);
    if (run === undefined || current === "queued" || current === "running") {
      send(response, 200, {});
      return;
    }
    send(response, 200, { [id]: historyEntry(run) });
    return;
  }
  if (route === "GET /view") {
    const image = outputs.get(url.searchParams.get("filename") ?? "");
    if (image === undefined) {
      send(response, 404, "not found", "text/plain");
      return;
    }
    send(response, 200, image, "image/png");
    return;
  }
  if (route === "GET /__fake/prompts") {
    send(response, 200, prompts);
    return;
  }
  if (route === "PUT /__fake/timing") {
    const body = await readJSON(request);
    if (Number.isFinite(body.queuedMs)) timing.queuedMs = body.queuedMs;
    if (Number.isFinite(body.runningMs)) timing.runningMs = body.runningMs;
    send(response, 200, timing);
    return;
  }
  if (route === "PUT /__fake/composite") {
    const body = await readJSON(request);
    composite = body.enabled === true;
    send(response, 200, { enabled: composite });
    return;
  }
  send(response, 404, "not found", "text/plain");
}

const server = createServer((request, response) => {
  handle(request, response).catch((error) => {
    console.error("fake ComfyUI:", error);
    if (!response.headersSent) send(response, 500, { error: String(error) });
    else response.end();
  });
});

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  server.listen(FAKE_COMFYUI_PORT, "127.0.0.1");
  process.stdin.resume();
  process.stdin.on("end", () => process.exit(0));
  process.stdin.on("close", () => process.exit(0));
}
