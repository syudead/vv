import {
  AlertCircle,
  CalendarPlus,
  Camera,
  Clock,
  Copy,
  ExternalLink,
  FileClock,
  HardDrive,
  Image,
  LoaderCircle,
  type LucideIcon,
  PencilLine,
  X,
} from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";

import {
  isAborted,
  openVideoFile,
  RequestFailed,
  setVideoThumbnailPosition,
  type Video,
} from "../api/client";
import { detailMark, type DetailMark } from "../api/useVideoDetail";
import { errorText, formatDate, formatDateTime, t, type UiText } from "../i18n";
import { copyText } from "../lib/clipboard";
import { cn } from "../lib/cn";
import { formatBytes, formatDuration } from "../lib/format";
import IconButton from "../ui/IconButton";
import FavoriteToggle from "../ui/FavoriteToggle";
import { PopoverContent, PopoverRoot, PopoverTrigger } from "../ui/legacy/Popover";
import { useToast } from "../ui/legacy/Toast";
import { technicalSummary } from "./properties";
import VersionsFact, { type VersionsNavigation } from "./VersionsFact";

/**
 * useOpenFile はサーバーの PC で動画ファイルを開き、開けなかったときの文言を持つ。
 * 文言は API エラーの表示（errorText）を埋め込む。ファイルが無い（`file_missing`）、
 * サーバーの PC からでない（`open_not_local`）などの理由がそこに出る。
 * 文言は、次に開く操作をしたとき、または別の動画へ移ったときに消える。
 */
export function useOpenFile(videoId: number): {
  open: () => void;
  failure: UiText | null;
} {
  const [failure, setFailure] = useState<UiText | null>(null);
  useEffect(() => setFailure(null), [videoId]);
  const open = useOpenFileRequest(videoId, setFailure);
  return { open, failure };
}

/**
 * useOpenFileRequest は「ファイルを開く」の要求を送り、始めるときに `report(null)`、
 * 開けなかったときにその文言を `report` へ渡す。失敗の行を他の操作と分け合う呼び出し側
 * （VideoFacts）が、行の中身を自分で持つために使う。
 */
function useOpenFileRequest(
  videoId: number,
  report: (failure: UiText | null) => void,
): () => void {
  const request = useRef<AbortController | null>(null);

  useEffect(() => () => request.current?.abort(), [videoId]);

  return useCallback(() => {
    report(null);
    request.current?.abort();
    const controller = new AbortController();
    request.current = controller;
    void openVideoFile(videoId, controller.signal).catch((error: unknown) => {
      if (controller.signal.aborted) return;
      report(t.player.facts.openFailed(errorText(error)));
    });
  }, [report, videoId]);
}

/**
 * useThumbnailPosition は代表サムネイルの位置の指定と解除を送る
 * （specs/029-video-overrides/ui-design.md「Thumbnail fact」）。送信中は次の要求を送らない。
 * 始めるときに `report(null)`、失敗したときにその文言を `report` へ渡し、`404`
 * （`video_not_found`・`file_unavailable`）では `onStale` で動画を取り直させる。
 * 成功したら応答の動画を、送る直前に取った detailMark と一緒に `onChanged` で渡す。
 */
function useThumbnailPosition(
  videoId: number,
  report: (failure: UiText | null) => void,
  onChanged: ((video: Video, mark: DetailMark) => void) | undefined,
  onStale: (() => void) | undefined,
): {
  sending: "capture" | "clear" | null;
  send: (positionMs: number | null) => void;
} {
  const [sending, setSending] = useState<"capture" | "clear" | null>(null);
  // 同じ瞬間の二度押しも止めるため、状態とは別に持つ。
  const busy = useRef(false);
  const request = useRef<AbortController | null>(null);

  useEffect(
    () => () => {
      request.current?.abort();
      busy.current = false;
      setSending(null);
    },
    [videoId],
  );

  const send = (positionMs: number | null) => {
    if (busy.current) return;
    busy.current = true;
    const controller = new AbortController();
    request.current = controller;
    const mark = detailMark();
    report(null);
    setSending(positionMs === null ? "clear" : "capture");
    setVideoThumbnailPosition(videoId, positionMs, controller.signal)
      .then(
        (video) => {
          if (controller.signal.aborted) return;
          onChanged?.(video, mark);
        },
        (error: unknown) => {
          if (isAborted(error) || controller.signal.aborted) return;
          report(t.player.facts.thumbnailFailed(errorText(error)));
          if (error instanceof RequestFailed && error.status === 404) onStale?.();
        },
      )
      .finally(() => {
        if (request.current !== controller) return;
        request.current = null;
        busy.current = false;
        setSending(null);
      });
  };

  return { sending, send };
}

/**
 * ThumbnailCapture は「今の場面を代表サムネイルにする」ボタンに渡すプレイヤーの状態である。
 * `enabled` は押せるか、`positionMs` は押した瞬間の論理上の再生位置（ミリ秒）を返す。
 */
export interface ThumbnailCapture {
  enabled: boolean;
  positionMs: () => number | null;
}

function Fact({
  icon: Icon,
  label,
  value,
}: {
  icon: LucideIcon;
  label: UiText;
  value: string;
}) {
  return (
    <li title={label} className="flex items-center gap-1.5 whitespace-nowrap">
      <Icon className="size-4 shrink-0 text-fg-subtle" aria-hidden="true" />
      <span className="sr-only">{label} </span>
      {value}
    </li>
  );
}

/**
 * DateFact は情報の行の日付の項目（追加日・更新日時・作成日時）である
 * （specs/033-video-dates/ui-design.md「Video facts」）。値は日付だけで、名前と時刻は
 * `title` と、押して開く吹き出しで読む（ポイントできない端末でも届くように）。
 * 見た目は `Fact` と同じで、引き金であることは hover の地だけで示す。
 */
function DateFact({
  icon: Icon,
  label,
  value,
}: {
  icon: LucideIcon;
  label: UiText;
  value: string;
}) {
  const date = formatDate(value);
  const dateTime = formatDateTime(value);
  return (
    <li className="flex items-center whitespace-nowrap">
      <PopoverRoot>
        <PopoverTrigger asChild>
          <button
            type="button"
            // 読み上げ名は隠した名前 + 値（ui-design「Accessibility」）。中の文字の空白の扱いに
            // 依らないよう、明示する。
            aria-label={`${label} ${date}`}
            title={t.player.facts.dateDetail(label, dateTime)}
            className="-mx-1 flex items-center gap-1.5 rounded-sm px-1 text-fg-muted transition-colors hover:bg-hover-wash hover:text-fg"
          >
            <Icon className="size-4 shrink-0 text-fg-subtle" aria-hidden="true" />
            <span className="sr-only">{label} </span>
            {date}
          </button>
        </PopoverTrigger>
        <PopoverContent
          side="bottom"
          align="start"
          className="w-auto px-3 py-2 text-sm whitespace-nowrap"
          // 中にフォーカスできるものは無い。開いてもフォーカスは引き金に置いたままにする。
          onOpenAutoFocus={(event) => event.preventDefault()}
        >
          <span className="text-fg">{label}</span>{" "}
          <span className="text-fg-muted tabular-nums">{dateTime}</span>
        </PopoverContent>
      </PopoverRoot>
    </li>
  );
}

/**
 * VideoFacts は題名とタグの下の 2 行である（ui-design「Video facts」）。
 *
 * 1 行目はファイルの情報（長さ・サイズ・追加日・更新日時・作成日時）をアイコンを添えて並べ、右端に
 * 「ファイルを開く」（開ける環境のときだけ）と「パスをコピー」を置く。2 行目は技術情報
 * （解像度・コンテナ・コーデック）で、いちばん小さく薄い文字にする。
 *
 * 所有者には、代表サムネイルの位置が指定されていれば 1 行目の 4 つ目の項目にその画像・位置・
 * 解除の × を置き、`capture` があれば右端の操作の先頭に「今の場面を代表サムネイルにする」
 * ボタンを置く（specs/029-video-overrides/ui-design.md「Thumbnail fact」）。
 *
 * 集まり（同じ動画の別バージョン）に属し、見せてよいバージョンが 2 本以上あれば、1 行目の
 * 追加日のあと（サムネイルの項目の前）に「3 versions」の項目を置く。所有者にもゲストにも出す
 * （specs/030-video-versions/ui-design.md「Versions fact」）。
 *
 * 所有者には、右端の操作の先頭（撮るボタンの左）にお気に入りの付け外しを置く
 * （specs/035-favorites/ui-design.md「Video page」）。ゲストの応答には `favorite` が無く、出ない。
 * 付け外しは `onFavorite` に任せ、それが取り直しまで終えたら送信中を解く。
 *
 * 開けなかったとき・サムネイルを変えられなかったとき・お気に入りを変えられなかったときは、
 * 1 行目のすぐ下に 1 行だけ出す。
 * 後から起きた失敗が前の行を置き換える。帯やトーストは使わない。
 */
export default function VideoFacts({
  video,
  owner = false,
  capture,
  onChanged,
  onStale,
  onFavorite,
  versions,
}: {
  video: Video;
  owner?: boolean;
  /** 所有者でプレイヤーが出ているときだけ渡す。 */
  capture?: ThumbnailCapture;
  onChanged?: (video: Video, mark: DetailMark) => void;
  onStale?: () => void;
  /** お気に入りを付け外しし、動画を取り直したら解決する。失敗は reject で返す。 */
  onFavorite?: (favorite: boolean) => Promise<void>;
  /** バージョンの一覧から別のバージョンへ移るときの値と、集まりが変わったときの動作。 */
  versions?: {
    navigation: VersionsNavigation;
    onReplace: (video: Video, mark: DetailMark) => void;
    onRefresh: () => void;
  };
}) {
  const [failure, setFailure] = useState<UiText | null>(null);
  useEffect(() => setFailure(null), [video.id]);
  const open = useOpenFileRequest(video.id, setFailure);
  const thumbnail = useThumbnailPosition(video.id, setFailure, onChanged, onStale);
  const toast = useToast();
  const location = video.location;
  const duration = formatDuration(video.durationMs);
  const technical = technicalSummary(video);
  const actionsRef = useRef<HTMLDivElement | null>(null);
  const versionCount = video.versions?.count ?? 0;
  const favorite = owner && onFavorite !== undefined ? video.favorite : undefined;

  const copyPath = () => {
    if (location === undefined) return;
    // 前の失敗の行は、次の操作で消す（specs/035-favorites/ui-design.md「Video page」）。
    setFailure(null);
    copyText(location.path, {
      onCopied: () => toast(t.player.facts.pathCopied),
      onFailed: () => toast(t.player.facts.copyFailed),
    });
  };

  return (
    <div className="flex flex-col gap-3 border-t border-border pt-3">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
        <ul
          aria-label={t.player.facts.label}
          className="flex min-w-0 flex-1 flex-wrap items-center gap-x-4 sm:gap-x-5 gap-y-2 text-sm text-fg-muted tabular-nums"
        >
          {duration !== "" && (
            <Fact icon={Clock} label={t.player.facts.duration} value={duration} />
          )}
          <Fact
            icon={HardDrive}
            label={t.player.facts.size}
            value={formatBytes(video.sizeBytes)}
          />
          <DateFact
            icon={CalendarPlus}
            label={t.player.facts.added}
            value={video.addedAt}
          />
          <DateFact
            icon={PencilLine}
            label={t.player.facts.edited}
            value={video.updatedAt}
          />
          <DateFact
            icon={FileClock}
            label={t.player.facts.created}
            value={video.fileCreatedAt}
          />
          {versions !== undefined && versionCount >= 2 && (
            <VersionsFact
              video={video}
              count={versionCount}
              owner={owner}
              navigation={versions.navigation}
              onReplace={versions.onReplace}
              onRefresh={versions.onRefresh}
              // 項目が消えるので、フォーカスを右端の操作の先頭へ移す。
              onDissolved={() =>
                actionsRef.current?.querySelector<HTMLElement>("button")?.focus()
              }
            />
          )}
          {owner && video.thumbnailPositionMs !== undefined && (
            <ThumbnailFact
              url={video.thumbnailUrl}
              positionMs={video.thumbnailPositionMs}
              clearing={thumbnail.sending === "clear"}
              onClear={() => thumbnail.send(null)}
            />
          )}
        </ul>
        {(location !== undefined ||
          (owner && capture !== undefined) ||
          favorite !== undefined) && (
          <div ref={actionsRef} className="ml-auto flex shrink-0 items-center">
            {favorite !== undefined && (
              <FavoriteToggle
                variant="page"
                favorite={favorite}
                label={t.player.facts.favorite}
                onToggle={async () => {
                  setFailure(null);
                  // 塗りは取り直した `favorite` で確かめる（R-6、033「Refresh after edits」）。
                  // 取り直しが終わるまで送信中のままにし、前の状態から重ねて送らない。
                  await onFavorite?.(!favorite);
                }}
                onFailed={(reason) => setFailure(t.player.facts.favoriteFailed(reason))}
              />
            )}
            {owner && capture !== undefined && (
              <CaptureButton
                capture={capture}
                sending={thumbnail.sending === "capture"}
                onCapture={(positionMs) => thumbnail.send(positionMs)}
              />
            )}
            {location?.openable === true && (
              <IconButton
                label={t.player.facts.openFile}
                size="sm"
                onClick={open}
                className="text-fg-muted! hover:text-fg!"
              >
                <ExternalLink aria-hidden="true" />
              </IconButton>
            )}
            {location !== undefined && (
              <IconButton
                label={t.player.facts.copyPath}
                size="sm"
                onClick={copyPath}
                className="text-fg-muted! hover:text-fg!"
              >
                <Copy aria-hidden="true" />
              </IconButton>
            )}
          </div>
        )}
      </div>
      {failure !== null && (
        <p role="alert" className="-mt-1 flex items-center gap-2 text-sm text-danger">
          <AlertCircle className="size-4 shrink-0" aria-hidden="true" />
          {failure}
        </p>
      )}
      <TechnicalLine summary={technical} />
    </div>
  );
}

/**
 * ThumbnailFact は情報の行の 4 つ目の項目で、指定した代表サムネイルの小さな画像・位置・
 * 解除の × を並べる（ui-design「Item」）。画像を読み込めない間・失敗したときは空の箱にする。
 */
function ThumbnailFact({
  url,
  positionMs,
  clearing,
  onClear,
}: {
  url: string | undefined;
  positionMs: number;
  clearing: boolean;
  onClear: () => void;
}) {
  const [brokenUrl, setBrokenUrl] = useState<string | null>(null);
  const label = t.player.facts.thumbnailAt(formatDuration(positionMs));
  return (
    <li title={label} className="flex items-center gap-1.5 whitespace-nowrap">
      <Image className="size-4 shrink-0 text-fg-subtle" aria-hidden="true" />
      <span className="aspect-video h-6 shrink-0 overflow-hidden rounded-sm bg-surface">
        {url !== undefined && url !== brokenUrl && (
          <img
            src={url}
            alt={label}
            onError={() => setBrokenUrl(url)}
            className="size-full object-cover"
          />
        )}
      </span>
      <span className="sr-only">{label} </span>
      {formatDuration(positionMs)}
      <button
        type="button"
        aria-label={t.player.facts.useAutomaticThumbnail}
        title={t.player.facts.useAutomaticThumbnail}
        aria-disabled={clearing || undefined}
        onClick={() => {
          if (!clearing) onClear();
        }}
        className="flex size-6 shrink-0 items-center justify-center rounded-sm text-fg-muted hover:bg-hover-wash hover:text-fg aria-disabled:cursor-default"
      >
        {clearing ? (
          <LoaderCircle
            className="size-3 animate-spin motion-reduce:animate-none"
            aria-hidden="true"
          />
        ) : (
          <X className="size-3" aria-hidden="true" />
        )}
      </button>
    </li>
  );
}

/**
 * CaptureButton は、押した瞬間の論理上の再生位置を代表サムネイルにするボタンである
 * （ui-design「Capture button」）。再生は止めず、時刻を入力させない。押せないとき・
 * 送信中は `aria-disabled` にし、押しても送らない。
 */
function CaptureButton({
  capture,
  sending,
  onCapture,
}: {
  capture: ThumbnailCapture;
  sending: boolean;
  onCapture: (positionMs: number) => void;
}) {
  const unavailable = !capture.enabled && !sending;
  return (
    <IconButton
      label={t.player.facts.useCurrentFrame}
      size="sm"
      aria-disabled={unavailable || sending || undefined}
      onClick={() => {
        if (!capture.enabled || sending) return;
        const positionMs = capture.positionMs();
        if (positionMs === null || !Number.isFinite(positionMs)) return;
        onCapture(Math.max(0, Math.round(positionMs)));
      }}
      className={cn(
        "text-fg-muted! hover:text-fg! aria-disabled:cursor-default",
        unavailable && "opacity-50",
      )}
    >
      {sending ? (
        <LoaderCircle
          aria-hidden="true"
          className="animate-spin motion-reduce:animate-none"
        />
      ) : (
        <Camera aria-hidden="true" />
      )}
    </IconButton>
  );
}

function TechnicalLine({ summary }: { summary: ReturnType<typeof technicalSummary> }) {
  const base = "text-xs tracking-wider tabular-nums";
  if (summary.kind === "pending") {
    return <p className={cn(base, "text-fg-muted")}>{t.player.facts.technicalPending}</p>;
  }
  if (summary.kind === "failed") {
    return <p className={cn(base, "text-warning")}>{t.player.facts.technicalFailed}</p>;
  }
  if (summary.values.length === 0) return null;
  // どの項目も左に縦線と余白を持ち、並び全体をその幅だけ左へずらして外側で切る。
  // 折り返した行の先頭の項目も、線と余白が切り落とされて行頭に残らない。
  return (
    <div className="overflow-hidden">
      <ul
        aria-label={t.player.facts.technical}
        lang="en"
        className={cn(
          base,
          "-ml-[calc(0.625rem+1px)] flex flex-wrap items-center gap-y-1 text-fg-muted uppercase",
        )}
      >
        {summary.values.map((value) => (
          <li key={value} className="border-l border-border-strong px-2.5 leading-none">
            {value}
          </li>
        ))}
      </ul>
    </div>
  );
}
