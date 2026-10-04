import { t } from "../i18n";

// 見本の各項目は、Tailwind が生成できるようクラス名をそのまま書く（組み立てない）。
// 値と役割の割り当ては src/ui/tokens.css と docs/design-docs/design-system.md の Foundations。

const surfaces = [
  { token: "navbar", className: "bg-navbar" },
  { token: "background", className: "bg-background" },
  { token: "muted", className: "bg-muted" },
  { token: "card", className: "bg-card" },
  { token: "popover", className: "bg-popover" },
] as const;

const colours = [
  { token: "foreground", className: "bg-foreground" },
  { token: "muted-foreground", className: "bg-muted-foreground" },
  { token: "secondary", className: "bg-secondary" },
  { token: "accent", className: "bg-accent" },
  { token: "primary", className: "bg-primary" },
  { token: "primary-hover", className: "bg-primary-hover" },
  { token: "primary-active", className: "bg-primary-active" },
  { token: "primary-soft", className: "bg-primary-soft" },
  { token: "border", className: "bg-border" },
  { token: "input", className: "bg-input" },
  { token: "ring", className: "bg-ring" },
  { token: "destructive", className: "bg-destructive" },
  { token: "destructive-strong", className: "bg-destructive-strong" },
  { token: "destructive-soft", className: "bg-destructive-soft" },
  { token: "warning", className: "bg-warning" },
  { token: "warning-soft", className: "bg-warning-soft" },
  { token: "success", className: "bg-success" },
  { token: "success-soft", className: "bg-success-soft" },
  { token: "favorite", className: "bg-favorite" },
  { token: "overlay", className: "bg-overlay" },
] as const;

const typeSteps = [
  { token: "text-xl font-semibold", className: "text-xl font-semibold" },
  { token: "text-lg font-semibold", className: "text-lg font-semibold" },
  { token: "text-base", className: "text-base" },
  { token: "text-sm font-medium", className: "text-sm font-medium" },
  { token: "text-sm", className: "text-sm" },
  { token: "text-xs", className: "text-xs text-muted-foreground" },
  { token: "text-2xs font-medium", className: "text-2xs font-medium" },
] as const;

const spacingSteps = [
  { token: "0.5", className: "w-0.5" },
  { token: "1", className: "w-1" },
  { token: "1.5", className: "w-1.5" },
  { token: "2", className: "w-2" },
  { token: "3", className: "w-3" },
  { token: "4", className: "w-4" },
  { token: "5", className: "w-5" },
  { token: "6", className: "w-6" },
  { token: "8", className: "w-8" },
  { token: "9", className: "w-9" },
  { token: "10", className: "w-10" },
  { token: "12", className: "w-12" },
  { token: "16", className: "w-16" },
] as const;

const namedSizes = [
  "navbar",
  "sidebar",
  "sidebar-rail",
  "card-0",
  "card-1",
  "card-2",
  "card-3",
  "list-thumb-cell",
  "list-thumb",
  "list-number",
  "list-number-wide",
  "list-date",
  "search-min",
  "search-min-sm",
  "zoom",
  "selection-bar",
  "selection-bar-clearance",
  "popover",
  "popover-wide",
  "chip-label",
  "menu",
  "detail-aside",
] as const;

const radii = [
  { token: "rounded-sm", className: "rounded-sm" },
  { token: "rounded-md", className: "rounded-md" },
  { token: "rounded-lg", className: "rounded-lg" },
  { token: "rounded-full", className: "rounded-full" },
] as const;

const shadows = [
  { token: "shadow-card-hover", className: "rounded-md bg-card shadow-card-hover" },
  { token: "shadow-elevated", className: "rounded-lg bg-popover shadow-elevated" },
  {
    token: "drop-shadow-mark",
    className: "rounded-md bg-foreground text-favorite drop-shadow-mark",
  },
] as const;

function Heading({ children }: { children: string }) {
  return <h3 className="text-sm font-semibold text-muted-foreground">{children}</h3>;
}

function Code({ children }: { children: string }) {
  return <code className="font-mono text-xs text-foreground">{children}</code>;
}

/**
 * Foundations は見本の基礎の節である。面の 5 段階、色の役割、文字・余白・角丸・影の段階を
 * 並べ、メンテナーが段階ごとに確かめる（specs/038-design-system/research.md R-1、R-2）。
 */
export default function Foundations() {
  const f = t.designSystem.foundation;
  return (
    <div className="grid gap-8">
      <div className="grid gap-3">
        <Heading>{f.surfaces}</Heading>
        <div className="grid overflow-hidden rounded-lg border border-border sm:grid-cols-5">
          {surfaces.map((surface) => (
            <div
              key={surface.token}
              data-token={surface.token}
              className={`grid min-h-16 content-end gap-1 p-3 ${surface.className}`}
            >
              <Code>{surface.token}</Code>
              <span className="text-xs text-muted-foreground">
                {f.onSurface(surface.token)}
              </span>
            </div>
          ))}
        </div>
      </div>

      <div className="grid gap-3">
        <Heading>{f.colours}</Heading>
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-4 lg:grid-cols-5">
          {colours.map((colour) => (
            <div
              key={colour.token}
              data-token={colour.token}
              className="grid overflow-hidden rounded-md border border-border bg-card"
            >
              <div className={`h-12 ${colour.className}`} />
              <div className="p-2">
                <Code>{colour.token}</Code>
              </div>
            </div>
          ))}
        </div>
      </div>

      <div className="grid gap-3">
        <Heading>{f.type}</Heading>
        <div className="grid rounded-lg border border-border bg-card">
          {typeSteps.map((step) => (
            <div
              key={step.token}
              className="grid gap-1 border-b border-border p-3 last:border-b-0 sm:grid-cols-4 sm:items-baseline sm:gap-4"
            >
              <Code>{step.token}</Code>
              <span className={`min-w-0 sm:col-span-3 ${step.className}`}>
                {f.sample}
              </span>
            </div>
          ))}
        </div>
      </div>

      <div className="grid gap-8 lg:grid-cols-2">
        <div className="grid content-start gap-3">
          <Heading>{f.spacing}</Heading>
          <div className="grid gap-2 rounded-lg border border-border bg-card p-3">
            {spacingSteps.map((step) => (
              <div key={step.token} className="flex items-center gap-3">
                <span className="w-10 shrink-0">
                  <Code>{step.token}</Code>
                </span>
                <span className={`h-3 rounded-sm bg-primary ${step.className}`} />
              </div>
            ))}
          </div>
        </div>
        <div className="grid content-start gap-3">
          <Heading>{f.namedSizes}</Heading>
          <ul className="flex flex-wrap gap-2 rounded-lg border border-border bg-card p-3">
            {namedSizes.map((name) => (
              <li key={name} className="rounded-sm bg-secondary px-2 py-0.5">
                <Code>{name}</Code>
              </li>
            ))}
          </ul>
        </div>
      </div>

      <div className="grid gap-8 lg:grid-cols-2">
        <div className="grid content-start gap-3">
          <Heading>{f.radius}</Heading>
          <div className="flex flex-wrap gap-4">
            {radii.map((radius) => (
              <div key={radius.token} className="grid justify-items-center gap-2">
                <div
                  className={`h-12 w-16 border border-input bg-card ${radius.className}`}
                />
                <Code>{radius.token}</Code>
              </div>
            ))}
          </div>
        </div>
        <div className="grid content-start gap-3">
          <Heading>{f.shadow}</Heading>
          <div className="flex flex-wrap gap-6 rounded-lg bg-background p-4">
            {shadows.map((shadow) => (
              <div key={shadow.token} className="grid justify-items-center gap-2">
                <div className={`h-12 w-16 ${shadow.className}`} />
                <Code>{shadow.token}</Code>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
}
