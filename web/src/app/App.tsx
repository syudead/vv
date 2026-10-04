import { lazy, Suspense } from "react";
import { BrowserRouter, Route, Routes, useLocation } from "react-router";

import { useAudience } from "../auth/audience";
import AuthGate from "../auth/AuthGate";
import LibraryPage from "../library/LibraryPage";
import VideoPage from "../player/VideoPage";
import AppShell from "../shell/AppShell";
import { ScanNoticeProvider } from "../shell/ScanNoticeProvider";
import ScanProgressIndicator from "../shell/ScanProgressIndicator";
import { ScanProvider } from "../shell/ScanProvider";
import { ToastProvider } from "../ui/legacy/Toast";
import { TooltipProvider } from "../ui/tooltip";
import { deferredRoute } from "./deferredRoute";

// 一覧と再生画面のほかは使うときだけ読み込む（deferredRoute）。再生画面から
// 一覧へ戻るたびの読み込みに、ほかの画面の部品を加えない。初回設定とログインの
// 画面も同じで、ゲートが送るときに読み込む。読み終えるまでゲートと同じく何も描かない。
const FolderPage = await deferredRoute(() => import("../folders/FolderPage"), "/folders");
const SettingsPage = await deferredRoute(
  () => import("../settings/SettingsPage"),
  "/settings",
);
const TagsPage = await deferredRoute(() => import("../tags/TagsPage"), "/tags");
const DuplicatesPage = await deferredRoute(
  () => import("../versions/DuplicatesPage"),
  "/duplicates",
);
const SetupPage = await deferredRoute(() => import("../auth/SetupPage"), "/setup");
const LoginPage = await deferredRoute(() => import("../auth/LoginPage"), "/login");
// デザインシステムの見本（/design-system）は開発時だけに置く
// （specs/038-design-system/research.md R-2）。本番のビルドでは import.meta.env.DEV が
// false に置き換わり、この import ごと出力から消える。
const DesignSystemPage = import.meta.env.DEV
  ? lazy(() => import("../designSystem/DesignSystemPage"))
  : null;

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
        <Route
          path="/duplicates"
          element={
            <AppShell>
              <DuplicatesPage />
            </AppShell>
          }
        />
        <Route path="/videos/:id" element={<VideoPage />} />
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
 * 置く。
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
