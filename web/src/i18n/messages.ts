import { en } from "./en";
import type { UiText } from "./uiText";

/** New catalogs must satisfy this type so missing keys and mismatched arguments fail compilation. */
export type MessageSource = typeof en;

export type Catalog<T> = T extends string
  ? UiText
  : T extends (...args: infer A) => string
    ? (...args: A) => UiText
    : { readonly [K in keyof T]: Catalog<T[K]> };

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
    if (key === "__proto__" || key === "constructor" || key === "prototype") continue;
    if (!Object.hasOwn(target, key)) continue;
    const current = target[key];
    if (isNode(value) && isNode(current)) assignTree(current, value);
    else target[key] = value;
  }
}

/**
 * The startup-fixed locale needs no React provider; import t directly. UiText branding is type-
 * only, with strings and functions at runtime.
 */
export const t = cloneTree(en) as unknown as Messages;

/** Test-only replacement preserves nested object identities so cached branches see updated leaves. */
export function replaceMessages(source: MessageSource): void {
  assignTree(t as unknown as Node, source as unknown as Node);
}

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

export const englishMessages: MessageSource = en;
