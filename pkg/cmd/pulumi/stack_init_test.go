package main

import (
	"fmt"
	"testing"

	"github.com/pulumi/pulumi/pkg/v3/backend"
	"github.com/stretchr/testify/assert"
)

// This test demonstrates that validateCreateStack will filter
// out teams consisting exclusively of whitespace.
func TestValidateCreateStackOpts(t *testing.T) {
	t.Parallel()

	var cases = []struct {
		name                 string
		rawTeams, validTeams []string
	}{
		{
			name: "Input Is Empty",
			// no raw or valid teams
			rawTeams:   []string{},
			validTeams: []string{},
		},
		{
			name:       "a aingle valid team is provided",
			rawTeams:   []string{"TeamRocket"},
			validTeams: []string{"TeamRocket"},
		},
		{
			name:       "only invalid teams are provided",
			rawTeams:   []string{" ", "\t", "\n"},
			validTeams: []string{},
		},
		{
			name:       "mixed valid and invalid teams are provided",
			rawTeams:   []string{" ", "Edward", "\t", "Jacob", "\n"},
			validTeams: []string{"Edward", "Jacob"},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(fmt.Sprintf("When %s", tc.name), func(t *testing.T) {
			t.Parallel()
			var stackName = "dev"
			var mockBackend = &backend.MockBackend{}
			// If the test case provides at least one valid team,
			// then the options should be non-nil.
			var expectTeams = len(tc.validTeams) > 0
			observed, err := validateCreateStackOpts(stackName, mockBackend, tc.rawTeams)
			assert.Nil(t, err)
			if !expectTeams {
				assert.Len(t, observed, 0)
				return
			}
			assert.NotNil(t, observed)
			teams, err := observed.Teams()
			assert.Nil(t, err)
			assert.ElementsMatch(t, teams, tc.validTeams)
		})
	}
}
