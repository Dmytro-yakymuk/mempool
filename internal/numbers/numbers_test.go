package numbers_test

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"

	"mempool/internal/numbers"
)

func TestExponentiation(t *testing.T) {
	reqs := []struct {
		value    *big.Int
		exponent int64
		result   *big.Int
	}{
		{
			value:    big.NewInt(2),
			exponent: 2,
			result:   big.NewInt(200),
		},
		{
			value:    big.NewInt(0),
			exponent: 2,
			result:   big.NewInt(0),
		},
		{
			value:    big.NewInt(2),
			exponent: 0,
			result:   big.NewInt(2),
		},
		{
			value:    big.NewInt(1),
			exponent: 8,
			result:   big.NewInt(100000000),
		},
		{
			value:    big.NewInt(200),
			exponent: -2,
			result:   big.NewInt(2),
		},
		{
			value:    big.NewInt(200),
			exponent: -3,
			result:   big.NewInt(0),
		},
		{
			value:    big.NewInt(1583000000000),
			exponent: -8,
			result:   big.NewInt(15830),
		},
	}

	for _, req := range reqs {
		res := numbers.Exponentiation(req.value, req.exponent)
		assert.Equal(t, req.result.String(), res.String())
	}
}

func TestExponentiationFloat(t *testing.T) {
	reqs := []struct {
		value    *big.Float
		exponent int64
		result   *big.Float
	}{
		{
			value:    big.NewFloat(2),
			exponent: 2,
			result:   big.NewFloat(200),
		},
		{
			value:    big.NewFloat(0),
			exponent: 2,
			result:   big.NewFloat(0),
		},
		{
			value:    big.NewFloat(2),
			exponent: 0,
			result:   big.NewFloat(2),
		},
		{
			value:    big.NewFloat(1),
			exponent: 8,
			result:   big.NewFloat(100000000),
		},
		{
			value:    big.NewFloat(200),
			exponent: -2,
			result:   big.NewFloat(2),
		},
		{
			value:    big.NewFloat(1583000000000),
			exponent: -8,
			result:   big.NewFloat(15830),
		},
		// additional.
		{
			value:    big.NewFloat(0.02),
			exponent: 3,
			result:   big.NewFloat(20),
		},
		{
			value:    big.NewFloat(0.02),
			exponent: -2,
			result:   big.NewFloat(0.0002),
		},
	}

	for _, req := range reqs {
		res := numbers.ExponentiationFloat(req.value, req.exponent)
		assert.Equal(t, req.result.String(), res.String())
	}
}
