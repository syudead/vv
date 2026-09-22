package scanner

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strconv"
)

// readWindow は先頭・末尾それぞれから読む量である。
//
// 数 TB のライブラリで全体をハッシュするのは非現実的なので、1ファイルあたりの
// 読み出しをここに固定する。先頭と末尾を取るのは、多くの動画コンテナが先頭に
// ヘッダ、末尾に索引（mp4 の moov が末尾に来る場合を含む）を持ち、内容が
// 違えばどちらかが変わるためである。
const readWindow = 1 << 20 // 1MiB

// ContentKey はファイルの内容由来の識別子を返す。
//
//	hex(sha256(先頭 1MiB ‖ 末尾 1MiB)) + ":" + ファイルサイズ
//
// パスを鍵にしないので、移動・改名しても同じ動画として扱える。
// サイズを鍵に含めることで、偶然の衝突は実質的に無視できる。
//
// ハッシュ関数は標準ライブラリの SHA-256 を使う。読む量が1ファイル 2MiB に
// 固定されており、ハッシュ関数の速度は取り込み時間を律速しない（律速は
// ffprobe の起動とディスク I/O）ため、依存を増やしてまで BLAKE3 にする
// 理由が無い。
func ContentKey(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("内容の識別子を計算できません (%s): %w", path, err)
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("内容の識別子を計算できません (%s): %w", path, err)
	}

	key, err := ContentKeyFrom(file, info.Size())
	if err != nil {
		return "", fmt.Errorf("内容の識別子を計算できません (%s): %w", path, err)
	}
	return key, nil
}

// ContentKeyFrom は開いた対象から識別子を組み立てる。ファイルシステムに
// 依らずに式そのものを検証できるよう、io.ReaderAt を受ける。
func ContentKeyFrom(r io.ReaderAt, size int64) (string, error) {
	head, tail, err := readEnds(r, size)
	if err != nil {
		return "", err
	}

	sum := sha256.New()
	sum.Write(head)
	sum.Write(tail)

	return hex.EncodeToString(sum.Sum(nil)) + ":" + strconv.FormatInt(size, 10), nil
}

// readEnds は先頭と末尾の窓を読み出す。
//
// 2MiB 以下のファイルでは窓が重なる（あるいは隣り合う）ので、全体を1度だけ
// 読んで両方を切り出す。読み出す回数を減らすための最適化であり、式は
// 大きなファイルと同じである。
func readEnds(r io.ReaderAt, size int64) (head, tail []byte, err error) {
	if size <= 0 {
		return nil, nil, nil
	}

	if size <= 2*readWindow {
		whole := make([]byte, size)
		if _, err := r.ReadAt(whole, 0); err != nil {
			return nil, nil, err
		}
		return whole[:min(size, readWindow)], whole[max(0, size-readWindow):], nil
	}

	head = make([]byte, readWindow)
	if _, err := r.ReadAt(head, 0); err != nil {
		return nil, nil, err
	}

	tail = make([]byte, readWindow)
	if _, err := r.ReadAt(tail, size-readWindow); err != nil {
		return nil, nil, err
	}
	return head, tail, nil
}
