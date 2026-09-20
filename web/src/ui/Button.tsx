import { type ButtonHTMLAttributes, forwardRef } from "react";

import { cn } from "../lib/cn";

export type ButtonVariant = "primary" | "secondary" | "ghost" | "danger";
export type ButtonSize = "sm" | "md" | "lg";

const base =
  "inline-flex shrink-0 items-center justify-center gap-2 rounded-md font-medium " +
  "whitespace-nowrap transition-colors duration-150 select-none " +
  "disabled:pointer-events-none disabled:opacity-50 [&>svg]:size-4";

const variants: Record<ButtonVariant, string> = {
  primary: "bg-accent text-accent-fg hover:bg-accent-hover active:bg-accent",
  secondary:
    "bg-surface text-fg border border-border hover:bg-surface-hover hover:border-border-strong active:bg-surface",
  ghost: "text-fg-muted hover:bg-surface-hover hover:text-fg active:bg-surface",
  danger: "bg-danger-soft text-danger hover:bg-danger/25",
};

const sizes: Record<ButtonSize, string> = {
  sm: "h-8 px-3 text-xs",
  md: "h-9 px-3.5 text-sm",
  lg: "h-10 px-4 text-sm",
};

/** buttonClassName はリンクなど button 以外の要素にボタンの見た目を与える。 */
export function buttonClassName(
  variant: ButtonVariant = "secondary",
  size: ButtonSize = "md",
  className?: string,
): string {
  return cn(base, variants[variant], sizes[size], className);
}

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
  size?: ButtonSize;
}

const Button = forwardRef<HTMLButtonElement, ButtonProps>(function Button(
  { variant = "secondary", size = "md", className, type = "button", ...rest },
  ref,
) {
  return (
    <button
      ref={ref}
      type={type}
      className={buttonClassName(variant, size, className)}
      {...rest}
    />
  );
});

export default Button;
