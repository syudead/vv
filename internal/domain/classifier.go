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

// ClefRequest is Clef's own decision request without the model and keep-alive, which the adapter adds; the
// assistant's clef tool sends it as the model wrote it, with each image reference replaced by the image's bytes.
type ClefRequest struct {
	State map[string]string
	// Images are encoded image files, shown in order.
	Images [][]byte
	// Questions are keyed by the caller's own keys.
	Questions map[string]ClefQuestion
}

type ClefQuestion struct {
	// Type is "noul" for a yes/no question answered with a probability.
	Type         string `json:"type"`
	Instructions string `json:"instructions"`
}
