package v3

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TotalItems is the catalog's row count, so unlike the paging scalars nothing
// upstream bounds it. Saturating keeps the contract's int32 field truthful at
// the edge instead of wrapping it negative.
func TestClampToInt32Saturates(t *testing.T) {
	tests := []struct {
		name  string
		given int
		want  int32
	}{
		{name: "an ordinary count passes through", given: 8, want: 8},
		{name: "zero passes through", given: 0, want: 0},
		{name: "the boundary passes through", given: math.MaxInt32, want: math.MaxInt32},
		{name: "above the boundary saturates", given: math.MaxInt32 + 1, want: math.MaxInt32},
		{name: "a negative count floors at zero", given: -1, want: 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, clampToInt32(test.given))
		})
	}
}
