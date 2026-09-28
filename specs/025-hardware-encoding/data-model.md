# Data model: ライブ変換の映像エンコード方式

親 Issue #370 の要件 2・3・8 と Edge Case「保存値が未知の値になっている」のうち、保存するものと
その規則だけを書く。既存の表は変えない。表の区分は [ARCHITECTURE.md](../../ARCHITECTURE.md) の
「Rebuildable and user data」に従い、この表は `media_folders` と同じ利用者・設定のデータである
（走査では戻らない）。

## 1. マイグレーション

`internal/store/migrations/000NN_settings.sql`（実装の時点の次の番号）を足す。

```sql
-- +goose Up
-- 所有者が設定画面で選ぶ値。設定のデータで、走査では戻らない
-- （specs/025-hardware-encoding/data-model.md）。
create table settings (
    key        text primary key,
    value      text    not null,
    updated_at integer not null
) without rowid;

-- +goose Down
drop table if exists settings;
```

行は書かない。行が無いキーは「一度も選んでいない」である。

## 2. キー `transcode.video_encoder`

| 項目 | 内容 |
| --- | --- |
| `key` | `transcode.video_encoder` |
| `value` | `domain.EncoderChoice` の文字列: `software`・`nvenc`・`qsv`・`vaapi`・`videotoolbox`・`auto` |
| `updated_at` | 保存した時刻（Unix 秒） |

- `SettingsStore.TranscodeEncoderChoice(ctx) (value string, found bool, err error)` は文字列を
  そのまま返し、`SettingsStore.SaveTranscodeEncoderChoice(ctx, value)` は upsert 1 文で保存する。
  複数のタブからの同時保存は後に書いた行が残る（親 Issue Edge Case）。
- 解釈は `internal/domain` の純粋関数 `ParseEncoderChoice(value string, found bool) EncoderChoice`
  が行う。行が無ければ `software`（要件 3）。知らない文字列も `software` として扱い、保存値は
  変えない（Edge Case「保存値が未知の値」。画面には `software` が選ばれた状態で出て、選び直せる）。
- 選んだ方式が起動時の確認で使えなかったときも保存値は変えない（要件 8）。実際に使う方式は
  保存しない（[research.md R-3](research.md#r-3-実際に使う方式はドメインの純粋関数が決めapp-がメモリに持つ)）。

## 3. メモリに持つ値（保存しない）

`internal/app` の `TranscodeSettings` が持ち、起動ごとに作り直す。

| 値 | 内容 |
| --- | --- |
| `domain.EncoderAvailability` | ハードウェアの方式ごとに `State`（`checking`・`available`・`unavailable`）と `Reason`（`unsupported_os`・`encoder_missing`・`check_failed`・`timed_out`。`unavailable` のときだけ） |
| `domain.TranscodeEncoding` | `Choice`（保存値の解釈）、`Effective`（実際に使う `VideoEncoder`）、`FallbackReason`（`selected_unavailable`・`checking`。fallback のときだけ）、`Checking`（確認が終わっていないか）、`Encoders`（上の一覧） |

`Effective` と `FallbackReason` は `domain.ResolveVideoEncoder(choice, availability)` が決める
（R-3）。確認の対象と OS の組は `domain.HardwareEncoderCandidates(goos)` が返し、対象外の方式は
`unavailable`／`unsupported_os` で一覧に載る。
