import { CircleHelp } from "lucide-react";
import { type ReactNode, useId, useState } from "react";

import { t, type UiText } from "../i18n";
import { Button } from "../ui/shadcn/button";
import { Kbd } from "../ui/shadcn/kbd";
import { Popover, PopoverContent, PopoverTrigger } from "../ui/shadcn/popover";
import { Separator } from "../ui/shadcn/separator";

const maxSearchTerms = 16;

/** Build syntax copy from the current catalog on each render. */
function syntaxRows(): { examples: UiText[]; meaning: ReactNode }[] {
  const help = t.list.searchHelp;
  return [
    { examples: [help.allWords.example], meaning: help.allWords.meaning },
    {
      examples: [help.phrase.example],
      meaning: (
        <>
          {help.phrase.before}
          <Kbd>"</Kbd>
          {help.phrase.after}
        </>
      ),
    },
    {
      examples: [help.exclude.example],
      meaning: (
        <>
          {help.exclude.before}
          <Kbd>-</Kbd>
          {help.exclude.after}
        </>
      ),
    },
    {
      examples: [help.either.example, help.either.alternative],
      meaning: help.either.meaning,
    },
  ];
}

/** Nonmodal help keeps trigger focus and closes on Tab or Escape; examples never modify the query. */
export default function SearchSyntaxHelp() {
  const [open, setOpen] = useState(false);
  const bodyId = useId();
  const headingId = useId();

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label={t.list.searchHelp.title}
          aria-describedby={open ? bodyId : undefined}
          className="size-6 text-muted-foreground data-[state=open]:bg-accent data-[state=open]:text-foreground"
        >
          <CircleHelp aria-hidden="true" />
        </Button>
      </PopoverTrigger>
      <PopoverContent
        align="end"
        className="w-popover-wide"
        aria-labelledby={headingId}
        onOpenAutoFocus={(event) => event.preventDefault()}
      >
        <h2 id={headingId} className="text-sm font-medium text-foreground">
          {t.list.searchHelp.title}
        </h2>
        {/* Exclude the heading from the description to avoid repeating the trigger name. */}
        <div id={bodyId} className="flex flex-col gap-3">
          <dl className="grid grid-cols-term gap-x-3 gap-y-2">
            {syntaxRows().map((row) => (
              <div key={row.examples.join()} className="contents">
                <dt className="flex flex-col gap-0.5 font-mono text-xs text-foreground">
                  {row.examples.map((example) => (
                    <span key={example} className="whitespace-pre">
                      {example}
                    </span>
                  ))}
                </dt>
                <dd className="text-xs text-muted-foreground">{row.meaning}</dd>
              </div>
            ))}
          </dl>
          <Separator />
          <div className="flex flex-col gap-1 text-xs text-muted-foreground">
            <p>{t.list.searchHelp.normalization}</p>
            <p>{t.list.searchHelp.termLimit(maxSearchTerms)}</p>
          </div>
        </div>
      </PopoverContent>
    </Popover>
  );
}
