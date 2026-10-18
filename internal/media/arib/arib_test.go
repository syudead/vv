package arib

import "testing"

// Fixtures start with GL=G0 Kanji, GR=G2 hiragana, G1 alphanumeric, and G3 katakana.

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
				0x34, 0x41, 0x3B, 0x7A, // Kanji via GL/G0.
				0xD2, 0xE9, 0xAC, 0xCA, // Hiragana via GR/G2.
				esc, 0x7C, // LS3R selects G3 katakana for GR.
				0xAB, 0xBF, 0xAB, 0xCA, 0xF9, // Katakana including a long-vowel mark.
				esc, 0x29, 0x49, ls1, // Designate JIS X 0201 katakana as G1 and invoke it in GL.
				0x31, 0x46, 0x52, // Half-width katakana.
				ls0,
				0x34, 0x41, // GL returns to G0 Kanji.
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
				0x79, 0x21, // Unsupported row 89.
				esc, 0x28, 0x20, 0x41, 0x21, // One character after designating G0 as DRCS-1.
				esc, 0x24, 0x42, // Restore G0 Kanji.
				esc, 0x24, 0x3A, 0x30, 0x21, // JIS-compatible Kanji plane 2.
				esc, 0x24, 0x42,
				0x34, 0x41,
			},
			want: "漢",
		},
		{
			name: "大きさ・色・位置の制御は無視する",
			data: []byte{
				0x89, 0x80, // Medium size, black.
				0x9B, '1', ';', '2', 0x20, 0x53, // CSI SWF.
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
