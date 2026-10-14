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
  /** 開いているグループ（グループのページ）。新しいグループはその中で落とした先の位置を取る。 */
  from: number | null;
  /**
   * 新しいグループの項目を作る。`itemId` は答えの前後で変わらない仮の項目の id。null の画面
   * （フォルダ画面）は落とした先にグループを置かず、名前の欄を落とした先の下の窓に出す。
   */
  groupItem: ((group: LibraryGroup, itemId: number) => T) | null;
  /** 一覧を静かに読み直す。 */
  reread: () => void;
  /** 操作を受け付けたら呼ぶ（選択を解く）。 */
  onAccepted?: () => void;
  undo: UndoToast;
}

/** isTargetGone は落とした先か持ち上げた動画がもう無い（404）かを返す。 */
function isTargetGone(error: unknown): boolean {
  return error instanceof RequestFailed && error.status === 404;
}

/**
 * useVideoDrops は動画を動画に落としたときのメニューと 2 つの操作である。結果はすぐ見せ
 * （`shown`）、失敗したら戻して理由を出し、受け付けたら取り消しの通知を出す。「Make a group」の
 * 後は新しいグループの名前の欄を開き（`nameFieldFor` か、フォルダ画面では `attached` の窓）、
 * 欄が閉じたら読み直す。束ねた後はすぐ読み直す。
 */
export function useVideoDrops<T>(options: VideoDropsOptions<T>) {
  const { items, keyOf, videoOf, from, groupItem, reread, onAccepted, undo } = options;
  const toast = useToast();
  const [shownDrop, setShownDrop] = useState<Shown | null>(null);
  const [nameTarget, setNameTarget] = useState<string | null>(null);
  const tempId = useRef(0);
  const awaitingRead = useRef(false);

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

  const { start } = naming;
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
      onAccepted?.();
      const ids = videos.map((video) => video.id);
      groupVideos(ids, targetVideo.id, from).then(
        ({ undoToken, groupId }) => {
          offer(t.organize.done.grouped(made.name), undoToken);
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
        (error: unknown) => failed(error, t.organize.failed.group),
      );
    },
    [failed, from, groupItem, liftedVideos, offer, onAccepted, readAgain, start, videoAt],
  );

  const bundle = useCallback(
    ({ lifted, target }: VideoDropChoice) => {
      const targetVideo = videoAt(target);
      if (targetVideo === undefined) return;
      const ids = liftedVideos(lifted).map((video) => video.id);
      setShownDrop({ target, lifted: new Set(lifted), group: null, itemId: 0 });
      onAccepted?.();
      bundleOnto(ids, targetVideo.id).then(
        ({ undoToken }) => {
          offer(t.versions.bundle.bundled(ids.length + 1, targetVideo.title), undoToken);
          readAgain();
        },
        (error: unknown) => failed(error, t.versions.bundle.failed),
      );
    },
    [failed, liftedVideos, offer, onAccepted, readAgain, videoAt],
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

  const shown = useMemo(() => {
    const made = shownDrop?.group ?? null;
    const overlay =
      shownDrop === null
        ? null
        : {
            target: shownDrop.target,
            lifted: shownDrop.lifted,
            made:
              made === null || groupItem === null
                ? null
                : groupItem(made, shownDrop.itemId),
          };
    return applyDropOverlay(items, keyOf, overlay);
  }, [groupItem, items, keyOf, shownDrop]);

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
