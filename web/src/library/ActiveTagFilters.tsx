import { Building2, Tag as TagIcon, User, X } from "lucide-react";
import {
  type ReactNode,
  type RefObject,
  useEffect,
  useEffectEvent,
  useRef,
  useState,
} from "react";

import { RequestFailed } from "../api/client";
import { type CreditKind, getCredit } from "../api/credits";
import {
  currentTags,
  refreshTags,
  revalidateTags,
  subscribeTags,
  type Tag,
} from "../api/tags";
import { compareNatural } from "../api/tagOrder";
import { t } from "../i18n";
import { Skeleton } from "../ui/shadcn/skeleton";
import { Toggle } from "../ui/shadcn/toggle";
import { type CreditFilter, creditKinds } from "./creditCriteria";

export interface ActiveTagFiltersProps {
  /** 絞り込み中のタグの id（並びは問わない。表示は名前の自然順にそろえる）。 */
  tagIds: readonly number[];
  onRemove: (id: number) => void;
  /** 絞り込み中の人物・団体（タグのチップの後に、人物、団体の順で 1 つずつ出す）。 */
  credits?: CreditFilter;
  onRemoveCredit?: (kind: CreditKind) => void;
  /** 名前を読んだら `404` だった（もう無い）人物・団体。呼び手が URL から外して知らせる。 */
  onCreditGone?: (kind: CreditKind) => void;
  /** 最後の1つを外したときのフォーカスの移し先（検索欄）。 */
  searchFieldRef: RefObject<HTMLInputElement | null>;
}

/** ChipKey はチップの鍵である（タグは `tag:<id>`、人物・団体は種類）。 */
type ChipKey = string;

function tagKey(id: number): ChipKey {
  return `tag:${String(id)}`;
}

/** useTagNames は共有のタグの一覧を読み、絞り込み中のタグの名前を引けるようにする。 */
function useTagNames(tagIds: readonly number[]): Map<number, Tag> {
  const [allTags, setAllTags] = useState<Tag[] | undefined>(currentTags());
  // 開いたときの確かめ直し（`revalidateTags`）が返ったか。それより前の一覧（古い保持）で
  // 足りない id を数えると、確かめ直しの取得と並べて全件をもう一度取ってしまう。
  const [revalidated, setRevalidated] = useState(false);
  useEffect(() => {
    let alive = true;
    revalidateTags()
      .then((loaded) => {
        if (!alive) return;
        setAllTags(loaded);
        setRevalidated(true);
      })
      .catch(() => undefined);
    const unsubscribe = subscribeTags((loaded) => {
      if (alive) setAllTags(loaded);
    });
    return () => {
      alive = false;
      unsubscribe();
    };
  }, []);

  // 確かめ直しのあとも共有の一覧に無いタグで絞り込んでいたら（使い回した一覧より後に
  // 別のタブや画面で作られたタグのカードを押したときなど）、一覧を取り直して名前を引く。消えたタグで取り直しを
  // 繰り返さないよう、取り直しを求めた id は覚えておく。
  const refreshedFor = useRef(new Set<number>());
  useEffect(() => {
    if (!revalidated || allTags === undefined) return;
    const known = new Set(allTags.map((tag) => tag.id));
    const missing = tagIds.filter(
      (id) => !known.has(id) && !refreshedFor.current.has(id),
    );
    if (missing.length === 0) return;
    for (const id of missing) refreshedFor.current.add(id);
    void refreshTags().catch(() => undefined);
  }, [allTags, revalidated, tagIds]);

  return new Map((allTags ?? []).map((tag) => [tag.id, tag]));
}

/**
 * useCreditName は人物か団体の名前を `GET /api/people/{id}`・`GET /api/organizations/{id}`
 * で読む。届くまでは undefined。`404` なら `onGone` を呼ぶ（ほかの失敗では名前の代わりの
 * Skeleton のまま、タグの一覧が読めないときのタグのチップと同じ）。
 */
function useCreditName(
  kind: CreditKind,
  id: number,
  onGone: (kind: CreditKind) => void,
): string | undefined {
  const [loaded, setLoaded] = useState<{ kind: CreditKind; id: number; name: string }>();
  const gone = useEffectEvent(() => onGone(kind));
  useEffect(() => {
    const controller = new AbortController();
    getCredit(kind, id, controller.signal)
      .then((credit) => setLoaded({ kind, id, name: credit.name }))
      .catch((failure: unknown) => {
        if (controller.signal.aborted) return;
        if (failure instanceof RequestFailed && failure.status === 404) gone();
      });
    return () => controller.abort();
  }, [kind, id]);
  return loaded?.kind === kind && loaded.id === id ? loaded.name : undefined;
}

interface ChipProps {
  icon: ReactNode;
  name: string | undefined;
  /** 名前が分かる前の読み上げ名。 */
  unnamedLabel: string;
  onPress: () => void;
  buttonRef: (node: HTMLButtonElement | null) => void;
}

/**
 * FilterChip は効いている絞り込み 1 つのチップである。効いている絞り込みは押された
 * Toggle で、押し戻すと外れる（web/registry/rules/components.md「Toggle and ToggleGroup」）。
 */
function FilterChip({ icon, name, unnamedLabel, onPress, buttonRef }: ChipProps) {
  // 名前をまだ読んでいない間も、読み上げる名前が無くならないようにする（N2）。
  // 名前が分かれば「〈名〉の絞り込みを外す」に差し替わる。
  const label =
    name === undefined ? unnamedLabel : t.library.activeTags.removeNamed(name);
  return (
    <li>
      <Toggle
        ref={buttonRef}
        variant="outline"
        size="sm"
        pressed
        onPressedChange={onPress}
        aria-label={label}
        title={label}
      >
        {icon}
        {name === undefined ? (
          <Skeleton className="h-3 w-12" />
        ) : (
          <span title={name} className="max-w-chip-label min-w-0 truncate">
            {name}
          </span>
        )}
        <X aria-hidden="true" />
      </Toggle>
    </li>
  );
}

function CreditChip({
  kind,
  id,
  onGone,
  ...rest
}: Omit<ChipProps, "icon" | "name" | "unnamedLabel"> & {
  kind: CreditKind;
  id: number;
  onGone: (kind: CreditKind) => void;
}) {
  const name = useCreditName(kind, id, onGone);
  const person = kind === "person";
  return (
    <FilterChip
      {...rest}
      icon={person ? <User aria-hidden="true" /> : <Building2 aria-hidden="true" />}
      name={name}
      unnamedLabel={
        person
          ? t.library.activeTags.removePerson
          : t.library.activeTags.removeOrganization
      }
    />
  );
}

const noCredits: CreditFilter = {};
const ignoreCredit = () => undefined;

/**
 * ActiveTagFilters はライブラリの本文の先頭、要約行の上に置く「絞り込み中の
 * 条件」の行である（specs/014-video-tags/ui-design.md「Active tag filters」、
 * specs/047-video-work-info/ui-design.md「Library: filter by a person or organization」）。
 * タグのチップの後に人物・団体のチップを 1 つずつ並べる。タグ・人物・団体のどれかで
 * 絞り込んでいるときだけ描画する（呼び出し側が出し分ける）。
 */
export default function ActiveTagFilters({
  tagIds,
  onRemove,
  credits = noCredits,
  onRemoveCredit = ignoreCredit,
  onCreditGone = ignoreCredit,
  searchFieldRef,
}: ActiveTagFiltersProps) {
  const byId = useTagNames(tagIds);

  const buttonRefs = useRef(new Map<ChipKey, HTMLButtonElement>());
  const pendingFocus = useRef<ChipKey | "search" | null>(null);
  useEffect(() => {
    const pending = pendingFocus.current;
    if (pending === null) return;
    pendingFocus.current = null;
    if (pending === "search") {
      searchFieldRef.current?.focus();
      return;
    }
    buttonRefs.current.get(pending)?.focus();
  }, [tagIds, credits.person, credits.organization, searchFieldRef]);

  const orderedTags = [...tagIds].sort((a, b) => {
    const nameA = byId.get(a)?.name;
    const nameB = byId.get(b)?.name;
    if (nameA === undefined || nameB === undefined) return a - b;
    return compareNatural(nameA, nameB) || a - b;
  });
  const activeCredits = creditKinds.flatMap((kind) => {
    const id = credits[kind];
    return id === undefined ? [] : [{ kind, id }];
  });
  const ordered: ChipKey[] = [
    ...orderedTags.map(tagKey),
    ...activeCredits.map(({ kind }) => kind),
  ];

  // 外したチップの次、無ければ前のチップへフォーカスを移す。最後の1つを外すと、
  // この行は呼び出し側で描画されなくなり、フォーカスを移す効果も走らないので、
  // 外す前に検索欄へ移しておく。
  function removeChip(key: ChipKey, remove: () => void) {
    const index = ordered.indexOf(key);
    const next = ordered[index + 1] ?? ordered[index - 1];
    if (next === undefined) {
      pendingFocus.current = null;
      searchFieldRef.current?.focus();
    } else {
      pendingFocus.current = next;
    }
    remove();
  }

  const refFor = (key: ChipKey) => (node: HTMLButtonElement | null) => {
    if (node) buttonRefs.current.set(key, node);
    else buttonRefs.current.delete(key);
  };

  return (
    <div className="flex flex-wrap items-center gap-2">
      <span className="text-xs text-muted-foreground">{t.library.activeTags.label}</span>
      <ul
        aria-label={t.library.activeTags.list}
        className="flex flex-wrap items-center gap-2"
      >
        {orderedTags.map((id) => (
          <FilterChip
            key={tagKey(id)}
            buttonRef={refFor(tagKey(id))}
            icon={<TagIcon aria-hidden="true" />}
            name={byId.get(id)?.name}
            unnamedLabel={t.library.activeTags.remove}
            onPress={() => removeChip(tagKey(id), () => onRemove(id))}
          />
        ))}
        {activeCredits.map(({ kind, id }) => (
          <CreditChip
            // id が変わったら名前を読み直す（鍵に id を入れて作り直す）。
            key={`${kind}:${String(id)}`}
            kind={kind}
            id={id}
            buttonRef={refFor(kind)}
            onGone={onCreditGone}
            onPress={() => removeChip(kind, () => onRemoveCredit(kind))}
          />
        ))}
      </ul>
    </div>
  );
}
