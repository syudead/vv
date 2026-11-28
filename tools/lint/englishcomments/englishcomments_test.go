package englishcomments_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/syudead/vv/tools/lint/englishcomments"
)

func TestReportsJapaneseProseOutsideBackticksDirectivesAndGeneratedFiles(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), englishcomments.Analyzer, "comments")
}
