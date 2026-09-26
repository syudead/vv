# Data model: ライブ変換用の解析情報

親 Issue #371 の要件 7〜10 のうち、保存するものとその規則だけを書く。既存の表（`videos`・
`video_locations`・`jobs` ほか）は変えない。表の区分は [ARCHITECTURE.md](../../ARCHITECTURE.md) の
「Two kinds of data」に従い、この表は索引である。

## 1. マイグレーション

`internal/store/migrations/00013_transcode_probes.sql` を足す。

```sql
-- ライブ変換に要る解析情報。索引であり、消えても次の変換か再解析で埋まる（要件 10）。
create table video_transcode_probes (
    video_id   integer primary key references videos (id) on delete cascade,
    -- domain.TranscodeProbeVersion。違えば「無い」と読む（§3）。
    version    integer not null,
    -- 解析したファイルの os.Stat の大きさと更新時刻（Unix ナノ秒）。要求時に開いたファイルと比べる（§4）。
    size_bytes integer not null,
    mtime_ns   integer not null,
    -- domain.TranscodeProbe の JSON（§2）。
    probe      text    not null,
    updated_at integer not null
) without rowid;
```

`videos` の行が消えると連鎖で消える。動画の内容が変わって行が作り直されるときも同じで、新しい
行は要件 10 の経路で埋まる。

## 2. 保存する値: `domain.TranscodeProbe`

`internal/domain` の値型。`domain.Probe` に `Transcode *TranscodeProbe` として載り、取り込みの解析
（`app.Ingest.Probe`）はそのまま store へ渡す。使える映像 stream（非添付で寸法がある）が無い動画では
nil で、行を書かない（その動画はライブ変換できない）。

| 項目 | 内容 | 使う場所 |
| --- | --- | --- |
| `FormatName` | `format.format_name`（小文字） | MOV の二入力の判定 |
| `Video.Index` | 選んだ映像 stream の `index`（最初の非添付 stream） | `-map` |
| `Video.CodecName`・`Profile`・`Level`・`PixelFormat`・`BitsPerRawSample` | 映像の符号化 | `videoCanCopy` |
| `Video.Width`・`Height`・`SampleAspectNum`・`SampleAspectDen`・`Rotation` | 幾何（回転は Display Matrix か `rotate` タグ） | 寸法・縦横比・回転の扱い |
| `Video.FPS`・`RealFPS` | `avg_frame_rate`・`r_frame_rate` | 可変フレームレートの判定と fps の上限 |
| `Audio`（nil 可） | 選んだ音声 stream（最初の stream）の `Index`・`CodecName`・`Profile`・`SampleRate`・`Channels` | `-map`・`audioCanCopy` |

項目は今の `transcodeMetadata`（[internal/media/transcode.go](../../internal/media/transcode.go)）と同じで、
型を `internal/domain` へ移す。JSON の欄名は Go の欄名をそのまま使い、項目を足す・意味を変えるときは
`domain.TranscodeProbeVersion` を上げる。

## 3. 使ってよいかの判定: `domain.TranscodeProbeUsable`

純粋関数。次のすべてが成り立つときだけ保存値を使う。

1. 行がある。
2. `version` が今の `domain.TranscodeProbeVersion` と等しい（Edge Case「ffprobe の出力形式が変わる」は、
   保存する形が parser 側の値なので版で扱う）。
3. `probe` が `TranscodeProbe` として読める。
4. `size_bytes` と `mtime_ns` が、変換で実際に開いたファイルの `Stat` と等しい（要件 9。同じ内容の
   別の所在でも、開いた所在の値で比べる）。

成り立たなければ「無い」として扱い、要求時に ffprobe を実行して結果で行を置き換える（要件 9・10）。

## 4. 書く時点と読む時点

| 時点 | 操作 | 同一性 |
| --- | --- | --- |
| 取り込みの解析ジョブ（`ApplyProbeForJob`）と `ApplyProbe` | `videos` の更新と同じ取引で upsert | `media.Probe` が ffprobe の直前に取った `os.Stat` |
| 再解析（`POST /api/videos/{id}/probe`） | 上と同じ（ジョブ経由） | 同上 |
| ライブ変換でその場の ffprobe を実行したとき（`SaveTranscodeProbe`） | upsert 1 文 | 変換で開いたファイルの `Stat` |
| ライブ変換の開始（`LibraryStore.TranscodeProbe`） | 1 件読む | §3 で比べる |

- upsert は 1 文（`insert … on conflict (video_id) do update`）で、取り込みと変換が同時に書いても
  壊れた値を残さず、後に書いた行が残る（Edge Case「同時に複数の要求」）。
- 途中で打ち切られた ffprobe は誤りで終わるので、保存に至らない（Edge Case「要求を途中でやめたとき」）。
- 起動時と取り込み時に既存の動画をまとめて埋める処理は無い（要件 10）。
