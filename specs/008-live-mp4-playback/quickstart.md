# Validation Quickstart: MP4 ライブ変換による動画再生

**Feature**: [spec.md](spec.md) | **Contract**: [live-playback.md](contracts/live-playback.md)

共通の環境準備と検査入口は [Taskfile.yml](../../Taskfile.yml) を使う。fixture生成と実再生には、起動前
確認と同じ`ffmpeg`/`ffprobe`、およびPlaywright browserが必要である。

## Automated validation

```powershell
task check
task test-e2e
```

`task check`はFFmpeg引数、互換属性とoutput envelopeの境界、process cancellation、HTTP契約、frontend
state machineと既存progressのunit/contract testを実行する。`task test-e2e`は一時media folderへBrowser
E2E対象のfixtureを生成し、実browserで検証したあとfixtureとdata directoryを削除する。

## Fixture matrix

`Browser E2E`と`Backend contract`はmedia fixtureを生成し、拡張子だけでなくffprobe結果が意図した
stream構成になっていることをtest setupでassertする。`Backend table`はprobe metadata fixtureで境界値と
FFmpeg引数選択を検証し、巨大な実動画を生成しない。

| Case | Example input | Expected route/action | Owner |
| --- | --- | --- | --- |
| direct | MP4 / H.264 / AAC | `/stream`のみ、再encodeなし | Browser E2E |
| containerのみ非対応 | MKV / H.264 / AAC | transcode、video/audio copy | Browser E2E |
| videoのみ非対応 | MP4 / HEVC / AAC | H.264へ変換、audio copy | Browser E2E |
| audioのみ非対応 | MP4 / H.264 / FLAC | video copy、AACへ変換 | Browser E2E |
| video/audio非対応 | AVI / MPEG-4 Part 2 / PCM | H.264/AACへ変換 | Browser E2E |
| audioなし | AVIまたはMKV / 非対応video / no audio | H.264へ変換、無音再生 | Browser E2E |
| cover先行 | attached JPEG、続いてH.264本編 | JPEGを除外し本編streamを明示map | Backend contract |
| coverのみ | attached JPEG、非添付videoなし | `409 conflict`、変換を開始しない | Backend contract |
| codec名だけ適合 | MKV / H.264 High 4:4:4 10-bit / AAC | videoをH.264へ変換、audio copy | Backend table |
| 奇数寸法 | MKV / VP9 / 641x359 | 642x360へpadしてH.264へ変換 | Backend contract |
| 高sample rate | MKV / H.264 / AAC 96 kHz | audioをAAC-LC 48 kHzへ変換 | Backend contract |
| envelope超過 | 8K/60fps video metadata | 3840x2160以下かつLevel 5.1 rate内へ変換 | Backend table |
| 長いGOPの途中seek | MKV / H.264 / AAC、10秒GOP | 映像・音声をencodeし要求位置から再生 | Browser E2E |
| runtime fallback | Playwrightがdirect応答を不正media bytesへ置換 | 1回だけnormalize transcodeへ切替 | Browser E2E |
| final failure | Playwrightがtranscode応答をconnection abort | 再試行せずerror表示 | Browser E2E |

## Observable checks

1. **開始**: Browser E2Eの各成功caseで動画選択から最初の映像まで3秒以内。
2. **経路**: direct caseでは`/transcode.mp4` requestが0件。既知の非対応caseでは`/stream`を試さず
   `/transcode.mp4`を1件開始する。
3. **copy/encode**: backend testでfixture metadataに対するFFmpeg引数をassertし、互換streamだけに
   `copy`が指定される。H.264 profile/level/pixel format/bit depth/寸法/frame rateとAAC profile/sample
   rate/channel数の各境界、欠落値がencodeへ倒れることもtable testで確認する。
4. **runtime fallback**: direct media error後、同じlogical positionを`startMs`にしてtranscodeへ
   1回だけ移る。次のerrorではrequestを増やさず10秒以内にalertを表示する。
5. **seek**: 未buffer位置を選択すると旧requestがcancelされ、新しい`startMs`から2秒以内に再開する。
   長いGOP内を選んでも最初の映像内容、seek bar、表示時刻、progress送信値が同じlogical timeを示す。
6. **resume**: 途中で画面を閉じ、開き直したとき保存位置の5秒以内から再開する。direct/transcodeで
   同じ`PUT /progress`契約を使う。
7. **cleanup**: seek、一覧へ戻る、page reload、connection abortの各caseで旧FFmpeg processが5秒以内に
   終了する。同時に開いた別tabのprocessと再生は継続する。ライブ変換中のSIGTERMではserver contextが
   request contextより先にcancelされ、processとhandlerがHTTP shutdownの10秒猶予内に終了する。
8. **source protection**: test前後の元動画hashが同じで、media/data directoryに代替動画fileが残らない。
9. **UI**: 360px、768px、1280pxでplayer、error、戻る操作が重ならず、keyboardで再生・seek・戻る操作が
   行える。実装PRへ画像とvisual/accessibility確認結果を添付する。
