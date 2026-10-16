package arib

import "testing"

// ARIB STD-B24 の 8 単位符号の文字列を組み立て、期待する UTF-8 になることを確かめる。
// 初期の状態は GL = G0 漢字、GR = G2 平仮名、G1 英数、G3 片仮名である。

const (
	esc = 0x1B
	ls0 = 0x0F
	ls1 = 0x0E
	ss2 = 0x19
	ss3 = 0x1D
)

func TestDecode(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want string
	}{
		{
			name: "漢字・ひらがな・カタカナ・半角カタカナの並び",
			data: []byte{
				0x34, 0x41, 0x3B, 0x7A, // 漢字（GL の G0 漢字）
				0xD2, 0xE9, 0xAC, 0xCA, // ひらがな（GR の G2 平仮名）
				esc, 0x7C, // LS3R: GR に G3 片仮名
				0xAB, 0xBF, 0xAB, 0xCA, 0xF9, // カタカナー
				esc, 0x29, 0x49, ls1, // G1 に JIS X 0201 片仮名を指示して GL に呼ぶ
				0x31, 0x46, 0x52, // ｱﾆﾒ
				ls0,
				0x34, 0x41, // 漢（GL が G0 に戻る）
			},
			want: "漢字ひらがなカタカナーｱﾆﾒ漢",
		},
		{
			name: "英数は LS1 で GL に呼ぶ",
			data: []byte{ls1, 'N', 'H', 'K', 0x20, '1', ls0, 0x41, 0x6D},
			want: "NHK 1総",
		},
		{
			name: "追加記号（丸数字と角括弧の字）",
			data: []byte{0x7E, 0x61, 0x7A, 0x56, 0x7A, 0x6A},
			want: "①\U0001F211\U0001F21E",
		},
		{
			name: "single shift は次の 1 文字だけ",
			data: []byte{ss2, 0x52, 0x34, 0x41, ss3, 0x2B, 0x3B, 0x7A},
			want: "ひ漢カ字",
		},
		{
			name: "locking shift の LS2 と LS3 は GL を切り替える",
			data: []byte{esc, 0x6E, 0x52, esc, 0x6F, 0x2B, ls0, 0x34, 0x41},
			want: "ひカ漢",
		},
		{
			name: "対応の無い符号は落とす",
			data: []byte{
				0x79, 0x21, // 表に無い 89 区
				esc, 0x28, 0x20, 0x41, 0x21, // G0 に DRCS-1 を指示した 1 文字
				esc, 0x24, 0x42, // G0 を漢字に戻す
				esc, 0x24, 0x3A, 0x30, 0x21, // JIS 互換漢字 2 面
				esc, 0x24, 0x42,
				0x34, 0x41,
			},
			want: "漢",
		},
		{
			name: "大きさ・色・位置の制御は無視する",
			data: []byte{
				0x89, 0x80, // MSZ、黒
				0x9B, '1', ';', '2', 0x20, 0x53, // CSI の SWF
				0x90, 0x48, // COL
				0x16, 0x41, // PAPF
				0x34, 0x41, 0x8A,
			},
			want: "漢",
		},
		{
			name: "途中で切れた 2 バイトの文字は落とす",
			data: []byte{0x34, 0x41, 0x3B},
			want: "漢",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Decode(tt.data); got != tt.want {
				t.Errorf("Decode = %q, want %q", got, tt.want)
			}
		})
	}
}
