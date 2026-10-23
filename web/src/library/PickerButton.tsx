import type { LucideIcon } from "lucide-react";
import { type ReactNode, useState } from "react";

import type { UiText } from "../i18n";
import { cn } from "../lib/cn";
import { Button } from "../ui/shadcn/button";
import { Popover, PopoverTrigger } from "../ui/shadcn/popover";

/** Keep triggers visible while open; isolate them from card navigation and selection. */
export default function PickerButton({
  variant,
  label,
  icon: Icon,
  picker,
}: {
  variant: "card" | "row" | "page";
  /** The page form's visible word; the other forms' accessible name. */
  label: UiText;
  icon: LucideIcon;
  picker: (open: boolean, onOpenChange: (open: boolean) => void) => ReactNode;
}) {
  const [open, setOpen] = useState(false);
  const content = picker(open, setOpen);

  if (variant === "page") {
    return (
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger asChild>
          <Button variant="secondary" size="sm" shape="pill" className="text-foreground">
            <Icon aria-hidden="true" />
            {label}
          </Button>
        </PopoverTrigger>
        {content}
      </Popover>
    );
  }

  const card = variant === "card";
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="ghost"
          aria-label={label}
          // Stop card navigation/selection without preventDefault, which would block Radix toggling.
          onClick={(event) => event.stopPropagation()}
          className={cn(
            "rounded-sm p-0 transition-opacity duration-150 hover:bg-transparent active:bg-transparent",
            card
              ? "size-8 text-foreground hover:text-foreground [&_svg]:size-5"
              : "size-6 text-muted-foreground hover:text-foreground",
            open
              ? "opacity-100"
              : "opacity-0 group-focus-within:opacity-100 group-hover:opacity-100 focus-visible:opacity-100 [@media(hover:none)]:opacity-100",
          )}
        >
          <Icon aria-hidden="true" className={cn(card && "drop-shadow-mark")} />
        </Button>
      </PopoverTrigger>
      {content}
    </Popover>
  );
}
