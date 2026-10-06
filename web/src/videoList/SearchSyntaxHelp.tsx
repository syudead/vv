import { CircleHelp } from "lucide-react";
import { type ReactNode, useId, useState } from "react";

import { t, type UiText } from "../i18n";
import { Button } from "../ui/shadcn/button";
import { Kbd } from "../ui/shadcn/kbd";
import { Popover, PopoverContent, PopoverTrigger } from "../ui/shadcn/popover";
import { Separator } from "../ui/shadcn/separator";

/** maxSearchTerms はサーバーが検索に使う語の数の上限である（手引きの説明に出す）。 */
const maxSearchTerms = 16;

/**
 * syntaxRows は手引きの4行の対応表である（ui-design.md「Syntax help」）。描画のたびに
 * カタログから作る。
 */
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

/**
 * SearchSyntaxHelp は検索欄の枠の内側に置く、検索の書き方の手引きである。
 * 読むだけの従の情報なので、開いてもフォーカスはボタンに残し、モーダルにしない。
 * Tab で外へ出るか Esc で閉じる。例は押しても検索欄に入らない。
 */
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
        {/* ボタンの説明は見出しを除いた中身にする（名前と同じ語を二度読ませない）。 */}
        <div id={bodyId} className="flex flex-col gap-3">
          {/* 例の列は中身の幅、説明の列は残り（grid-cols-term）。 */}
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
