import { FolderInput, Merge } from "lucide-react";
import {
  type KeyboardEvent,
  type ReactNode,
  useCallback,
  useEffect,
  useMemo,
  useState,
} from "react";

import { t, type UiText } from "../i18n";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from "../ui/shadcn/dropdown-menu";
import type { CardAttachment, CardDrop, HeldDrop } from "./cardDrag";
import type { OntoAnswer } from "./cardDragRules";

// Closing via Escape, outside click or Tab is cancellation; no separate Cancel item is needed.

/** Place inside the target card's relative box, with the menu below it. */
function DropMenuAnchor() {
  return (
    <DropdownMenuTrigger asChild>
      <span
        aria-hidden="true"
        className="pointer-events-none absolute inset-x-0 bottom-0"
      />
    </DropdownMenuTrigger>
  );
}

export function DropMenu({
  name,
  onClose,
  children,
}: {
  name: string;
  onClose: () => void;
  children: ReactNode;
}) {
  const closeOnTab = (event: KeyboardEvent) => {
    if (event.key === "Tab") onClose();
  };
  return (
    <DropdownMenu
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DropMenuAnchor />
      <DropdownMenuContent
        side="bottom"
        onKeyDown={closeOnTab}
        // The anchor never receives focus; the screen owns close-time focus restoration.
        onCloseAutoFocus={(event) => event.preventDefault()}
      >
        <DropdownMenuLabel>{t.organize.menu.label(name)}</DropdownMenuLabel>
        {children}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

export interface DropChoice {
  lifted: readonly string[];
  target: string;
}

/**
 * Accepting a drop retains marks and attaches its menu; return true so the screen stops other drop
 * handling. Remove both when the target no longer satisfies accepts.
 */
export function useDropMenu(
  accepts: (drop: CardDrop) => boolean,
  menu: (choice: DropChoice, close: () => void) => ReactNode,
) {
  const [held, setHeld] = useState<HeldDrop | null>(null);
  const hold = useCallback(
    (drop: CardDrop) => {
      if (drop.sign.kind !== "onto" || !accepts(drop)) return false;
      setHeld({ lifted: drop.lifted, sign: drop.sign });
      return true;
    },
    [accepts],
  );
  const close = useCallback(() => setHeld(null), []);
  useEffect(() => {
    if (held === null || held.sign.kind === "refused") return;
    if (!accepts({ lifted: held.lifted, sign: held.sign })) setHeld(null);
  }, [accepts, held]);
  const attached = useMemo((): CardAttachment | null => {
    if (held === null) return null;
    const choice = { lifted: held.lifted, target: held.sign.key };
    return { key: choice.target, node: menu(choice, close) };
  }, [close, held, menu]);
  return { held, attached, hold };
}

/** Both nest and merge are normal actions; closing without selection does nothing. */
function useGroupDropMenu({
  accepts,
  nameOf,
  onNest,
  onMerge,
}: {
  accepts: (drop: CardDrop) => boolean;
  nameOf: (key: string) => string;
  onNest: (choice: DropChoice) => void;
  onMerge: (choice: DropChoice) => void;
}) {
  const menu = useCallback(
    (choice: DropChoice, close: () => void) => (
      <DropMenu name={nameOf(choice.target)} onClose={close}>
        <DropdownMenuItem onSelect={() => onNest(choice)}>
          <FolderInput aria-hidden="true" />
          {t.organize.menu.nest}
        </DropdownMenuItem>
        <DropdownMenuItem onSelect={() => onMerge(choice)}>
          <Merge aria-hidden="true" />
          {t.organize.menu.merge}
        </DropdownMenuItem>
      </DropMenu>
    ),
    [nameOf, onMerge, onNest],
  );
  return useDropMenu(accepts, menu);
}

/**
 * Video-only drops move directly; drops containing groups offer nesting or merging. Preserve
 * display order and pass held/attached to CardDragLayer.
 */
export function useGroupDrops<I, G>({
  byKey,
  groupOf,
  isGroup,
  nameOf,
  onMove,
  onMerge,
}: {
  byKey: ReadonlyMap<string, I>;
  groupOf: (item: I | undefined) => G | null;
  isGroup: (item: I | undefined) => boolean;
  nameOf: (key: string) => string;
  onMove: (moved: readonly I[], to: G) => void;
  onMerge: (merged: readonly I[], into: G) => void;
}) {
  const liftsGroup = useCallback(
    (lifted: readonly string[]) => lifted.some((key) => isGroup(byKey.get(key))),
    [byKey, isGroup],
  );
  const onto = useCallback(
    (target: string, lifted: readonly string[]): OntoAnswer<UiText> => {
      if (groupOf(byKey.get(target)) === null) return null;
      return {
        label: liftsGroup(lifted) ? t.organize.drop.putInOrMerge : t.organize.drop.putIn,
      };
    },
    [byKey, groupOf, liftsGroup],
  );
  const chosen = useCallback(
    (run: (moved: readonly I[], to: G) => void) =>
      ({ lifted, target }: DropChoice) => {
        const to = groupOf(byKey.get(target));
        if (to === null) return;
        run(
          lifted.flatMap((key) => {
            const item = byKey.get(key);
            return item === undefined ? [] : [item];
          }),
          to,
        );
      },
    [byKey, groupOf],
  );
  const accepts = useCallback(
    ({ lifted, sign }: CardDrop) =>
      groupOf(byKey.get(sign.key)) !== null && liftsGroup(lifted),
    [byKey, groupOf, liftsGroup],
  );
  const { held, attached, hold } = useGroupDropMenu({
    accepts,
    nameOf,
    onNest: useMemo(() => chosen(onMove), [chosen, onMove]),
    onMerge: useMemo(() => chosen(onMerge), [chosen, onMerge]),
  });
  const drop = useCallback(
    (dropped: CardDrop) => {
      if (dropped.sign.kind !== "onto" || hold(dropped)) return;
      chosen(onMove)({ lifted: dropped.lifted, target: dropped.sign.key });
    },
    [chosen, hold, onMove],
  );
  return { onto, drop, held, attached };
}
