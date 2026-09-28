import { CircleHelp } from "lucide-react";
import { type ReactNode, useId, useState } from "react";

import { t, type UiText } from "../i18n";
import { cn } from "../lib/cn";
import { PopoverContent, PopoverRoot, PopoverTrigger } from "../ui/Popover";

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
          <code className="font-mono">"</code>
          {help.phrase.after}
        </>
      ),
    },
    {
      examples: [help.exclude.example],
      meaning: (
        <>
          {help.exclude.before}
          <code className="font-mono">-</code>
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
export default function SearchSyntaxHelp({ className }: { className?: string }) {
  const [open, setOpen] = useState(false);
  const bodyId = useId();
  const headingId = useId();

  return (
    <PopoverRoot open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <button
          type="button"
          aria-label={t.list.searchHelp.title}
          aria-describedby={open ? bodyId : undefined}
          className={cn(
            "flex size-6 items-center justify-center rounded-sm text-fg-muted transition-colors hover:bg-hover-wash hover:text-fg active:bg-active-wash data-[state=open]:bg-active-wash data-[state=open]:text-fg",
            className,
          )}
        >
          <CircleHelp className="size-4" />
        </button>
      </PopoverTrigger>
      <PopoverContent
        align="end"
        className="w-80"
        aria-labelledby={headingId}
        onOpenAutoFocus={(event) => event.preventDefault()}
      >
        <h2 id={headingId} className="text-sm font-medium text-fg">
          {t.list.searchHelp.title}
        </h2>
        {/* ボタンの説明は見出しを除いた中身にする（名前と同じ語を二度読ませない）。 */}
        <div id={bodyId}>
          <dl className="mt-3 grid grid-cols-[auto_1fr] gap-x-3 gap-y-2">
            {syntaxRows().map((row) => (
              <div key={row.examples.join()} className="contents">
                <dt className="flex flex-col gap-0.5 font-mono text-xs text-fg">
                  {row.examples.map((example) => (
                    <span key={example} className="whitespace-pre">
                      {example}
                    </span>
                  ))}
                </dt>
                <dd className="text-xs text-fg-muted">{row.meaning}</dd>
              </div>
            ))}
          </dl>
          <div role="none" className="my-3 h-px bg-border" />
          <p className="text-xs text-fg-muted">{t.list.searchHelp.normalization}</p>
          <p className="mt-1 text-xs text-fg-muted">
            {t.list.searchHelp.termLimit(maxSearchTerms)}
          </p>
        </div>
      </PopoverContent>
    </PopoverRoot>
  );
}
