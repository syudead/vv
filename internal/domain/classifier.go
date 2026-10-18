package domain

// Classifier requests are independent of features and providers; adapters translate this contract (docs/design-docs/ai-layer.md).

type ClassifierInput struct {
	// Facts contains named text cues such as title, file_name, and folder.
	Facts map[string]string
	// Images are ordered JPEGs; empty means text-only classification.
	Images [][]byte
}

type ClassifierQuestion struct {
	// Key must be unique within the request.
	Key string
	// Text is an English yes/no question.
	Text string
}

// ClassifierRequest imposes no question limit; adapters split requests to satisfy provider limits.
type ClassifierRequest struct {
	Input     ClassifierInput
	Questions []ClassifierQuestion
}
