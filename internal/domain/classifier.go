package domain

// 判定モデル（AI）への問い合わせの語彙である。どの機能が使うか（自動タグ付けなど）にも、
// どの提供元が答えるか（Ollama など）にも依らない。機能は見せるものと質問をこの形で作り、
// 提供元のアダプタはこの形を自分の通信の形へ写す（docs/design-docs/ai-layer.md）。

// ClassifierInput は判定モデルに見せるものである。
type ClassifierInput struct {
	// Facts は名前つきの文字の手がかり（例: title・file_name・folder）。
	Facts map[string]string
	// Images は添える JPEG で、並びの順に見せる。無ければ文字だけで判定する。
	Images [][]byte
}

// ClassifierQuestion は、はい／いいえで答える質問 1 つである。
type ClassifierQuestion struct {
	// Key は答えを引き当てる名前で、要求の中で重ならない。
	Key string
	// Text は質問の文（英語）である。
	Text string
}

// ClassifierRequest は判定 1 件である。質問の数に上限は無く、提供元の制約に合わせて
// 分けて問い合わせるのはアダプタの仕事である。
type ClassifierRequest struct {
	Input     ClassifierInput
	Questions []ClassifierQuestion
}
