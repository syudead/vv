import {
  AlertCircle,
  ChevronDown,
  Ellipsis,
  Layers,
  LoaderCircle,
  Star,
  Unlink,
} from "lucide-react";
import { useCallback, useEffect, useId, useRef, useState } from "react";
import { Link } from "react-router";

import {
  isAborted,
  listVideoVersions,
  makeRepresentativeVersion,
  RequestFailed,
  unbundleVideo,
  type Video,
  type VideoVersions,
} from "../api/client";
import { detailMark, type DetailMark } from "../api/useVideoDetail";
import { errorText, t, type UiText } from "../i18n";
import { cn } from "../lib/cn";
import Button from "../ui/Button";
import IconButton from "../ui/IconButton";
import {
  MenuContent,
  MenuItem,
  MenuRoot,
  MenuSeparator,
  MenuTrigger,
} from "../ui/legacy/Menu";
import { PopoverContent, PopoverRoot, PopoverTrigger } from "../ui/legacy/Popover";
import Skeleton from "../ui/legacy/Skeleton";
import { useToast } from "../ui/legacy/Toast";
import {
  VersionDetailsLine,
  versionDetails,
  versionDetailsText,
} from "../versions/VersionDetails";

/** VersionsNavigation は、一覧の行から別のバージョンへ移るときに渡す値である。 */
export interface VersionsNavigation {
  /** 今の画面の戻り先。移った先の戻り先も変えない。 */
  backTo: string;
  /** 再生中（または見終えた後）なら true。移った先で再生を続ける。 */
  autoplay: boolean;
}

type ListState =
  { kind: "loading" } | { kind: "ready"; versions: VideoVersions } | { kind: "failed" };

/** failureIsStale は、別のタブで先に集まりが変わった失敗（取り直しが要る）かを返す。 */
function failureIsStale(error: unknown): boolean {
  return (
    error instanceof RequestFailed &&
    (error.status === 404 || error.reason === "not_bundled")
  );
}

/**
 * VersionsFact は、集まり（同じ動画の別バージョン）に属する動画の情報の行に置く「3 versions」の
 * 項目と、押すと開くバージョンの一覧である（specs/030-video-versions/ui-design.md「Video page」）。
 *
 * 一覧は開くたびに取り直す。行を押すとそのバージョンのページへ移る。所有者は行のメニューで
 * 代表を替え、集まりから外せる。失敗は一覧の下の 1 行で伝え、浮き出しは閉じない。
 */
export default function VersionsFact({
  video,
  count,
  owner,
  navigation,
  onReplace,
  onRefresh,
  onDissolved,
}: {
  video: Video;
  /** 情報の行に出す本数（`Video.versions.count`）。 */
  count: number;
  owner: boolean;
  navigation: VersionsNavigation;
  /** 今の動画を外した応答で、手元の動画を差し替えさせる。 */
  onReplace: (video: Video, mark: DetailMark) => void;
  /** 動画を取り直させる。 */
  onRefresh: () => void;
  /** 浮き出しが閉じて項目が消えるとき、フォーカスの行き先を呼び出し側に決めさせる。 */
  onDissolved: () => void;
}) {
  const toast = useToast();
  const headingId = useId();
  const [open, setOpen] = useState(false);
  const [list, setList] = useState<ListState>({ kind: "loading" });
  const [pending, setPending] = useState<ReadonlySet<number>>(() => new Set());
  const [failure, setFailure] = useState<UiText | null>(null);
  const listRef = useRef<HTMLUListElement | null>(null);
  // 応答が届いた時点の一覧。続けて外した行の応答が、前の行を消した結果の上に重なるようにする。
  const latestList = useRef(list);
  latestList.current = list;
  const request = useRef<AbortController | null>(null);
  // 開いて最初に一覧が届いたとき、今の動画以外の最初の行へフォーカスを移す。
  const focusFirst = useRef(false);
  // 閉じたあと項目が消えるとき、フォーカスを項目へ戻さない。
  const dissolving = useRef(false);
  const alive = useRef(true);
  useEffect(() => {
    alive.current = true;
    return () => {
      alive.current = false;
      request.current?.abort();
    };
  }, []);

  // 取れた一覧が 2 本未満なら、開いた後に集まりが変わっている（別のタブで外した、見せてよい
  // 範囲が変わった）。項目の本数は古い `Video.versions.count` なので、閉じて動画を取り直し、
  // 項目を消す（ui-design.md「Video page」の `count` 1 と「Unbundle」の残り 1 本と同じ扱い）。
  const onDwindled = useRef<() => void>(() => undefined);

  const load = useCallback(
    (showLoading: boolean) => {
      request.current?.abort();
      const controller = new AbortController();
      request.current = controller;
      if (showLoading) setList({ kind: "loading" });
      listVideoVersions(video.id, controller.signal).then(
        (versions) => {
          if (controller.signal.aborted) return;
          setList({ kind: "ready", versions });
          if (versions.items.length < 2) onDwindled.current();
        },
        (error: unknown) => {
          if (isAborted(error) || controller.signal.aborted) return;
          setList((previous) =>
            previous.kind === "ready" ? previous : { kind: "failed" },
          );
        },
      );
    },
    [video.id],
  );

  // 開いている間に動画を取り直したら（`video` の知らせなど）、一覧も取り直す。
  const loadedFor = useRef<Video | null>(null);
  useEffect(() => {
    if (!open) {
      loadedFor.current = null;
      return;
    }
    if (loadedFor.current === video) return;
    const first = loadedFor.current === null;
    loadedFor.current = video;
    if (first) focusFirst.current = true;
    load(first);
  }, [load, open, video]);

  useEffect(() => {
    if (list.kind !== "ready" || !focusFirst.current) return;
    focusFirst.current = false;
    const root = listRef.current;
    const target =
      root?.querySelector<HTMLElement>("a[href]") ?? root?.querySelector("button");
    target?.focus();
  }, [list]);

  const changeOpen = (next: boolean) => {
    setOpen(next);
    if (!next) {
      setFailure(null);
      request.current?.abort();
    }
  };

  const dissolve = () => {
    dissolving.current = true;
    changeOpen(false);
    onDissolved();
  };

  onDwindled.current = () => {
    dissolve();
    onRefresh();
  };

  const run = (target: Video, action: "representative" | "remove") => {
    if (pending.has(target.id)) return;
    setFailure(null);
    setPending((previous) => new Set(previous).add(target.id));
    const mark = detailMark();
    const index =
      list.kind === "ready"
        ? list.versions.items.findIndex((item) => item.id === target.id)
        : -1;
    const done = () =>
      setPending((previous) => {
        const next = new Set(previous);
        next.delete(target.id);
        return next;
      });
    const onFailure = (error: unknown) => {
      if (!alive.current) return;
      done();
      setFailure(t.player.versions.changeFailed(errorText(error)));
      // 動画を取り直すと、開いている一覧も取り直す（上の effect）。
      if (failureIsStale(error)) onRefresh();
    };

    if (action === "representative") {
      makeRepresentativeVersion(target.id).then((versions) => {
        if (!alive.current) return;
        done();
        setList({ kind: "ready", versions });
        onRefresh();
      }, onFailure);
      return;
    }

    unbundleVideo(target.id).then((removed) => {
      if (!alive.current) return;
      done();
      toast(t.player.versions.removed(target.title));
      if (target.id === video.id) {
        dissolve();
        onReplace(removed, mark);
        return;
      }
      const latest = latestList.current;
      const items =
        latest.kind === "ready"
          ? latest.versions.items.filter((item) => item.id !== target.id)
          : [];
      if (latest.kind === "ready") {
        setList({ kind: "ready", versions: { ...latest.versions, items } });
      }
      if (items.length <= 1) {
        dissolve();
        onRefresh();
        return;
      }
      // 開いたままなら、消した行の位置に来た行（無ければ前の行）へフォーカスを移す。
      requestAnimationFrame(() => {
        const rows = listRef.current?.querySelectorAll<HTMLElement>(":scope > li");
        if (rows === undefined || rows.length === 0) return;
        const row = rows[Math.min(Math.max(index, 0), rows.length - 1)];
        row?.querySelector<HTMLElement>("a[href], button")?.focus();
      });
    }, onFailure);
  };

  const shown = list.kind === "ready" ? list.versions.items.length : count;

  return (
    <li className="flex items-center whitespace-nowrap">
      <PopoverRoot open={open} onOpenChange={changeOpen}>
        <PopoverTrigger asChild>
          <button
            type="button"
            aria-label={t.player.versions.show(shown)}
            title={t.player.versions.show(shown)}
            className="-mx-1 flex items-center gap-1.5 rounded-sm px-1 text-fg-muted transition-colors hover:bg-hover-wash hover:text-fg"
          >
            <Layers className="size-4 shrink-0 text-fg-subtle" aria-hidden="true" />
            <span className="tabular-nums">{t.player.versions.count(shown)}</span>
            <ChevronDown className="size-3.5 shrink-0" aria-hidden="true" />
          </button>
        </PopoverTrigger>
        <PopoverContent
          side="bottom"
          align="start"
          aria-labelledby={headingId}
          className="w-[min(28rem,calc(100vw-2rem))] overflow-hidden p-0"
          onOpenAutoFocus={(event) => {
            // 一覧が届いてから最初の別のバージョンの行へ移す。
            event.preventDefault();
          }}
          onCloseAutoFocus={(event) => {
            if (!dissolving.current) return;
            dissolving.current = false;
            event.preventDefault();
          }}
        >
          <h2 id={headingId} className="sr-only">
            {t.player.versions.label}
          </h2>
          {list.kind === "loading" && (
            <div className="flex flex-col gap-1 p-2" aria-busy="true">
              {Array.from({ length: Math.max(count, 1) }, (_, index) => (
                <Skeleton key={index} className="h-10" />
              ))}
            </div>
          )}
          {list.kind === "failed" && (
            <div className="flex items-center gap-2 px-3 py-2">
              <p role="alert" className="text-xs text-danger">
                {t.player.versions.loadFailed}
              </p>
              <Button variant="ghost" size="sm" onClick={() => load(true)}>
                {t.player.versions.retry}
              </Button>
            </div>
          )}
          {list.kind === "ready" && (
            <ul
              ref={listRef}
              aria-label={t.player.versions.label}
              className="max-h-80 divide-y divide-border overflow-y-auto"
            >
              {list.versions.items.map((item) => (
                <VersionRow
                  key={item.id}
                  item={item}
                  current={item.id === video.id}
                  representative={item.id === list.versions.representativeId}
                  owner={owner}
                  pending={pending.has(item.id)}
                  navigation={navigation}
                  onMakeRepresentative={() => run(item, "representative")}
                  onRemove={() => run(item, "remove")}
                />
              ))}
            </ul>
          )}
          {failure !== null && (
            <p
              role="alert"
              className="flex items-center gap-2 border-t border-border px-3 py-2 text-xs text-danger"
            >
              <AlertCircle className="size-4 shrink-0" aria-hidden="true" />
              {failure}
            </p>
          )}
        </PopoverContent>
      </PopoverRoot>
    </li>
  );
}

function VersionRow({
  item,
  current,
  representative,
  owner,
  pending,
  navigation,
  onMakeRepresentative,
  onRemove,
}: {
  item: Video;
  current: boolean;
  representative: boolean;
  owner: boolean;
  pending: boolean;
  navigation: VersionsNavigation;
  onMakeRepresentative: () => void;
  onRemove: () => void;
}) {
  const details = versionDetails(item);
  // 所有者は行の title で絶対パスを読める。ゲストは相対の置き場所だけ
  // （specs/016-single-account-auth/ui-design.md「Guest degradation」）。
  const fullPlace = owner ? (item.location?.path ?? details.place) : details.place;
  const detailsTitle = versionDetailsText(details, " · ", fullPlace);
  const spoken = versionDetailsText(details, " ");

  const text = (
    <>
      <span className="flex min-w-0 items-baseline gap-2">
        {current && <span className="sr-only">{t.player.versions.nowPlaying} </span>}
        <span className="min-w-0 truncate text-sm text-fg" title={item.title}>
          {item.title}
        </span>
        {representative && (
          <span className="shrink-0 text-xs text-fg-muted">
            {t.player.versions.representative}
          </span>
        )}
      </span>
      <VersionDetailsLine details={details} title={detailsTitle} />
    </>
  );

  return (
    <li
      aria-current={current ? "true" : undefined}
      className={cn(
        "grid grid-cols-[minmax(0,1fr)_auto] items-center gap-x-2 pr-2",
        current && "border-l-2 border-primary bg-active-wash",
      )}
    >
      {current ? (
        <div className="flex min-w-0 flex-col gap-0.5 py-2 pl-2.5">{text}</div>
      ) : (
        <Link
          to={`/videos/${String(item.id)}`}
          state={{ from: navigation.backTo, autoplay: navigation.autoplay }}
          aria-label={t.player.versions.play(item.title, spoken)}
          className="flex min-w-0 flex-col gap-0.5 py-2 pl-3 transition-colors hover:bg-hover-wash focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-link"
        >
          {text}
        </Link>
      )}
      {owner && (
        <RowActions
          title={item.title}
          representative={representative}
          pending={pending}
          onMakeRepresentative={onMakeRepresentative}
          onRemove={onRemove}
        />
      )}
    </li>
  );
}

function RowActions({
  title,
  representative,
  pending,
  onMakeRepresentative,
  onRemove,
}: {
  title: string;
  representative: boolean;
  pending: boolean;
  onMakeRepresentative: () => void;
  onRemove: () => void;
}) {
  const label = t.player.versions.more(title);
  const [menuOpen, setMenuOpen] = useState(false);
  // 送っている間も同じボタンを残し、フォーカスを失わせない。メニューは開かない。
  return (
    <MenuRoot
      open={menuOpen && !pending}
      onOpenChange={(next) => setMenuOpen(next && !pending)}
    >
      <MenuTrigger asChild>
        <IconButton
          label={label}
          size="sm"
          tooltip={false}
          aria-disabled={pending || undefined}
          className="text-fg-muted! hover:text-fg! aria-disabled:cursor-default"
        >
          {pending ? (
            <LoaderCircle
              aria-hidden="true"
              className="animate-spin motion-reduce:animate-none"
            />
          ) : (
            <Ellipsis aria-hidden="true" />
          )}
        </IconButton>
      </MenuTrigger>
      <MenuContent>
        {!representative && (
          <>
            <MenuItem
              onSelect={() => {
                setMenuOpen(false);
                onMakeRepresentative();
              }}
            >
              <Star aria-hidden="true" />
              {t.player.versions.makeRepresentative}
            </MenuItem>
            <MenuSeparator />
          </>
        )}
        <MenuItem
          onSelect={() => {
            setMenuOpen(false);
            onRemove();
          }}
        >
          <Unlink aria-hidden="true" />
          {t.player.versions.remove}
        </MenuItem>
      </MenuContent>
    </MenuRoot>
  );
}
