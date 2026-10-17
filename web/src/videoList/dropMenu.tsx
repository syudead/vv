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

// 落とした先のカードに開くメニューである。
// メニューは落とした先のカードの下に開き、ラベルは落とした先の名前を言う。Esc、外の押下、Tab で
// 閉じると何も送らない。閉じることが取り消しなので「Cancel」の項目は無い。

/**
 * DropMenuAnchor は落とした先のカードの下端に置く、メニューの位置の目印である。カードの箱
 * （`relative`）の中に置き、メニューはその下に開く。
 */
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

/**
 * DropMenu は落とした先のカードに開いたままのメニューの枠である。ラベル「Dropped on "{name}"」の
 * 下に `children`（DropdownMenuItem）を並べる。閉じたら（項目を選んだ後も）`onClose` を呼ぶ。
 */
export function DropMenu({
  name,
  onClose,
  children,
}: {
  /** 落とした先のカードの題名か名前。 */
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
        // 位置の目印はフォーカスを受けない。閉じた後のフォーカスは画面に任せる。
        onCloseAutoFocus={(event) => event.preventDefault()}
      >
        <DropdownMenuLabel>{t.organize.menu.label(name)}</DropdownMenuLabel>
        {children}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

/** DropChoice は落とした先のメニューで選んだ操作の値である。 */
export interface DropChoice {
  /** 持ち上げたカード（画面の順）。 */
  lifted: readonly string[];
  /** 落とした先のカード。 */
  target: string;
}

/**
 * useDropMenu は落とした先のカードにメニューを開く。`hold` は落とし方を受け、`accepts` が true
 * なら印を保って（`held`）メニューを開き、true を返す（画面はほかの落とし方として扱わない）。
 * メニューの間は持ち上げたカードは半透明、落とした先は輪と札のまま残る。メニューは `menu` が
 * 選んだ操作の値と閉じる関数から描き、落とした先のカードに重ねる（`attached`）。落とした先が
 * `accepts` を満たさなくなったら（別の画面の操作や知らせで一覧が変わり、落とした先のカードが
 * 消えた）、メニューと印を外す。
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

/**
 * useGroupDropMenu はグループを含むカードをグループのカードに落としたときのメニューを開く。`hold` は落とし方を受け、`accepts` が true なら
 * メニューを開いて true を返す。項目は「Put in as a child group」（`onNest`）と「Merge」
 * （`onMerge`）の 2 つで、どちらも `destructive` にしない。閉じると何もしない。
 */
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
 * useGroupDrops は、カードをグループのカードの上に落としたときの扱いを画面に渡す。画面はカードの鍵ごとの項目 `byKey`、項目が落とす先の
 * グループならその値を返す `groupOf`、項目がグループかを返す `isGroup` を渡す。
 *
 * - `onto` はグループのカードに「Put in」、持ち上げたカードにグループがあれば「Put in or merge…」の
 *   札を出す。グループでないカードは null（上に落とす先ではない）。
 * - `drop` は上に落とした落とし方を受ける。動画だけなら `onMove` を呼び、グループを含めば
 *   メニューを開き、選んだ項目で `onMove`（「Put in as a child group」）か `onMerge` を呼ぶ。
 *   どちらにも持ち上げたカードの項目を画面の順に渡す。
 * - `held` と `attached` は CardDragLayer に渡す（メニューの間の印と、メニューそのもの）。
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
  /** chosen は選んだ操作 `run` に、持ち上げた項目と落とした先のグループを渡す。 */
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
