import { Search, X } from "lucide-react";
import { type RefObject, useCallback, useEffect, useRef, useState } from "react";

import { MAX_QUERY_LENGTH } from "../api/client";
import { t, type UiText } from "../i18n";
import { cn } from "../lib/cn";
import { isComposingKeyEvent } from "../ui/legacy/Combobox";
import { type HistoryMode, normalizeQuery, SearchSession } from "./listCriteria";
import SearchSyntaxHelp from "./SearchSyntaxHelp";

/** searchDebounceMs は入力が落ち着くのを待つ時間。打鍵ごとに一覧が入れ替わらないようにする。 */
export const searchDebounceMs = 250;

export interface SearchBoxProps {
  /** URL から読んだ今の検索語。戻る・進むで変わると入力欄が追従する。 */
  query: string;
  /**
   * onCommit は検索語を確定する。mode は履歴の増やし方で、フォーカスが入ってから
   * 外れるか Esc で抜けるまでの一続きで最初の確定だけが push になる
   * （specs/013-library-search/contracts/list-url.md §3）。
   */
  onCommit: (next: string, mode: HistoryMode) => void;
  /** 入力欄を外から指す。 */
  inputRef?: RefObject<HTMLInputElement | null>;
  /** 読み上げ名。 */
  label?: UiText;
  placeholder?: UiText;
  /**
   * 枠の右端に動画の検索の書き方の手引き（`SearchSyntaxHelp`）を置くか。動画の検索
   * 構文を持たない一覧（タグ管理画面）は false にする。
   */
  syntaxHelp?: boolean;
  /**
   * 入力が落ち着くのを待つ時間（ms）。既定は `searchDebounceMs`。タグ管理画面は
   * 打鍵ごとにサーバーへ引き直す（前の要求は打ち切る）ので 0 にする。
   */
  debounceMs?: number;
  disabled?: boolean;
  className?: string;
}

/**
 * limitQueryInput は入力を検索語の上限に収める。HTML の maxLength は UTF-16 の単位で
 * 数えるので使わず、URL・サーバーと同じく符号位置で数える（list-url.md §1）。
 *
 * 上限を超えたときは、入力前の値と比べて新しく入った部分だけを切る。先頭や途中への
 * 入力で、もとからあった末尾を消さないためである。上限を超える貼り付けも、入る分
 * だけが入る（Issue の Edge Case「語の長さの上限」）。
 *
 * `caret` は入力後のカーソル位置（UTF-16 の単位）で、新しく入った部分はその手前で
 * 終わる。選択範囲を、選んだ文字で終わる語に置き換えたときも、入った部分を既存の
 * 末尾と取り違えないよう、共通の先頭と末尾をカーソルの前後に収める。
 */
export function limitQueryInput(next: string, previous = "", caret?: number): string {
  const after = Array.from(next);
  if (after.length <= MAX_QUERY_LENGTH) return next;
  const before = Array.from(previous);
  // カーソル位置が分からないときは、先頭と末尾を縛らない。
  const cursor =
    caret === undefined ? undefined : Array.from(next.slice(0, caret)).length;
  let prefix = 0;
  while (
    prefix < before.length &&
    prefix < (cursor ?? after.length) &&
    before[prefix] === after[prefix]
  ) {
    prefix++;
  }
  let suffix = 0;
  while (
    suffix < before.length - prefix &&
    suffix < after.length - Math.max(prefix, cursor ?? prefix) &&
    before[before.length - 1 - suffix] === after[after.length - 1 - suffix]
  ) {
    suffix++;
  }
  const inserted = after.slice(prefix, after.length - suffix);
  const room = Math.max(MAX_QUERY_LENGTH - prefix - suffix, 0);
  return [
    ...after.slice(0, prefix),
    ...inserted.slice(0, room),
    ...after.slice(after.length - suffix),
  ]
    .slice(0, MAX_QUERY_LENGTH)
    .join("");
}

/**
 * SearchBox は一覧の条件の `q` を入力する検索欄である。
 * `/` でフォーカス、Esc でクリアしてフォーカスを外す。枠の右端に検索の書き方の手引きを持つ。
 */
export default function SearchBox({
  query,
  onCommit,
  inputRef,
  label = t.list.search.label,
  placeholder = t.list.search.placeholder,
  syntaxHelp = true,
  debounceMs = searchDebounceMs,
  disabled = false,
  className,
}: SearchBoxProps) {
  const [input, setInputState] = useState(query);
  // 最新の入力。blur は同じ操作の中の setInput より先に走ることがあるので、
  // 描画を待たずに読める控えを持つ。
  const latest = useRef(query);
  const setInput = useCallback((next: string) => {
    latest.current = next;
    setInputState(next);
  }, []);
  const committed = useRef(query);
  const ownField = useRef<HTMLInputElement | null>(null);
  const field = inputRef ?? ownField;
  const session = useRef(new SearchSession());

  const commit = useCallback(
    (next: string) => {
      if (next === committed.current) return;
      committed.current = next;
      onCommit(next, session.current.commit());
    },
    [onCommit],
  );

  // 戻る・進むなどで URL 側が変わったら入力欄を追従させる。
  useEffect(() => {
    if (query !== committed.current) {
      committed.current = query;
      setInput(query);
      // 外から変わった検索語は、打鍵の一続きの外である。次の入力は履歴を1つ増やす。
      session.current.end();
    }
  }, [query, setInput]);

  useEffect(() => {
    // `input` ではなく控えを読む。URL 側の変更に追従した描画では、上の effect が控えを
    // 新しい語にしたあとも `input` はまだ前の語なので、それで確定し直すと消した検索語が
    // 戻る（`onCommit` が描画ごとに変わると、この effect も同じ描画で走り直す）。
    const next = normalizeQuery(latest.current);
    if (next === committed.current) return;
    const timer = setTimeout(() => commit(next), debounceMs);
    return () => clearTimeout(timer);
  }, [commit, input, debounceMs]);

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key !== "/" || event.metaKey || event.ctrlKey || event.altKey) return;
      const target = event.target as HTMLElement | null;
      if (target?.closest("input, textarea, [contenteditable=true]")) return;
      if (field.current === null || field.current.disabled) return;
      event.preventDefault();
      field.current?.focus();
      field.current?.select();
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [field]);

  const clear = () => {
    setInput("");
    commit("");
    field.current?.focus();
  };

  return (
    <div className={cn("group relative flex h-9 w-full items-center", className)}>
      <Search className="pointer-events-none absolute left-3 size-4 text-muted-foreground transition-colors group-focus-within:text-muted-foreground" />
      <input
        ref={field}
        type="search"
        value={input}
        onChange={(event) =>
          setInput(
            limitQueryInput(
              event.target.value,
              latest.current,
              event.target.selectionEnd ?? undefined,
            ),
          )
        }
        onFocus={() => session.current.start()}
        onBlur={() => {
          // 待っている確定があれば、続きを閉じる前に済ませる。
          commit(normalizeQuery(latest.current));
          session.current.end();
        }}
        onKeyDown={(event) => {
          // IME の変換中の Esc は変換を取り消すためのもので、検索語を消さない。
          if (isComposingKeyEvent(event)) return;
          if (event.key === "Escape") {
            event.preventDefault();
            setInput("");
            commit("");
            field.current?.blur();
          }
        }}
        placeholder={placeholder}
        aria-label={label}
        disabled={disabled}
        autoComplete="off"
        spellCheck={false}
        className={cn(
          // 右端のボタンの分だけ空ける。検索語が空で sm 未満なら手引きのボタンだけなので狭くてよい。
          // 手引きを置かない検索欄は、クリアか `/` の1つ分だけ空ける。
          !syntaxHelp ? "pr-9" : input === "" ? "pr-9 sm:pr-15" : "pr-15",
          "h-full w-full rounded-md border border-input bg-muted pl-9 text-sm text-foreground shadow-[inset_0_1px_2px_var(--color-border)]",
          "placeholder:text-muted-foreground transition-[border-color,box-shadow] duration-150",
          "focus:border-primary focus:outline-none focus:ring-2 focus:ring-ring",
          "disabled:cursor-not-allowed disabled:opacity-50",
          "[&::-webkit-search-cancel-button]:hidden",
        )}
      />
      <div className="absolute right-1.5 flex items-center gap-0.5">
        {input !== "" && (
          <button
            type="button"
            // 押した瞬間に入力欄のフォーカスを外さない。外すと blur が入力途中の語を
            // 確定して履歴を1つ増やし、続くクリアがもう1つ増やしてしまう。
            onMouseDown={(event) => event.preventDefault()}
            onClick={clear}
            aria-label={t.list.search.clear}
            className="flex size-6 items-center justify-center rounded-sm text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
          >
            <X className="size-4" />
          </button>
        )}
        {input === "" && (
          <kbd className="pointer-events-none mr-1 hidden rounded-sm border border-input px-1.5 font-sans text-2xs text-muted-foreground sm:block">
            /
          </kbd>
        )}
        {syntaxHelp && <SearchSyntaxHelp />}
      </div>
    </div>
  );
}
