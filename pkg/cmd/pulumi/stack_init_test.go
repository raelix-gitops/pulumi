package main

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateCreateStackOpts(t *testing.T) {
	t.Parallel()

	var cases = []struct {
		name                 string
		rawTeams, validTeams []string
		hasValidTeams        bool
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
			// If the test case provides at least one valid team,
			// then the options should be non-nil.
			var expectTeams = len(tc.validTeams) > 0
			var observed = validateCreateStackOpts(tc.rawTeams)
			if !expectTeams {
				assert.Nil(t, observed)
				return
			}
			assert.NotNil(t, observed)
			assert.ElementsMatch(t, observed.teams, tc.validTeams)
		})
	}
}
