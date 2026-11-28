package ollama

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// Image classification can take minutes on CPU-only hosts.
const requestTimeout = 10 * time.Minute

const maxResponseBytes = 1 << 20

// Keep models loaded between consecutive classifications.
const keepAlive = "10m"

// Batch sixteen questions to fit CPU hosts where sixty-four image-backed questions can exhaust
// memory; see docs/design-docs/ai-layer.md.
const BatchSize = 16

const (
	DefaultEndpoint = "http://127.0.0.1:11434"
	// DefaultModel favors the smaller, faster Clef variant.
	DefaultModel = "clef-flash"
)

// DefaultSettings are the connection's values before the owner saves one; cmd/mdm passes them to
// app.OllamaConnection, so the settings screen shows them.
func DefaultSettings() domain.OllamaSettings {
	return domain.OllamaSettings{Endpoint: DefaultEndpoint, Model: DefaultModel}
}

type SettingsSource interface {
	OllamaSettings(ctx context.Context) (domain.OllamaSettings, error)
}

// defaulted fills the values the source leaves empty from DefaultSettings.
type defaulted struct{ source SettingsSource }

func (d defaulted) OllamaSettings(ctx context.Context) (domain.OllamaSettings, error) {
	settings, err := d.source.OllamaSettings(ctx)
	if err != nil {
		return domain.OllamaSettings{}, err
	}
	return settings.WithDefaults(DefaultSettings()), nil
}

// Classifier requires New; the zero value is unusable.
type Classifier struct {
	settings SettingsSource
	http     *http.Client
}

// New honors environment proxy settings.
func New(settings SettingsSource) *Classifier {
	return NewWithHTTPClient(settings, &http.Client{Timeout: requestTimeout})
}

func NewWithHTTPClient(settings SettingsSource, client *http.Client) *Classifier {
	return &Classifier{settings: defaulted{settings}, http: client}
}

// Clef takes at most 255 options in a choice question; one is the none option.
const maxChoiceOptions = 254

// noneKey is the option Choose adds for "none of these"; callers' option keys must differ from it.
const noneKey = "_none"

// DecisionQuestion is one question of a decision request: "noul" for yes/no, or "choice" over Criteria.
type DecisionQuestion struct {
	Type         string            `json:"type"`
	Instructions string            `json:"instructions"`
	Criteria     map[string]string `json:"criteria,omitempty"`
}

// DecisionRequest is the body of POST /v1/systemone; scripts/clefrelay reads it too.
type DecisionRequest struct {
	Model     string                      `json:"model"`
	State     map[string]string           `json:"state"`
	Images    []string                    `json:"images,omitempty"`
	Questions map[string]DecisionQuestion `json:"questions"`
	KeepAlive string                      `json:"keep_alive"`
}

type answer struct {
	Type          string             `json:"type"`
	Noul          *float64           `json:"noul"`
	Probabilities map[string]float64 `json:"probabilities"`
}

type responseBody struct {
	Answers map[string]answer `json:"answers"`
	Error   string            `json:"error"`
}

// Classify returns yes probabilities, omits unanswered questions, and combines BatchSize batches
// using the same input.
func (c *Classifier) Classify(ctx context.Context, req domain.ClassifierRequest) (map[string]float64, error) {
	probabilities := map[string]float64{}
	if len(req.Questions) == 0 {
		return probabilities, nil
	}
	settings, err := c.settings.OllamaSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("cannot read the Ollama settings: %w", err)
	}
	images := encodeBase64(req.Input.Images)
	for start := 0; start < len(req.Questions); start += BatchSize {
		batch := req.Questions[start:min(start+BatchSize, len(req.Questions))]
		questions := make(map[string]DecisionQuestion, len(batch))
		for _, q := range batch {
			questions[q.Key] = DecisionQuestion{Type: "noul", Instructions: q.Text}
		}
		answers, err := c.post(ctx, settings, req.Input.Facts, images, questions)
		if err != nil {
			return nil, err
		}
		for key, a := range answers {
			if a.Noul != nil {
				probabilities[key] = *a.Noul
			}
		}
	}
	return probabilities, nil
}

// Choose asks one choice question per request, since a question with all its options costs about as many input
// tokens as a batch of yes/no questions. More options than Clef takes are dealt round-robin into equal questions, so
// options next to each other in the caller's order do not compete in the same question.
func (c *Classifier) Choose(ctx context.Context, req domain.ClassifierChoiceRequest) (map[string]float64, error) {
	probabilities := map[string]float64{}
	if len(req.Options) == 0 {
		return probabilities, nil
	}
	settings, err := c.settings.OllamaSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("cannot read the Ollama settings: %w", err)
	}
	images := encodeBase64(req.Input.Images)
	groups := (len(req.Options) + maxChoiceOptions - 1) / maxChoiceOptions
	for g := range groups {
		criteria := map[string]string{noneKey: req.None}
		for i := g; i < len(req.Options); i += groups {
			criteria[req.Options[i].Key] = req.Options[i].Text
		}
		questions := map[string]DecisionQuestion{"choice": {Type: "choice", Instructions: req.Instructions, Criteria: criteria}}
		answers, err := c.post(ctx, settings, req.Input.Facts, images, questions)
		if err != nil {
			return nil, err
		}
		for key, p := range answers["choice"].Probabilities {
			if key != noneKey {
				probabilities[key] = p
			}
		}
	}
	return probabilities, nil
}

func encodeBase64(images [][]byte) []string {
	var encoded []string
	for _, image := range images {
		encoded = append(encoded, base64.StdEncoding.EncodeToString(image))
	}
	return encoded
}

func (c *Classifier) post(ctx context.Context, settings domain.OllamaSettings, facts map[string]string,
	images []string, questions map[string]DecisionQuestion,
) (map[string]answer, error) {
	if facts == nil {
		facts = map[string]string{}
	}
	body := DecisionRequest{
		Model:     settings.Model,
		State:     facts,
		Images:    images,
		Questions: questions,
		KeepAlive: keepAlive,
	}
	status, raw, err := postJSON(ctx, c.http, settings.Endpoint, "/v1/systemone", body)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, answerError(status, raw)
	}
	var decoded responseBody
	if decodeErr := json.Unmarshal(raw, &decoded); decodeErr != nil {
		return nil, fmt.Errorf("cannot read the classifier response: %w", decodeErr)
	}
	if decoded.Answers == nil {
		return nil, errors.New("the classifier response has no answers")
	}
	return decoded.Answers, nil
}

// answerError passes on only the error text of an Ollama JSON answer.
func answerError(status int, raw []byte) error {
	var decoded responseBody
	if json.Unmarshal(raw, &decoded) != nil {
		decoded.Error = ""
	}
	return errors.New(statusMessage(status, decoded.Error))
}

// clefFormat is Clef's own request and answer as the assistant's model writes and reads them.
var clefFormat = domain.ClassifierWireFormat{
	Description: "Ask Clef, the app's judging model, yes/no questions about images and facts. The arguments are " +
		"Clef's own request: state (named facts as strings), images (image references) and questions (keyed by your " +
		"own keys, each {\"type\":\"noul\",\"instructions\":\"the question in English\"}). Answers Clef's answer as it " +
		"is: for each key, noul is the probability of yes from 0 to 1. Clef answers each question once for all the " +
		"images of the call together, so judge each video in its own call. One call can take minutes on a CPU.",
	QuestionTypes: []string{"noul"},
	QuestionsDescription: "Yes/no questions keyed by your own keys, such as " +
		"{\"cat\":{\"type\":\"noul\",\"instructions\":\"Is a cat visible?\"}}.",
}

// WireFormat describes Clef's decision request, whose yes/no questions are "noul" questions.
func (c *Classifier) WireFormat() domain.ClassifierWireFormat {
	return clefFormat
}

// SendWire sends Clef's own request with the saved decision model and keep-alive, and answers the endpoint's JSON
// as it is. More images than Ollama takes are tiled into one, since splitting would change what Clef judges together.
func (c *Classifier) SendWire(ctx context.Context, req domain.ClassifierWireRequest) (json.RawMessage, error) {
	settings, err := c.settings.OllamaSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("cannot read the Ollama settings: %w", err)
	}
	images, err := encodeImages(req.Images, maxImages)
	if err != nil {
		return nil, fmt.Errorf("cannot tile the images: %w", err)
	}
	body := DecisionRequest{
		Model: settings.Model, State: req.Facts, Images: images,
		Questions: make(map[string]DecisionQuestion, len(req.Questions)), KeepAlive: keepAlive,
	}
	if body.State == nil {
		body.State = map[string]string{}
	}
	for key, q := range req.Questions {
		body.Questions[key] = DecisionQuestion{Type: q.Type, Instructions: q.Instructions}
	}
	status, raw, err := postJSON(ctx, c.http, settings.Endpoint, "/v1/systemone", body)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, answerError(status, raw)
	}
	if !json.Valid(raw) {
		return nil, errors.New("the classifier response is not JSON")
	}
	return raw, nil
}
