import { lazy, type ReactNode, Suspense } from "react";
import { BrowserRouter, Route, Routes, useLocation } from "react-router";

import { useAudience } from "../auth/audience";
import AuthGate from "../auth/AuthGate";
import { LIBRARY_PATH } from "../library/libraryPath";
import LibraryPage from "../library/LibraryPage";
import VideoPage from "../player/VideoPage";
import AppShell from "../shell/AppShell";
import PeerNotice, { usePeerNoticeShown } from "../shell/PeerNotice";
import { PeerProvider } from "../shell/PeerProvider";
import { ScanNoticeProvider } from "../shell/ScanNoticeProvider";
import ScanProgressIndicator from "../shell/ScanProgressIndicator";
import { ScanProvider } from "../shell/ScanProvider";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import { isPersonFacesPath } from "../credits/person/personPath";
import { isPersonEditPath } from "../lib/personEditPath";
import { isVideoEditPath } from "../lib/videoEditPath";
import { OnboardingGate } from "../onboarding/OnboardingGate";
import { ONBOARDING_PATH } from "../onboarding/onboardingRoute";
import { deferredRoute, matchesBelowPath, matchesExactPath } from "./deferredRoute";
import LibraryRedirect from "./LibraryRedirect";
const FolderPage = await deferredRoute(() => import("../folders/FolderPage"), "/folders");
const SettingsPage = await deferredRoute(
  () => import("../settings/SettingsPage"),
  "/settings",
);
const TagsPage = await deferredRoute(() => import("../tags/TagsPage"), "/tags");
const PeoplePage = await deferredRoute(
  () => import("../credits/people/PeoplePage"),
  (pathname) => matchesExactPath(pathname, "/people"),
);
const PersonPage = await deferredRoute(
  () => import("../credits/person/PersonPage"),
  (pathname) =>
    matchesBelowPath(pathname, "/people") &&
    !isPersonEditPath(pathname) &&
    !isPersonFacesPath(pathname),
);
const PersonFacesPage = await deferredRoute(
  () => import("../credits/person/PersonFacesPage"),
  isPersonFacesPath,
);
const PersonEditPage = await deferredRoute(
  () => import("../credits/person/edit/PersonEditPage"),
  isPersonEditPath,
);
const OrganizationsPage = await deferredRoute(
  () => import("../credits/OrganizationsPage"),
  "/organizations",
);
const HistoryPage = await deferredRoute(
  () => import("../history/HistoryPage"),
  "/history",
);
const PlaylistsPage = await deferredRoute(
  () => import("../playlists/PlaylistsPage"),
  (pathname) => matchesExactPath(pathname, "/playlists"),
);
const PlaylistPage = await deferredRoute(
  () => import("../playlists/PlaylistPage"),
  (pathname) => matchesBelowPath(pathname, "/playlists"),
);
const GroupPage = await deferredRoute(() => import("../groups/GroupPage"), "/groups");
const SmartFolderPage = await deferredRoute(
  () => import("../smartFolders/SmartFolderPage"),
  "/smart-folders",
);
const DuplicatesPage = await deferredRoute(
  () => import("../versions/DuplicatesPage"),
  "/duplicates",
);
const ActivityPage = await deferredRoute(
  () => import("../activity/ActivityPage"),
  (pathname) => matchesExactPath(pathname, "/activity"),
);
const ActivityDetailPage = await deferredRoute(
  () => import("../activity/ActivityDetailPage"),
  (pathname) => matchesBelowPath(pathname, "/activity"),
);
const VideoDetailsEditor = await deferredRoute(
  () => import("../player/detailsEditor/VideoDetailsEditor"),
  isVideoEditPath,
);
const OnboardingPage = await deferredRoute(
  () => import("../onboarding/OnboardingPage"),
  ONBOARDING_PATH,
);
const SetupPage = await deferredRoute(() => import("../auth/SetupPage"), "/setup");
const LoginPage = await deferredRoute(() => import("../auth/LoginPage"), "/login");
const DesignSystemPage = import.meta.env.DEV
  ? lazy(() => import("../designSystem/DesignSystemPage"))
  : null;

/** A shell page with the peer notice above it while the peer cannot be reached. */
function ShellPage({ children }: { children: ReactNode }) {
  const notice = usePeerNoticeShown();
  return <AppShell notice={notice ? <PeerNotice /> : undefined}>{children}</AppShell>;
}

function AppRoutes() {
  const location = useLocation();
  const owner = useAudience() === "owner";
  const scanPlacement = location.pathname.startsWith("/videos/") ? "playback" : "default";

  return (
    <ToastProvider placement={scanPlacement}>
      {owner && <ScanProgressIndicator />}
      {/* 所有者の段階が done でない間は、どの経路も /welcome へ置き換える
          （docs/design-docs/home.md）。 */}
      <OnboardingGate>
        <Routes>
          <Route path="/library" element={<LibraryRedirect />} />
          <Route
            path={LIBRARY_PATH}
            element={
              <ShellPage>
                <LibraryPage />
              </ShellPage>
            }
          />
          {["/folders", "/folders/*"].map((path) => (
            <Route
              key={path}
              path={path}
              element={
                <ShellPage>
                  <FolderPage />
                </ShellPage>
              }
            />
          ))}
          {["/settings", "/settings/*"].map((path) => (
            <Route
              key={path}
              path={path}
              element={
                <ShellPage>
                  <SettingsPage />
                </ShellPage>
              }
            />
          ))}
          <Route
            path="/tags"
            element={
              <ShellPage>
                <TagsPage />
              </ShellPage>
            }
          />
          <Route
            path="/people"
            element={
              <ShellPage>
                <PeoplePage />
              </ShellPage>
            }
          />
          <Route
            path="/people/:id"
            element={
              <ShellPage>
                <PersonPage />
              </ShellPage>
            }
          />
          <Route
            path="/people/:id/faces"
            element={
              <ShellPage>
                <PersonFacesPage />
              </ShellPage>
            }
          />
          <Route
            path="/organizations"
            element={
              <ShellPage>
                <OrganizationsPage />
              </ShellPage>
            }
          />
          <Route
            path="/history"
            element={
              <ShellPage>
                <HistoryPage />
              </ShellPage>
            }
          />
          <Route
            path="/playlists"
            element={
              <ShellPage>
                <PlaylistsPage />
              </ShellPage>
            }
          />
          <Route
            path="/playlists/:id"
            element={
              <ShellPage>
                <PlaylistPage />
              </ShellPage>
            }
          />
          <Route
            path="/groups/:id"
            element={
              <ShellPage>
                <GroupPage />
              </ShellPage>
            }
          />
          <Route
            path="/smart-folders/:id"
            element={
              <ShellPage>
                <SmartFolderPage />
              </ShellPage>
            }
          />
          <Route
            path="/duplicates"
            element={
              <ShellPage>
                <DuplicatesPage />
              </ShellPage>
            }
          />
          <Route
            path="/activity"
            element={
              <ShellPage>
                <ActivityPage />
              </ShellPage>
            }
          />
          <Route
            path="/activity/:id"
            element={
              <ShellPage>
                <ActivityDetailPage />
              </ShellPage>
            }
          />
          <Route path={ONBOARDING_PATH} element={<OnboardingPage />} />
          <Route path="/videos/:id" element={<VideoPage />} />
          <Route path="/videos/:id/edit" element={<VideoDetailsEditor />} />
          <Route path="/people/:id/edit" element={<PersonEditPage />} />
          {DesignSystemPage && (
            <Route
              path="/design-system"
              element={
                <Suspense fallback={null}>
                  <DesignSystemPage />
                </Suspense>
              }
            />
          )}
        </Routes>
      </OnboardingGate>
    </ToastProvider>
  );
}

function LibraryApp() {
  return (
    <TooltipProvider>
      <ScanProvider>
        <ScanNoticeProvider>
          <PeerProvider>
            <AppRoutes />
          </PeerProvider>
        </ScanNoticeProvider>
      </ScanProvider>
    </TooltipProvider>
  );
}

export default function App() {
  return (
    <BrowserRouter>
      <AuthGate>
        <Routes>
          <Route path="/setup" element={<SetupPage />} />
          <Route path="/login" element={<LoginPage />} />
          <Route path="*" element={<LibraryApp />} />
        </Routes>
      </AuthGate>
    </BrowserRouter>
  );
}
