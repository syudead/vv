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
//   画像を出力して終わる。
// - GET /view: その一色の画像。
// - POST /interrupt、POST /queue（delete）: 実行を止める。
//
// WebSocket の /ws は持たない。アダプタは接続できなければ進み具合を出さないだけである。
//
// テストが読み書きする /__fake/* は ComfyUI のルートではない。
// - GET /__fake/prompts: 送られたプロンプトと、そのときまでに上げた画像の名前。
// - PUT /__fake/timing: 待ち行列と実行中の長さ（ミリ秒）を変える。
//
// 標準入力が閉じたら（起動した run-e2e.mjs が終わったら）終わる。

import { randomUUID } from "node:crypto";
import { readFileSync } from "node:fs";
import { createServer } from "node:http";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { deflateSync } from "node:zlib";

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

/** solidPNG は width×height の一色 color（RGB）の PNG を作る。 */
function solidPNG(width, height, color) {
  const header = Buffer.alloc(13);
  header.writeUInt32BE(width, 0);
  header.writeUInt32BE(height, 4);
  header[8] = 8; // ビット深度
  header[9] = 2; // RGB
  const row = Buffer.alloc(1 + width * 3);
  for (let x = 0; x < width; x += 1) row.set(color, 1 + x * 3);
  const raw = Buffer.concat(Array.from({ length: height }, () => row));
  return Buffer.concat([
    Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]),
    pngChunk("IHDR", header),
    pngChunk("IDAT", deflateSync(raw)),
    pngChunk("IEND", Buffer.alloc(0)),
  ]);
}

const outputImage = solidPNG(OUTPUT_WIDTH, OUTPUT_HEIGHT, OUTPUT_COLOR);
const outputName = "vv-fake-output.png";

const timing = { queuedMs: 600, runningMs: 2500 };
/** uploads は上げた画像の名前で、上げた順に並ぶ。 */
const uploads = [];
/** prompts は送られたプロンプトで、送られた順に並ぶ。 */
const prompts = [];
/** runs は実行の id ごとの、始めた時刻・出力のノード・止めたか。 */
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
      [run.output]: { images: [{ filename: outputName, subfolder: "", type: "output" }] },
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
    // multipart の本文から、ファイルの部分の名前だけを読む。
    const body = (await readBody(request)).toString("latin1");
    const match = /name="image"; filename="([^"]+)"/.exec(body);
    if (match === null) {
      send(response, 400, { error: "no image" });
      return;
    }
    const name = Buffer.from(match[1], "latin1").toString("utf8");
    uploads.push(name);
    send(response, 200, { name, subfolder: "", type: "input" });
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
    runs.set(id, {
      id,
      number,
      output,
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
    if (url.searchParams.get("filename") !== outputName) {
      send(response, 404, "not found", "text/plain");
      return;
    }
    send(response, 200, outputImage, "image/png");
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
