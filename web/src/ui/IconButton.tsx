import { type ButtonHTMLAttributes, forwardRef } from "react";

import { cn } from "../lib/cn";
import Tooltip from "./Tooltip";

export interface IconButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  /** 読み上げ名。ツールチップにも使う。 */
  label: string;
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
        "disabled:pointer-events-none disabled:opacity-50",
        variant === "ghost"
          ? "text-fg hover:bg-hover-wash active:bg-active-wash data-active:bg-active-wash"
          : "bg-elevated text-fg hover:bg-hover-wash active:bg-active-wash data-active:bg-active-wash",
        sizes[size],
        className,
      )}
      {...rest}
    />
  );
  return tooltip ? <Tooltip content={label}>{button}</Tooltip> : button;
});

export default IconButton;
