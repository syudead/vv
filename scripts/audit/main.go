// audit fails when an npm lockfile has a known advisory that accepted.json does not list, or when an accepted advisory no longer applies.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path"
	"sort"
)

type advisory struct {
	ID       string
	Package  string
	Severity string
	Range    string
	Title    string
	URL      string
}

// An acceptance records why an advisory stays unfixed and what ends that.
type acceptance struct {
	ID      string `json:"id"`
	Package string `json:"package"`
	Reason  string `json:"reason"`
	Revisit string `json:"revisit"`
}

func main() {
	acceptedPath := flag.String("accepted", "scripts/audit/accepted.json", "accepted advisories")
	flag.Parse()
	if flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: audit [-accepted file] <npm project dir>...")
		os.Exit(2)
	}
	accepted, err := readAccepted(*acceptedPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "audit: %v\n", err)
		os.Exit(2)
	}

	failed := false
	seen := map[string]bool{}
	for _, dir := range flag.Args() {
		report, err := npmAudit(dir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "audit: %s: %v\n", dir, err)
			os.Exit(2)
		}
		found, err := parseAdvisories(report)
		if err != nil {
			fmt.Fprintf(os.Stderr, "audit: %s: %v\n", dir, err)
			os.Exit(2)
		}
		for _, a := range found {
			seen[a.ID] = true
		}
		for _, a := range unaccepted(found, accepted) {
			failed = true
			fmt.Fprintf(os.Stderr, "%s: %s %s %s (%s): %s\n  %s\n", dir, a.Severity, a.Package, a.Range, a.ID, a.Title, a.URL)
		}
	}
	for _, a := range stale(accepted, seen) {
		failed = true
		fmt.Fprintf(os.Stderr, "%s (%s) is accepted but no lockfile has it any more; remove it from %s\n", a.ID, a.Package, *acceptedPath)
	}
	if failed {
		fmt.Fprintln(os.Stderr, "audit: fix the advisories above or accept them; see docs/how-to/dependency-updates.md#vulnerability-audit")
		os.Exit(1)
	}
	fmt.Printf("audit: no unaccepted npm advisory (%d accepted)\n", len(accepted))
}

func readAccepted(file string) ([]acceptance, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var accepted []acceptance
	if err := json.Unmarshal(data, &accepted); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	for _, a := range accepted {
		if a.ID == "" || a.Package == "" || a.Reason == "" || a.Revisit == "" {
			return nil, fmt.Errorf("%s: every entry needs id, package, reason and revisit", file)
		}
	}
	return accepted, nil
}

// npm audit exits 1 when it finds advisories, so only a missing report is an error.
func npmAudit(dir string) ([]byte, error) {
	cmd := exec.Command("npm", "audit", "--package-lock-only", "--json")
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	var exit *exec.ExitError
	foundAdvisories := errors.As(err, &exit) && exit.ExitCode() == 1 && len(out) > 0
	if err != nil && !foundAdvisories {
		return nil, fmt.Errorf("npm audit: %w: %s", err, stderr.String())
	}
	return out, nil
}

// parseAdvisories returns each advisory of an npm audit report once, however many packages inherit it.
func parseAdvisories(report []byte) ([]advisory, error) {
	var doc struct {
		Error *struct {
			Summary string `json:"summary"`
		} `json:"error"`
		Vulnerabilities map[string]struct {
			Via []json.RawMessage `json:"via"`
		} `json:"vulnerabilities"`
	}
	if err := json.Unmarshal(report, &doc); err != nil {
		return nil, fmt.Errorf("npm audit report: %w", err)
	}
	if doc.Error != nil {
		return nil, fmt.Errorf("npm audit: %s", doc.Error.Summary)
	}
	byID := map[string]advisory{}
	for _, v := range doc.Vulnerabilities {
		for _, raw := range v.Via {
			var via struct {
				Name     string `json:"name"`
				Severity string `json:"severity"`
				Range    string `json:"range"`
				Title    string `json:"title"`
				URL      string `json:"url"`
			}
			// A string entry names the dependency an advisory is inherited from.
			if json.Unmarshal(raw, &via) != nil || via.URL == "" {
				continue
			}
			id := path.Base(via.URL)
			byID[id] = advisory{ID: id, Package: via.Name, Severity: via.Severity, Range: via.Range, Title: via.Title, URL: via.URL}
		}
	}
	found := make([]advisory, 0, len(byID))
	for _, a := range byID {
		found = append(found, a)
	}
	sort.Slice(found, func(i, j int) bool { return found[i].ID < found[j].ID })
	return found, nil
}

func unaccepted(found []advisory, accepted []acceptance) []advisory {
	var rest []advisory
	for _, a := range found {
		if !isAccepted(a, accepted) {
			rest = append(rest, a)
		}
	}
	return rest
}

func isAccepted(a advisory, accepted []acceptance) bool {
	for _, acc := range accepted {
		if acc.ID == a.ID && acc.Package == a.Package {
			return true
		}
	}
	return false
}

// An acceptance that matches nothing would silently cover a later regression.
func stale(accepted []acceptance, seen map[string]bool) []acceptance {
	var rest []acceptance
	for _, a := range accepted {
		if !seen[a.ID] {
			rest = append(rest, a)
		}
	}
	return rest
}
