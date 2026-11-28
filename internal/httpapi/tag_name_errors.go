package httpapi

import (
	"errors"
	"strconv"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/extgen"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// tagNameUnusable answers a tag name error that names no problem.
const tagNameUnusable = "The tag name cannot be used."

// tagNameProblem is how the screen and external APIs answer one problem of a tag name the rule refuses.
type tagNameProblem struct {
	screen   gen.ErrorReason
	external extgen.ErrorReason
	message  string
	// limit is the length limit the problem exceeds, 0 for a problem without one.
	limit int
}

var tagNameProblems = map[domain.TagNameProblem]tagNameProblem{
	domain.TagNameEmpty: {
		screen: gen.ErrorReasonTagNameEmpty, external: extgen.TagNameEmpty, message: "Enter a tag name.",
	},
	domain.TagNameControlCharacters: {
		screen: gen.ErrorReasonTagNameControlCharacters, external: extgen.TagNameControlCharacters,
		message: "Tag names cannot contain control characters.",
	},
	domain.TagNameTooLong: {
		screen: gen.ErrorReasonTagNameTooLong, external: extgen.TagNameTooLong,
		message: "Tag names must be at most " + strconv.Itoa(domain.TagNameMaxLength) + " characters.",
		limit:   domain.TagNameMaxLength,
	},
}

// tagNameProblemOf is the problem err names; false when err names none of tagNameProblems.
func tagNameProblemOf(err error) (tagNameProblem, bool) {
	var invalid *domain.InvalidTagNameError
	if !errors.As(err, &invalid) {
		return tagNameProblem{}, false
	}
	problem, ok := tagNameProblems[invalid.Problem]
	return problem, ok
}

// limitRef is limit for an error body, nil for 0.
func (p tagNameProblem) limitRef() *int {
	if p.limit == 0 {
		return nil
	}
	limit := p.limit
	return &limit
}

// tagNameErrorBody is the screen API's 400 for err, its message after prefix.
func tagNameErrorBody(err error, prefix string) gen.Error {
	body := gen.Error{Code: codeInvalidRequest, Message: prefix + tagNameUnusable}
	if problem, ok := tagNameProblemOf(err); ok {
		body.Reason, body.Message, body.Limit = &problem.screen, prefix+problem.message, problem.limitRef()
	}
	return body
}

// externalTagNameErrorBody is the external API's 400 for err, its message after prefix.
func externalTagNameErrorBody(err error, prefix string) extgen.Error {
	body := extgen.Error{Code: extgen.ErrorCodeInvalidRequest, Message: prefix + tagNameUnusable}
	if problem, ok := tagNameProblemOf(err); ok {
		body.Reason, body.Message, body.Limit = &problem.external, prefix+problem.message, problem.limitRef()
	}
	return body
}
