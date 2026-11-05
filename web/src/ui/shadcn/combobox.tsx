import { CheckIcon, ChevronsUpDownIcon, PlusIcon } from "lucide-react";
import { Popover as PopoverPrimitive } from "radix-ui";
import { type ReactNode, useId, useState } from "react";

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

function matches(option: ComboboxOption, query: string): boolean {
  const needle = query.toLowerCase();
  return [option.value, option.label, ...(option.keywords ?? [])].some((text) =>
    text.toLowerCase().includes(needle),
  );
}

/**
 * `createLabel` lets the viewer keep a typed value that no option has: the create row comes first
 * and the trigger shows the value itself. `status` replaces the empty text, for a list still
 * loading or one that could not be read.
 */
function Combobox({
  options,
  value,
  onValueChange,
  onOpenChange,
  placeholder,
  searchPlaceholder,
  emptyText,
  status,
  createLabel,
  disabled,
  className,
  id,
  "aria-label": ariaLabel,
}: {
  options: readonly ComboboxOption[];
  value: string;
  onValueChange: (value: string) => void;
  onOpenChange?: (open: boolean) => void;
  placeholder: ReactNode;
  searchPlaceholder?: string;
  emptyText: ReactNode;
  status?: ReactNode;
  createLabel?: (typed: string) => ReactNode;
  disabled?: boolean;
  className?: string;
  id?: string;
  "aria-label"?: string;
}) {
  const [open, setOpen] = useState(false);
  const [search, setSearch] = useState("");
  // A per-instance prefix keeps the create row's cmdk value apart from every option's value.
  const createId = useId();
  const selected = options.find((option) => option.value === value);
  const creatable = createLabel !== undefined;
  const typed = search.trim();
  const showCreate =
    creatable && typed !== "" && !options.some((option) => option.value === typed);
  // cmdk would rank the create row among the options, so a creatable list filters itself.
  const shown =
    creatable && typed !== "" ? options.filter((o) => matches(o, typed)) : options;
  const label = selected?.label ?? (creatable && value !== "" ? value : undefined);

  const changeOpen = (next: boolean) => {
    setOpen(next);
    if (!next) setSearch("");
    onOpenChange?.(next);
  };
  const choose = (next: string) => {
    onValueChange(next);
    changeOpen(false);
  };

  return (
    <PopoverPrimitive.Root open={open} onOpenChange={changeOpen}>
      <PopoverPrimitive.Trigger asChild>
        <Button
          id={id}
          variant="outline"
          role="combobox"
          aria-expanded={open}
          aria-label={ariaLabel}
          disabled={disabled}
          className={cn("w-full justify-between font-normal", className)}
        >
          <span
            className={cn("truncate", label === undefined && "text-muted-foreground")}
          >
            {label ?? placeholder}
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
          <Command label={ariaLabel} shouldFilter={!creatable}>
            <CommandInput
              placeholder={searchPlaceholder}
              value={search}
              onValueChange={setSearch}
            />
            <CommandList label={ariaLabel}>
              {status === undefined ? (
                <CommandEmpty>{emptyText}</CommandEmpty>
              ) : (
                <div
                  data-slot="combobox-status"
                  role="status"
                  className="flex items-center gap-2 px-3 py-2 text-sm text-muted-foreground"
                >
                  {status}
                </div>
              )}
              {showCreate && (
                <CommandGroup>
                  <CommandItem
                    value={`${createId}${typed}`}
                    onSelect={() => choose(typed)}
                  >
                    <PlusIcon aria-hidden="true" />
                    <span className="truncate">{createLabel(typed)}</span>
                  </CommandItem>
                </CommandGroup>
              )}
              <CommandGroup>
                {shown.map((option) => (
                  <CommandItem
                    key={option.value}
                    value={option.value}
                    keywords={[option.label, ...(option.keywords ?? [])]}
                    disabled={option.disabled}
                    onSelect={() => choose(option.value)}
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
