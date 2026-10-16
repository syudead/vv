---
source: docs/design-docs/home.md
sourceHash: 5bcdc445f3aa01b7c78ed0471d64a90ad778e9ef8c83da5e8f30e8643a77f60e
---

# ホームの棚と規則の整理役 {#home-shelves-and-the-rule-organizer}

ホームは棚の一覧であり、各棚はホームでの位置を持つスマートフォルダである。規則の整理役は、よく使われたタグ、人物、組織の棚を AI なしで加える。整理役は [`internal/app/home_organizer.go`](../../internal/app/home_organizer.go)、棚の選び方は [`internal/domain/home_rules.go`](../../internal/domain/home_rules.go)、適用のトランザクションは [`internal/store/home.go`](../../internal/store/home.go) にある。この文書の背後にある判断は [057 research](../../specs/057-home/research.md) にある。

図は、何が実行を始めるかと、実行が何を読み書きするかを示す。

```mermaid
flowchart LR
  owner[Reorganize] -->|作り直し| pending[(home.organize.pending)]
  scan[スキャン完了] --> drain{キューが 2 分間空?}
  drain -->|初回の実行| pending
  drain -->|棚にない動画が 20 本以上| pending
  pending --> run[整理役の実行]
  run --> rules[規則の整理役]
  rules --> apply[提案を適用]
  apply --> shelves[(smart_folders)]
  run --> last[(home.organize.last)]
  run --> event[HomeChanged]
```

## タグ、人物、組織からの棚 {#shelves-from-tags-people-and-organizations}

規則の整理役は、見える動画の少なくとも 5 本に付いたタグ、人物、組織ごとに、その名前を持つ棚を 1 つ提案する。1 回の実行で最大 24 個である ([R-2](../../specs/057-home/research.md#r-2-the-rule-organizer-makes-shelves-from-tags-people-and-organizations))。5 本は、棚がカード 1 枚だけになるのを防ぐ。仮のタグは、所有者が受け入れたタグではなく自動タグ付けの推測なので除く。

| 元 | 棚の規則 | 順序 |
| --- | --- | --- |
| 種類を持つタグ | `{"tags": [id]}` | 1 番目。種類の id 順、次に動画の数の順 |
| 種類を持たないタグ | `{"tags": [id]}` | 2 番目。動画の数の順 |
| 人物 | `{"person": id}` | 3 番目。動画の数の順 |
| 組織 | `{"organization": id}` | 最後。動画の数の順 |

数は、棚の規則が所有者に見せる動画の数である。そのため、タグは手で付けたタグとフォルダ名からのタグの両方を数える。

## 作り直しと追加 {#rebuild-and-extend}

作り直しは見えるすべての動画から提案し、整理役の棚を置き換える。追加は棚にない動画だけから提案し、加えるだけである ([R-8](../../specs/057-home/research.md#r-8-a-rebuild-replaces-the-organizers-folders-in-one-transaction-an-extend-only-adds))。どちらの実行も、所有者が加えた、動かした、または保った棚や、オンボーディングが選んだ興味から作った棚を変えない。どれも `owner_kept` 1 を持つからである。

| 実行 | 始めるもの | 提案の元 |
| --- | --- | --- |
| `rebuild` | `Reorganize` (`POST /api/home/organize`)、または実行が 1 つも終わっていないときの最初のスキャン | 見えるすべての動画 |
| `extend` | 見える動画の 20 本以上が棚にない状態で終わるスキャン | 棚にない動画だけ |

## 実行は一度に 1 つ {#one-run-at-a-time}

待つ実行は設定 `home.organize.pending` の中に最大 1 つで、作り直しは待っている追加を吸収する ([R-9](../../specs/057-home/research.md#r-9-one-run-at-a-time-queued-in-settings-triggered-by-events))。設定は待っている実行を再起動の後まで保ち、シャットダウンで止まった実行もそこに残るので、次の起動がそれをもう一度実行する。

図は整理役の状態を示す。

```mermaid
stateDiagram-v2
  [*] --> Idle
  Idle --> Pending: Reorganize かスキャンの終わり
  Pending --> Running: 実行中のものがない
  Running --> Idle: 完了か失敗
  Running --> Pending: その間に作り直しを求められた
```

終わった実行は、その結果 (`done`、`failed`、または提案が何も変えなかったときの `unchanged`) と、加えた棚と取り除いた棚の数を `home.organize.last` に書く。状態が変わるたびに `HomeChanged` を発行するので、開いている画面はホームを読み直す。

ストレージのエラーが起きても実行は失われない。整理役は、起動時に待っている実行を読めないとき、または実行の後に `home.organize.last` を書けないとき、キューのポーリングのたび (10 秒) に `home.organize.pending` を読み直して見つけたものを実行するので、結果が記録されなかった実行はもう一度実行される。設定は、結果を書いた後にだけ消す。

## スキャンの後にキューを待つ {#waiting-for-the-queues-after-a-scan}

スキャンが起こす実行は、スキャンが `done` として終わった後、ジョブのキューと自動タグ付けのキューがどちらも 2 分間空のままになるまで待つ ([R-10](../../specs/057-home/research.md#r-10-an-extend-runs-when-20-or-more-shown-videos-are-unshelved))。新しい動画はスキャンの後に自動タグ付けからタグを得るので、それより早い実行は、新しい動画をどれもタグを持つ前に棚にない動画として数えてしまう。失敗したスキャンや取り消したスキャンは何も始めない。

失敗したキューの確認は空でないとみなすので、2 分間は次に成功した確認の後に改めて始まる。2 分間の後の判断または待ちへの追加が失敗したときは、整理役はそのスキャンを待ち続け、次のポーリングでもう一度試す。

| 採用しなかった案 | 理由 |
| --- | --- |
| `jobs` テーブルの種類 | `jobs` の行は 1 つの動画とその場所に属する |
| 数時間ごとのタイマー | 要件 8 は棚の追加をスキャンの終わりに結びつける |
| 追加のしきい値にライブラリの割合を使う | 20,000 本の動画では 2,000 本の棚にない動画を待つ |

## 所有者の固定の棚 {#the-owners-fixed-shelves}

フォルダの棚の後に、所有者のホームは続きを見る動画、最近追加した動画、棚にない動画、おすすめを並べる。ゲストは最近追加した動画と棚にない動画だけを得る。視聴履歴は所有者だけのものだからである ([R-13](../../specs/057-home/research.md#r-13-four-fixed-shelves-two-of-them-the-owners-only))。

- **Continue watching** は、ライブラリの `watch=inProgress` の一覧を最後の再生の順に並べたもので、[`internal/store/home_reads.go`](../../internal/store/home_reads.go) が読む。保持した項目は再生位置を保存するたびに捨てる。動画を視聴中のままにする再生は、何の通知もなく順序を変えるからである。
- **Recommended** は種があるときだけ並ぶ。種は、直近 90 日に最も多く再生した最大 10 本の動画と、最新のお気に入り最大 10 本である ([`internal/store/home_recommend.go`](../../internal/store/home_recommend.go))。[`internal/app/home.go`](../../internal/app/home.go) は、種でも見終わった動画でもない見える動画を、関連動画の `RelatedScorer` で求めたいずれかの種への最も高い近さで順位付けし、先頭の 24 本を残し、その順序を 10 分間保持する。20,000 本の動画を 20 個の種に対して採点すると 1 コアで数秒かかるので、候補は並列に採点し、棚の読み取りは store が他の棚を読むのと並行して順位を計算する。

| 採用しなかった案 | 理由 |
| --- | --- |
| すべての種への近さの合計 | 種の大きな 1 つの集まりに近い動画が棚全体を埋める |
| 読み取りのたびに順位付けする | 1 回の処理がすべての動画の手がかりを読み、ホームの 1 秒に収まらない |
| 次の通知まで保持する Continue watching | 視聴中の動画を再生しても何も発行しないので、棚は 10 分間古い順序を保つ |
