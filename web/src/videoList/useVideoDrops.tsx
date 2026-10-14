import { type ReactNode, useCallback, useEffect, useMemo, useRef, useState } from "react";

import { type LibraryGroup, RequestFailed, type Video } from "../api/client";
import { bundleOnto, groupVideos } from "../api/organize";
import { errorText, t, type UiText } from "../i18n";
import { useToast } from "../ui/Toast";
import type { CardAttachment, CardDrop } from "./cardDrag";
import type { OntoAnswer } from "./cardDragRules";
import { GroupNameField, GroupNamePopover } from "./GroupNameField";
import type { UndoToast } from "./undoToast";
import { useGroupNaming } from "./useGroupNaming";
import {
  applyDropOverlay,
  type DropOverlay,
  provisionalGroup,
  provisionalGroupName,
  videoOnto,
} from "./videoDrop";
import { useVideoDropMenu, type VideoDropChoice } from "./VideoDropMenu";

// 動画を動画に落としたときの「Make a group」と「Bundle as versions」である
// （specs/050-drag-organize/ui-design.md「What a drop does」「Drop menus」「The new group's
// name」、research.md R-6、R-8）。ライブラリ、グループのページ、フォルダ画面が共有する。

/** Shown はサーバーが答える前に見せる落とした結果である。 */
interface Shown {
  target: string;
  lifted: ReadonlySet<string>;
  /** 落とした先に置く新しいグループ。null は持ち上げたカードだけが消える（束ねた）。 */
  group: LibraryGroup | null;
  /** 新しいグループの仮の項目の id（答えの前後で変わらない）。 */
  itemId: number;
}

export interface VideoDropsOptions<T> {
  /** 画面の並び（サーバーの値）。 */
  items: readonly T[];
  keyOf: (item: T) => string;
  /** 項目が動画ならその動画。 */
  videoOf: (item: T) => Video | undefined;
  /** 項目がグループならそのグループ。グループのカードを置く画面（`groupItem` のある画面）が渡す。 */
  groupOf?: (item: T) => LibraryGroup | undefined;
  /** 開いているグループ（グループのページ）。新しいグループはその中で落とした先の位置を取る。 */
  from: number | null;
  /**
   * 新しいグループの項目を作る。`itemId` は答えの前後で変わらない仮の項目の id。null の画面
   * （フォルダ画面）は落とした先にグループを置かず、名前の欄を落とした先の下の窓に出す。
   */
  groupItem: ((group: LibraryGroup, itemId: number) => T) | null;
  /** 一覧を静かに読み直す。 */
  reread: () => void;
  /** サーバーが操作を受け付けたら呼ぶ（選択を解く）。失敗した操作では呼ばず、選択は残る。 */
  onAccepted?: () => void;
  undo: UndoToast;
}

/** isTargetGone は落とした先か持ち上げた動画がもう無い（404）かを返す。 */
function isTargetGone(error: unknown): boolean {
  return error instanceof RequestFailed && error.status === 404;
}

/**
 * overlayOf は見せている結果 `shown` を並びに重ねる値を返す。読み直した並びがもう落とした先を
 * 持たず、代わりに答えで分かった新しいグループを持てば（知らせで読み直した）、仮の項目がその
 * サーバーの項目の位置と中身を取る。項目の鍵が変わらないので、名前の欄は入力ごと残る。
 */
function overlayOf<T>(
  shown: Shown,
  items: readonly T[],
  keyOf: (item: T) => string,
  groupOf: ((item: T) => LibraryGroup | undefined) | undefined,
  groupItem: ((group: LibraryGroup, itemId: number) => T) | null,
): DropOverlay<T> {
  const made = shown.group;
  if (made === null || groupItem === null) {
    return { target: shown.target, lifted: shown.lifted, made: null };
  }
  if (items.some((item) => keyOf(item) === shown.target)) {
    return {
      target: shown.target,
      lifted: shown.lifted,
      made: groupItem(made, shown.itemId),
    };
  }
  const landed = items.find((item) => groupOf?.(item)?.id === made.id);
  const group = landed === undefined ? undefined : groupOf?.(landed);
  if (landed === undefined || group === undefined) {
    return {
      target: shown.target,
      lifted: shown.lifted,
      made: groupItem(made, shown.itemId),
    };
  }
  return {
    target: keyOf(landed),
    lifted: shown.lifted,
    made: groupItem({ ...group, name: made.name }, shown.itemId),
  };
}

/**
 * useVideoDrops は動画を動画に落としたときのメニューと 2 つの操作である。結果はすぐ見せ
 * （`shown`）、失敗したら戻して理由を出し、受け付けたら取り消しの通知を出す。「Make a group」の
 * 後は新しいグループの名前の欄を開き（`nameFieldFor` か、フォルダ画面では `attached` の窓）、
 * 欄が閉じたら読み直す。束ねた後はすぐ読み直す。取り消したら見せていた結果と名前の欄を外す。
 * 画面を離れた後に答えた要求は何も見せない（トークンは忘れる。ui-design.md「Undo toast」の Gone）。
 */
export function useVideoDrops<T>(options: VideoDropsOptions<T>) {
  const { items, keyOf, videoOf, groupOf, from, groupItem, reread, onAccepted, undo } =
    options;
  const toast = useToast();
  const [shownDrop, setShownDrop] = useState<Shown | null>(null);
  const [nameTarget, setNameTarget] = useState<string | null>(null);
  const tempId = useRef(0);
  const awaitingRead = useRef(false);
  const alive = useRef(true);
  useEffect(() => {
    alive.current = true;
    return () => {
      alive.current = false;
    };
  }, []);

  // 読み直した一覧が届いたら、見せていた結果を外す（サーバーの値がそれを持つ）。
  useEffect(() => {
    if (!awaitingRead.current) return;
    awaitingRead.current = false;
    setShownDrop(null);
  }, [items]);

  const readAgain = useCallback(() => {
    awaitingRead.current = true;
    reread();
  }, [reread]);

  const byKey = useMemo(
    () => new Map(items.map((item) => [keyOf(item), item])),
    [items, keyOf],
  );
  const videoAt = useCallback(
    (key: string) => {
      const item = byKey.get(key);
      return item === undefined ? undefined : videoOf(item);
    },
    [byKey, videoOf],
  );

  const naming = useGroupNaming({
    onShowName: (groupId, name) =>
      setShownDrop((current) =>
        current?.group?.id === groupId
          ? { ...current, group: { ...current.group, name } }
          : current,
      ),
    onClosed: () => {
      setNameTarget(null);
      readAgain();
    },
  });

  const failed = useCallback(
    (error: unknown, message: (reason: string) => UiText) => {
      setShownDrop(null);
      toast(message(errorText(error)));
      if (isTargetGone(error)) reread();
    },
    [reread, toast],
  );

  const liftedVideos = useCallback(
    (lifted: readonly string[]) =>
      lifted.flatMap((key) => {
        const video = videoAt(key);
        return video === undefined ? [] : [video];
      }),
    [videoAt],
  );

  const { start, discard } = naming;
  // 取り消しの要求が答えたら、名前の欄を何も送らずに閉じる。取り消せたら見せていた結果も外す
  // （読み直しが元の並びを持ってくる）。取り消せなかったら読み直しを待つ。
  const undone = useCallback(
    (ok: boolean) => {
      discard();
      setNameTarget(null);
      if (ok) setShownDrop(null);
      else awaitingRead.current = true;
    },
    [discard],
  );
  const { offer } = undo;
  const makeGroup = useCallback(
    ({ lifted, target }: VideoDropChoice) => {
      const targetVideo = videoAt(target);
      if (targetVideo === undefined) return;
      const videos = liftedVideos(lifted);
      tempId.current -= 1;
      const made = provisionalGroup(tempId.current, targetVideo, videos);
      if (groupItem !== null) {
        setShownDrop({ target, lifted: new Set(lifted), group: made, itemId: made.id });
      }
      const ids = videos.map((video) => video.id);
      groupVideos(ids, targetVideo.id, from).then(
        ({ undoToken, groupId }) => {
          if (!alive.current) return;
          onAccepted?.();
          offer(t.organize.done.grouped(made.name), undoToken, undone);
          if (groupId === undefined) {
            readAgain();
            return;
          }
          setShownDrop((current) =>
            current === null || current.group === null
              ? current
              : { ...current, group: { ...current.group, id: groupId } },
          );
          setNameTarget(target);
          start({
            groupId,
            provisional: provisionalGroupName(targetVideo.title),
            undoToken,
          });
        },
        (error: unknown) => {
          if (alive.current) failed(error, t.organize.failed.group);
        },
      );
    },
    [
      failed,
      from,
      groupItem,
      liftedVideos,
      offer,
      onAccepted,
      readAgain,
      start,
      undone,
      videoAt,
    ],
  );

  const bundle = useCallback(
    ({ lifted, target }: VideoDropChoice) => {
      const targetVideo = videoAt(target);
      if (targetVideo === undefined) return;
      const ids = liftedVideos(lifted).map((video) => video.id);
      setShownDrop({ target, lifted: new Set(lifted), group: null, itemId: 0 });
      bundleOnto(ids, targetVideo.id).then(
        ({ undoToken }) => {
          if (!alive.current) return;
          onAccepted?.();
          offer(
            t.versions.bundle.bundled(ids.length + 1, targetVideo.title),
            undoToken,
            undone,
          );
          readAgain();
        },
        (error: unknown) => {
          if (alive.current) failed(error, t.versions.bundle.failed);
        },
      );
    },
    [failed, liftedVideos, offer, onAccepted, readAgain, undone, videoAt],
  );

  const isVideo = useCallback((key: string) => videoAt(key) !== undefined, [videoAt]);
  const nameOf = useCallback((key: string) => videoAt(key)?.title ?? "", [videoAt]);
  const menu = useVideoDropMenu({
    isVideo,
    nameOf,
    onGroup: makeGroup,
    onBundle: bundle,
  });

  const onto = useCallback(
    (target: string, lifted: readonly string[]): OntoAnswer<UiText> =>
      isVideo(target) ? videoOnto(lifted.every(isVideo)) : null,
    [isVideo],
  );

  const shown = useMemo(
    () =>
      applyDropOverlay(
        items,
        keyOf,
        shownDrop === null
          ? null
          : overlayOf(shownDrop, items, keyOf, groupOf, groupItem),
      ),
    [groupItem, groupOf, items, keyOf, shownDrop],
  );

  const field = naming.naming;
  const nameFieldFor = useCallback(
    (groupId: number): ReactNode =>
      field === null || field.groupId !== groupId ? undefined : (
        <GroupNameField
          provisional={field.provisional}
          onRename={naming.rename}
          onClose={naming.close}
        />
      ),
    [field, naming.close, naming.rename],
  );

  const attached = useMemo((): CardAttachment | null => {
    if (menu.attached !== null) return menu.attached;
    if (groupItem !== null || field === null || nameTarget === null) return null;
    return {
      key: nameTarget,
      node: (
        <GroupNamePopover
          provisional={field.provisional}
          onRename={naming.rename}
          onClose={naming.close}
        />
      ),
    };
  }, [field, groupItem, menu.attached, nameTarget, naming.close, naming.rename]);

  /** hold は落とし方を受け、動画の上ならメニューを開いて true を返す。 */
  const hold: (drop: CardDrop) => boolean = menu.hold;

  return { shown, onto, hold, held: menu.held, attached, nameFieldFor };
}
