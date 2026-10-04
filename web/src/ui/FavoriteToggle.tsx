import { Heart, LoaderCircle } from "lucide-react";
import { type MouseEvent, useEffect, useRef, useState } from "react";

import { errorText, t, type UiText } from "@/i18n";
import { cn } from "@/lib/cn";
import IconButton from "./IconButton";
import { useToast } from "./legacy/Toast";

/**
 * FavoriteToggle はカードと行のお気に入りの付け外しである。vv 固有の部品で、使い方は
 * web/registry/rules/components.md「FavoriteToggle」
 * （specs/035-favorites/ui-design.md「Mark」「Card」）。印を兼ね、塗りのハートがお気に入り、
 * 線のハートがお気に入りでない。所有者の画面でだけ描く（呼び出し側が決める）。
 *
 * - `card` は格子のカードのサムネイルの右上に重ねる形で、面も枠も付けず、押せる範囲
 *   `size-8` の中に `size-5` のハートだけを置き、暗い影（`drop-shadow-mark`）で明るい絵柄の上でも
 *   読めるようにする。オフは白の線（`text-foreground`）である。`row` はリスト表示の行の列に置く
 *   面も影も無い形（`size-6`、ハート 16px）で、オフは `text-muted-foreground`。オンはどちらも
 *   `text-favorite` の塗りである。オフはカード・行の hover、中へのフォーカス、`hover:none` の
 *   端末でだけ見える（選択のチェックと同じ条件）。
 * - 押すと `onToggle` を 1 回呼ぶ。`click` の伝播を止め、カードのリンクも選択も動かさない。
 *   決着するまでは `aria-disabled` で回る印に替え、重ねて送らない。印は応答の通知で
 *   一覧が差し替える（api/favorites.ts）。失敗はトーストで伝え、印は変えない。
 * - `page` は再生画面の情報の行の右端の一群に置く形で、`IconButton`（`sm`、ghost）である
 *   （ui-design.md「Video page」）。オフは一群の他の操作と同じ `text-muted-foreground`、オンは
 *   `active` の面（`bg-primary-soft`）のまま、ハートの色だけを `text-favorite!` で上書きする
 *   （`data-active:text-primary` より詳細度が高いため `!` が要る）。常に見える。失敗は `onFailed` へ渡し、
 *   呼び出し側が情報の行の直下の 1 行に出す（トーストは出さない）。
 */
export default function FavoriteToggle({
  favorite,
  label,
  onToggle,
  variant,
  onFailed,
}: {
  favorite: boolean;
  label: UiText;
  onToggle: () => Promise<unknown>;
  variant: "card" | "row" | "page";
  /** 失敗の理由を受け取る。渡さなければトーストで伝える。 */
  onFailed?: (reason: UiText) => void;
}) {
  const toast = useToast();
  const [pending, setPending] = useState(false);
  const pendingRef = useRef(false);
  const mounted = useRef(true);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);

  function press(event: MouseEvent<HTMLButtonElement>) {
    event.stopPropagation();
    event.preventDefault();
    if (pendingRef.current) return;
    pendingRef.current = true;
    setPending(true);
    void onToggle()
      .catch((error: unknown) => {
        if (onFailed !== undefined) onFailed(errorText(error));
        else toast(t.list.card.favoriteFailed(errorText(error)));
      })
      .finally(() => {
        pendingRef.current = false;
        if (mounted.current) setPending(false);
      });
  }

  const Icon = pending ? LoaderCircle : Heart;
  const icon = (
    <Icon
      aria-hidden="true"
      className={cn(
        variant === "card" ? "size-5 drop-shadow-mark" : "size-4",
        pending && "animate-spin motion-reduce:animate-none",
        favorite && !pending && "fill-current",
      )}
    />
  );
  if (variant === "page") {
    return (
      <IconButton
        label={label}
        size="sm"
        active={favorite}
        aria-pressed={favorite}
        aria-disabled={pending || undefined}
        onClick={press}
        className={cn(
          favorite ? "text-favorite!" : "text-muted-foreground! hover:text-foreground!",
          pending && "cursor-progress",
        )}
      >
        {icon}
      </IconButton>
    );
  }
  return (
    <button
      type="button"
      aria-label={label}
      aria-pressed={favorite}
      aria-disabled={pending || undefined}
      onClick={press}
      className={cn(
        "inline-flex shrink-0 items-center justify-center rounded-sm transition-opacity duration-150 outline-none focus-visible:ring-3 focus-visible:ring-ring/50",
        variant === "card" ? "size-8" : "size-6",
        favorite
          ? "text-favorite"
          : variant === "card"
            ? "text-foreground"
            : "text-muted-foreground hover:text-foreground",
        favorite || pending
          ? "opacity-100"
          : "opacity-0 group-focus-within:opacity-100 group-hover:opacity-100 focus-visible:opacity-100 [@media(hover:none)]:opacity-100",
        pending && "cursor-progress",
      )}
    >
      {icon}
    </button>
  );
}
