import { BrowserRouter, Route, Routes } from "react-router";

import LibraryPage from "./pages/LibraryPage";

/**
 * App は画面の割り当てである（R-113）。
 *
 * 画面が2つになり、再生中の動画を URL で開ける（共有・再読み込みできる）
 * 必要が出たので、ここで最小のルータを入れている。宣言的モードだけを使い、
 * データ取得はルータに寄せない。
 */
export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/" element={<LibraryPage />} />
      </Routes>
    </BrowserRouter>
  );
}
