import { en } from "./en";
import type { UiText } from "./uiText";

/**
 * MessageSource はカタログの形である。言語を足すときは、この型を満たすオブジェクトを
 * 書くので、鍵の漏れと引数の食い違いは型検査で落ちる（specs/023-english-i18n/research.md R-1）。
 */
export type MessageSource = typeof en;

/** Catalog は文言の葉をすべて UiText（関数なら UiText を返す関数）にした形である。 */
export type Catalog<T> = T extends string
  ? UiText
  : T extends (...args: infer A) => string
    ? (...args: A) => UiText
    : { readonly [K in keyof T]: Catalog<T[K]> };

/** Messages はコンポーネントが引くカタログの型である。 */
export type Messages = Catalog<MessageSource>;

type Node = { [key: string]: unknown };

function isNode(value: unknown): value is Node {
  return typeof value === "object" && value !== null;
}

function cloneTree(source: Node): Node {
  const copy: Node = {};
  for (const [key, value] of Object.entries(source)) {
    copy[key] = isNode(value) ? cloneTree(value) : value;
  }
  return copy;
}

function assignTree(target: Node, source: Node): void {
  for (const [key, value] of Object.entries(source)) {
    const current = target[key];
    if (isNode(value) && isNode(current)) assignTree(current, value);
    else target[key] = value;
  }
}

/**
 * t は今の言語のカタログである。言語は起動時に決まる 1 つの定数なので、React の
 * context や provider を挟まず、コンポーネントはこれを静的に import する。
 *
 * 実行時の値は文字列と文字列を返す関数で、型だけを UiText にする（ブランドは型の上の
 * 印である）。
 */
export const t = cloneTree(en) as unknown as Messages;

/**
 * replaceMessages はカタログの中身を置き換える。入れ子のオブジェクトはそのまま使い、
 * 葉だけを差し替えるので、枝を変数に取ったコンポーネントにも届く。疑似ロケール
 * （pseudo.ts）への差し替えのためだけにある。テスト専用。
 */
export function replaceMessages(source: MessageSource): void {
  assignTree(t as unknown as Node, source as unknown as Node);
}

/** mapMessages は葉を変えたカタログの写しを作る（pseudo.ts が使う）。 */
export function mapMessages(
  source: MessageSource,
  mapText: (text: string) => string,
): MessageSource {
  const walk = (node: Node): Node => {
    const copy: Node = {};
    for (const [key, value] of Object.entries(node)) {
      if (typeof value === "string") copy[key] = mapText(value);
      else if (typeof value === "function") {
        const fn = value as (...args: unknown[]) => string;
        copy[key] = (...args: unknown[]) => mapText(fn(...args));
      } else if (isNode(value)) copy[key] = walk(value);
      else copy[key] = value;
    }
    return copy;
  };
  return walk(source as unknown as Node) as unknown as MessageSource;
}

/** englishMessages は英語のカタログの元である（疑似ロケールを戻すときに使う）。 */
export const englishMessages: MessageSource = en;
