import type { ReactNode } from "react";

// One subject's profile above its items; see web/registry/rules/patterns.md, Profile page.

export interface ProfilePageProps {
  /** Names the profile for screen readers. */
  profileLabel: string;
  /** The subject's image: above the rest below sm, at the left from sm. */
  face: ReactNode;
  /** The name (the page's h1) and its muted second line. */
  identity: ReactNode;
  /** One row of `secondary` `sm` actions: after the name from xl, under it below. */
  actions?: ReactNode;
  /** Key figures, facts, links and text under the name; they wrap within the column. */
  details?: ReactNode;
  /** Replaces the whole page, such as when the subject no longer exists. */
  status?: ReactNode;
  /** The subject's items: one section per kind, or one state block. */
  children: ReactNode;
}

export function ProfilePage({
  profileLabel,
  face,
  identity,
  actions,
  details,
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
            className="flex flex-col gap-4 sm:flex-row sm:items-start sm:gap-6"
          >
            <div className="w-1/2 shrink-0 self-center sm:w-profile-face sm:self-start">
              {face}
            </div>
            <div className="flex min-w-0 flex-1 flex-col gap-5">
              <div className="flex flex-col gap-3 xl:flex-row xl:items-start xl:justify-between">
                <div className="flex min-w-0 flex-col gap-1">{identity}</div>
                {actions !== undefined && (
                  <div className="flex flex-wrap items-center gap-2 xl:shrink-0 xl:justify-end">
                    {actions}
                  </div>
                )}
              </div>
              {details}
            </div>
          </section>
          <div data-slot="profile-page-main" className="flex min-w-0 flex-col gap-8">
            {children}
          </div>
        </>
      )}
    </div>
  );
}
