// クラス名の結合と Tailwind の競合解消は cn パッケージが行う（shadcn/ui の部品が前提にする）。
// 場所を変えないのは、既存の呼び出しと components.json の aliases.utils をここに揃えるため。
export { cn } from "cn";
