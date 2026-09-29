# Contract: ライブ変換の画質

正本は `api/openapi.yaml` で、この文書は `transcodeVideo` に足すパラメータと、画質ごとの変換の約束
だけを書く。応答の形（fragmented MP4、`Cache-Control: no-store`）、`transcode-start` の報告、
エラーの形（[specs/023-english-i18n/contracts/error-api.md](../../023-english-i18n/contracts/error-api.md)）
は変えない。

## 1. `GET /api/videos/{id}/transcode.mp4` の `quality`

```yaml
- name: quality
  in: query
  required: false
  description: |
    縮める画質。無ければ元の画質（今までどおり、映像をコピーできればコピーする）。
    あれば映像を必ずエンコードし、表示の短辺をこの値に縮め、ビットレートに上限を付ける。
    動画の表示の短辺（`Video.width`・`height` の小さい方）より小さい画質だけを受け付ける。
  schema:
    type: string
    enum: [1080p, 720p, 480p, 360p]
```

- 列挙に無い値: 400 `invalid_request`。
- 動画の短辺以上の画質、または寸法の無い動画: 400 `invalid_request`
  （[research.md R-3](../research.md#r-3-画質が使えるかは動画の短辺で決めサーバーは使えない画質を-400-で拒む)）。
- `startMs`・`attempt` は今までどおり組み合わせられる。`quality` のある変換は `startMs` の位置
  そのものから始まるので、`transcode-start` の報告は `startMs` と同じ値になる。
- 境界は「ゲストも」のまま（親 Issue 要件 8）。

## 2. 画質ごとの変換の約束

| `quality` | 表示の短辺 | 映像の上限（`-maxrate`） | `-bufsize` | 音声（AAC） |
| --- | --- | --- | --- | --- |
| `1080p` | 1080 | 5000 kbps | 10000 kbps | 128 kbps |
| `720p` | 720 | 2500 kbps | 5000 kbps | 128 kbps |
| `480p` | 480 | 1200 kbps | 2400 kbps | 96 kbps |
| `360p` | 360 | 700 kbps | 1400 kbps | 64 kbps |

- 寸法は表示の向き（回転を反映済み）で決め、縦横比を保って短辺を表の値にし、幅・高さとも偶数に
  丸める。縦長の 1080×1920 の `720p` は 720×1280 になる。
- 符号化器ごとの上限の付け方は
  [research.md R-2](../research.md#r-2-画質は表示の短辺で縮めビットレートは--maxrate-bufsize-で上限を付ける)。
  H.264 High・Level 5.1・4:2:0 8bit と、出力の時刻で 2 秒以下のキーフレーム間隔は変わらない。
- 音声は画質があれば常に表の kbps でエンコードし（`-ac 2 -ar 48000`）、コピーしない。
- ハードウェアの方式が使えず software に切り替わったときも、同じ短辺と上限で変換する。
