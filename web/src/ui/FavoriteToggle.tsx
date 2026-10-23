import { Heart, LoaderCircle } from "lucide-react";
import { type MouseEvent, useEffect, useRef, useState } from "react";

import { errorText, t, type UiText } from "@/i18n";
import { cn } from "@/lib/cn";
import { Toggle } from "./shadcn/toggle";
import { useToast } from "./Toast";

/**
 * Render only for owners; stop click propagation and reject duplicate requests while pending. The
 * page form shows `label` as its word and reports via onFailed, while card and row placements use
 * it as the name and default to a toast; see web/registry/rules/components.md.
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
  /** Failures use a toast when this callback is absent. */
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
    // The visible word names the toggle, so the page form needs no tooltip.
    return (
      <Toggle
        size="sm"
        shape="pill"
        pressed={favorite}
        aria-disabled={pending || undefined}
        onClick={press}
        className={cn(
          "bg-secondary px-3 text-foreground hover:bg-secondary/80 hover:text-foreground data-[state=on]:bg-favorite-soft data-[state=on]:text-favorite aria-pressed:bg-favorite-soft aria-pressed:text-favorite",
          pending && "cursor-progress",
        )}
      >
        {icon}
        {label}
      </Toggle>
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
        "inline-flex shrink-0 items-center justify-center rounded-sm transition-opacity duration-150",
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
