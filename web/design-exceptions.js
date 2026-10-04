// デザインシステムの検査（web/eslint.config.js）の例外の一覧。noInlineConfig のため
// 例外はコメントでなくここに置く（specs/038-design-system/contracts/registry.md の
// Exception list、docs/design-docs/design-system.md の Checks and exceptions）。
//
// 項目の欄:
// - file: web/src からのパス。
// - rules: 外す規則。no-restricted-syntax は生の部品の選択子だけを外し、訳し漏れの
//   検査は残す。
// - classes: 任意。許すクラスだけを正規表現で並べる。無ければ rules をファイル全体で外す。
// - kind: "special"（残る）。移行中に使った "migration" は、全画面の移行とテーマの
//   リセットの後は src/theme/designExceptions.test.ts が落とす。
// - reason: "special" では必須。デザインシステムでその見た目を表せない理由。
//
// 型は design-exceptions.d.ts、一覧の検査は src/theme/designExceptions.test.ts にある。
// classes は検査するクラス全体に当てる正規表現で、no-restricted-syntax には効かない。

/** @type {import("./design-exceptions").DesignException[]} */
export default [
  {
    file: "player/VideoPlayer.tsx",
    rules: [
      "no-restricted-syntax",
      "better-tailwindcss/no-restricted-classes",
      "better-tailwindcss/no-unknown-classes",
    ],
    classes: [
      "size-\\[1\\.6em\\]",
      "vv-video-player",
      "vjs-control",
      "vjs-button",
      "vv-bar-button",
    ],
    kind: "special",
    reason:
      "The video.js control bar is third-party DOM skinned by CSS in index.css; the buttons vv inserts into it (restart) are plain buttons carrying video.js's own classes and sized in em like its icons, so they match the bar instead of a design-system Button.",
  },
  {
    file: "player/seekPreview.ts",
    rules: ["better-tailwindcss/no-unknown-classes"],
    classes: ["vv-seek-preview(?:-frame|-image|-time)?"],
    kind: "special",
    reason:
      "The seek preview over the video.js progress bar (.vv-seek-preview in index.css) is placed from the pointer and the frame size at run time and sized from the seek-preview tokens; it is not a card scrub (ScrubPreview) and has no design-system component.",
  },
  {
    file: "ui/shadcn/combobox.tsx",
    rules: ["better-tailwindcss/no-restricted-classes"],
    classes: [
      "w-\\(--radix-popover-trigger-width\\)",
      "origin-\\(--radix-popover-content-transform-origin\\)",
    ],
    kind: "special",
    reason:
      "Radix sets the list's width and transform origin at run time through its CSS variables; they are positions, not design values.",
  },
  {
    file: "ui/shadcn/select.tsx",
    rules: ["better-tailwindcss/no-restricted-classes"],
    classes: [
      "max-h-\\(--radix-select-content-available-height\\)",
      "origin-\\(--radix-select-content-transform-origin\\)",
      "h-\\(--radix-select-trigger-height\\)",
      "min-w-\\(--radix-select-trigger-width\\)",
    ],
    kind: "special",
    reason:
      "Radix sets the list's height, width and transform origin at run time through its CSS variables; they are positions, not design values.",
  },
  {
    file: "ui/shadcn/alert.tsx",
    rules: ["better-tailwindcss/no-restricted-classes"],
    classes: ["has-\\[>svg\\]:grid-cols-\\[auto_1fr\\]"],
    kind: "special",
    reason:
      "The upstream Alert lays its icon and text out as an auto-width column and a flexible one; Tailwind has no grid template utility for that pair.",
  },
  {
    file: "ui/shadcn/dropdown-menu.tsx",
    rules: ["better-tailwindcss/no-restricted-classes"],
    classes: [
      "max-h-\\(--radix-dropdown-menu-content-available-height\\)",
      "origin-\\(--radix-dropdown-menu-content-transform-origin\\)",
    ],
    kind: "special",
    reason:
      "Radix sets the floating layer's transform origin and available height as CSS variables at runtime; the class has to read them, and no token can name a value Radix computes.",
  },
  {
    file: "ui/shadcn/popover.tsx",
    rules: ["better-tailwindcss/no-restricted-classes"],
    classes: ["origin-\\(--radix-popover-content-transform-origin\\)"],
    kind: "special",
    reason:
      "Radix sets the floating layer's transform origin and available height as CSS variables at runtime; the class has to read them, and no token can name a value Radix computes.",
  },
  {
    file: "ui/shadcn/tooltip.tsx",
    rules: ["better-tailwindcss/no-restricted-classes"],
    classes: ["origin-\\(--radix-tooltip-content-transform-origin\\)"],
    kind: "special",
    reason:
      "Radix sets the floating layer's transform origin and available height as CSS variables at runtime; the class has to read them, and no token can name a value Radix computes.",
  },
];
