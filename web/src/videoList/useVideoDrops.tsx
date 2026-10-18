import { type ReactNode, useCallback, useEffect, useMemo, useRef, useState } from "react";

import { type LibraryGroup, RequestFailed, type Video } from "../api/client";
import { bundleOnto, groupVideos } from "../api/organize";
import { errorText, t, type UiText } from "../i18n";
import { useToast } from "../ui/Toast";
import type { CardAttachment, CardDrop } from "./cardDrag";
import type { OntoAnswer } from "./cardDragRules";
import { GroupNameField, GroupNamePopover } from "./GroupNameField";
import type { UndoToast } from "./undoToast";
import { type GroupNaming, useGroupNaming } from "./useGroupNaming";
import {
  applyDropOverlay,
  type DropOverlay,
  provisionalGroup,
  provisionalGroupName,
  videoOnto,
} from "./videoDrop";
import { useVideoDropMenu, type VideoDropChoice } from "./VideoDropMenu";

interface Shown {
  /** Monotonic operation ID guards async result application. */
  op: number;
  target: string;
  lifted: ReadonlySet<string>;
  /** Null removes only lifted cards for bundling; otherwise replace the target with the group. */
  group: LibraryGroup | null;
  /** Keep the provisional item ID stable before and after the response. */
  itemId: number;
}

interface Naming {
  op: number;
  target: string;
  field: GroupNaming;
}

export interface VideoDropsOptions<T> {
  items: readonly T[];
  keyOf: (item: T) => string;
  videoOf: (item: T) => Video | undefined;
  groupOf?: (item: T) => LibraryGroup | undefined;
  /** A new group replaces the target's position inside the current group. */
  from: number | null;
  /** Null means folders keep the target card and show naming in an attached popover. */
  groupItem: ((group: LibraryGroup, itemId: number) => T) | null;
  reread: () => void;
  /** Call only for accepted operations to clear selection; failures retain it. */
  onAccepted?: () => void;
  undo: UndoToast;
}

function isTargetGone(error: unknown): boolean {
  return error instanceof RequestFailed && error.status === 404;
}

/**
 * When refreshed server data replaces the target, reuse the provisional item's stable key with the
 * real group's position and content so naming input survives.
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
 * Only the latest nonfailed operation owns optimistic content, naming and undo UI; late responses
 * must not modify newer operations. Ignore responses after unmount, and defer an earlier accepted
 * group's naming until every newer pending operation fails.
 */
export function useVideoDrops<T>(options: VideoDropsOptions<T>) {
  const { items, keyOf, videoOf, groupOf, from, groupItem, reread, onAccepted, undo } =
    options;
  const toast = useToast();
  const [shownDrop, setShownDrop] = useState<Shown | null>(null);
  const [nameTarget, setNameTarget] = useState<string | null>(null);
  const tempId = useRef(0);
  // Track failed generations so late older responses cannot overwrite the latest nonfailed operation's UI.
  const latestOp = useRef(0);
  const failedOps = useRef(new Set<number>());
  // Older accepted responses must not replace a newer undo notification.
  const acceptedOp = useRef(0);
  const namingOp = useRef<{ op: number; groupId: number } | null>(null);
  // Defer earlier accepted naming while newer operations are pending; show it only if all newer operations fail.
  const deferred = useRef<Naming | null>(null);
  const awaitingRead = useRef<number | null>(null);
  const alive = useRef(true);
  useEffect(() => {
    alive.current = true;
    return () => {
      alive.current = false;
    };
  }, []);

  // Remove the waiting overlay when refreshed server data arrives; retain any newer pending overlay.
  useEffect(() => {
    const op = awaitingRead.current;
    if (op === null) return;
    awaitingRead.current = null;
    setShownDrop((current) => (current?.op === op ? null : current));
  }, [items]);

  /** True only if every operation after op failed. */
  const isLatest = useCallback((op: number) => {
    for (let later = latestOp.current; later > op; later -= 1) {
      if (!failedOps.current.has(later)) return false;
    }
    return true;
  }, []);

  // Only the latest operation may claim refresh-driven overlay cleanup.
  const readAgain = useCallback(
    (op: number | null) => {
      if (op !== null && isLatest(op)) awaitingRead.current = op;
      reread();
    },
    [isLatest, reread],
  );

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
    // A delayed close must not affect a newer naming field.
    onClosed: (closed) => {
      const current = namingOp.current;
      if (current === null || current.groupId !== closed.groupId) {
        reread();
        return;
      }
      namingOp.current = null;
      setNameTarget(null);
      readAgain(current.op);
    },
  });

  const { start, discard } = naming;
  const openNaming = useCallback(
    ({ op, target, field }: Naming) => {
      setNameTarget(target);
      namingOp.current = { op, groupId: field.groupId };
      start(field);
    },
    [start],
  );

  const failed = useCallback(
    (op: number, error: unknown, message: (reason: string) => UiText) => {
      failedOps.current.add(op);
      setShownDrop((current) => (current?.op === op ? null : current));
      toast(message(errorText(error)));
      if (isTargetGone(error)) reread();
      const waiting = deferred.current;
      if (waiting !== null && isLatest(waiting.op)) {
        deferred.current = null;
        openNaming(waiting);
      }
    },
    [isLatest, openNaming, reread, toast],
  );

  const liftedVideos = useCallback(
    (lifted: readonly string[]) =>
      lifted.flatMap((key) => {
        const video = videoAt(key);
        return video === undefined ? [] : [video];
      }),
    [videoAt],
  );

  // Undo closes only this operation's naming field; success removes its overlay, failure waits for refresh. Never affect newer operations.
  const undoneFor = useCallback(
    (op: number) => (ok: boolean) => {
      if (deferred.current?.op === op) deferred.current = null;
      if (namingOp.current?.op === op) {
        namingOp.current = null;
        discard();
        setNameTarget(null);
      }
      if (ok) setShownDrop((current) => (current?.op === op ? null : current));
      else if (isLatest(op)) awaitingRead.current = op;
    },
    [discard, isLatest],
  );
  const nextOp = useCallback(() => {
    latestOp.current += 1;
    return latestOp.current;
  }, []);
  const { offer } = undo;
  // Show Undo only if no newer accepted operation owns it, then discard older deferred naming.
  const accepted = useCallback(
    (op: number, message: UiText, token: string) => {
      onAccepted?.();
      if (deferred.current !== null && deferred.current.op < op) deferred.current = null;
      if (op < acceptedOp.current) return;
      acceptedOp.current = op;
      offer(message, token, undoneFor(op));
    },
    [offer, onAccepted, undoneFor],
  );
  const makeGroup = useCallback(
    ({ lifted, target }: VideoDropChoice) => {
      const targetVideo = videoAt(target);
      if (targetVideo === undefined) return;
      const videos = liftedVideos(lifted);
      const op = nextOp();
      tempId.current -= 1;
      const made = provisionalGroup(tempId.current, targetVideo, videos);
      if (groupItem !== null) {
        setShownDrop({
          op,
          target,
          lifted: new Set(lifted),
          group: made,
          itemId: made.id,
        });
      }
      const ids = videos.map((video) => video.id);
      groupVideos(ids, targetVideo.id, from).then(
        ({ undoToken, groupId }) => {
          if (!alive.current) return;
          accepted(op, t.organize.done.grouped(made.name), undoToken);
          if (groupId === undefined) {
            readAgain(op);
            return;
          }
          const next: Naming = {
            op,
            target,
            field: {
              groupId,
              provisional: provisionalGroupName(targetVideo.title),
              undoToken,
            },
          };
          // Do not open naming over a newer nonfailed operation; retain it only while newer outcomes are still pending.
          if (!isLatest(op)) {
            if (acceptedOp.current === op) deferred.current = next;
            readAgain(op);
            return;
          }
          setShownDrop((current) =>
            current?.op !== op || current.group === null
              ? current
              : { ...current, group: { ...current.group, id: groupId } },
          );
          openNaming(next);
        },
        (error: unknown) => {
          if (alive.current) failed(op, error, t.organize.failed.group);
        },
      );
    },
    [
      accepted,
      failed,
      from,
      groupItem,
      isLatest,
      liftedVideos,
      nextOp,
      openNaming,
      readAgain,
      videoAt,
    ],
  );

  const bundle = useCallback(
    ({ lifted, target }: VideoDropChoice) => {
      const targetVideo = videoAt(target);
      if (targetVideo === undefined) return;
      const ids = liftedVideos(lifted).map((video) => video.id);
      const op = nextOp();
      setShownDrop({ op, target, lifted: new Set(lifted), group: null, itemId: 0 });
      bundleOnto(ids, targetVideo.id).then(
        ({ undoToken }) => {
          if (!alive.current) return;
          accepted(
            op,
            t.versions.bundle.bundled(ids.length + 1, targetVideo.title),
            undoToken,
          );
          readAgain(op);
        },
        (error: unknown) => {
          if (alive.current) failed(op, error, t.versions.bundle.failed);
        },
      );
    },
    [accepted, failed, liftedVideos, nextOp, readAgain, videoAt],
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

  const hold: (drop: CardDrop) => boolean = menu.hold;

  return { shown, onto, hold, held: menu.held, attached, nameFieldFor };
}
