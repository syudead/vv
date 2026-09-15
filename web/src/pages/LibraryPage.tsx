import {
  type CSSProperties,
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
} from "react";
import { useSearchParams } from "react-router";

import { MAX_QUERY_LENGTH, type Scan, type VideoSort } from "../api/client";
import {
  clearListSnapshot,
  saveListSnapshot,
  takeListSnapshot,
} from "../api/listSnapshot";
import { useVideos } from "../api/useVideos";
import DensitySelect from "../components/DensitySelect";
import ScanStatus from "../components/ScanStatus";
import Skeleton from "../components/Skeleton";
import StateNotice from "../components/StateNotice";
import Toolbar from "../components/Toolbar";
import VideoCard from "../components/VideoCard";
import { usePublishVideoCount } from "../layout/AppShell";
import { headerHeight } from "../layout/Header";
import {
  type Density,
  readViewPreferences,
  type ViewPreferences,
  writeViewPreferences,
} from "../preferences/viewPreferences";

/**
 * searchDebounceMs は入力が落ち着くのを待つ時間である。
 *
 * 1 打鍵ごとに問い合わせると、打っている間ずっと一覧が入れ替わって読めない。
 * 取りこぼしは起きない（打ち終えた値で必ず1回引く）。
 *
 * **この値は 002 のまま変えない**（FR-025 / SC-009）。
 * LibraryPage.search.test.tsx がこの振る舞いを守っている。
 */
const searchDebounceMs = 250;

/** sortLabels は並び順の選択肢である。 */
const sortLabels: { value: VideoSort; label: string }[] = [
  { value: "addedDesc", label: "追加が新しい順" },
  { value: "titleAsc", label: "題名順" },
];

/** skeletonCount は通信中に並べる骨組みの数である（最初の画面がほぼ埋まる数）。 */
const skeletonCount = 12;

/**
 * toSort は URL のクエリを VideoSort に直す。想定外なら undefined を返す。
 *
 * URL は利用者が編集できるし、古い共有リンクに廃止された値が残ることもある —
 * 一覧が出ないより、表示設定の並びで出るほうがよい。値が無い場合と壊れている
 * 場合を呼び出し側で分けないのは、どちらも「URL は並び順を指していない」で
 * あり、そのときの落とし先が同じ（表示設定。data-model.md 1.）だからである。
 * 照合を sortLabels に対して行うのは、選べる値と受け付ける値を1か所に保つ
 * ためである（生成物の VideoSort から外れた値は型検査で落ちる）。
 */
function toSort(value: string | null): VideoSort | undefined {
  return sortLabels.find((option) => option.value === value)?.value;
}

/**
 * tileMin は密度ごとの最小列幅である（contracts/design-tokens.md 3.）。
 *
 * 密度が変えるのはこの 1 つの変数だけで、列の式には触れない。
 */
const tileMin: Record<Density, string> = {
  dense: "var(--size-tile-dense)",
  standard: "var(--size-tile-standard)",
  relaxed: "var(--size-tile-relaxed)",
};

/**
 * gridStyle は一覧の格子である（contracts/design-tokens.md 3.）。
 *
 * 列数を JavaScript で計算しない。min(--tile-min, (100% - gap) / 2) を挟むのは、
 * どの画面幅でも列が 1 本にならないことを保証するためで、幅 360px でも 2 列に
 * なり横スクロールが出ない（SC-004）。その結果、狭い画面では 3 つの密度の
 * 見た目が同じになる — これは意図した動作である。
 */
function gridStyle(density: Density): CSSProperties {
  return {
    "--tile-min": tileMin[density],
    "--tile-gap": "1rem",
    gap: "var(--tile-gap)",
    gridTemplateColumns:
      "repeat(auto-fill, minmax(min(var(--tile-min), (100% - var(--tile-gap)) / 2), 1fr))",
  } as CSSProperties;
}

/**
 * topmostId は固定領域の下端に最も近い項目の id を返す（R-411）。
 *
 * 座標ではなく**項目**を覚えるのが要点である。密度を変えれば 1 行の本数が
 * 変わり、同じスクロール座標は別の項目を指す。
 *
 * 基準は 004 ではビューポートの上端（0）だったが、005 ではその上にヘッダーと
 * ツールバーが載る。0 のままだと、ヘッダーの裏に隠れて**見えていない**項目を
 * 「上端に最も近い項目」として覚えてしまう（contracts/layout.md 1.）。
 */
function topmostId(
  list: HTMLUListElement | null,
  /** 固定領域の下端（px）。ここから下が実際に見えている領域である。 */
  top: number,
): number | undefined {
  if (list === null) {
    return undefined;
  }

  for (const child of Array.from(list.children)) {
    // 下端が固定領域の下端より下にある最初の項目が、いちばん上に見えている。
    if (child.getBoundingClientRect().bottom > top) {
      const id = Number((child as HTMLElement).dataset.videoId);
      return Number.isNaN(id) ? undefined : id;
    }
  }
  return undefined;
}

/**
 * LibraryPage は動画の一覧である。
 *
 * 無限スクロールにするのは、1万件でも最初の画面が 2 秒以内に出る（SC-002）
 * ようにするためで、最初に待つのは1ページ（60 件）だけである（R-114）。
 *
 * 検索語と並び順は URL のクエリ（/?q=...&sort=...）に置き、コンポーネントの
 * state を真実にしない（R-403）。戻る／進む・再読み込み・共有のすべてで
 * 同じ一覧が再現でき、保存先を別に持たなくて済む。
 */
export default function LibraryPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const query = (searchParams.get("q") ?? "").trim().slice(0, MAX_QUERY_LENGTH);

  // 表示設定はこの端末で選ばれた見せ方である（FR-018）。読み出しは開くときの
  // 1 回だけでよい — ほかのタブが書き換えても、いま見ている画面を作り直す
  // 理由にはならない。readViewPreferences は決して投げないので、壊れていても
  // 既定値でここに来る（FR-019）。
  const [preferences, setPreferences] = useState(readViewPreferences);

  // 並び順は URL が勝ち、URL が指していなければ表示設定を使う
  // （data-model.md 1.「並び順の 2 つの役割」）。共有されたリンクは送った人の
  // 並びで開き、自分で `/` を開くときは自分の好みで開く。
  const sort = toSort(searchParams.get("sort")) ?? preferences.sort;

  const density = preferences.density;

  // 表示設定は画面の state と localStorage の両方に置く。書き込みは
  // setPreferences の更新関数の**外**で行う — 更新関数は React が開発時に
  // 2 回呼ぶので、副作用を中に置くと書き込みも 2 回走る。
  const savePreferences = useCallback((updated: ViewPreferences) => {
    setPreferences(updated);
    writeViewPreferences(updated);
  }, []);

  // input は入力欄の値。打鍵のたびに URL を書き換えると履歴も一覧も
  // 落ち着かないので、打鍵の受け皿だけを手元に持つ。
  const [input, setInput] = useState(query);

  // committed は自分が最後に URL へ書いた検索語である。これを覚えておかないと、
  // 「自分が書いた変化」と「外から来た変化」を見分けられない。
  const committed = useRef(query);

  const setQuery = useCallback(
    (next: string) => {
      committed.current = next;
      setSearchParams(
        (current) => {
          const params = new URLSearchParams(current);
          if (next === "") {
            params.delete("q");
          } else {
            params.set("q", next);
          }
          return params;
        },
        // 打鍵ごとの絞り込みで履歴を埋めない。戻るで1画面前へ戻れる。
        { replace: true },
      );
    },
    [setSearchParams],
  );

  // 外から query が変わったら（戻る／進む、一覧へのリンク）入力欄を合わせる。
  // input は初回描画のときだけ query から作られるので、これが無いと戻ったあとも
  // 前の語が入力欄に残り、下の待ち合わせがその古い語を URL へ書き戻してしまう
  // ── 戻る／進むで検索語が復元されない（R-403）。
  useEffect(() => {
    if (query !== committed.current) {
      committed.current = query;
      setInput(query);
    }
  }, [query]);

  // 入力が止まってから URL を書き換える。すでにその語で引いていれば何もしない
  // （書き換えるたびに再描画が起き、この効果が走り直すため）。
  useEffect(() => {
    const next = input.trim();
    if (next === query) {
      return;
    }

    const timer = setTimeout(() => setQuery(next), searchDebounceMs);
    return () => clearTimeout(timer);
  }, [input, query, setQuery]);

  // 並び順を変えたら URL を書き換え、同時に表示設定にも書く（次回 `/` を
  // 開いたときの初期値になる。data-model.md 1.）。density は現在値のまま
  // にする（contracts/view-preferences.md 4.）。
  const changeSort = useCallback(
    (value: string) => {
      const next = toSort(value) ?? preferences.sort;

      setSearchParams(
        (current) => {
          const params = new URLSearchParams(current);
          params.set("sort", next);
          return params;
        },
        { replace: true },
      );

      savePreferences({ ...preferences, sort: next });
    },
    [preferences, savePreferences, setSearchParams],
  );

  // anchor は密度を変える直前に覚えた「画面上端に最も近い項目」である。
  const anchor = useRef<number | undefined>(undefined);
  const list = useRef<HTMLUListElement | null>(null);
  const bar = useRef<HTMLDivElement | null>(null);

  // fixedBottom は画面に固定されている領域の下端（px）である
  // （contracts/layout.md 1.「固定領域の下端」）。
  //
  //     固定領域の下端 = --size-header + ツールバーの実測高
  //
  // 帯は狭い画面で折り返して高くなるので**実測**で取る。ヘッダーの分を足すのは
  // 005 で帯がヘッダーの下に粘るようになったためで、足さないと戻した項目が
  // ヘッダーの裏に 56px ぶん隠れる（R-501 の「1 つの例外」）。
  const fixedBottom = useCallback(
    () => headerHeight() + (bar.current?.getBoundingClientRect().height ?? 0),
    [],
  );

  // 密度を変えたら表示設定に書く（sort は現在値のまま）。列幅が変わると同じ
  // 座標が別の項目を指すので、戻す先の項目を**変える直前に**覚える（R-411）。
  const changeDensity = useCallback(
    (next: Density) => {
      // 先頭を見ているときは覚えない。そのまま戻すと、件数まで画面の外へ
      // 送ってしまう — 利用者は何も見失っていないのに画面が動く。
      anchor.current =
        window.scrollY > 0 ? topmostId(list.current, fixedBottom()) : undefined;
      savePreferences({ ...preferences, density: next });
    },
    [fixedBottom, preferences, savePreferences],
  );

  // 覚えた項目を画面の上端へ戻す。列が組み直されたあとでなければ意味が無いので
  // useLayoutEffect で行う（描いた同じフレームのうちに戻す。R-411）。
  useLayoutEffect(() => {
    const id = anchor.current;
    if (id === undefined) {
      return;
    }
    anchor.current = undefined;

    const target = list.current?.querySelector(`[data-video-id="${String(id)}"]`);
    if (target === null || target === undefined) {
      return;
    }

    // 逃げる高さは固定領域の**実測値**である。scrollIntoView + 固定の
    // scroll-margin では足りない — 帯は折り返すので、狭い画面では 2 行以上に
    // なって 64px を大きく超え、戻した項目がその裏に隠れる。
    const offset = fixedBottom();
    const top = window.scrollY + target.getBoundingClientRect().top - offset;
    window.scrollTo({ top: Math.max(top, 0), behavior: "auto" });
  }, [density, fixedBottom]);

  const clearQuery = useCallback(() => {
    setInput("");
    setQuery("");
  }, [setQuery]);

  // 復元は一覧を**開く瞬間**に 1 回だけ読む（data-model.md 2.）。描画のたびに
  // 読むと、自分が離れ際に書いた控えを拾い直してしまう。鍵が違えば
  // undefined が返り、そのまま 1 ページ目から読む — 復元できないことは
  // 異常ではない。
  const [restored] = useState(() => takeListSnapshot({ query, sort }));

  const { items, total, cursor, hasMore, loading, loadingMore, error, loadMore, reload } =
    useVideos(sort, query, restored);

  // 総件数をサイドバーの「すべての動画」へ届ける（T021）。ここは**公開する
  // だけ**で、どこにどう出るかは骨格（AppShell）が決める。
  //
  // **`total` をそのまま公開しない。** useVideos は total を 0 で初期化し、
  // 取り直すあいだも前の値を消さないので、total だけを見ると初回に「0 本」、
  // 絞り込みの最中に前の件数が出る。どちらも嘘であり、同じ場面で帯が
  // 「読み込み中…」と出しているのとも食い違う（004 の FR-008）。
  //
  // 件数が嘘になる場面は 2 つある。どちらも undefined に倒す。
  //
  // 1. `loading` — 最初の 1 ページを待っている、または取り直しの最中
  // 2. `error !== null && items.length === 0` — 取得に失敗した。失敗しても
  //    useVideos は total を書き換えないので、保持された前の件数（初回なら
  //    初期値の 0）が残る。一覧がエラーを出している横でサイドバーが「12」と
  //    言う状態になる
  //
  // 2 に `items.length === 0` が要る。**続きのページだけが失敗した場合は、
  // すでに読めている一覧も総件数も有効**だからである。そこまで隠すと、読めて
  // いる事実まで取り下げることになる。
  usePublishVideoCount(
    loading || (error !== null && items.length === 0) ? undefined : total,
  );

  // 戻したいスクロール位置。項目を描いたあとに 1 回だけ使う。
  const pendingScroll = useRef(restored?.scrollY);

  // ブラウザ自身の復元を止める（R-403）。無限スクロールでは復元の時点で
  // 文書にまだ中身が無く、ブラウザの自動復元は空の文書に対して働いて失敗する。
  useEffect(() => {
    const previous = history.scrollRestoration;
    history.scrollRestoration = "manual";
    return () => {
      history.scrollRestoration = previous;
    };
  }, []);

  // 位置を戻すのは**項目を描いたあと**でなければならない。中身が無いうちに
  // scrollTo を呼んでも、文書の高さが足りず途中で止まる（data-model.md 2.）。
  // 描いた同じフレームのうちに戻すので useLayoutEffect を使う。
  useLayoutEffect(() => {
    const top = pendingScroll.current;
    if (top === undefined || items.length === 0) {
      return;
    }
    pendingScroll.current = undefined;
    window.scrollTo({ top, behavior: "auto" });
  }, [items.length]);

  // listUrl はいま見ている一覧の URL である。項目のリンクに持たせて、
  // 再生画面の「一覧へ戻る」がこの一覧へ帰れるようにする（FR-016）。
  // `/` へ戻すだけでは、検索語と並び順が消えて控えの鍵とも一致しない。
  const search = searchParams.toString();
  const listUrl = search === "" ? "/" : `/?${search}`;

  // 一覧を離れる瞬間（項目のリンクを踏んだとき）に控えを書く。Enter でも
  // click は起きるので、キーボードだけで往復しても復元は効く（SC-005）。
  const saveSnapshot = useCallback(() => {
    saveListSnapshot(
      { query, sort },
      {
        items,
        total,
        cursor,
        hasMore,
        scrollY: window.scrollY,
        // どの取り込みまでを映した一覧なのかを添える。戻ってきたときに
        // これと違う取り込みが終わっていれば、控えは使わずに読み直す。
        scanId: knownScanId.current,
      },
    );
  }, [cursor, hasMore, items, query, sort, total]);

  const sentinel = useRef<HTMLDivElement | null>(null);

  // 末尾の観測点が見えたら次のページを読む。スクロール位置を自分で測るより、
  // ブラウザに任せる方が取りこぼしが少ない。
  useEffect(() => {
    const target = sentinel.current;
    if (target === null || !hasMore) {
      return;
    }

    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) {
          loadMore();
        }
      },
      // 画面に入る少し手前で読み始める。継ぎ目で待たされないようにする。
      { rootMargin: "400px" },
    );
    observer.observe(target);

    return () => observer.disconnect();
  }, [hasMore, loadMore]);

  // knownScanId は「この一覧に反映済みの取り込み」の id である。
  //
  // ScanStatus は終わっている取り込みを観測するたびに知らせてくる。実行中が
  // 無ければ最後に終わったものが返る仕様なので、そのほとんどは「前回の残り」
  // である。新しいかどうかを決められるのは、いまの一覧が**どの取り込みまでを
  // 映しているか**を知っているこちら側だけである。
  const knownScanId = useRef(restored?.scanId);

  // 取り込みが終わったら控えを捨ててから読み直す。取り込む前の一覧に
  // 戻してはならない（data-model.md 2.）。
  const onScanFinished = useCallback(
    (scan: Scan, firstSight: boolean) => {
      const known = knownScanId.current;
      knownScanId.current = scan.id;

      if (known === scan.id) {
        // 前回の残り。控えも一覧もそのままでよい。
        return;
      }
      if (known === undefined && firstSight) {
        // 一覧を読んだ時点で既に終わっていた取り込みである。その結果は
        // すでに映っているので読み直さない（毎回二重に取得することになる）。
        //
        // **firstSight が要る。** 「初めて観測した」だけを根拠にすると、
        // 空のライブラリで最初の取り込みを走らせた場合や、開いた時点で
        // 取り込みが実行中だった場合まで「反映済み」に倒れてしまい、
        // 取り込んだ動画がいつまでも出てこない。
        return;
      }

      clearListSnapshot();
      reload();
    },
    [reload],
  );

  // 空の言い分けは 2 通りある（FR-009）。「0 本」とだけ出すと、置き場所が
  // 違うのか検索語が悪いのかを利用者から区別できない。
  const empty = !loading && error === null && items.length === 0;

  return (
    // 画面を満たす役目は骨格（AppShell）が持つ。ここでも 100dvh を求めると、
    // 骨格の上余白と足し合わさって文書が画面より高くなり、件数が少ないときに
    // 余計な縦スクロールが出る。
    <>
      {/* 帯は状態によらず**先に**出す。通信中も失敗中も、探す・並べ替える・
          取り込むは押せる（FR-002 / contracts/screen-states.md 1.）。 */}
      <Toolbar
        ref={bar}
        search={
          // 固定幅を持たせない。basis-48 は「これを下回るなら自分の行へ
          // 折り返す」目安であって幅ではなく、flex-1 が帯の残りをそのまま
          // 吸うので、広い画面では検索欄が伸びる。min-w-0 が要るのは、入力欄の
          // 既定の最小幅が flex の縮小を止め、幅 360px で帯からはみ出すため
          // である（FR-022 / SC-004）。入力欄そのものの下限は帯が与える
          // --size-tap（44px）で、ここでは重ねて書かない。
          <label className="flex min-w-0 flex-1 basis-48 items-center gap-2 text-sm text-muted">
            <span className="sr-only">題名で探す</span>
            <input
              type="search"
              value={input}
              onChange={(event) => setInput(event.target.value)}
              maxLength={MAX_QUERY_LENGTH}
              placeholder="題名で探す"
              className="w-full rounded-control border border-border bg-surface-raised px-2 text-sm text-body"
            />
          </label>
        }
        sort={
          <label className="flex items-center gap-2 text-sm text-muted">
            並び順
            {/* 当たり判定（--size-tap）は帯が帯の中のすべてに与えるので、
                ここでは重ねて書かない（Toolbar 参照）。 */}
            <select
              value={sort}
              onChange={(event) => changeSort(event.target.value)}
              className="rounded-control border border-border bg-surface-raised px-2 text-sm text-body"
            >
              {sortLabels.map((option) => (
                <option key={option.value} value={option.value}>
                  {option.label}
                </option>
              ))}
            </select>
          </label>
        }
        density={<DensitySelect value={density} onChange={changeDensity} />}
        scan={<ScanStatus onFinished={onScanFinished} />}
        count={
          /*
            件数の文言は「いま絞られているのか」を文言だけで判断できる形にする
            （FR-008 / contracts/screen-states.md 1.「件数の文言」）。検索の結果
            が変わったことが読み上げに届くよう、変化を知らせる領域にする（FR-021）。

            005 で変わったのは**置き場所だけ**である（原案どおり帯の右端へ。
            R-508）。文言も role="status" / aria-live も 004 のまま連れて来る。
          */
          <p role="status" aria-live="polite" className="text-sm text-muted">
            {loading
              ? "読み込み中…"
              : query === ""
                ? `${String(total)} 本`
                : `「${query}」に一致 ${String(total)} 本`}
          </p>
        }
      />

      {/* 見出し文字は置かない。h1 はロゴ（C2）が持つ（R-508）。 */}
      <main className="mx-auto flex max-w-6xl flex-col gap-4 px-4 py-4">
        {error !== null && (
          <StateNotice tone="danger" title="一覧を取得できません" description={error}>
            <button
              type="button"
              onClick={reload}
              className="min-h-[var(--size-tap)] rounded-control border border-border px-3 text-sm outline-offset-2 focus-visible:outline-2 focus-visible:outline-focus"
            >
              再試行
            </button>
          </StateNotice>
        )}

        {empty &&
          (query === "" ? (
            <EmptyLibrary />
          ) : (
            <NoMatches query={query} onClear={clearQuery} />
          ))}

        {loading ? (
          // 骨組みは 1 つずつ読ませない。伝えたいのは「この領域はいま読み込み中
          // である」という 1 つの事実である（contracts/screen-states.md 3.）。
          <div
            role="status"
            aria-label="読み込み中"
            className="grid"
            style={gridStyle(density)}
          >
            {Array.from({ length: skeletonCount }, (_, index) => (
              <Skeleton key={index} />
            ))}
          </div>
        ) : (
          // 控えを書くのは離れる瞬間だけである。個々の項目に配るより、
          // 一覧そのもので受けたほうが、項目の描画（数百件）に手が入らない。
          <ul
            ref={list}
            className="grid"
            style={gridStyle(density)}
            onClick={saveSnapshot}
          >
            {items.map((video) => (
              <li
                key={video.id}
                // 密度を変えたときに戻す先を探すための印である（R-411）。
                data-video-id={video.id}
                // 画面外の項目は描画を省かせる（R-408 / SC-008）。見込みの
                // 大きさを必ず与える — 省いた項目の高さを 0 と見積もらせると
                // スクロールバーが暴れる。
                //
                // p-1 は狙いを合わせた印のための余地である。content-visibility:
                // auto は paint containment を伴うので、この箱の外へはみ出した
                // 描画が切られる。項目の輪郭は `outline-offset-2`（2px）+ 2px の
                // 太さで外側 4px に描かれるから、同じ 4px を内側に空けておかないと
                // 輪郭が端で切れる（contracts/screen-states.md 3.「印の視認」）。
                className="p-1 [content-visibility:auto] [contain-intrinsic-size:auto_14rem]"
              >
                <VideoCard video={video} backTo={listUrl} />
              </li>
            ))}
          </ul>
        )}

        {/* 観測点。ここが見えたら次のページを読む。 */}
        <div ref={sentinel} aria-hidden className="h-px" />

        {/* 続きの読み込みでは、すでに読めている項目を骨組みに置き換えない。
            末尾に細い知らせを出すだけにする。 */}
        {loadingMore && (
          <p className="py-4 text-center text-sm text-muted">読み込み中…</p>
        )}
      </main>
    </>
  );
}

/**
 * NoMatches は該当が1本も無いときに、結果が無いことと次に取れる操作を
 * 示す（FR-009）。EmptyLibrary とは**別の文言**であることが要件である。
 */
function NoMatches({ query, onClear }: { query: string; onClear: () => void }) {
  return (
    <StateNotice
      tone="empty"
      title={`「${query}」に一致する動画はありません`}
      description="別の語で探すか、検索語を短くしてみてください。"
    >
      <button
        type="button"
        onClick={onClear}
        className="min-h-[var(--size-tap)] rounded-control border border-border px-3 outline-offset-2 focus-visible:outline-2 focus-visible:outline-focus"
      >
        検索語を消す
      </button>
    </StateNotice>
  );
}

/**
 * EmptyLibrary は1本も無いときに、置き場所と次に取れる操作を示す（FR-009）。
 * 「0 本」とだけ出すと、置き場所が違うのか取り込みが済んでいないのかが
 * 利用者から区別できない。
 */
function EmptyLibrary() {
  return (
    <StateNotice
      tone="empty"
      title="動画がまだありません"
      description={
        <p>
          <code className="rounded-control bg-surface-sunken px-1 py-0.5 font-mono">
            MDM_MEDIA_DIR
          </code>{" "}
          に指定した場所へ動画を置き、「取り込む」を押してください。取り込みは起動直後にも
          1 回自動で走ります。
        </p>
      }
    />
  );
}
