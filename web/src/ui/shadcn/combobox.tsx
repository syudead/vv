import { CheckIcon, ChevronsUpDownIcon } from "lucide-react";
import { Popover as PopoverPrimitive } from "radix-ui";
import { type ReactNode, useState } from "react";

import { cn } from "@/lib/cn";
import { Button } from "@/ui/shadcn/button";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/ui/shadcn/command";

// Adapted from shadcn/ui Combobox using Radix Popover and Command.
// Radix positioning variables are explicitly allowed in web/design-exceptions.js.

export interface ComboboxOption {
  value: string;
  label: string;
  keywords?: string[];
  disabled?: boolean;
}

function Combobox({
  options,
  value,
  onValueChange,
  placeholder,
  searchPlaceholder,
  emptyText,
  disabled,
  className,
  "aria-label": ariaLabel,
}: {
  options: readonly ComboboxOption[];
  value: string;
  onValueChange: (value: string) => void;
  placeholder: ReactNode;
  searchPlaceholder?: string;
  emptyText: ReactNode;
  disabled?: boolean;
  className?: string;
  "aria-label"?: string;
}) {
  const [open, setOpen] = useState(false);
  const selected = options.find((option) => option.value === value);

  return (
    <PopoverPrimitive.Root open={open} onOpenChange={setOpen}>
      <PopoverPrimitive.Trigger asChild>
        <Button
          variant="outline"
          role="combobox"
          aria-expanded={open}
          aria-label={ariaLabel}
          disabled={disabled}
          className={cn("w-full justify-between font-normal", className)}
        >
          <span
            className={cn("truncate", selected === undefined && "text-muted-foreground")}
          >
            {selected?.label ?? placeholder}
          </span>
          <ChevronsUpDownIcon className="text-muted-foreground" />
        </Button>
      </PopoverPrimitive.Trigger>
      <PopoverPrimitive.Portal>
        <PopoverPrimitive.Content
          data-slot="combobox-content"
          align="start"
          sideOffset={4}
          className="z-50 w-(--radix-popover-trigger-width) origin-(--radix-popover-content-transform-origin) rounded-lg border border-border bg-popover p-0 text-popover-foreground shadow-elevated outline-hidden data-open:animate-pop-in motion-reduce:animate-none"
        >
          <Command label={ariaLabel}>
            <CommandInput placeholder={searchPlaceholder} />
            <CommandList label={ariaLabel}>
              <CommandEmpty>{emptyText}</CommandEmpty>
              <CommandGroup>
                {options.map((option) => (
                  <CommandItem
                    key={option.value}
                    value={option.value}
                    keywords={[option.label, ...(option.keywords ?? [])]}
                    disabled={option.disabled}
                    onSelect={(next) => {
                      onValueChange(next);
                      setOpen(false);
                    }}
                  >
                    <span className="truncate">{option.label}</span>
                    <CheckIcon
                      className={cn(
                        "ml-auto text-primary",
                        option.value === value ? "opacity-100" : "opacity-0",
                      )}
                    />
                  </CommandItem>
                ))}
              </CommandGroup>
            </CommandList>
          </Command>
        </PopoverPrimitive.Content>
      </PopoverPrimitive.Portal>
    </PopoverPrimitive.Root>
  );
}

export { Combobox };
