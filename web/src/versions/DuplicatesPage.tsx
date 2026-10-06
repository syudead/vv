import { Layers } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import { Link } from "react-router";

import {
  dismissVersionCandidate,
  isAborted,
  listVersionCandidates,
  RequestFailed,
  type VersionCandidate,
  type Video,
} from "../api/client";
import { subscribeServerEvents } from "../api/serverEvents";
import { errorText, t } from "../i18n";
import { inProgress } from "../shell/ScanProvider";
import { VideoThumbnail, videoLinkLabel } from "../player/RelatedVideos";
import { EmptyState } from "../ui/patterns/empty-state";
import { ErrorState } from "../ui/patterns/error-state";
import { ListPage } from "../ui/patterns/list-page";
import { PageHeader } from "../ui/patterns/page-header";
import { Button } from "../ui/shadcn/button";
import { Skeleton } from "../ui/shadcn/skeleton";
import { Spinner } from "../ui/shadcn/spinner";
import { useToast } from "../ui/Toast";
import BundleDialog from "./BundleDialog";
import { VersionDetailsLine, versionDetails, versionDetailsText } from "./VersionDetails";

/** 候補の画面から開いた再生画面の戻り先（ui-design.md「Pair」）。 */
const backTo = "/duplicates";

/** Pair は候補の 1 組である。`videos` は id の昇順（contracts/screen-api.md §5）。 */
interface Pair {
  key: string;
  videos: readonly [Video, Video];
}

type Page =
  | { kind: "loading" }
  | { kind: "failed" }
  | { kind: "ready"; items: Pair[]; total: number };

/** toPair は応答の組を画面の形にする。2 本でない組は並べない。 */
function toPair(candidate: VersionCandidate): Pair | null {
  const [first, second] = candidate.videos;
  if (first === undefined || second === undefined || candidate.videos.length !== 2)
    return null;
  return { key: `${String(first.id)}-${String(second.id)}`, videos: [first, second] };
}

function pairIds(pair: Pair): [number, number] {
  return [pair.videos[0].id, pair.videos[1].id];
}

/**
 * DuplicatesPage は「同じ動画かもしれない」候補の組を並べ、所有者に「同じ動画」（代表を選んで
 * 束ねる）か「違う動画」（二度と出さない）かを決めさせる画面である
 * （specs/030-video-versions/ui-design.md「Duplicates page」）。
 *
 * 開いたときに候補を読み、取り込みが終わったという `scan` の知らせで取り直す。取り直しの間は一覧の見た目を変えず、
 * 届いたら差し替える。取り直しが重なったら、今の取得が終わってからもう 1 度だけ取る。
 * 決めた組はその場で一覧から消し、フォーカスを次の組（無ければ前の組、無ければ見出し）へ移す。
 */
export default function DuplicatesPage() {
  const toast = useToast();
  const [page, setPage] = useState<Page>({ kind: "loading" });
  const [dismissing, setDismissing] = useState<ReadonlySet<string>>(new Set());
  const [bundling, setBundling] = useState<Pair | null>(null);
  const heading = useRef<HTMLHeadingElement>(null);
  const differentButtons = useRef(new Map<string, HTMLButtonElement>());
  // 送信の応答は後から届くので、そのときの一覧を読めるようにする。描画を待たずに ref も
  // 更新するので、同じ描画の前に重なって届いた決定（別々の組の「Different videos」）も
  // 互いの結果の上に重なり、先に消した組を戻さない。
  const pageRef = useRef(page);
  const showPage = useCallback((next: Page) => {
    pageRef.current = next;
    setPage(next);
  }, []);

  useEffect(() => {
    const previous = document.title;
    document.title = t.versions.duplicates.documentTitle;
    return () => {
      document.title = previous;
    };
  }, []);

  // 取得は 1 本ずつ。重なった取り直しは again に畳む。
  const fetching = useRef<AbortController | null>(null);
  const again = useRef(false);
  // 取得の間に決めた組。取得が決める前の一覧を返しても、その組を戻さない。
  const decidedDuringFetch = useRef(new Set<string>());

  const load = useCallback(() => {
    if (fetching.current !== null) {
      again.current = true;
      return;
    }
    const controller = new AbortController();
    fetching.current = controller;
    again.current = false;
    decidedDuringFetch.current = new Set();
    listVersionCandidates(controller.signal).then(
      (result) => {
        if (controller.signal.aborted) return;
        fetching.current = null;
        const decided = decidedDuringFetch.current;
        const pairs = result.items
          .map(toPair)
          .filter((pair): pair is Pair => pair !== null);
        const items = pairs.filter((pair) => !decided.has(pair.key));
        const dropped = pairs.length - items.length;
        let total = Math.max(0, result.total - dropped);
        if (items.length === 0 && total > 0) {
          // 見せる組が尽きたが、上限 200 件の外に組が残っている。取得の間に決めた組で
          // 尽きたのなら取り直して補う。応答が組を返さなかったのなら、見せる組は無い。
          if (dropped > 0) again.current = true;
          else total = 0;
        }
        showPage({ kind: "ready", items, total });
        if (again.current) load();
      },
      (error: unknown) => {
        if (isAborted(error) || controller.signal.aborted) return;
        fetching.current = null;
        // 取り直しの失敗では今の一覧を残す。見せる組が無いときの失敗だけを画面に出す。
        const current = pageRef.current;
        if (current.kind !== "ready" || current.items.length === 0)
          showPage({ kind: "failed" });
        if (again.current) load();
      },
    );
  }, [showPage]);

  useEffect(() => {
    // 取り込みの知らせは、取り込み中は1ファイルごとに届き、つないだ直後にも届く。候補は
    // 取り込みが終わったときに1度取り直せば足りるので、終わったとき（進行中から終わりへ、
    // または別の取り込みが終わった状態で届いたとき）だけ取り直す。購読して最初の知らせは
    // 開いたときの取得と同じ内容なので取り直さない（issue 674）。
    let lastScan: { id: number; active: boolean } | undefined;
    // 購読してから最初の取得をすると、取得のあとに起きた変化を取りこぼさない。
    const unsubscribe = subscribeServerEvents({
      scan: (scan) => {
        const active = inProgress(scan);
        const previous = lastScan;
        lastScan = { id: scan.id, active };
        if (previous === undefined || active) return;
        if (previous.active || previous.id !== scan.id) load();
      },
      open: (reconnected) => {
        if (reconnected) load();
      },
    });
    load();
    return () => {
      unsubscribe();
      fetching.current?.abort();
      fetching.current = null;
    };
  }, [load]);

  function retry() {
    showPage({ kind: "loading" });
    load();
  }

  /**
   * removePair は決めた組を一覧から消し、件数を減らし、フォーカスを次の組の「Different
   * videos」へ移す。無ければ前の組、1 組も無ければ見出しへ。窓を閉じたときの戻りのフォーカス
   * （Dialog が閉じたときに戻す）より後に移すため、タイマーで待つ。
   *
   * 見せていた組が尽きても `total` が残っていれば（候補が上限 200 件を超えていた）、一覧を
   * 取り直して残りの組を出す。取り直しの間は読み込み中と同じ見え方にする。
   */
  function removePair(key: string) {
    decidedDuringFetch.current.add(key);
    const page = pageRef.current;
    if (page.kind !== "ready") return;
    const index = page.items.findIndex((item) => item.key === key);
    if (index < 0) return;
    const items = page.items.filter((_, position) => position !== index);
    const total = Math.max(0, page.total - 1);
    showPage({ kind: "ready", items, total });
    if (items.length === 0 && total > 0) load();
    const next = items[index] ?? items[index - 1];
    const nextKey = next === undefined ? undefined : next.key;
    setTimeout(() => {
      const target =
        nextKey === undefined ? heading.current : differentButtons.current.get(nextKey);
      target?.focus();
    }, 0);
  }

  function dismiss(pair: Pair) {
    const key = pair.key;
    if (dismissing.has(key)) return;
    setDismissing((current) => new Set(current).add(key));
    const settle = () =>
      setDismissing((current) => {
        const rest = new Set(current);
        rest.delete(key);
        return rest;
      });
    dismissVersionCandidate(pairIds(pair)).then(
      () => {
        settle();
        removePair(key);
        toast(t.versions.duplicates.dismissed);
      },
      (error: unknown) => {
        settle();
        if (error instanceof RequestFailed && error.status === 404) {
          // 確かめている間に片方がスキャンで消えた（Edge Case）。
          toast(t.versions.duplicates.gone);
          load();
          return;
        }
        toast(errorText(error));
      },
    );
  }

  // 見せていた組が尽き、上限の外に残る組を取り直している間。
  const refilling = page.kind === "ready" && page.items.length === 0 && page.total > 0;
  const loading = page.kind === "loading" || refilling;
  const count = loading
    ? t.versions.duplicates.loading
    : page.kind === "ready"
      ? t.versions.duplicates.count(page.items.length, page.total)
      : null;
  const empty = page.kind === "ready" && page.items.length === 0 && page.total === 0;

  return (
    <ListPage
      header={
        <PageHeader
          titleRef={heading}
          title={t.versions.duplicates.title}
          // 0 組のときは件数を空にし、空の状態の題が件数の代わりになる。
          count={
            <span role="status" aria-live="polite">
              {empty ? "" : count}
            </span>
          }
        />
      }
    >
      {loading && (
        <div className="flex flex-col gap-3" aria-hidden="true">
          {Array.from({ length: 3 }, (_, index) => (
            <Skeleton key={index} className="h-16 w-full" />
          ))}
        </div>
      )}

      {page.kind === "failed" && (
        <ErrorState
          title={<h2>{t.versions.duplicates.loadFailed}</h2>}
          retryLabel={t.common.retry}
          onRetry={retry}
        />
      )}

      {empty && (
        <EmptyState
          icon={<Layers aria-hidden="true" />}
          title={<h2>{t.versions.duplicates.empty.title}</h2>}
          description={t.versions.duplicates.empty.description}
        />
      )}

      {page.kind === "ready" && page.items.length > 0 && (
        <ul className="flex flex-col divide-y divide-border rounded-md border border-border bg-card text-card-foreground">
          {page.items.map((pair) => {
            const key = pair.key;
            return (
              <PairRow
                key={key}
                pair={pair}
                pending={dismissing.has(key)}
                differentRef={(element) => {
                  if (element === null) differentButtons.current.delete(key);
                  else differentButtons.current.set(key, element);
                }}
                onDifferent={() => dismiss(pair)}
                onSame={() => setBundling(pair)}
              />
            );
          })}
        </ul>
      )}

      {/* 開いている窓の組が取り直しで消えても窓は閉じない。「Bundle」の 404 で伝える。 */}
      {bundling !== null && (
        <BundleDialog
          videoIds={pairIds(bundling)}
          videos={bundling.videos}
          onClose={() => setBundling(null)}
          onBundled={() => {
            const key = bundling.key;
            setBundling(null);
            removePair(key);
          }}
        />
      )}
    </ListPage>
  );
}

function PairRow({
  pair,
  pending,
  differentRef,
  onDifferent,
  onSame,
}: {
  pair: Pair;
  pending: boolean;
  differentRef: (element: HTMLButtonElement | null) => void;
  onDifferent: () => void;
  onSame: () => void;
}) {
  const [first, second] = pair.videos;
  return (
    <li className="flex flex-col gap-3 p-3">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <p className="text-xs text-muted-foreground">{t.versions.duplicates.reason}</p>
        {/* 判断が先、動画の中身は後。DOM の順もこの順にする。 */}
        <div className="ml-auto flex gap-2">
          <Button
            ref={differentRef}
            variant="outline"
            size="sm"
            disabled={pending}
            aria-label={t.versions.duplicates.differentFor(first.title, second.title)}
            onClick={onDifferent}
          >
            {pending && <Spinner aria-hidden="true" />}
            {t.versions.duplicates.different}
          </Button>
          <Button
            size="sm"
            disabled={pending}
            aria-label={t.versions.duplicates.sameFor(first.title, second.title)}
            onClick={onSame}
          >
            <Layers aria-hidden="true" />
            {t.versions.duplicates.same}
          </Button>
        </div>
      </div>
      <div className="grid gap-3 sm:grid-cols-2">
        {pair.videos.map((video) => (
          <CandidateVideo key={video.id} video={video} />
        ))}
      </div>
    </li>
  );
}

/**
 * CandidateVideo は組の 1 本である。サムネイルと、題名・違い・置き場所の 3 行を並べ、全体を
 * 再生画面へのリンクにする。2 本のどちらも強調しない（ui-design.md「Pair」）。
 */
function CandidateVideo({ video }: { video: Video }) {
  const details = versionDetails(video);
  // 置き場所は 3 行目に分ける。2 行目は解像度・コンテナ・コーデック・サイズだけ。
  const specs = { specs: details.specs, place: "" };
  const absolute = video.location?.path ?? details.place;
  return (
    <Link
      to={`/videos/${String(video.id)}`}
      state={{ from: backTo }}
      aria-label={videoLinkLabel(video)}
      className="-m-1.5 flex min-w-0 gap-3 rounded-md p-1.5 transition-colors hover:bg-accent"
    >
      <VideoThumbnail video={video} className="w-list-thumb-cell" />
      <span className="flex min-w-0 flex-1 flex-col gap-1">
        <span
          className="line-clamp-2 text-sm font-medium wrap-anywhere text-foreground"
          title={video.title}
        >
          {video.title}
        </span>
        <VersionDetailsLine details={specs} title={versionDetailsText(specs, " · ")} />
        {details.place !== "" && (
          // 末尾（ファイルに近い側）を優先して残す。
          <span
            dir="rtl"
            className="truncate text-left text-xs text-muted-foreground"
            title={absolute}
          >
            <bdi dir="ltr">{details.place}</bdi>
          </span>
        )}
      </span>
    </Link>
  );
}
