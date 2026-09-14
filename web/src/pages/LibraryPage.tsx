import { type CSSProperties, useCallback, useEffect, useRef, useState } from "react";
import { useSearchParams } from "react-router";

import { MAX_QUERY_LENGTH, type VideoSort } from "../api/client";
import { useVideos } from "../api/useVideos";
import ScanStatus from "../components/ScanStatus";
import Skeleton from "../components/Skeleton";
import StateNotice from "../components/StateNotice";
import Toolbar from "../components/Toolbar";
import VideoCard from "../components/VideoCard";

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

/** defaultSort は並び順が指定されていない（または壊れている）ときの値である。 */
const defaultSort: VideoSort = "addedDesc";

/** skeletonCount は通信中に並べる骨組みの数である（最初の画面がほぼ埋まる数）。 */
const skeletonCount = 12;

/**
 * toSort は URL のクエリを VideoSort に直す。
 *
 * 想定外の値は既定値に落とす。URL は利用者が編集できるし、古い共有リンクに
 * 廃止された値が残ることもある — 一覧が出ないより、既定の並びで出るほうがよい。
 * 照合を sortLabels に対して行うのは、選べる値と受け付ける値を1か所に保つ
 * ためである（生成物の VideoSort から外れた値は型検査で落ちる）。
 */
function toSort(value: string | null): VideoSort {
  return sortLabels.find((option) => option.value === value)?.value ?? defaultSort;
}

/**
 * gridStyle は一覧の格子である（contracts/design-tokens.md 3.）。
 *
 * 列数を JavaScript で計算しない。min(--tile-min, (100% - gap) / 2) を挟むのは、
 * どの画面幅でも列が 1 本にならないことを保証するためで、幅 360px でも 2 列に
 * なり横スクロールが出ない（SC-004）。
 *
 * --tile-min はいま「標準」に固定してある。密度（US3 / T035）はこの変数の
 * 指す先を差し替えるだけで、列の式には触れない。
 */
const gridStyle = {
  "--tile-min": "var(--size-tile-standard)",
  "--tile-gap": "1rem",
  gap: "var(--tile-gap)",
  gridTemplateColumns:
    "repeat(auto-fill, minmax(min(var(--tile-min), (100% - var(--tile-gap)) / 2), 1fr))",
} as CSSProperties;

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
  const sort = toSort(searchParams.get("sort"));

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

  const changeSort = useCallback(
    (value: string) => {
      setSearchParams(
        (current) => {
          const params = new URLSearchParams(current);
          params.set("sort", toSort(value));
          return params;
        },
        { replace: true },
      );
    },
    [setSearchParams],
  );

  const clearQuery = useCallback(() => {
    setInput("");
    setQuery("");
  }, [setQuery]);

  const { items, total, hasMore, loading, loadingMore, error, loadMore, reload } =
    useVideos(sort, query);

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

  const onScanFinished = useCallback(() => reload(), [reload]);

  // 空の言い分けは 2 通りある（FR-009）。「0 本」とだけ出すと、置き場所が
  // 違うのか検索語が悪いのかを利用者から区別できない。
  const empty = !loading && error === null && items.length === 0;

  return (
    <div className="min-h-dvh">
      {/* 帯は状態によらず**先に**出す。通信中も失敗中も、探す・並べ替える・
          取り込むは押せる（FR-002 / contracts/screen-states.md 1.）。 */}
      <Toolbar
        search={
          <label className="flex flex-1 items-center gap-2 text-sm text-muted">
            <span className="sr-only">題名で探す</span>
            <input
              type="search"
              value={input}
              onChange={(event) => setInput(event.target.value)}
              maxLength={MAX_QUERY_LENGTH}
              placeholder="題名で探す"
              className="min-h-[var(--size-tap)] w-48 rounded-control border border-border bg-surface-raised px-2 text-sm text-body"
            />
          </label>
        }
        sort={
          <label className="flex items-center gap-2 text-sm text-muted">
            並び順
            <select
              value={sort}
              onChange={(event) => changeSort(event.target.value)}
              className="min-h-[var(--size-tap)] rounded-control border border-border bg-surface-raised px-2 text-sm text-body"
            >
              {sortLabels.map((option) => (
                <option key={option.value} value={option.value}>
                  {option.label}
                </option>
              ))}
            </select>
          </label>
        }
        scan={<ScanStatus onFinished={onScanFinished} />}
      />

      <main className="mx-auto flex max-w-6xl flex-col gap-4 px-4 py-4">
        <div className="flex flex-wrap items-baseline justify-between gap-2">
          <h1 className="text-2xl font-semibold tracking-tight">vv</h1>
          {/*
            件数の文言は「いま絞られているのか」を文言だけで判断できる形にする
            （FR-008 / contracts/screen-states.md 1.「件数の文言」）。検索の結果
            が変わったことが読み上げに届くよう、変化を知らせる領域にする（FR-021）。
          */}
          <p role="status" aria-live="polite" className="text-sm text-muted">
            {loading
              ? "読み込み中…"
              : query === ""
                ? `${String(total)} 本`
                : `「${query}」に一致 ${String(total)} 本`}
          </p>
        </div>

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
          <div role="status" aria-label="読み込み中" className="grid" style={gridStyle}>
            {Array.from({ length: skeletonCount }, (_, index) => (
              <Skeleton key={index} />
            ))}
          </div>
        ) : (
          <ul className="grid" style={gridStyle}>
            {items.map((video) => (
              <li
                key={video.id}
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
                <VideoCard video={video} />
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
    </div>
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
