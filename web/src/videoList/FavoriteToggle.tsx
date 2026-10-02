import { Heart, LoaderCircle } from "lucide-react";
import { type MouseEvent, useEffect, useRef, useState } from "react";

import { errorText, t, type UiText } from "../i18n";
import { cn } from "../lib/cn";
import { useToast } from "../ui/Toast";

/**
 * FavoriteToggle はカードと行のお気に入りの付け外しである
 * （specs/035-favorites/ui-design.md「Mark」「Card」）。印を兼ね、塗りのハートがお気に入り、
 * 線のハートがお気に入りでない。所有者の画面でだけ描く（呼び出し側が決める）。
 *
 * - `card` は格子のカードのサムネイルの右上に重ねる面付きの形、`row` はリスト表示の行の
 *   列に置く面なしの形である。オフはカード・行の hover、中へのフォーカス、`hover:none` の
 *   端末でだけ見える（選択のチェックと同じ条件）。
 * - 押すと `onToggle` を 1 回呼ぶ。`click` の伝播を止め、カードのリンクも選択も動かさない。
 *   決着するまでは `aria-disabled` で回る印に替え、重ねて送らない。印は応答の通知で
 *   一覧が差し替える（api/favorites.ts）。失敗はトーストで伝え、印は変えない。
 */
export default function FavoriteToggle({
  favorite,
  label,
  onToggle,
  variant,
  previewing = false,
}: {
  favorite: boolean;
  label: UiText;
  onToggle: () => Promise<unknown>;
  variant: "card" | "row";
  /** カードのホバープレビュー中は、面を不透明にする（チェックと同じ）。 */
  previewing?: boolean;
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
        toast(t.list.card.favoriteFailed(errorText(error)));
      })
      .finally(() => {
        pendingRef.current = false;
        if (mounted.current) setPending(false);
      });
  }

  const Icon = pending ? LoaderCircle : Heart;
  return (
    <button
      type="button"
      aria-label={label}
      aria-pressed={favorite}
      aria-disabled={pending || undefined}
      onClick={press}
      className={cn(
        "inline-flex size-6 shrink-0 items-center justify-center rounded-sm transition-opacity duration-150 outline-none focus-visible:outline-2 focus-visible:outline-link",
        variant === "card" &&
          (previewing ? "bg-navbar" : "bg-navbar/90 backdrop-blur-sm"),
        favorite
          ? "text-link"
          : variant === "card"
            ? "text-fg"
            : "text-fg-muted hover:text-fg",
        favorite || pending
          ? "opacity-100"
          : "opacity-0 group-focus-within:opacity-100 group-hover:opacity-100 focus-visible:opacity-100 [@media(hover:none)]:opacity-100",
        pending && "cursor-progress",
      )}
    >
      <Icon
        aria-hidden="true"
        className={cn(
          "size-4",
          pending && "animate-spin motion-reduce:animate-none",
          favorite && !pending && "fill-current",
        )}
      />
    </button>
  );
}
