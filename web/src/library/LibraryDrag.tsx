import { type ReactNode, useCallback, useEffect, useMemo, useRef, useState } from "react";

import type { LibraryGroup, LibraryItem } from "../api/client";
import { groupKey, itemKey, videoKey } from "../api/libraryItems";
import {
  mergeGroups,
  moveCards,
  type OrganizeRef,
  type OrganizeResult,
} from "../api/organize";
import { errorText, t, type UiText } from "../i18n";
import { useToast } from "../ui/Toast";
import {
  type CardAttachment,
  type CardDrop,
  CardDragLayer,
  type HeldDrop,
  useCardDrag,
} from "../videoList/cardDrag";
import { liftedKeys, type OntoAnswer } from "../videoList/cardDragRules";
import { useGroupDrops } from "../videoList/dropMenu";
import { useOrderedItemDrag } from "../videoList/OrderedItemFace";
import { useUndoToast } from "../videoList/undoToast";
import { VideoRow, type VideoCardProps } from "../videoList/VideoCard";
import { type GroupCardProps, GroupRow } from "./GroupCard";
import { favoriteTargets, type LibrarySelection } from "./selection";

// ライブラリのドラッグである（specs/050-drag-organize/ui-design.md「Library」）。所有者の格子と
// リストを包み、動画だけをグループのカードの上に落とすと、そのグループへ移す。グループを含む
// カードを落とすと「Put in as a child group」と「Merge」のメニューを開く。動画のカードの
// 上に落としたときの扱いは画面が決める（`onto`、`onDrop`）。ライブラリは並べ替えないので、
// カード全体が「上」になる。

/**
 * selectedKeys は選択から持ち上げるカードの鍵を返す。「Favorite」と同じ分け方で、選んだ
 * グループはグループのカード、選んだグループのメンバーでない動画は動画のカードになる
 * （specs/050-drag-organize/ui-design.md「Lifting」）。
 */
export function selectedKeys(selection: LibrarySelection): string[] {
  const { videoIds, groupIds } = favoriteTargets(selection);
  return [...groupIds.map(groupKey), ...videoIds.map(videoKey)];
}

/** isLibraryGroup は項目がグループかを返す。 */
function isLibraryGroup(item: LibraryItem | undefined): boolean {
  return item?.kind === "group";
}

/** libraryGroupOf は項目がグループならそのグループ、でなければ null を返す（落とす先）。 */
function libraryGroupOf(item: LibraryItem | undefined): LibraryGroup | null {
  return item?.kind === "group" ? item.group : null;
}

/** refOf は項目を移す操作で指す値である。 */
function refOf(item: LibraryItem): OrganizeRef {
  return item.kind === "video"
    ? { kind: "video", id: item.video.id }
    : { kind: "group", id: item.group.id };
}

/**
 * LibraryDragLayer はライブラリの所有者の格子とリストを包むドラッグの層である。選択した
 * カード（「Favorite」と同じ分け方。選んだグループのメンバーの動画は入らない）を持ち上げると、
 * 選択した読み込み済みのカードがすべて画面の順でいっしょに動く。グループを含むカードを
 * グループに落とすとメニューを開き、選んだ操作で `onMove` か `onMerge` を呼ぶ。
 */
export function LibraryDragLayer({
  items,
  list,
  selection,
  onto: ontoVideo,
  onDrop,
  onMove,
  onMerge,
  onLift,
  held,
  attached,
  children,
}: {
  /** 画面に出している項目（画面の順）。 */
  items: readonly LibraryItem[];
  list: boolean;
  selection: LibrarySelection;
  /** 動画のカードの上に落とせるか（グループのカードはこの層が決める）。 */
  onto: (target: string, lifted: readonly string[]) => OntoAnswer<UiText>;
  /** 動画のカードの上に落とした。 */
  onDrop: (drop: CardDrop) => void;
  /** 項目 `moved`（画面の順）をグループ `to` へ移す。 */
  onMove: (moved: readonly LibraryItem[], to: LibraryGroup) => void;
  /** 項目 `merged`（画面の順、グループを含む）をグループ `into` へマージする。 */
  onMerge: (merged: readonly LibraryItem[], into: LibraryGroup) => void;
  onLift: () => void;
  held: HeldDrop | null;
  attached: CardAttachment | null;
  children: ReactNode;
}) {
  const { order, byKey, nameOf, preview } = useOrderedItemDrag(items, itemKey, list);
  const selected = useMemo(() => selectedKeys(selection), [selection]);
  const liftedFor = useCallback(
    (key: string) => liftedKeys(order, key, selected),
    [order, selected],
  );
  const groups = useGroupDrops({
    byKey,
    groupOf: libraryGroupOf,
    isGroup: isLibraryGroup,
    nameOf,
    onMove,
    onMerge,
  });
  const { onto: ontoGroup, drop: dropOnGroup } = groups;
  const onto = useCallback(
    (target: string, lifted: readonly string[]): OntoAnswer<UiText> =>
      isLibraryGroup(byKey.get(target))
        ? ontoGroup(target, lifted)
        : ontoVideo(target, lifted),
    [byKey, ontoGroup, ontoVideo],
  );
  const drop = useCallback(
    (drop: CardDrop) => {
      const onGroup =
        drop.sign.kind === "onto" && isLibraryGroup(byKey.get(drop.sign.key));
      if (onGroup) dropOnGroup(drop);
      else onDrop(drop);
    },
    [byKey, dropOnGroup, onDrop],
  );

  return (
    <CardDragLayer
      order={order}
      liftedFor={liftedFor}
      reorder={false}
      onto={onto}
      nameOf={nameOf}
      preview={preview}
      list={list}
      onDrop={drop}
      onLift={onLift}
      instructions={t.organize.instructions}
      held={held ?? groups.held}
      attached={attached ?? groups.attached}
    >
      {children}
    </CardDragLayer>
  );
}

/** DraggableVideoRow はライブラリのリスト表示の、持ち上げられる動画の行である。 */
export function DraggableVideoRow(props: Omit<VideoCardProps, "drag">) {
  const drag = useCardDrag(videoKey(props.video.id));
  return <VideoRow {...props} drag={drag} />;
}

/** DraggableGroupRow はライブラリのリスト表示の、持ち上げられるグループの行である。 */
export function DraggableGroupRow(props: Omit<GroupCardProps, "drag">) {
  const drag = useCardDrag(groupKey(props.group.id));
  return <GroupRow {...props} drag={drag} />;
}

const noKeys: ReadonlySet<string> = new Set();

/**
 * pruned は `hidden` のうち、もう一覧に無い項目の鍵（見せるときに隠す必要が無い）と、受け入れられた
 * 操作の項目の鍵 `released` を除く。読み直した一覧がまだ持つ項目はサーバーの答えなので見せる
 * （ほかのグループにも入っている動画は、グループへ足した後も一覧に残る。048 research.md R-6）。
 */
function pruned(
  hidden: ReadonlySet<string>,
  items: readonly LibraryItem[],
  released: ReadonlySet<string>,
) {
  if (hidden.size === 0) return hidden;
  const present = new Set(items.map(itemKey));
  const kept = new Set(
    Array.from(hidden).filter((key) => present.has(key) && !released.has(key)),
  );
  return kept.size === hidden.size ? hidden : kept;
}

/** without は `hidden` から `keys` を除く。 */
function without(hidden: ReadonlySet<string>, keys: readonly string[]) {
  return new Set(Array.from(hidden).filter((key) => !keys.includes(key)));
}

/** LibraryOperation はライブラリで落とした項目に送る操作 1 つである。 */
interface LibraryOperation {
  send: () => Promise<OrganizeResult>;
  done: UiText;
  failed: (reason: string) => UiText;
}

/**
 * useLibraryMoves はライブラリでグループへ落とした項目を移すかマージする（research.md R-8）。
 * 落とした項目はすぐ一覧から隠し（マージしたグループのカードも消える）、選択を解き、`move` か
 * `merge` を送る。要求は落とした順に 1 つずつ送るので、
 * 「Undo」の通知はいつも最後に落とした移動のものになる。受け入れられたら「Undo」付きの通知を
 * 出して一覧をその場で読み直し、失敗したら隠した項目を戻して知らせる。画面を離れた後は
 * どちらの通知も出さない。隠した項目は、読み直した一覧から消えたら覚えるのをやめ、受け入れられた
 * 操作の項目は次の一覧で見せる（一覧がまだ持つなら、それがサーバーの答え）。取り消しの後は、
 * まだ答えを待っている移動の項目を除いてすべて見せる。
 */
export function useLibraryMoves(
  items: readonly LibraryItem[],
  refreshInPlace: () => void,
  clearSelection: () => void,
) {
  const toast = useToast();
  const [hidden, setHidden] = useState<ReadonlySet<string>>(noKeys);
  const shown = useRef(true);
  /** 落とした順に移動を送る鎖と、答えを待っている移動の項目の鍵。 */
  const chain = useRef<Promise<void>>(Promise.resolve());
  const pending = useRef(new Set<string>());
  /** 受け入れられて、次の一覧で見せる項目の鍵。 */
  const released = useRef(new Set<string>());
  useEffect(() => {
    shown.current = true;
    return () => {
      shown.current = false;
    };
  }, []);

  useEffect(() => {
    const done = released.current;
    released.current = new Set();
    setHidden((current) => pruned(current, items, done));
  }, [items]);

  const afterUndo = useCallback(() => {
    setHidden(pending.current.size === 0 ? noKeys : new Set(pending.current));
    refreshInPlace();
  }, [refreshInPlace]);
  const { offer } = useUndoToast(afterUndo);

  const run = useCallback(
    (moved: readonly LibraryItem[], operation: LibraryOperation) => {
      if (moved.length === 0) return;
      const keys = moved.map(itemKey);
      for (const key of keys) pending.current.add(key);
      const settle = () => {
        for (const key of keys) pending.current.delete(key);
      };
      setHidden((current) => new Set([...current, ...keys]));
      clearSelection();
      chain.current = chain.current.then(() =>
        operation.send().then(
          ({ undoToken }) => {
            settle();
            for (const key of keys) released.current.add(key);
            if (!shown.current) return;
            offer(operation.done, undoToken);
            refreshInPlace();
          },
          (error: unknown) => {
            settle();
            if (!shown.current) return;
            setHidden((current) => without(current, keys));
            toast(operation.failed(errorText(error)));
          },
        ),
      );
    },
    [clearSelection, offer, refreshInPlace, toast],
  );

  const move = useCallback(
    (moved: readonly LibraryItem[], to: LibraryGroup) =>
      run(moved, {
        send: () => moveCards(moved.map(refOf), { from: null, to: to.id }),
        done: t.organize.done.moved(moved.length, to.name),
        failed: t.organize.failed.move,
      }),
    [run],
  );
  const merge = useCallback(
    (merged: readonly LibraryItem[], into: LibraryGroup) =>
      run(merged, {
        send: () => mergeGroups(merged.map(refOf), into.id, null),
        done: t.organize.done.merged(
          merged.length,
          into.name,
          merged.some((item) => item.kind === "video"),
        ),
        failed: t.organize.failed.merge,
      }),
    [run],
  );

  const visible = useMemo(
    () =>
      hidden.size === 0 ? items : items.filter((item) => !hidden.has(itemKey(item))),
    [hidden, items],
  );
  return { visible, move, merge };
}
