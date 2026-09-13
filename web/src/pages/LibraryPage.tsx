import { useCallback, useEffect, useRef, useState } from "react";

import type { VideoSort } from "../api/client";
import { useVideos } from "../api/useVideos";
import ScanStatus from "../components/ScanStatus";
import VideoCard from "../components/VideoCard";

/** sortLabels は並び順の選択肢である（FR-013）。 */
const sortLabels: { value: VideoSort; label: string }[] = [
  { value: "addedDesc", label: "追加が新しい順" },
  { value: "titleAsc", label: "題名順" },
];

/**
 * LibraryPage は動画の一覧である（FR-011〜FR-014）。
 *
 * 無限スクロールにするのは、1万件でも最初の画面が 2 秒以内に出る（SC-003）
 * ようにするためで、最初に待つのは1ページ（60 件）だけである（R-114）。
 */
export default function LibraryPage() {
  const [sort, setSort] = useState<VideoSort>("addedDesc");
  const { items, total, hasMore, loading, loadingMore, error, loadMore, reload } =
    useVideos(sort);

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

  return (
    <main className="mx-auto flex min-h-dvh max-w-6xl flex-col gap-6 p-6">
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">vv</h1>
          <p className="text-sm text-neutral-600">
            {loading ? "読み込み中…" : `${String(total)} 本`}
          </p>
        </div>

        <div className="flex flex-wrap items-center gap-4">
          <ScanStatus onFinished={onScanFinished} />

          <label className="flex items-center gap-2 text-sm text-neutral-600">
            並び順
            <select
              value={sort}
              onChange={(event) => setSort(event.target.value as VideoSort)}
              className="rounded border border-neutral-300 px-2 py-1 text-sm text-neutral-900"
            >
              {sortLabels.map((option) => (
                <option key={option.value} value={option.value}>
                  {option.label}
                </option>
              ))}
            </select>
          </label>
        </div>
      </header>

      {error !== null && (
        <p className="rounded border border-red-200 bg-red-50 p-3 text-sm text-red-800">
          一覧を取得できません: {error}
        </p>
      )}

      {!loading && items.length === 0 && error === null && <EmptyLibrary />}

      <ul className="grid grid-cols-2 gap-x-4 gap-y-6 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5">
        {items.map((video) => (
          <li key={video.id}>
            <VideoCard video={video} />
          </li>
        ))}
      </ul>

      {/* 観測点。ここが見えたら次のページを読む。 */}
      <div ref={sentinel} aria-hidden className="h-px" />

      {loadingMore && (
        <p className="py-4 text-center text-sm text-neutral-500">読み込み中…</p>
      )}
    </main>
  );
}

/**
 * EmptyLibrary は1本も無いときに、次に取れる操作を示す。
 * 「0 件」とだけ出すと、置き場所が違うのか取り込みが済んでいないのかが
 * 利用者から区別できない。
 */
function EmptyLibrary() {
  return (
    <div className="rounded-lg border border-neutral-200 p-6 text-sm text-neutral-700">
      <p className="mb-2 font-medium text-neutral-900">動画がまだありません</p>
      <p>
        <code className="rounded bg-neutral-100 px-1 py-0.5 font-mono">
          MDM_MEDIA_DIR
        </code>{" "}
        に指定した場所へ動画を置き、「取り込む」を押してください。取り込みは起動直後にも 1
        回自動で走ります。
      </p>
    </div>
  );
}
