package httpapi

import (
	"errors"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// 作品情報と人物・団体の誤りの写し方。画面 API の人物・団体と作品情報の経路、外部連携 API の経路が
// 同じ workInfoFailureOf を呼び、応答の型（gen.Error・extgen.Error）への書き出しだけをそれぞれが持つ。

// workInfoFailure は誤り 1 件の API の形である。code と reason は契約の値の文字列で、
// field と limit は無ければ空と 0。
type workInfoFailure struct {
	status  int
	code    string
	reason  string
	field   string
	limit   int
	message string
}

// workInfoProblemReasons は domain の規則違反の種類から reason への対応である。
var workInfoProblemReasons = map[domain.WorkInfoProblem]gen.ErrorReason{
	domain.WorkInfoControlCharacters: gen.ErrorReasonWorkInfoControlCharacters,
	domain.WorkInfoTooLong:           gen.ErrorReasonWorkInfoTooLong,
	domain.WorkInfoTooMany:           gen.ErrorReasonWorkInfoTooMany,
	domain.WorkInfoInvalidDate:       gen.ErrorReasonWorkInfoInvalidDate,
	domain.WorkInfoInvalidURL:        gen.ErrorReasonWorkInfoInvalidUrl,
	domain.WorkInfoInvalidLanguage:   gen.ErrorReasonWorkInfoInvalidLanguage,
	domain.WorkInfoDuplicate:         gen.ErrorReasonWorkInfoDuplicate,
	domain.WorkInfoEmpty:             gen.ErrorReasonWorkInfoEmpty,
}

// creditNotFoundReasons は実体の種類から、その実体が無いときの reason への対応である。
var creditNotFoundReasons = map[domain.CreditKind]gen.ErrorReason{
	domain.CreditPerson:       gen.ErrorReasonPersonNotFound,
	domain.CreditOrganization: gen.ErrorReasonOrganizationNotFound,
}

// workInfoFailureOf は *domain.InvalidWorkInfoError・*domain.CreditNotFoundError・
// *domain.CreditExternalIDTakenError と domain.ErrCreditParentCycle を API の形にする。どれでもなければ
// false を返し、呼び手が自分の経路の誤り（対象の実体が無いなど）を写す。
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

// writeWorkInfoFailure は画面 API の Error として failure を書き出す。
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
