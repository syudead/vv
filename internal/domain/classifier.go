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

// ClassifierChoiceRequest asks which one of Options fits the input best. The probabilities of the options in one
// provider question sum to one, so they rank options; they are not independent yes/no answers.
type ClassifierChoiceRequest struct {
	Input ClassifierInput
	// Instructions is the English question the options answer.
	Instructions string
	// Options use Key as the answer key and Text as the option's description; keys must be unique.
	Options []ClassifierQuestion
	// None describes the answer that no option fits. Adapters add it to every provider question they send and
	// leave it out of the probabilities they return.
	None string
}

// ClassifierWireRequest is a request in the provider's own question format, which a model writes from the provider's
// ClassifierWireFormat; the adapter adds the model and whatever else the provider needs.
type ClassifierWireRequest struct {
	Facts map[string]string
	// Images are encoded image files, shown in order.
	Images [][]byte
	// Questions are keyed by the caller's own keys.
	Questions map[string]ClassifierWireQuestion
}

type ClassifierWireQuestion struct {
	// Type is one of the format's QuestionTypes.
	Type         string `json:"type"`
	Instructions string `json:"instructions"`
}

// ClassifierWireFormat tells a model how to write a provider's request and how to read its answer.
type ClassifierWireFormat struct {
	// Description says what the provider judges, how to write the questions and how to read the answer.
	Description   string
	QuestionTypes []string
	// QuestionsDescription describes the questions argument, with an example.
	QuestionsDescription string
}
