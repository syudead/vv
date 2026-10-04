// デザインシステムの検査（web/eslint.config.js）の例外の一覧。noInlineConfig のため
// 例外はコメントでなくここに置く（specs/038-design-system/contracts/registry.md の
// Exception list、docs/design-docs/design-system.md の Checks and exceptions）。
//
// 項目の欄:
// - file: web/src からのパス。
// - rules: 外す規則。no-restricted-syntax は生の部品の選択子だけを外し、訳し漏れの
//   検査は残す。
// - classes: 任意。許すクラスだけを正規表現で並べる。無ければ rules をファイル全体で外す。
// - kind: "migration"（そのファイルを移す PR が消す）か "special"（残る）。
// - reason: "special" では必須。デザインシステムでその見た目を表せない理由。
//
// 型は design-exceptions.d.ts、一覧の検査は src/theme/designExceptions.test.ts にある。
// classes は検査するクラス全体に当てる正規表現で、no-restricted-syntax には効かない。

/** @type {import("./design-exceptions").DesignException[]} */
export default [
  {
    file: "auth/CredentialScreen.tsx",
    rules: ["no-restricted-syntax", "better-tailwindcss/no-restricted-classes"],
    classes: ["sm:pt-24"],
    kind: "migration",
  },
  {
    file: "folders/Breadcrumbs.tsx",
    rules: ["better-tailwindcss/no-restricted-classes"],
    classes: ["size-3\\.5", "h-3\\.5", "w-20", "max-w-40"],
    kind: "migration",
  },
  {
    file: "folders/FolderCard.tsx",
    rules: ["better-tailwindcss/no-restricted-classes"],
    kind: "migration",
  },
  {
    file: "folders/FolderGroupingMenu.tsx",
    rules: ["better-tailwindcss/no-restricted-classes"],
    classes: ["max-w-80", "px-2\\.5", "size-3\\.5"],
    kind: "migration",
  },
  {
    file: "folders/FolderPage.tsx",
    rules: ["better-tailwindcss/no-restricted-classes"],
    classes: ["pb-24"],
    kind: "migration",
  },
  {
    file: "folders/FolderToolbar.tsx",
    rules: ["better-tailwindcss/no-restricted-classes"],
    classes: ["min-w-20", "sm:min-w-40", "w-24", "px-2\\.5", "w-80"],
    kind: "migration",
  },
  {
    file: "library/CardTagRow.tsx",
    rules: ["no-restricted-syntax", "better-tailwindcss/no-restricted-classes"],
    kind: "migration",
  },
  {
    file: "player/AutoplayNotice.tsx",
    rules: ["better-tailwindcss/no-restricted-classes"],
    kind: "migration",
  },
  {
    file: "player/EndedOverlay.tsx",
    rules: ["better-tailwindcss/no-restricted-classes"],
    kind: "migration",
  },
  {
    file: "player/NeighborArrows.tsx",
    rules: ["no-restricted-syntax", "better-tailwindcss/no-restricted-classes"],
    kind: "migration",
  },
  {
    file: "player/RelatedVideos.tsx",
    rules: ["better-tailwindcss/no-restricted-classes"],
    kind: "migration",
  },
  {
    file: "player/StallWarning.tsx",
    rules: ["no-restricted-syntax", "better-tailwindcss/no-restricted-classes"],
    kind: "migration",
  },
  {
    file: "player/StatusOverlays.tsx",
    rules: ["better-tailwindcss/no-restricted-classes"],
    classes: ["pb-14", "gap-2\\.5", "size-3\\.5"],
    kind: "migration",
  },
  {
    file: "player/TouchControls.tsx",
    rules: ["no-restricted-syntax", "better-tailwindcss/no-restricted-classes"],
    classes: ["size-15", "size-7"],
    kind: "migration",
  },
  {
    file: "player/VersionsFact.tsx",
    rules: ["no-restricted-syntax", "better-tailwindcss/no-restricted-classes"],
    kind: "migration",
  },
  {
    file: "player/GroupLine.tsx",
    rules: ["better-tailwindcss/no-restricted-classes"],
    classes: ["size-3\\.5"],
    kind: "migration",
  },
  {
    file: "player/VideoFacts.tsx",
    rules: ["no-restricted-syntax", "better-tailwindcss/no-restricted-classes"],
    kind: "migration",
  },
  {
    file: "player/VideoHeader.tsx",
    rules: ["better-tailwindcss/no-restricted-classes"],
    classes: ["size-3\\.5", "max-w-40"],
    kind: "migration",
  },
  {
    file: "player/VideoPage.tsx",
    rules: ["better-tailwindcss/no-restricted-classes"],
    kind: "migration",
  },
  {
    file: "player/VideoPlayer.tsx",
    rules: [
      "no-restricted-syntax",
      "better-tailwindcss/no-restricted-classes",
      "better-tailwindcss/no-unknown-classes",
    ],
    kind: "migration",
  },
  {
    file: "player/VideoTags.tsx",
    rules: ["no-restricted-syntax", "better-tailwindcss/no-restricted-classes"],
    classes: ["size-3\\.5", "h-3\\.5"],
    kind: "migration",
  },
  {
    file: "player/VideoTitle.tsx",
    rules: ["no-restricted-syntax", "better-tailwindcss/no-restricted-classes"],
    kind: "migration",
  },
  {
    file: "player/VisibilitySwitch.tsx",
    rules: ["no-restricted-syntax", "better-tailwindcss/no-restricted-classes"],
    classes: ["px-2\\.5"],
    kind: "migration",
  },
  {
    file: "settings/APITokensSection.tsx",
    rules: ["no-restricted-syntax", "better-tailwindcss/no-restricted-classes"],
    classes: ["size-3\\.5"],
    kind: "migration",
  },
  {
    file: "settings/FolderPicker.tsx",
    rules: ["no-restricted-syntax", "better-tailwindcss/no-restricted-classes"],
    classes: ["sm:min-h-72", "min-h-48", "min-h-11"],
    kind: "migration",
  },
  {
    file: "settings/NetworkSection.tsx",
    rules: ["no-restricted-syntax", "better-tailwindcss/no-restricted-classes"],
    classes: ["w-11", "size-3\\.5"],
    kind: "migration",
  },
  {
    file: "settings/ScanIssueList.tsx",
    rules: ["better-tailwindcss/no-restricted-classes"],
    kind: "migration",
  },
  {
    file: "settings/TranscodingSection.tsx",
    rules: ["no-restricted-syntax", "better-tailwindcss/no-restricted-classes"],
    classes: ["size-3\\.5", "w-48"],
    kind: "migration",
  },
  {
    file: "shell/AppShell.tsx",
    rules: ["better-tailwindcss/no-restricted-classes"],
    kind: "migration",
  },
  {
    file: "shell/ScanProgressBar.tsx",
    rules: ["better-tailwindcss/no-restricted-classes"],
    kind: "migration",
  },
  {
    file: "shell/ScanProgressIndicator.tsx",
    rules: ["no-restricted-syntax", "better-tailwindcss/no-restricted-classes"],
    kind: "migration",
  },
  {
    file: "shell/Sidebar.tsx",
    rules: ["no-restricted-syntax", "better-tailwindcss/no-restricted-classes"],
    kind: "migration",
  },
  {
    file: "shell/TopBar.tsx",
    rules: ["no-restricted-syntax"],
    kind: "migration",
  },
  {
    file: "tags/CreateTagRow.tsx",
    rules: ["no-restricted-syntax"],
    kind: "migration",
  },
  {
    file: "tags/MergeTagDialog.tsx",
    rules: ["better-tailwindcss/no-restricted-classes"],
    classes: ["max-h-32"],
    kind: "migration",
  },
  {
    file: "tags/SynonymsDialog.tsx",
    rules: ["no-restricted-syntax"],
    kind: "migration",
  },
  {
    file: "tags/TagRow.tsx",
    rules: ["no-restricted-syntax", "better-tailwindcss/no-restricted-classes"],
    classes: ["sm:w-20"],
    kind: "migration",
  },
  {
    file: "tags/TagToolbar.tsx",
    rules: ["better-tailwindcss/no-restricted-classes"],
    classes: ["min-w-20", "sm:min-w-40", "px-2\\.5", "w-72"],
    kind: "migration",
  },
  {
    file: "tags/TagsPage.tsx",
    rules: ["better-tailwindcss/no-restricted-classes"],
    kind: "migration",
  },
  {
    file: "ui/BrandHomeLink.tsx",
    rules: ["better-tailwindcss/no-restricted-classes"],
    classes: ["size-7"],
    kind: "migration",
  },
  {
    file: "ui/Checkbox.tsx",
    rules: ["better-tailwindcss/no-restricted-classes"],
    classes: ["size-3\\.5"],
    kind: "migration",
  },
  {
    file: "ui/Chip.tsx",
    rules: ["better-tailwindcss/no-restricted-classes"],
    classes: ["\\[&>svg\\]:size-3\\.5"],
    kind: "migration",
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
    file: "ui/Combobox.tsx",
    rules: ["better-tailwindcss/no-restricted-classes"],
    kind: "migration",
  },
  {
    file: "ui/Menu.tsx",
    rules: ["better-tailwindcss/no-restricted-classes"],
    kind: "migration",
  },
  {
    file: "ui/ModalFrame.tsx",
    rules: ["better-tailwindcss/no-restricted-classes"],
    kind: "migration",
  },
  {
    file: "ui/Popover.tsx",
    rules: ["better-tailwindcss/no-restricted-classes"],
    kind: "migration",
  },
  {
    file: "ui/ScrubPreview.tsx",
    rules: ["better-tailwindcss/no-restricted-classes"],
    kind: "migration",
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
    file: "ui/Skeleton.tsx",
    rules: ["better-tailwindcss/no-restricted-classes"],
    kind: "migration",
  },
  {
    file: "ui/TentativeMark.tsx",
    rules: ["better-tailwindcss/no-restricted-classes"],
    classes: ["size-3\\.5"],
    kind: "migration",
  },
  {
    file: "ui/Toast.tsx",
    rules: ["better-tailwindcss/no-restricted-classes"],
    kind: "migration",
  },
  {
    file: "versions/BundleDialog.tsx",
    rules: ["no-restricted-syntax", "better-tailwindcss/no-restricted-classes"],
    classes: ["max-h-80"],
    kind: "migration",
  },
  {
    file: "versions/DuplicatesPage.tsx",
    rules: ["better-tailwindcss/no-restricted-classes"],
    kind: "migration",
  },
  {
    file: "videoList/FavoriteToggle.tsx",
    rules: ["no-restricted-syntax", "better-tailwindcss/no-restricted-classes"],
    classes: ["size-5\\.5", "size-7"],
    kind: "migration",
  },
  {
    file: "videoList/FilterMenu.tsx",
    rules: ["no-restricted-syntax"],
    kind: "migration",
  },
  {
    file: "videoList/FolderArt.tsx",
    rules: ["better-tailwindcss/no-restricted-classes"],
    kind: "migration",
  },
  {
    file: "videoList/Grid.tsx",
    rules: ["better-tailwindcss/no-restricted-classes"],
    kind: "migration",
  },
  {
    file: "videoList/SearchBox.tsx",
    rules: ["no-restricted-syntax", "better-tailwindcss/no-restricted-classes"],
    kind: "migration",
  },
  {
    file: "videoList/SearchSyntaxHelp.tsx",
    rules: ["no-restricted-syntax", "better-tailwindcss/no-restricted-classes"],
    kind: "migration",
  },
  {
    file: "videoList/SortControls.tsx",
    rules: ["no-restricted-syntax"],
    kind: "migration",
  },
  {
    file: "videoList/cardPreview.tsx",
    rules: ["better-tailwindcss/no-restricted-classes"],
    kind: "migration",
  },
];
