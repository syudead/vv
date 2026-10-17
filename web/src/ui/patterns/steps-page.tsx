import { Check } from "lucide-react";
import { type ReactNode, useId } from "react";

import { cn } from "@/lib/cn";
import BrandHomeLink from "@/ui/BrandHomeLink";

// 手順の画面（骨格）。シェルの外の 1 枚の画面で、決まった順の手順を 1 つずつ進める
// （オンボードのような最初の設定）。上端の帯（navbar の高さ）の左に BrandHomeLink、中央に
// 手順の帯を置く。帯は手順の名前に番号を添え、今の手順を primary、済んだ手順を印で示す。
// sm 未満は「Step 2 of 3」と今の名前だけにする。帯の下は max-w-2xl の 1 列を中央に置き
// （p-4、sm から p-6）、題（h1、text-xl）・説明・中身・失敗の行・下端の行（左に Skip、右に
// 1 つの default の操作）を縦に並べる。管理画面の密度。
// 規則は web/registry/rules/patterns.md の Steps page。

export interface StepsPageStep {
  /** 手順を区別する鍵。 */
  key: string;
  /** 帯に出す手順の名前。 */
  name: ReactNode;
  /** 読み上げの名前。済んだ手順は済んだことを、今の手順は今であることを含める。 */
  label: string;
}

export interface StepsPageProps {
  /** 手順を進む順に並べたもの。 */
  steps: readonly StepsPageStep[];
  /** 今の手順の位置（steps の添字）。 */
  current: number;
  /** 手順の帯（nav 要素）の読み上げ名。 */
  stepsLabel: string;
  /** sm 未満の帯に今の名前の前に出す位置（「Step 2 of 3」）。 */
  progress: ReactNode;
  /** 手順の題。h1 になる。 */
  title: ReactNode;
  /** 題の下の説明。 */
  description?: ReactNode;
  /** 手順の中身。上から並べ、骨格が gap-4 で離す。 */
  children?: ReactNode;
  /** 下端の行の上に置く失敗の Alert。 */
  notice?: ReactNode;
  /** 下端の行の左の `Skip`（ghost の sm）。 */
  skip: ReactNode;
  /** 下端の行の右の主操作（default の sm、1 つ）。 */
  action: ReactNode;
}

function StepMark({ index, done, current }: { index: number; done: boolean; current: boolean }) {
  return (
    <span
      aria-hidden="true"
      className={cn(
        "flex size-6 shrink-0 items-center justify-center rounded-full border text-xs font-medium tabular-nums",
        current && "border-primary bg-primary text-primary-foreground",
        done && "border-primary text-primary",
        !current && !done && "border-border text-muted-foreground",
      )}
    >
      {done ? <Check className="size-3" /> : index + 1}
    </span>
  );
}

export function StepsPage({
  steps,
  current,
  stepsLabel,
  progress,
  title,
  description,
  children,
  notice,
  skip,
  action,
}: StepsPageProps) {
  const titleId = useId();
  const currentStep = steps[current];
  return (
    <div
      data-slot="steps-page"
      className="flex min-h-dvh flex-col bg-background text-foreground"
    >
      <header
        data-slot="steps-page-header"
        className="sticky top-0 z-20 flex h-navbar shrink-0 items-center gap-3 border-b border-border bg-background/90 px-4 backdrop-blur-md sm:px-6"
      >
        <div className="flex flex-1 basis-0 items-center">
          <BrandHomeLink />
        </div>
        <nav aria-label={stepsLabel} className="flex min-w-0 shrink justify-center">
          <ol className="hidden items-center gap-4 sm:flex">
            {steps.map((step, index) => {
              const isCurrent = index === current;
              return (
                <li
                  key={step.key}
                  aria-current={isCurrent ? "step" : undefined}
                  className="flex items-center gap-2 text-sm"
                >
                  <StepMark index={index} done={index < current} current={isCurrent} />
                  <span
                    aria-hidden="true"
                    className={cn(
                      "whitespace-nowrap",
                      isCurrent ? "font-medium text-primary" : "text-muted-foreground",
                    )}
                  >
                    {step.name}
                  </span>
                  <span className="sr-only">{step.label}</span>
                </li>
              );
            })}
          </ol>
          <p className="flex min-w-0 items-baseline gap-2 text-sm sm:hidden">
            <span className="shrink-0 text-muted-foreground">{progress}</span>
            <span className="truncate font-medium text-primary">{currentStep?.name}</span>
          </p>
        </nav>
        <div aria-hidden="true" className="hidden flex-1 basis-0 sm:block" />
      </header>
      <main
        aria-labelledby={titleId}
        data-slot="steps-page-body"
        className="mx-auto flex w-full max-w-2xl flex-1 flex-col gap-6 p-4 sm:p-6"
      >
        <div className="flex flex-col gap-1">
          <h1 id={titleId} className="text-xl font-semibold">
            {title}
          </h1>
          {description !== undefined && (
            <p className="text-sm text-muted-foreground">{description}</p>
          )}
        </div>
        {children !== undefined && (
          <div data-slot="steps-page-content" className="flex min-w-0 flex-col gap-4">
            {children}
          </div>
        )}
        {notice}
        <div
          data-slot="steps-page-footer"
          className="flex items-center justify-between gap-2"
        >
          {skip}
          {action}
        </div>
      </main>
    </div>
  );
}
