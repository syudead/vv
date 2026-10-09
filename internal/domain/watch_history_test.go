package domain

import (
	"errors"
	"testing"
)

// 視聴の識別子は RFC 4122 の文字列の形（36 文字、8-4-4-4-12 の 16 進）だけを受け付ける。
func TestValidatePlaybackID(t *testing.T) {
	for id, valid := range map[string]bool{
		"0f8fad5b-d9cb-469f-a165-70867728950e":  true,
		"0F8FAD5B-D9CB-469F-A165-70867728950E":  true,
		"00000000-0000-0000-0000-000000000000":  true,
		"":                                      false,
		"0f8fad5b-d9cb-469f-a165-70867728950":   false,
		"0f8fad5b-d9cb-469f-a165-70867728950e0": false,
		"0f8fad5bd-9cb-469f-a165-70867728950e":  false,
		"0f8fad5b-d9cb-469f-a165-70867728950g":  false,
		"{f8fad5b-d9cb-469f-a165-70867728950}":  false,
		"0f8fad5b_d9cb_469f_a165_70867728950e":  false,
	} {
		err := ValidatePlaybackID(id)
		if valid && err != nil {
			t.Errorf("ValidatePlaybackID(%q) = %v, want nil", id, err)
		}
		if !valid && !errors.Is(err, ErrInvalidPlaybackID) {
			t.Errorf("ValidatePlaybackID(%q) = %v, want ErrInvalidPlaybackID", id, err)
		}
	}
}
