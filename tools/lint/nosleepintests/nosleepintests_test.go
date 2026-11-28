package nosleepintests_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/syudead/vv/tools/lint/nosleepintests"
)

func TestReportsSleepsInTestsOutsideASynctestCallback(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), nosleepintests.Analyzer, "sleeps")
}
