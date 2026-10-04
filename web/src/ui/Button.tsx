import { type ButtonHTMLAttributes, forwardRef } from "react";

import { cn } from "../lib/cn";

export type ButtonVariant = "primary" | "secondary" | "ghost" | "danger";
export type ButtonSize = "sm" | "md" | "lg";

const base =
  "inline-flex shrink-0 items-center justify-center gap-2 rounded-md font-medium " +
  "whitespace-nowrap transition-colors duration-150 select-none " +
  "aria-busy:cursor-progress disabled:cursor-not-allowed disabled:opacity-50 [&>svg]:size-4";

const variants: Record<ButtonVariant, string> = {
  primary:
    "bg-primary text-primary-foreground not-disabled:hover:bg-primary-hover not-disabled:active:bg-primary-active",
  secondary:
    "border border-input bg-popover text-foreground not-disabled:hover:bg-secondary not-disabled:active:bg-card",
  ghost: "text-foreground not-disabled:hover:bg-accent not-disabled:active:bg-secondary",
  danger:
    "bg-destructive-strong text-destructive-foreground not-disabled:hover:bg-destructive-strong/80",
};

const sizes: Record<ButtonSize, string> = {
  sm: "h-8 px-2.5 text-xs",
  md: "h-9 px-3 text-sm",
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
