// Design-system exceptions live here because ESLint disallows inline overrides.
// file is relative to web/src; rules lists overrides; optional classes regexes match whole class names but do not affect no-restricted-syntax.
// no-restricted-syntax overrides remove only raw-component restrictions, preserving translation checks.
// Only kind "special" remains valid and requires a reason; types live in design-exceptions.d.ts and validation in src/theme/designExceptions.test.ts.

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
    classes: ["vv-seek-preview(?:-frame|-image|-time|-title)?"],
    kind: "special",
    reason:
      "The seek preview over the video.js progress bar (.vv-seek-preview in index.css) is placed from the pointer and the frame size at run time and sized from the seek-preview tokens; it is not a card scrub (ScrubPreview) and has no design-system component.",
  },
  {
    file: "player/chapters.ts",
    rules: ["better-tailwindcss/no-unknown-classes"],
    classes: ["vv-chapter-(?:marks|mark|start)"],
    kind: "special",
    reason:
      "The chapter marks inside the video.js progress bar and the start time in its chapters menu items (.vv-chapter-* in index.css) are third-party DOM placed from each chapter's start at run time; they belong to the special player skin and have no design-system component.",
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
    classes: [
      "max-h-\\(--radix-popover-content-available-height\\)",
      "origin-\\(--radix-popover-content-transform-origin\\)",
    ],
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
