import { type ButtonHTMLAttributes, forwardRef } from "react";

import type { UiText } from "../i18n";
import { cn } from "../lib/cn";
import Tooltip from "./Tooltip";

export interface IconButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  /** 読み上げ名。ツールチップにも使う。 */
  label: UiText;
  size?: "sm" | "md" | "lg";
  variant?: "ghost" | "solid";
  active?: boolean;
  /** ツールチップを出さない（メニューのトリガなど、別の説明がある場合）。 */
  tooltip?: boolean;
}

const sizes = { sm: "h-8 w-8", md: "h-9 w-9", lg: "h-10 w-10" };

const IconButton = forwardRef<HTMLButtonElement, IconButtonProps>(function IconButton(
  {
    label,
    size = "md",
    variant = "ghost",
    active = false,
    tooltip = true,
    className,
    type = "button",
    ...rest
  },
  ref,
) {
  const button = (
    <button
      ref={ref}
      type={type}
      aria-label={label}
      data-active={active || undefined}
      className={cn(
        "inline-flex shrink-0 items-center justify-center rounded-md transition-colors duration-150 select-none [&>svg]:size-4",
        "aria-busy:cursor-progress disabled:cursor-not-allowed disabled:opacity-50",
        variant === "ghost"
          ? "text-foreground enabled:hover:bg-accent enabled:active:bg-secondary data-active:bg-primary-soft data-active:text-primary"
          : "border border-input bg-popover text-foreground enabled:hover:bg-secondary enabled:active:bg-card data-active:border-primary-active data-active:bg-primary-soft data-active:text-primary",
        sizes[size],
        className,
      )}
      {...rest}
    />
  );
  return tooltip ? <Tooltip content={label}>{button}</Tooltip> : button;
});

export default IconButton;
