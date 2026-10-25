// Package assistantcheck_test checks the assistant catalog against api/openapi.yaml; it lives outside domain so the
// file read does not violate depguard.
package assistantcheck_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// A new owner write operation must be classified, so the assistant never gains or silently lacks it.
func TestAssistantCatalogCoversOwnerWriteOperations(t *testing.T) {
	writes, owner := writeOperations(t)
	if len(owner) < 100 {
		t.Fatalf("openapi.yaml から owner の書き込み操作を %d 件しか読み取れていない", len(owner))
	}
	classified := map[string]string{}
	for _, row := range domain.AssistantCatalog() {
		classified[row.Operation] = "カタログ"
	}
	for _, exclusion := range domain.AssistantExclusions() {
		if where, ok := classified[exclusion.Operation]; ok {
			t.Errorf("%s が%sと除外の両方にある", exclusion.Operation, where)
		}
		classified[exclusion.Operation] = "除外"
	}

	for operation, path := range owner {
		if _, ok := classified[operation]; !ok && !strings.HasPrefix(path, domain.AssistantExcludedPathPrefix) {
			t.Errorf("%s (%s) がカタログにも除外にも無い。contracts の表に従ってどちらかに加えること", operation, path)
		}
	}
	for operation, where := range classified {
		if _, ok := writes[operation]; !ok {
			t.Errorf("%sの %s は openapi.yaml の書き込み操作ではない", where, operation)
		}
	}
}

var (
	openAPIPathLine      = regexp.MustCompile(`^  (/\S+):$`)
	openAPIMethodLine    = regexp.MustCompile(`^    (get|post|put|delete|patch):$`)
	openAPIOperationLine = regexp.MustCompile(`^      operationId: (\S+)$`)
	openAPISecurityLine  = regexp.MustCompile(`^      security:`)
)

// writeOperations maps each non-GET operation to its path; owner keeps those without a security override, which
// makes them owner-only.
func writeOperations(t *testing.T) (all, owner map[string]string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "api", "openapi.yaml"))
	if err != nil {
		t.Fatalf("api/openapi.yaml を読めない: %v", err)
	}
	all, owner = map[string]string{}, map[string]string{}
	var path, method, operation string
	for _, line := range strings.Split(string(raw), "\n") {
		if m := openAPIPathLine.FindStringSubmatch(line); m != nil {
			path = m[1]
			continue
		}
		if m := openAPIMethodLine.FindStringSubmatch(line); m != nil {
			method, operation = m[1], ""
			continue
		}
		if m := openAPIOperationLine.FindStringSubmatch(line); m != nil {
			operation = m[1]
			if method != "get" {
				all[operation], owner[operation] = path, path
			}
			continue
		}
		if openAPISecurityLine.MatchString(line) && operation != "" {
			delete(owner, operation)
		}
	}
	return all, owner
}
