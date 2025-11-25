package main

import (
	"encoding/json"
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
	if len(os.Args) < 2 {
		fmt.Println("usage: go run main.go <ASN> [ASN...]")
		os.Exit(2)
	}
	for _, a := range os.Args[1:] {
		asn, err := strconv.ParseInt(a, 10, 64)
		if err != nil {
			fmt.Printf("%s: invalid ASN: %v\n", a, err)
			continue
		}
		name, err := rdapASNLookup(asn)
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

// rdapASNLookup queries RDAP for the given ASN and returns a best-effort name.
func rdapASNLookup(asn int64) (string, error) {
	if asn <= 0 {
		return "", fmt.Errorf("invalid ASN: %d", asn)
	}
	// skip private ASN range
	if asn >= 64512 && asn <= 65535 {
		return "", nil
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

		if n := strings.TrimSpace(extractAutnumName(aut)); n != "" {
			return n, nil
		}
		// successful response but no name found
		return "", nil
	}

	if lastErr != nil {
		return "", lastErr
	}
	return "", nil
}

// extractAutnumName tries common locations for a usable name.
func extractAutnumName(aut *rdap.Autnum) string {
	if aut == nil {
		return ""
	}
	if n := strings.TrimSpace(aut.Name); n != "" {
		return n
	}
	if h := strings.TrimSpace(aut.Handle); h != "" {
		return h
	}
	for _, e := range aut.Entities {
		if en := nameFromEntity(e); en != "" {
			return en
		}
	}
	for _, r := range aut.Remarks {
		if len(r.Description) > 0 {
			if d := strings.TrimSpace(r.Description[0]); d != "" {
				return d
			}
		}
	}
	return ""
}

// nameFromEntity attempts to find a "name" in an entity by marshaling to JSON.
func nameFromEntity(e rdap.Entity) string {
	b, err := json.Marshal(e)
	if err != nil {
		return ""
	}
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		return ""
	}
	for _, k := range []string{"name", "Name", "handle", "Handle"} {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	if vc, ok := m["vcardArray"]; ok {
		if arr, ok := vc.([]interface{}); ok && len(arr) >= 2 {
			if attrs, ok := arr[1].([]interface{}); ok {
				for _, a := range attrs {
					if pair, ok := a.([]interface{}); ok && len(pair) >= 3 {
						if key, ok := pair[0].(string); ok && strings.EqualFold(key, "fn") {
							// some implementations put the value at index 3 or 2; try both
							if val, ok := pair[3].(string); ok && strings.TrimSpace(val) != "" {
								return strings.TrimSpace(val)
							}
							if val, ok := pair[2].(string); ok && strings.TrimSpace(val) != "" {
								return strings.TrimSpace(val)
							}
						}
					}
				}
			}
		}
	}
	return ""
}
