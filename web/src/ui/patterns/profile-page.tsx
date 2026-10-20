import type { ReactNode } from "react";

// One subject's profile spread above its items; see web/registry/rules/patterns.md, Profile page.

export interface ProfilePageProps {
  /** Names the profile spread for screen readers. */
  profileLabel: string;
  /** The subject's image: above the identity below sm, beside it from sm. */
  face: ReactNode;
  /** The name (the page's h1), its second line, the key figures, the text and the actions row. */
  identity: ReactNode;
  /** The remaining facts, links and IDs, on a card at the spread's end from lg and under it below. */
  facts?: ReactNode;
  /** Replaces the whole page, such as when the subject no longer exists. */
  status?: ReactNode;
  /** The subject's items: `Shelf`s, or one header and grid. */
  children: ReactNode;
}

export function ProfilePage({
  profileLabel,
  face,
  identity,
  facts,
  status,
  children,
}: ProfilePageProps) {
  return (
    <div data-slot="profile-page" className="flex min-h-full flex-col gap-8 p-3 sm:p-4">
      {status ?? (
        <>
          <section
            data-slot="profile-page-profile"
            aria-label={profileLabel}
            className="flex flex-col gap-6 lg:flex-row lg:items-start"
          >
            <div className="flex min-w-0 flex-1 flex-col gap-4 sm:flex-row sm:items-start sm:gap-6">
              <div className="w-1/2 shrink-0 self-center sm:w-profile-face sm:self-start">
                {face}
              </div>
              <div className="flex min-w-0 flex-1 flex-col gap-4">{identity}</div>
            </div>
            {facts !== undefined && (
              <div
                data-slot="profile-page-facts"
                className="flex min-w-0 flex-col gap-4 rounded-lg bg-muted p-4 text-sm lg:w-profile-facts lg:shrink-0"
              >
                {facts}
              </div>
            )}
          </section>
          <div data-slot="profile-page-main" className="flex min-w-0 flex-col gap-8">
            {children}
          </div>
        </>
      )}
    </div>
  );
}
