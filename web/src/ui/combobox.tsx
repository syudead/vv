import { CheckIcon, ChevronsUpDownIcon } from "lucide-react";
import { Popover as PopoverPrimitive } from "radix-ui";
import { type ReactNode, useState } from "react";

import { cn } from "@/lib/cn";
import { Button } from "@/ui/button";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/ui/command";

// shadcn/ui の Combobox（Popover の中の Command）。多くの候補から 1 つを、打って絞って選ぶ。
// 浮く層は Radix Popover をここで直に組む（Popover の部品は重ね表示の単位が作る）。
// 一覧の幅と原点は Radix の CSS 変数で決まるので、その変数を読むクラスだけを
// web/design-exceptions.js の special で許す。
// 規則は web/registry/rules/components.md の Combobox and Command。

export interface ComboboxOption {
  value: string;
  label: string;
  /** 絞り込みで label のほかに当てる語（シノニムなど）。 */
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
  /** 選んでいる値。何も選んでいなければ空文字。 */
  value: string;
  onValueChange: (value: string) => void;
  /** 何も選んでいないときにボタンに出す文言。 */
  placeholder: ReactNode;
  searchPlaceholder?: string;
  /** 打った文字に合う候補が無いときの文言。 */
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
          className="z-50 w-(--radix-popover-trigger-width) origin-(--radix-popover-content-transform-origin) rounded-lg border border-border bg-popover p-0 text-popover-foreground shadow-elevated outline-hidden data-[side=bottom]:slide-in-from-top-2 data-[side=left]:slide-in-from-right-2 data-[side=right]:slide-in-from-left-2 data-[side=top]:slide-in-from-bottom-2 data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=closed]:zoom-out-95 data-[state=open]:animate-in data-[state=open]:fade-in-0 data-[state=open]:zoom-in-95"
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
