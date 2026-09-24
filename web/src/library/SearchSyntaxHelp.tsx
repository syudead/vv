import { CircleHelp } from "lucide-react";
import { type ReactNode, useId, useState } from "react";

import { cn } from "../lib/cn";
import { PopoverContent, PopoverRoot, PopoverTrigger } from "../ui/Popover";

/** syntaxRows は手引きの4行の対応表である（ui-design.md「Syntax help」）。 */
const syntaxRows: { examples: string[]; meaning: ReactNode }[] = [
  { examples: ["京都 2024"], meaning: "空白で区切った語をすべて含む" },
  {
    examples: ['"京都旅行 2024"'],
    meaning: (
      <>
        <code className="font-mono">"</code> で囲んだ部分を、空白ごと1つの語として探す
      </>
    ),
  },
  {
    examples: ["京都 -2023"],
    meaning: (
      <>
        <code className="font-mono">-</code> を付けた語を含むものを除く
      </>
    ),
  },
  {
    examples: ["京都 OR 奈良", "京都 | 奈良"],
    meaning: "どちらかを含む。空白より強く結び付く",
  },
];

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
          aria-label="検索の書き方"
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
          検索の書き方
        </h2>
        {/* ボタンの説明は見出しを除いた中身にする（名前と同じ語を二度読ませない）。 */}
        <div id={bodyId}>
          <dl className="mt-3 grid grid-cols-[auto_1fr] gap-x-3 gap-y-2">
            {syntaxRows.map((row) => (
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
          <p className="text-xs text-fg-muted">
            全角と半角、大文字と小文字、ひらがなとカタカナは区別しません。
          </p>
          <p className="mt-1 text-xs text-fg-muted">語は先頭から 16 個まで使います。</p>
        </div>
      </PopoverContent>
    </PopoverRoot>
  );
}
