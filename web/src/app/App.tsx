import { BrowserRouter, Route, Routes, useLocation } from "react-router";

import { useAudience } from "../auth/audience";
import AuthGate from "../auth/AuthGate";
import LoginPage from "../auth/LoginPage";
import SetupPage from "../auth/SetupPage";
import FolderPage from "../folders/FolderPage";
import LibraryPage from "../library/LibraryPage";
import VideoPage from "../player/VideoPage";
import SettingsPage from "../settings/SettingsPage";
import AppShell from "../shell/AppShell";
import { ScanNoticeProvider } from "../shell/ScanNoticeProvider";
import ScanProgressIndicator from "../shell/ScanProgressIndicator";
import { ScanProvider } from "../shell/ScanProvider";
import TagsPage from "../tags/TagsPage";
import { ToastProvider } from "../ui/Toast";
import { TooltipProvider } from "../ui/Tooltip";

function AppRoutes() {
  const location = useLocation();
  const owner = useAudience() === "owner";
  const scanPlacement = location.pathname.startsWith("/videos/") ? "playback" : "default";

  return (
    <ToastProvider placement={scanPlacement}>
      {/* 取り込みの進捗と通知は所有者だけに出す（ui-design.md「Top bar」）。 */}
      {owner && <ScanProgressIndicator />}
      <Routes>
        <Route
          path="/"
          element={
            <AppShell>
              <LibraryPage />
            </AppShell>
          }
        />
        {["/folders", "/folders/*"].map((path) => (
          <Route
            key={path}
            path={path}
            element={
              <AppShell>
                <FolderPage />
              </AppShell>
            }
          />
        ))}
        <Route
          path="/settings"
          element={
            <AppShell>
              <SettingsPage />
            </AppShell>
          }
        />
        <Route
          path="/tags"
          element={
            <AppShell>
              <TagsPage />
            </AppShell>
          }
        />
        <Route path="/videos/:id" element={<VideoPage />} />
      </Routes>
    </ToastProvider>
  );
}

/** LibraryApp はシェルとプロバイダの内側に置く、ライブラリの画面群である。 */
function LibraryApp() {
  return (
    <TooltipProvider>
      <ScanProvider>
        <ScanNoticeProvider>
          <AppRoutes />
        </ScanNoticeProvider>
      </ScanProvider>
    </TooltipProvider>
  );
}

/**
 * App は画面の割り当てである。
 *
 * すべての経路をゲート（AuthGate）の内側に置き、見る人の状態が分かるまで何も
 * 描かない。初回設定（/setup）とログイン（/login）はシェルとプロバイダの外に
 * 置く（specs/016-single-account-auth/plan.md Structural Decisions 2）。
 * それ以外は、一覧・フォルダ・設定をシェル（トップバー + サイドバー）で包み、
 * 再生画面はシアターモードとして包まない。この分岐はここ 1 か所に閉じる。
 */
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
