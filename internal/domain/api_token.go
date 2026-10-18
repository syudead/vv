package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// APITokenNameMaxLength counts Unicode code points and matches the tag-name limit.
const APITokenNameMaxLength = 100

// APIToken contains neither plaintext nor a hash; plaintext is returned only at creation and never stored.
type APIToken struct {
	ID        int64
	Name      string
	CreatedAt time.Time
	// LastUsedAt is zero until first use.
	LastUsedAt time.Time
}

// ErrInvalidAPITokenName maps to invalid_request (400); NormalizeAPITokenName wraps it in InvalidAPITokenNameError so the API can report the specific problem.
var ErrInvalidAPITokenName = errors.New("invalid API token name")

type APITokenNameProblem int

const (
	APITokenNameEmpty APITokenNameProblem = iota + 1
	APITokenNameControlCharacters
	APITokenNameTooLong
)

type InvalidAPITokenNameError struct {
	Problem APITokenNameProblem
}

func (e *InvalidAPITokenNameError) Error() string {
	switch e.Problem {
	case APITokenNameEmpty:
		return "API token name is empty"
	case APITokenNameControlCharacters:
		return "API token name contains control characters"
	case APITokenNameTooLong:
		return fmt.Sprintf("API token name must be at most %d characters", APITokenNameMaxLength)
	default:
		return ErrInvalidAPITokenName.Error()
	}
}

func (e *InvalidAPITokenNameError) Unwrap() error { return ErrInvalidAPITokenName }

// NormalizeAPITokenName rejects control characters before trimming Unicode whitespace, then rejects empty or overlong names; it performs no Unicode normalization and permits duplicate names.
func NormalizeAPITokenName(input string) (string, error) {
	for _, r := range input {
		if unicode.Is(unicode.Cc, r) {
			return "", &InvalidAPITokenNameError{Problem: APITokenNameControlCharacters}
		}
	}
	trimmed := strings.TrimFunc(input, func(r rune) bool {
		return unicode.Is(unicode.White_Space, r)
	})
	if trimmed == "" {
		return "", &InvalidAPITokenNameError{Problem: APITokenNameEmpty}
	}
	if utf8.RuneCountInString(trimmed) > APITokenNameMaxLength {
		return "", &InvalidAPITokenNameError{Problem: APITokenNameTooLong}
	}
	return trimmed, nil
}
