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
    rules: ["no-restricted-syntax"],
    kind: "migration",
  },
  {
    file: "folders/FolderCard.tsx",
    rules: ["better-tailwindcss/no-restricted-classes"],
    kind: "migration",
  },
  {
    file: "library/CardTagRow.tsx",
    rules: ["no-restricted-syntax", "better-tailwindcss/no-restricted-classes"],
    kind: "migration",
  },
  {
    file: "library/GroupCard.tsx",
    rules: ["better-tailwindcss/no-restricted-classes"],
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
    file: "player/TouchControls.tsx",
    rules: ["no-restricted-syntax"],
    kind: "migration",
  },
  {
    file: "player/VersionsFact.tsx",
    rules: ["no-restricted-syntax", "better-tailwindcss/no-restricted-classes"],
    kind: "migration",
  },
  {
    file: "player/VideoFacts.tsx",
    rules: ["no-restricted-syntax", "better-tailwindcss/no-restricted-classes"],
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
    rules: ["no-restricted-syntax"],
    kind: "migration",
  },
  {
    file: "player/VideoTitle.tsx",
    rules: ["no-restricted-syntax", "better-tailwindcss/no-restricted-classes"],
    kind: "migration",
  },
  {
    file: "player/VisibilitySwitch.tsx",
    rules: ["no-restricted-syntax"],
    kind: "migration",
  },
  {
    file: "settings/APITokensSection.tsx",
    rules: ["no-restricted-syntax"],
    kind: "migration",
  },
  {
    file: "settings/FolderPicker.tsx",
    rules: ["no-restricted-syntax"],
    kind: "migration",
  },
  {
    file: "settings/NetworkSection.tsx",
    rules: ["no-restricted-syntax"],
    kind: "migration",
  },
  {
    file: "settings/ScanIssueList.tsx",
    rules: ["better-tailwindcss/no-restricted-classes"],
    kind: "migration",
  },
  {
    file: "settings/TranscodingSection.tsx",
    rules: ["no-restricted-syntax"],
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
    file: "tags/SynonymsDialog.tsx",
    rules: ["no-restricted-syntax"],
    kind: "migration",
  },
  {
    file: "tags/TagRow.tsx",
    rules: ["no-restricted-syntax"],
    kind: "migration",
  },
  {
    file: "tags/TagsPage.tsx",
    rules: ["better-tailwindcss/no-restricted-classes"],
    kind: "migration",
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
    file: "ui/Skeleton.tsx",
    rules: ["better-tailwindcss/no-restricted-classes"],
    kind: "migration",
  },
  {
    file: "ui/Toast.tsx",
    rules: ["better-tailwindcss/no-restricted-classes"],
    kind: "migration",
  },
  {
    file: "versions/BundleDialog.tsx",
    rules: ["no-restricted-syntax"],
    kind: "migration",
  },
  {
    file: "versions/DuplicatesPage.tsx",
    rules: ["better-tailwindcss/no-restricted-classes"],
    kind: "migration",
  },
  {
    file: "videoList/FavoriteToggle.tsx",
    rules: ["no-restricted-syntax"],
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
    file: "videoList/VideoCard.tsx",
    rules: ["better-tailwindcss/no-restricted-classes"],
    kind: "migration",
  },
  {
    file: "videoList/cardPreview.tsx",
    rules: ["better-tailwindcss/no-restricted-classes"],
    kind: "migration",
  },
];
