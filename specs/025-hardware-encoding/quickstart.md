# Quickstart: GPU のあるホストでハードウェアエンコードを確かめる

CI にはハードウェアエンコーダーが無いので、親 Issue #370 の受け入れ条件 2・3・5・6・7・11 は
この手順で GPU のあるホストで確かめ、結果（ホストの GPU、方式、各手順の結果）を実装 PR の本文に
残す（[research.md R-10](research.md#r-10-検査は-ffmpeg-を差し替えたテストで行い実機の確認は-quickstart-に置く)）。
ffmpeg を差し替えた自動テストは `task check` が走らせる。

## 前提

- Intel/AMD の GPU（VAAPI・Quick Sync）か NVIDIA の GPU（NVENC。NVIDIA Container Toolkit 導入済み）
  のある Linux ホスト。手順は [docs/how-to/running-vv.md](../../docs/how-to/running-vv.md) の
  「Hardware encoding」の節に従う（この feature が書く）。
- 受け入れ条件 7 の動画: ブラウザで再生できない MP4（HEVC など）、MKV、回転付き、縦長、4K 超
  （`web/e2e/media-fixtures.mjs` の生成物を流用してよい）。

## 手順

1. `docs/how-to/running-vv.md` の節のとおり GPU を渡す override を置き、`task up` で起動する。
   起動ログに `live transcode video encoder` の行があり、`effective` が `software`、各方式の
   確認結果が出ていること（要件 9）。
2. 所有者でログインし、設定画面の「動画の変換」を開く。ソフトウェアが選ばれ、ホストにある方式が
   選べ、無い方式は選べずに理由が添えられていること（受け入れ条件 1・4）。
3. ホストにある方式を選ぶ。保存中の表示のあと「今使われている方式」がその方式になること。
   再生できない形式の動画を開いて変換再生し、ログの `live transcoding started` にその方式が
   出ること（受け入れ条件 2）。`docker compose exec mdm sh -c 'ps -o args= -C ffmpeg'` で
   `-c:v h264_<方式>` が見えること。
4. コンテナを再起動し、設定画面に選んだ方式が残り、変換再生も同じ方式で動くこと（受け入れ条件 3）。
5. 前提の 5 種類の動画を、最初から再生・シーク・一時停止して再読み込みからの再開で確かめ、向き・
   縦横比・長さがソフトウェアのときと同じであること（受け入れ条件 7）。
6. 変換の出力を確かめる: `ffprobe -show_streams` で `profile=High`、`level` が 51 以下、
   `pix_fmt=yuv420p`、`-show_frames -select_streams v` でキーフレームの間隔が出力の時刻で 2 秒以下
   （要件 10）。出力は `curl -o out.mp4 --cookie "<セッション>" "http://localhost:8080/api/videos/<id>/transcode.mp4?startMs=30000"` で数秒分取れば足りる。
7. 自動選択にする。「今使われている方式」とログがホストの方式になること（受け入れ条件 6）。
8. GPU を渡さずに（override を外して）再起動する。サーバーが起動し、設定画面に「選んだ方式が
   使えずソフトウェアで変換している」警告が出て、変換再生がソフトウェアで動くこと
   （受け入れ条件 5）。
9. NVIDIA の場合: 同時に変換再生するタブを GPU のセッション上限を超える数だけ開く。超えた分が
   ソフトウェアで再生され、ログに `hardware encoder failed; transcoding with software` が出ること
   （Edge Case「同時セッション数の上限」）。
10. 変換の配信中に方式を変える。配信中の再生が止まらず、次のシークから新しい方式になること
    （Edge Case「変換の配信中に方式を変える」）。

## 期待する結果

上のそれぞれが成り立つ。1 つでも成り立たなければ、その方式の引数
（[research.md R-7](research.md#r-7-エンコード引数は方式ごとの符号化器の指定だけを差し替え出力の約束は共通の引数で守る)）
か確認の手順（R-2）を直してからやり直す。
