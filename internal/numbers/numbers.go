/*
Package numbers raises a number to a power.
*/
package numbers

import "math/big"

// Exponentiation raises a number to a power.
func Exponentiation(value *big.Int, exponent int64) *big.Int {
	var (
		base   = big.NewInt(10)
		result *big.Int
	)

	if exponent >= 0 {
		exp := new(big.Int).Exp(base, big.NewInt(exponent), nil)
		result = new(big.Int).Mul(value, exp)
	} else {
		exp := new(big.Int).Exp(base, big.NewInt(-exponent), nil)
		result = new(big.Int).Quo(value, exp)
	}

	return result
}

// ExponentiationFloat raises a number to a power and convert to float.
func ExponentiationFloat(value *big.Float, exponent int64) *big.Float {
	var (
		base   = big.NewInt(10)
		result *big.Float
	)

	if exponent >= 0 {
		exp := new(big.Int).Exp(base, big.NewInt(exponent), nil)
		result = new(big.Float).Mul(value, new(big.Float).SetInt(exp))
	} else {
		exp := new(big.Int).Exp(base, big.NewInt(-exponent), nil)
		result = new(big.Float).Quo(value, new(big.Float).SetInt(exp))
	}

	return result
}
