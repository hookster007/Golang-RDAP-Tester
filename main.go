package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	rdap "github.com/openrdap/rdap"
	"github.com/openrdap/rdap/bootstrap"
)

func main() {
	verbose := flag.Bool("v", false, "verbose: print full RDAP autnum JSON")
	flag.Parse()
	args := flag.Args()
	if len(args) < 1 {
		fmt.Println("usage: go run main.go [-v] <ASN> [ASN...]")
		os.Exit(2)
	}

	for _, a := range args {
		asn, err := strconv.ParseInt(a, 10, 64)
		if err != nil {
			fmt.Printf("%s: invalid ASN: %v\n", a, err)
			continue
		}
		name, err := rdapASNLookup(asn, *verbose)
		if err != nil {
			fmt.Printf("AS%d: error: %v\n", asn, err)
			continue
		}
		if name == "" {
			fmt.Printf("AS%d: (no name found)\n", asn)
		} else {
			fmt.Printf("AS%d: %s\n", asn, name)
		}
	}
}

func rdapASNLookup(asn int64, verbose bool) (string, error) {
	if asn <= 0 {
		return "", fmt.Errorf("invalid ASN: %d", asn)
	}
	// skip private ASN range
	if asn >= 64512 && asn <= 65535 {
		return "Private ASN", nil
	}

	httpClient := &http.Client{Timeout: 6 * time.Second}
	client := &rdap.Client{HTTP: httpClient, Bootstrap: &bootstrap.Client{}}

	forms := []string{"AS" + strconv.FormatInt(asn, 10), strconv.FormatInt(asn, 10)}
	var lastErr error

	for _, q := range forms {
		aut, err := client.QueryAutnum(q)
		if err != nil {
			lastErr = err
			continue
		}
		if aut == nil {
			lastErr = fmt.Errorf("nil RDAP autnum response for %s", q)
			continue
		}

		if verbose {
			if b, err := json.MarshalIndent(aut, "", "  "); err == nil {
				fmt.Printf("RDAP autnum for %s:\n%s\n", q, string(b))
			}
		}

		if n := extractAutnumName(aut); n != "" {
			return n, nil
		}
		return "", nil
	}

	if lastErr != nil {
		return "", lastErr
	}
	return "", nil
}

func extractAutnumName(aut *rdap.Autnum) string {
	if aut == nil {
		return ""
	}

	// 1: Look for entity vCard with kind="org" and return its fn (formatted name)
	for _, entity := range aut.Entities {
		if name := getOrgNameFromVCard(entity.VCard); name != "" {
			return name
		}
	}

	// 2: Fall back to remarks with title "description"
	for _, r := range aut.Remarks {
		if strings.EqualFold(strings.TrimSpace(r.Title), "description") && len(r.Description) > 0 {
			if d := strings.TrimSpace(r.Description[0]); d != "" {
				return d
			}
		}
	}

	// 3: Any remark description
	for _, r := range aut.Remarks {
		if len(r.Description) > 0 {
			if d := strings.TrimSpace(r.Description[0]); d != "" {
				return d
			}
		}
	}

	// 4: Last resorts
	if n := strings.TrimSpace(aut.Name); n != "" {
		return n
	}
	if h := strings.TrimSpace(aut.Handle); h != "" {
		return h
	}

	return ""
}

func getOrgNameFromVCard(vcard *rdap.VCard) string {
	if vcard == nil || len(vcard.Properties) == 0 {
		return ""
	}

	// Check if vCard kind is "org"
	isOrg := false
	for _, prop := range vcard.Properties {
		if strings.EqualFold(prop.Name, "kind") {
			vals := prop.Values()
			if len(vals) > 0 {
				kind := strings.ToLower(strings.TrimSpace(vals[len(vals)-1]))
				if strings.Contains(kind, "org") {
					isOrg = true
					break
				}
			}
		}
	}

	if !isOrg {
		return ""
	}

	// Find fn (formatted name) property
	for _, prop := range vcard.Properties {
		if strings.EqualFold(prop.Name, "fn") {
			vals := prop.Values()
			for i := len(vals) - 1; i >= 0; i-- {
				if v := strings.TrimSpace(vals[i]); v != "" {
					return v
				}
			}
		}
	}

	return ""
}
