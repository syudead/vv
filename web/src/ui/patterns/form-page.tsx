import type { ReactNode } from "react";

import { cn } from "@/lib/cn";
import { FieldLegend, FieldSet } from "@/ui/shadcn/field";

// 入力画面（骨格）。ダイアログではない 1 枚の画面で、まとめて保存する多くの値を入れる
// （FormDialog は少しの値、SettingsPage は行ごとに保存）。上端の帯（題・薄い詳細・右端の
// 操作、下に線）はどの幅でも上に貼り付く。帯の下に失敗の行（notice）、その下に左の列（aside）と
// 欄（panels）を置く。lg からは grid-cols-form-page で aside を form-page-aside の幅の列に、
// 欄を 5:6 で並べ、ページを画面の高さに留めて欄ごとに中でスクロールさせる。lg 未満は上から
// 順に積み、ページ全体が帯の下でスクロールする。管理画面の密度。
// 規則は web/registry/rules/patterns.md の Form page。

export interface FormPagePanel {
  /** 欄を区別する鍵。 */
  key: string;
  /** 欄の名前（FieldLegend）。 */
  legend: ReactNode;
  /** 欄の中身。上から入力の順に並べ、骨格が gap-4 で離す。 */
  content: ReactNode;
}

export interface FormPageProps {
  /** 画面の名前（h1、「Edit details」）。 */
  title: ReactNode;
  /** 題の後ろの薄い詳細（何を編集しているか）。sm 未満は出さない。 */
  detail?: ReactNode;
  /** 帯の右端の操作。`Cancel`（ghost）と 1 つの default のボタン。 */
  actions: ReactNode;
  /** 帯の下、列の上に置く失敗の行。 */
  notice?: ReactNode;
  /** 左の列（何を編集しているかを示す、編集できない値）。 */
  aside?: ReactNode;
  /** 左の列（aside 要素）の読み上げ名。 */
  asideLabel?: string;
  /** 欄。2 つまでを左から並べる。 */
  panels: readonly FormPagePanel[];
  /** 列の代わりに本文に置く状態の表示（読み込みの失敗など）。渡すと列を出さない。 */
  status?: ReactNode;
}

export function FormPage({
  title,
  detail,
  actions,
  notice,
  aside,
  asideLabel,
  panels,
  status,
}: FormPageProps) {
  return (
    <div
      data-slot="form-page"
      className="flex min-h-dvh flex-col bg-background text-foreground lg:h-dvh lg:min-h-0 lg:overflow-hidden"
    >
      <header
        data-slot="form-page-header"
        className="sticky top-0 z-20 flex h-navbar shrink-0 items-center gap-3 border-b border-border bg-background/90 px-4 backdrop-blur-md sm:px-6"
      >
        <div className="flex min-w-0 flex-1 items-baseline gap-3">
          <h1 className="shrink-0 text-lg leading-none font-semibold">{title}</h1>
          {detail !== undefined && (
            <p className="hidden min-w-0 truncate text-sm text-muted-foreground sm:block">
              {detail}
            </p>
          )}
        </div>
        <div className="flex shrink-0 items-center gap-2">{actions}</div>
      </header>
      <div
        data-slot="form-page-body"
        className="flex flex-1 flex-col gap-6 px-4 py-6 sm:px-6 lg:min-h-0"
      >
        {notice}
        {status ?? (
          <div className="grid flex-1 grid-cols-1 gap-6 lg:min-h-0 lg:grid-cols-form-page">
            {aside !== undefined && (
              <aside
                aria-label={asideLabel}
                className="flex min-w-0 flex-col gap-3 text-sm"
              >
                {aside}
              </aside>
            )}
            {panels.map((panel, index) => (
              <div
                key={panel.key}
                data-slot="form-page-panel"
                className={cn(
                  // 欄の端の入力のフォーカスの輪が欄のスクロールで切れないよう、1 段の余白を持つ。
                  "min-w-0 lg:-mx-1 lg:min-h-0 lg:overflow-y-auto lg:px-1",
                  // 左の列が無いときは、最初の欄が左の列の分まで広がる。
                  aside === undefined && index === 0 && "lg:col-span-2",
                )}
              >
                <FieldSet className="min-h-full min-w-0">
                  <FieldLegend>{panel.legend}</FieldLegend>
                  {panel.content}
                </FieldSet>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
