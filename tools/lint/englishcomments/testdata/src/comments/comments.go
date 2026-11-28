package comments

// Reports prose with an ideographic full stop.
// サムネイルを作る。 // want "write this comment in English"
func a() {}

// せめてひらがなが続けば日本語の文 // want "write this comment in English"
func b() {}

/* 読点だけ、でも報告する */ // want "write this comment in English"

func c() {}

// One report per group, at its first Japanese line.
// 最初の行。 // want "write this comment in English"
// 次の行。
func d() {}

// Quoted values stay: the tag `お気に入り` and the title `夏の旅行。` are data.
func e() {}

// A name in Kanji or Katakana alone, such as 東京 or サムネイル, is not prose.
func f() {}

//go:generate echo ひらがなの指令。
func g() {}
