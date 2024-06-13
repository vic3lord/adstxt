package adstxt

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  Record
	}{
		{
			name:  "happy path",
			input: "example.com, pub_id, DIRECT, 1234abcd",
			want:  Record{ExchangeDomain: "example.com", PublisherAccountID: "pub_id", AccountType: "DIRECT", AuthorityID: "1234abcd"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := Parse(strings.NewReader(tt.input))
			require.NoError(t, err)
			require.Equal(t, got[0], tt.want)
		})
	}
}
