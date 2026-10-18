package httpapi

import (
	"errors"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// UI and external APIs share workInfoFailureOf, then serialize into their respective generated error types.

// workInfoFailure uses contract code/reason values; empty field and zero limit mean absent.
type workInfoFailure struct {
	status  int
	code    string
	reason  string
	field   string
	limit   int
	message string
}

var workInfoProblemReasons = map[domain.WorkInfoProblem]gen.ErrorReason{
	domain.WorkInfoControlCharacters: gen.ErrorReasonWorkInfoControlCharacters,
	domain.WorkInfoTooLong:           gen.ErrorReasonWorkInfoTooLong,
	domain.WorkInfoTooMany:           gen.ErrorReasonWorkInfoTooMany,
	domain.WorkInfoInvalidDate:       gen.ErrorReasonWorkInfoInvalidDate,
	domain.WorkInfoInvalidURL:        gen.ErrorReasonWorkInfoInvalidUrl,
	domain.WorkInfoInvalidLanguage:   gen.ErrorReasonWorkInfoInvalidLanguage,
	domain.WorkInfoDuplicate:         gen.ErrorReasonWorkInfoDuplicate,
	domain.WorkInfoEmpty:             gen.ErrorReasonWorkInfoEmpty,
	domain.WorkInfoOutOfRange:        gen.ErrorReasonWorkInfoOutOfRange,
}

var creditNotFoundReasons = map[domain.CreditKind]gen.ErrorReason{
	domain.CreditPerson:       gen.ErrorReasonPersonNotFound,
	domain.CreditOrganization: gen.ErrorReasonOrganizationNotFound,
}

// workInfoFailureOf maps domain work-info and credit errors; false leaves endpoint-specific errors to the caller.
func workInfoFailureOf(err error) (workInfoFailure, bool) {
	var invalid *domain.InvalidWorkInfoError
	var missing *domain.CreditNotFoundError
	var taken *domain.CreditExternalIDTakenError
	switch {
	case errors.As(err, &invalid):
		reason, ok := workInfoProblemReasons[invalid.Problem]
		if !ok {
			return workInfoFailure{}, false
		}
		failure := workInfoFailure{
			status: http.StatusBadRequest, code: string(codeInvalidRequest), reason: string(reason),
			field: invalid.Field, message: invalid.Error() + ".",
		}
		if invalid.Problem == domain.WorkInfoTooLong || invalid.Problem == domain.WorkInfoTooMany {
			failure.limit = invalid.Limit
		}
		return failure, true
	case errors.As(err, &missing):
		return workInfoFailure{
			status: http.StatusNotFound, code: string(codeNotFound), reason: string(creditNotFoundReasons[missing.Kind]),
			field: missing.Field, message: "The " + string(missing.Kind) + " named by " + missing.Field + " was not found.",
		}, true
	case errors.As(err, &taken):
		return workInfoFailure{
			status: http.StatusConflict, code: string(codeConflict), reason: string(gen.ErrorReasonExternalIdTaken),
			field: taken.Field, message: "Another " + string(taken.Kind) + " already has the external ID in " + taken.Field + ".",
		}, true
	case errors.Is(err, domain.ErrCreditParentCycle):
		return workInfoFailure{
			status: http.StatusBadRequest, code: string(codeInvalidRequest), reason: string(gen.ErrorReasonOrganizationParentCycle),
			message: "The parent can't be the organization itself or one inside it.",
		}, true
	}
	return workInfoFailure{}, false
}

func (s *server) writeWorkInfoFailure(w http.ResponseWriter, failure workInfoFailure) {
	reason := gen.ErrorReason(failure.reason)
	body := gen.Error{Code: gen.ErrorCode(failure.code), Reason: &reason, Message: failure.message}
	if failure.field != "" {
		field := failure.field
		body.Field = &field
	}
	if failure.limit > 0 {
		limit := failure.limit
		body.Limit = &limit
	}
	s.writeErrorBody(w, failure.status, body)
}
