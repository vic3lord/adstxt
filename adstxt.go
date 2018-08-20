package adstxt

import (
	"bufio"
	"io"
	"strings"
)

// Record is ads.txt data field defined in iab.
type Record struct {
	// ExchangeDomain is domain name of the advertising system
	ExchangeDomain string

	// PublisherAccountID is the identifier associated with the seller
	// or reseller account within the advertising system.
	PublisherAccountID string

	// AccountType is an enumeration of the type of account.
	AccountType AccountType

	// AuthorityID is an ID that uniquely identifies the advertising system
	// within a certification authority.
	AuthorityID string
}

// AccountType specify account enum
type AccountType int

const (
	AccountDirect AccountType = iota
	AccountReseller
	AccountOther
)

func parseAccountType(s string) AccountType {
	switch s {
	case "direct":
		return AccountDirect
	case "reseller":
		return AccountReseller
	default:
		return AccountOther
	}
}

func sanitize(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func parseRow(row string) (Record, error) {
	comment := strings.Index(row, "#")
	if comment != -1 {
		row = row[:comment]
	}
	fields := strings.Split(row, ",")

	if len(fields) < 2 || len(fields) > 4 {
		return Record{}, nil
	}

	var r Record
	r.ExchangeDomain = sanitize(fields[0])
	r.PublisherAccountID = sanitize(fields[1])

	if len(fields) >= 3 {
		r.AccountType = parseAccountType(fields[2])
	}

	// AuthorityID is optional
	if len(fields) == 4 {
		r.AuthorityID = sanitize(fields[3])
	}
	return r, nil
}

// Parse takes a text and returns a slice of Records
func Parse(in io.Reader) ([]Record, error) {
	records := make([]Record, 0)
	scanner := bufio.NewScanner(in)
	for scanner.Scan() {
		r, err := parseRow(scanner.Text())
		if err != nil {
			return nil, err
		}
		if r.ExchangeDomain == "" {
			continue
		}
		records = append(records, r)
	}
	return records, scanner.Err()
}
