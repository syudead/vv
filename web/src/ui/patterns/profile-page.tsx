import type { ReactNode } from "react";

// One subject's profile beside its items; see web/registry/rules/patterns.md, Profile page.

export interface ProfilePageProps {
  /** Names the profile column for screen readers. */
  profileLabel: string;
  /** The subject's image, beside the identity below lg and above it from lg. */
  face: ReactNode;
  /** The name (the page's h1), its second line and the actions row. */
  identity: ReactNode;
  /** Facts, text and links under the face and identity. */
  details?: ReactNode;
  /** The main column's PageHeader (headingLevel 2). */
  header: ReactNode;
  /** One line of filters; it scrolls sideways instead of wrapping. */
  toolbar?: ReactNode;
  /** Replaces both columns, such as when the subject no longer exists. */
  status?: ReactNode;
  children: ReactNode;
}

export function ProfilePage({
  profileLabel,
  face,
  identity,
  details,
  header,
  toolbar,
  status,
  children,
}: ProfilePageProps) {
  return (
    <div data-slot="profile-page" className="flex min-h-full flex-col gap-3 p-3 sm:p-4">
      {status ?? (
        <div className="flex flex-col gap-6 lg:grid lg:grid-cols-profile-page lg:items-start">
          <aside
            data-slot="profile-page-profile"
            aria-label={profileLabel}
            // Pad the scrolling column so focus rings inside it are not clipped.
            className="flex min-w-0 flex-col gap-4 lg:sticky lg:top-navbar lg:-mx-1 lg:max-h-profile-aside-max lg:overflow-y-auto lg:px-1"
          >
            <div className="flex items-start gap-4 lg:flex-col lg:items-stretch">
              <div className="w-1/2 shrink-0 md:w-1/3 lg:w-full">{face}</div>
              <div className="flex min-w-0 flex-1 flex-col gap-2">{identity}</div>
            </div>
            {details}
          </aside>
          <div data-slot="profile-page-main" className="flex min-w-0 flex-col gap-3">
            {header}
            {toolbar !== undefined && (
              <div data-slot="profile-page-toolbar" className="flex overflow-x-auto">
                {toolbar}
              </div>
            )}
            {children}
          </div>
        </div>
      )}
    </div>
  );
}
