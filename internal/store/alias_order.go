package store

import (
	"fmt"

	"modernc.org/sqlite"

	"github.com/syudead/vv/internal/domain"
)

// Migration 00098 numbers stored names by this collation; it stays registered so a database migrated later gets the
// same order.
const aliasOrderCollation = "vv_alias_order"

func init() {
	if err := sqlite.RegisterCollationUtf8(aliasOrderCollation, domain.CompareAliasOrder); err != nil {
		panic(fmt.Sprintf("cannot register %s: %v", aliasOrderCollation, err))
	}
}
