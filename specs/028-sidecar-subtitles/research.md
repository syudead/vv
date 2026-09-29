# Research: 動画の隣に置いた字幕ファイルの表示

技術スタック、境界と依存方向、ファイルを開いてよいかの規則、ライブ変換の時間軸の扱いは正本に従う
（[docs/design-docs/tech-stack-selection.md](../../docs/design-docs/tech-stack-selection.md)、
[ARCHITECTURE.md](../../ARCHITECTURE.md)、[internal/mediafs](../../internal/mediafs/media_file.go)、
[docs/design-docs/live-transcode-seek.md](../../docs/design-docs/live-transcode-seek.md)）。
ここにはこの feature が足す決定だけを書く。

## R-1: 字幕ファイルは要求のたびにフォルダを読んで見つけ、SQLite には置かない

- Decision: 字幕ファイルの一覧は、再生画面が呼ぶ `GET /api/videos/{id}/subtitles`
  （[contracts/subtitles-api.md §1](contracts/subtitles-api.md#1-get-apivideosidsubtitles)）が
  そのたびに動画のフォルダを `ReadDir` して作る。表も列も足さず、スキャンも取り込みの job も
  関わらない。
- Rationale: 要件 2 は「再スキャンなしで見つかる」「開き直せば反映される」である。フォルダ 1 つの
  `ReadDir` は再生画面を開く 1 回につき 1 回で、動画の本体を開く費用に比べて無視できる。索引に
  持つと、スキャンと無関係に変わるファイルの状態を追う仕組み（監視か、開くたびの照合）が要る。
- Alternatives considered: スキャンで `video_locations` に字幕の有無を記録する（要件 2 に反し、
  再スキャンまで反映されない）。`GET /api/videos/{id}` の応答に載せる（`Video` は一覧・関連動画・
  取り込み中の再取得でも返る型で、そのたびにフォルダを読むことになる。字幕は再生画面だけが使う）。

## R-2: 探すフォルダは、配信が開く所在のフォルダである

- Decision: 字幕を探すフォルダは、`StreamVideo`・`TranscodeVideo` の `openMediaFile` と同じ順で
  所在を試し、最初に開けた所在のフォルダとする。同じ規則を `internal/mediafs` の
  `ListSidecarFiles`（フォルダの通常ファイルのうち、名前が動画の `<名前>` で始まるものの名前・
  大きさ）と `OpenSidecarFile`（一覧に出た名前の 1 つを開く）に置き、`internal/httpapi` は
  所在と登録フォルダを渡すだけにする。
- Rationale: 親 Issue の Edge Case「動画が複数の場所にある」は「再生に使う場所の隣だけ」と決めて
  いる。再生に使う場所を決めているのは `openMediaFile` なので、同じ順序で同じ判定を通す。
  フォルダを読んでよいかの判定は `internal/mediafs` が一元に持つ規則で、`httpapi` が自分で
  `ReadDir` してはいけない（ARCHITECTURE.md）。
- Alternatives considered: `Video.location`（詳細の応答の代表の所在）のフォルダを使う
  （代表の所在は開けるかを確かめずに選ばれ、開けないときは別の所在で再生する。再生と字幕の
  フォルダがずれる）。全部の所在のフォルダを合わせて探す（Edge Case に反する）。

## R-3: 名前の照合と重複の規則は `internal/domain` の純粋関数が持つ

- Decision: `domain.SubtitleSidecars(videoFileName string, entries []SidecarEntry) []SubtitleSidecar`
  が、フォルダの項目から字幕の一覧（ファイル名、ラベル、形式）を作る。規則は次のとおり。
  - `<名前>` は動画のファイル名から最後の拡張子だけを除いたもの（`my.movie.2024.mp4` →
    `my.movie.2024`）。
  - 候補は `<名前>.srt`・`<名前>.vtt`・`<名前>.<ラベル>.srt`・`<名前>.<ラベル>.vtt`。`<名前>` の
    部分と拡張子は Unicode 正規化（NFC）のうえ大文字・小文字を区別せずに照合する。`<ラベル>` は
    空でない残りの部分で、ドットを含んでよい（`en.forced`）。表示にはファイル名に書かれた
    ままのラベルを使う。
  - 大きさが `domain.SubtitleFileLimit`（4 MiB）を超える項目は候補にしない。
  - ラベルが同じ（大文字・小文字を区別しない）`.srt` と `.vtt` があれば `.vtt` だけを残す。
    両方が同じ拡張子で大文字・小文字だけ違う（Linux でありうる）ときは、名前の自然順で先の
    1 つを残す。
  - 並びは、ラベルの無いものを先頭に、続いてラベルの自然順（`domain.CompareNatural`）。
- Rationale: 純粋関数なら、ファイルシステムを作らずに大文字・小文字、複数ドット、重複、上限の
  各ケースを表で検査できる。`internal/mediafs` は「読んでよいか」だけを持ち、名前の意味は持たない。
  4 MiB は、2 時間の映画の SRT が数百 KB で、テレビ番組の 1 話が 100 KB 前後であることから、
  正しい字幕を落とさず、誤って置かれた大きなファイルをメモリに読まない値として決めた。
- Alternatives considered: 上限 1 MiB（会話の多い長編や、装飾タグの多い SRT が超えうる）。
  上限なし（壊れたファイルや誤配置の大きなファイルを 1 要求で全部読む）。ラベルの重複を
  両方出す（親 Issue の要件 6 に反する）。

## R-4: 変換は Go で行い、ffmpeg は使わない

- Decision: `internal/media` の `SubtitleConverter.Convert(src []byte, format, offsetMs) ([]byte, error)`
  が、文字コードを判定して UTF-8 にし、SRT なら WebVTT にし、WebVTT ならヘッダーを確かめて
  そのまま通し、どちらも `offsetMs` だけ時刻をずらす（R-6）。外部プロセスは起動しない。
  `internal/httpapi` はこれを自分が宣言する `SubtitleConverter` interface で受け取り、
  `cmd/mdm` が配線する（`Transcoder` と同じ形）。
- Rationale: ffmpeg の `srt` demuxer は文字コードを自分では判定せず（`-sub_charenc` は
  iconv 付きのビルドが要り、同梱イメージと利用者の ffmpeg で揃わない）、判定はどのみち Go で
  要る。判定のあとに残るのは、番号行と時刻の行の書き換えという小さなテキスト処理で、要求ごとに
  プロセスを起動する理由が無い。Go だけなら `ffmpeg` の無いテストで全部の入力を表で検査できる。
  `internal/media` に置くのは、`fmp4.go` と同じく「メディアの形式の扱い」であり、兄弟パッケージの
  import を増やさないためである。
- Alternatives considered: `ffmpeg -i x.srt -f webvtt -` を要求ごとに起動する（上記）。
  `internal/subtitles` を新設する（interface と配線が 1 つ増えるだけで、`internal/media` の
  中の 1 ファイルと変わらない）。`internal/domain` に置く（`golang.org/x/text/encoding` を
  domain に持ち込む。domain は値と規則の置き場で、バイト列の復号は adapter の仕事）。

## R-5: 文字コードは BOM → UTF-8 の妥当性 → Shift_JIS の順で決める

- Decision: 先頭のバイトで決める。`EF BB BF` は UTF-8 として BOM を除く。`FF FE`・`FE FF` は
  UTF-16（LE・BE）として `golang.org/x/text/encoding/unicode` で復号する。BOM が無く
  `utf8.Valid` なら UTF-8 のまま。それ以外は Shift_JIS として `golang.org/x/text/encoding/japanese`
  で復号し、復号できない列があれば壊れたファイルとして扱う（R-7）。
- Rationale: 親 Issue の要件 4 が挙げる 4 種類はこの順で互いに区別できる。Shift_JIS の
  日本語のバイト列が偶然 UTF-8 として妥当になることは、2 バイト目の範囲が UTF-8 の
  継続バイトと重ならないので実用上起きない。`golang.org/x/text` は既に依存にある
  （`internal/mediafs` の NFC 正規化）。
- Alternatives considered: 文字コード推定のライブラリを足す（依存が増え、要件 4 の 4 種類の
  外まで当てにいく必要が無い）。BOM 無しの UTF-16 も受ける（要件 4 は BOM 付きだけで、BOM 無しは
  0x00 の混じる列の推定になる）。

## R-6: ライブ変換の時刻合わせは、サーバーが `offsetMs` だけ時刻をずらした WebVTT を返す

- Decision: 字幕の取得経路は `offsetMs`（省略時 0）を取り、すべての cue の時刻からその値を
  引いて返す。終了時刻が 0 以下になる cue は落とし、開始時刻が負になる cue は 0 から始める
  （[contracts/subtitles-api.md §2](contracts/subtitles-api.md#2-get-apivideosidsubtitlesfile)）。
  プレイヤーは、再生の時間軸の 0 が元動画のどの時刻か（`liveOffset.ts` の offset）が決まる
  たびに、その値を `offsetMs` に付けた URL で字幕トラックを付け直す。直接再生では 0、ライブ変換では
  `transcode-start` の報告が届いた実際の開始位置（報告が 404 なら指定位置）である。報告を待って
  いる間はトラックを付けず、決まってから付ける。
- Rationale: video.js の字幕の表示（エミュレーションでもブラウザ標準でも）は `<video>` 要素の
  `currentTime`（変換の出力の時間軸）で cue を選び、`liveOffset.ts` の仲立ちが足す offset は
  通らない。ずらす場所はサーバーか、ブラウザで cue を作り直すかのどちらかで、サーバーなら
  純粋関数 1 つで済み、Go のテストで表にできる。ブラウザで作り直すには WebVTT の解析器が要り、
  video.js が同梱する vtt.js はグローバル変数としてしか届かない。字幕ファイルは小さく、
  付け直しはシークで変換をやり直すとき（もともと数秒かかる）にしか起きない。
  「開始位置が分からない間はずれた字幕を出さない」（親 Issue の Edge Case）は、報告が決まる
  までトラックを付けないことで満たす。
- Alternatives considered: ブラウザで VTT を取得して解析し、`VTTCue` の時刻をずらして
  `addTextTrack` で足す（解析器の問題と、シークのたびに cue を全部作り直す）。仲立ちで
  `TextTrack` の cue を書き換える（video.js の内部にある `activeCues` の計算に手を入れる
  ことになり、ブラウザ標準のトラック（Safari）には効かない）。

## R-7: 壊れたファイルは一覧には出し、取得で 404 にする

- Decision: 一覧（R-1）はファイルの名前と大きさだけを見て作り、中身は読まない。取得の経路が
  読んで変換し、復号できない（R-5）、SRT の cue が 1 つも読めない、WebVTT のヘッダーが無い、
  空である、上限を超えたときは 404 `subtitle_unavailable`（`reason`）を返し、理由をサーバーの
  ログに `Warn` で残す。ブラウザはそのトラックの読み込みに失敗し、何も表示しない。
- Rationale: 親 Issue の Edge Case は「メニューに出さない」と「選んでも何も出ない」のどちらでも
  よいとしている。一覧のたびに全部の字幕を読んで変換すると、再生画面を開くたびに使わない
  字幕まで読むことになる。取得で失敗した字幕はブラウザの `<track>` が `error` になるだけで、
  再生も他の字幕も止まらない。
- Alternatives considered: 一覧のときに読んで壊れたものを除く（上記）。壊れた SRT の読めた
  cue だけを返す（読めた分は返す。cue が 1 つも無いときだけ 404 にする、という形で採る）。

## R-8: SRT の書式の揺れは、時刻の行だけを正規化し、cue の本文はそのまま通す

- Decision: 変換は、行末を LF に揃え、BOM を除き、`WEBVTT` ヘッダーを付け、番号行（数字だけの
  行のあとに時刻の行が続くもの）を除き、時刻の `,` を `.` にし（`.` はそのまま）、時・分・秒の
  桁の揺れ（`0:01:02,5`）を `00:01:02.500` に整える。時刻の行が読めない cue は落とす。cue の
  本文（`<i>`・`<b>`・`<font …>`・`{\an8}` などのタグを含む）はそのまま通す。
- Rationale: WebVTT の cue 本文の解析は、ブラウザが知らないタグを捨てて中の文字を残す。
  ブラウザ側に任せれば、サーバーはタグの表を持たずに済み、表を持たないので新しいタグで
  壊れない。時刻の行は WebVTT の文法が厳密（`hh:mm:ss.ttt`、区切りは `.`）なので、ここだけを
  揃える。
- Alternatives considered: タグを全部除いて平文にする（要件に無く、イタリックなどの意味を
  失う）。`<font color>` を `<c.色>` に写す（WebVTT の `::cue(c.色)` の CSS を用意する必要があり、
  親 Issue は装飾の再現を対象外にしている）。

## R-9: 字幕の選択は `web/src/preferences` に音量と同じ形で保存する

- Decision: `web/src/preferences/subtitlePreference.ts` が
  `{ enabled: boolean, label: string }`（ラベルの無い字幕は `""`）を `localStorage` の
  `vv.subtitles.v1` に、`playbackVolume.ts` と同じ「読めなければ既定、書けなくても続ける」
  総関数で読み書きする。既定は `{ enabled: false, label: "" }`。書くのは、利用者がメニューか
  `c` キーで選択を変えたときだけで、offset の変化でトラックを付け直すとき（R-6）や、動画を
  移ったときに一致するラベルが無くてオフになるときには書かない。
- Rationale: 親 Issue の要件 7 は「音量の記憶と同じく、ブラウザごと」と決めている。
  一致しない動画でオフになったときに保存値を消すと、次に `ja` のある動画を開いてもオンに
  ならず、要件 7 の「同じラベルの字幕があれば自動でオン」が破れる。
- Alternatives considered: video.js の `textTrackSettings` の保存（表示の設定の保存で、
  どのトラックを選んだかは持たない。設定画面も出てしまうので `textTrackSettings: false` にする）。
  サーバーに保存する（ゲストの再生にも要り、要件 7 がブラウザごとと決めている）。

## R-10: 字幕ボタンとメニューは video.js の `SubsCapsButton` を使う

- Decision: 操作バーの `children` に `subsCapsButton` を再生速度の前に入れ、
  `textTrackSettings: false` で「字幕の設定」の項目を出さない。トラックは
  `player.addRemoteTextTrack({ kind: "subtitles", src, label, default: false }, true)` で足す。
  メニューの文言（`Subtitles`、`subtitles off`、`captions off` など）は `playerDictionary()` で
  カタログから置き換え、ボタンには `withKey(…, "C")` と `aria-keyshortcuts="C"` を付ける。
  ラベルの無い字幕の表示名はカタログの `t.player.subtitles.default`。字幕の表示は video.js の
  `vjs-text-track-display` に任せ、操作バーが見えている間の下端の余白（`vjs-user-active` の
  `bottom`）だけを `index.css` の操作バーの高さ（再生バーの 2em を含む）に合わせる。
- Rationale: `SubsCapsButton` は、字幕のトラックが 1 つも無いと自分を隠す（要件 5）、
  「オフ」の項目を持つ（要件 5）、既存の再生速度メニューと同じ部品で同じ余白と文字の大きさに
  なる（親 Issue の UI 品質）。React で作るメニューは、これらを全部作り直すうえ、トラックの
  `mode` と表示の同期も自分で持つことになる。
- Alternatives considered: `TranscodeIndicator` と同じ Radix の Popover でメニューを作る
  （上記）。`captionsButton`・`subtitlesButton` を別に出す（`kind: "subtitles"` しか使わないので
  1 つで足りる）。

## R-11: `c` キーは `keyboard.ts` に足し、切り替えの判断はプレイヤーが持つ

- Decision: `shortcutFor` に `c`／`C` → `"subtitles"` を足し、`PlayerControls` に
  `toggleSubtitles()` を足す。`VideoPlayer` の実装は、トラックが無ければ何もしない、表示中の
  トラックがあれば全部 `disabled` にしてオフを保存する、無ければ保存済みのラベルと一致する
  トラック、無ければメニューの最初のトラックを `showing` にして保存する。
- Rationale: 既存のキー（Space・F・M・0・Esc）と同じ経路で、入力欄やメニューの中では
  効かない規則をそのまま使える。「最後に選んだ字幕」は R-9 の保存値そのものである。
- Alternatives considered: video.js の `hotkeys` を有効にする（既存の設計が画面全体の捕捉で
  受けると決めている。specs/012-video-detail-ia の Structural Decisions 9）。
