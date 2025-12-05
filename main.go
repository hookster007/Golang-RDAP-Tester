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
	// Skip private ASN range (RFC 6996)
	if asn >= 64512 && asn <= 65535 {
		return "Private ASN", nil
	}

	httpClient := &http.Client{Timeout: 6 * time.Second}
	client := &rdap.Client{HTTP: httpClient, Bootstrap: &bootstrap.Client{}}

	// Try both "AS12345" and "12345" formats
	queryFormats := []string{"AS" + strconv.FormatInt(asn, 10), strconv.FormatInt(asn, 10)}
	var lastErr error

	for _, queryString := range queryFormats {
		autnumRecord, err := client.QueryAutnum(queryString)
		if err != nil {
			lastErr = err
			continue
		}
		if autnumRecord == nil {
			lastErr = fmt.Errorf("nil RDAP autnum response for %s", queryString)
			continue
		}

		if verbose {
			if jsonBytes, err := json.MarshalIndent(autnumRecord, "", "  "); err == nil {
				fmt.Printf("RDAP autnum for %s:\n%s\n", queryString, string(jsonBytes))
			}
		}

		if organizationName := extractAutnumName(autnumRecord); organizationName != "" {
			return organizationName, nil
		}
		return "", nil
	}

	if lastErr != nil {
		return "", lastErr
	}
	return "", nil
}

func extractAutnumName(autnumRecord *rdap.Autnum) string {
	if autnumRecord == nil {
		return ""
	}

	// Step 1: Look for an organization vCard with kind="org" and extract its formatted name (fn)
	for _, entity := range autnumRecord.Entities {
		if organizationName := getOrgNameFromVCard(entity.VCard); organizationName != "" {
			return shortenTo40Chars(organizationName)
		}
	}

	// Step 2: Fall back to remarks with title "description" (common in APNIC records)
	for _, remark := range autnumRecord.Remarks {
		if strings.EqualFold(strings.TrimSpace(remark.Title), "description") && len(remark.Description) > 0 {
			if description := strings.TrimSpace(remark.Description[0]); description != "" {
				return shortenTo40Chars(description)
			}
		}
	}

	// Step 3: Try any remark description as a fallback
	for _, remark := range autnumRecord.Remarks {
		if len(remark.Description) > 0 {
			if description := strings.TrimSpace(remark.Description[0]); description != "" {
				return shortenTo40Chars(description)
			}
		}
	}

	// Step 4: Last resorts - use the RDAP name field or handle
	if name := strings.TrimSpace(autnumRecord.Name); name != "" {
		return shortenTo40Chars(name)
	}
	if handle := strings.TrimSpace(autnumRecord.Handle); handle != "" {
		return shortenTo40Chars(handle)
	}

	return ""
}

func getOrgNameFromVCard(vcard *rdap.VCard) string {
	if vcard == nil || len(vcard.Properties) == 0 {
		return ""
	}

	// First pass: Check if this vCard represents an organization (kind="org")
	isOrganization := false
	for _, property := range vcard.Properties {
		if strings.EqualFold(property.Name, "kind") {
			values := property.Values()
			if len(values) > 0 {
				kindValue := strings.ToLower(strings.TrimSpace(values[len(values)-1]))
				if strings.Contains(kindValue, "org") {
					isOrganization = true
					break
				}
			}
		}
	}

	// If this isn't an organization vCard, skip it
	if !isOrganization {
		return ""
	}

	// Second pass: Extract the formatted name (fn) from the organization vCard
	for _, property := range vcard.Properties {
		if strings.EqualFold(property.Name, "fn") {
			values := property.Values()
			// Search from the end of values array backwards for the first non-empty value
			for i := len(values) - 1; i >= 0; i-- {
				if formattedName := strings.TrimSpace(values[i]); formattedName != "" {
					return formattedName
				}
			}
		}
	}

	return ""
}

// shortenTo40Chars trims whitespace and limits the string to 40 Unicode characters (runes)
func shortenTo40Chars(text string) string {
	runes := []rune(text)
	if len(runes) > 40 {
		return string(runes[:40])
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return text
	}
	return text
}
