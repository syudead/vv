// design-exceptions.js の項目の型。欄の意味は design-exceptions.js の先頭にある。
export type DesignRule =
  | "no-restricted-syntax"
  | "better-tailwindcss/no-restricted-classes"
  | "better-tailwindcss/no-unknown-classes";

export interface DesignException {
  file: string;
  rules: DesignRule[];
  classes?: string[];
  kind: "migration" | "special";
  reason?: string;
}

declare const designExceptions: DesignException[];
export default designExceptions;
