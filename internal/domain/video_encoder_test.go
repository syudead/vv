package domain

import (
	"reflect"
	"testing"
)

func TestParseEncoderChoiceFallsBackToSoftware(t *testing.T) {
	cases := []struct {
		value string
		found bool
		want  EncoderChoice
	}{
		{"", false, EncoderChoiceSoftware},
		{"nvenc", false, EncoderChoiceSoftware},
		{"h265_magic", true, EncoderChoiceSoftware},
		{"", true, EncoderChoiceSoftware},
		{"NVENC", true, EncoderChoiceSoftware},
		{"software", true, EncoderChoiceSoftware},
		{"nvenc", true, EncoderChoiceNVENC},
		{"qsv", true, EncoderChoiceQSV},
		{"vaapi", true, EncoderChoiceVAAPI},
		{"videotoolbox", true, EncoderChoiceVideoToolbox},
		{"auto", true, EncoderChoiceAuto},
	}
	for _, c := range cases {
		if got := ParseEncoderChoice(c.value, c.found); got != c.want {
			t.Errorf("ParseEncoderChoice(%q, %v) = %q, want %q", c.value, c.found, got, c.want)
		}
	}
}

func availability(states map[VideoEncoder]EncoderState) []EncoderAvailability {
	list := []EncoderAvailability{}
	for _, encoder := range HardwareVideoEncoders {
		state, ok := states[encoder]
		if !ok {
			list = append(list, EncoderAvailability{Encoder: encoder, State: EncoderUnavailable, Reason: EncoderReasonUnsupportedOS})
			continue
		}
		a := EncoderAvailability{Encoder: encoder, State: state}
		if state == EncoderUnavailable {
			a.Reason = EncoderReasonCheckFailed
		}
		list = append(list, a)
	}
	return list
}

func TestResolveVideoEncoder(t *testing.T) {
	allAvailable := availability(map[VideoEncoder]EncoderState{
		VideoEncoderNVENC: EncoderAvailable, VideoEncoderQSV: EncoderAvailable,
		VideoEncoderVAAPI: EncoderAvailable, VideoEncoderVideoToolbox: EncoderAvailable,
	})
	cases := []struct {
		name       string
		choice     EncoderChoice
		list       []EncoderAvailability
		want       VideoEncoder
		wantReason EncoderFallbackReason
	}{
		{"software ignores hardware", EncoderChoiceSoftware, allAvailable, VideoEncoderSoftware, ""},
		{"auto picks nvenc first", EncoderChoiceAuto, allAvailable, VideoEncoderNVENC, ""},
		{"auto picks qsv after nvenc", EncoderChoiceAuto, availability(map[VideoEncoder]EncoderState{
			VideoEncoderNVENC: EncoderUnavailable, VideoEncoderQSV: EncoderAvailable, VideoEncoderVAAPI: EncoderAvailable,
		}), VideoEncoderQSV, ""},
		{"auto picks vaapi after qsv", EncoderChoiceAuto, availability(map[VideoEncoder]EncoderState{
			VideoEncoderNVENC: EncoderChecking, VideoEncoderQSV: EncoderUnavailable, VideoEncoderVAAPI: EncoderAvailable,
		}), VideoEncoderVAAPI, ""},
		{"auto picks videotoolbox last", EncoderChoiceAuto, availability(map[VideoEncoder]EncoderState{
			VideoEncoderVideoToolbox: EncoderAvailable,
		}), VideoEncoderVideoToolbox, ""},
		{"auto without hardware is software without fallback", EncoderChoiceAuto, availability(map[VideoEncoder]EncoderState{
			VideoEncoderNVENC: EncoderUnavailable, VideoEncoderQSV: EncoderUnavailable, VideoEncoderVAAPI: EncoderUnavailable,
		}), VideoEncoderSoftware, ""},
		{"auto while checking is software without fallback", EncoderChoiceAuto, availability(map[VideoEncoder]EncoderState{
			VideoEncoderNVENC: EncoderChecking,
		}), VideoEncoderSoftware, ""},
		{"available hardware is used", EncoderChoiceVAAPI, allAvailable, VideoEncoderVAAPI, ""},
		{"unavailable hardware falls back", EncoderChoiceNVENC, availability(map[VideoEncoder]EncoderState{
			VideoEncoderNVENC: EncoderUnavailable, VideoEncoderQSV: EncoderAvailable,
		}), VideoEncoderSoftware, EncoderFallbackSelectedUnavailable},
		{"unsupported OS falls back", EncoderChoiceVideoToolbox, availability(map[VideoEncoder]EncoderState{
			VideoEncoderNVENC: EncoderAvailable,
		}), VideoEncoderSoftware, EncoderFallbackSelectedUnavailable},
		{"checking hardware falls back", EncoderChoiceQSV, availability(map[VideoEncoder]EncoderState{
			VideoEncoderQSV: EncoderChecking,
		}), VideoEncoderSoftware, EncoderFallbackChecking},
		{"unknown choice is software", EncoderChoice("h265_magic"), allAvailable, VideoEncoderSoftware, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, reason := ResolveVideoEncoder(c.choice, c.list)
			if got != c.want || reason != c.wantReason {
				t.Fatalf("ResolveVideoEncoder = (%q, %q), want (%q, %q)", got, reason, c.want, c.wantReason)
			}
		})
	}
}

func TestNewTranscodeEncodingReportsChecking(t *testing.T) {
	list := InitialEncoderAvailability("linux")
	got := NewTranscodeEncoding(EncoderChoiceNVENC, list)
	if !got.Checking || got.Effective != VideoEncoderSoftware || got.FallbackReason != EncoderFallbackChecking {
		t.Fatalf("encoding while checking = %+v", got)
	}
	for i := range list {
		if list[i].State == EncoderChecking {
			list[i] = EncoderAvailability{Encoder: list[i].Encoder, State: EncoderAvailable}
		}
	}
	got = NewTranscodeEncoding(EncoderChoiceNVENC, list)
	if got.Checking || got.Effective != VideoEncoderNVENC || got.FallbackReason != "" {
		t.Fatalf("encoding after checks = %+v", got)
	}
}

func TestHardwareEncoderCandidatesByOS(t *testing.T) {
	cases := map[string][]VideoEncoder{
		"linux":   {VideoEncoderNVENC, VideoEncoderQSV, VideoEncoderVAAPI},
		"windows": {VideoEncoderNVENC, VideoEncoderQSV},
		"darwin":  {VideoEncoderVideoToolbox},
		"freebsd": nil,
	}
	for goos, want := range cases {
		if got := HardwareEncoderCandidates(goos); !reflect.DeepEqual(got, want) {
			t.Errorf("HardwareEncoderCandidates(%q) = %v, want %v", goos, got, want)
		}
	}
}

func TestInitialEncoderAvailabilityListsEveryHardwareEncoder(t *testing.T) {
	want := []EncoderAvailability{
		{Encoder: VideoEncoderNVENC, State: EncoderChecking},
		{Encoder: VideoEncoderQSV, State: EncoderChecking},
		{Encoder: VideoEncoderVAAPI, State: EncoderUnavailable, Reason: EncoderReasonUnsupportedOS},
		{Encoder: VideoEncoderVideoToolbox, State: EncoderUnavailable, Reason: EncoderReasonUnsupportedOS},
	}
	if got := InitialEncoderAvailability("windows"); !reflect.DeepEqual(got, want) {
		t.Fatalf("InitialEncoderAvailability(windows) = %+v, want %+v", got, want)
	}
	for _, a := range InitialEncoderAvailability("plan9") {
		if a.State != EncoderUnavailable || a.Reason != EncoderReasonUnsupportedOS {
			t.Fatalf("plan9 availability = %+v", a)
		}
	}
}
