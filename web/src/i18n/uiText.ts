declare const uiTextBrand: unique symbol;

/**
 * Only catalogs and formatters create UiText, rejecting uncatalogued fixed literals in props. Keep
 * user data such as names as string.
 */
export type UiText = string & { readonly [uiTextBrand]: true };

/** Internal i18n escape hatch; do not call outside this module. */
export function asUiText(text: string): UiText {
  return text as UiText;
}
