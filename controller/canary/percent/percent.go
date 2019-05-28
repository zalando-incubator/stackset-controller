package percent

import (
	"fmt"
	"math"
)

// Percent is a fixed-precision numeric type for representing percentages
// with one decimal place.
// It is a struct in order to prevent direct comparisons of the underlying
// representation with unrelated values, which may return irrelevant results
// if the internal representation uses a different format.
//
// Compared to float, Percent does range validation, captures intent, and allows
// simple comparisons (whereas comparing floats can be dangerous).
// Compared to math/big.Rat, Percent does range validation and captures intent,
// while keeping a simple implementation.
//
// Range errors are stored in the corresponding (invalid) Percent result, instead of
// returning error on each method call, and are returned by the Error() method.
// A Percent in error state will short-circuit all arithmetic operations and panic in
// conversion methods like AsFloat.
type Percent struct {
	val int
	err error
}

// NewFromFloat converts a float64 into a Percent value.
// Since Percent has a precision of only one decimal place, f is rounded to the closest 0.1.
// An "error" Percent is returned if f represents a value larger than 100 or negative.
func NewFromFloat(f float64) Percent {
	expanded := math.Round(f * 10)
	if expanded < 0 || expanded > 1000 || math.IsNaN(expanded) {
		return Percent{err: fmt.Errorf("invalid percentage: %v", f)}
	}
	return Percent{val: int(expanded)}
}

// NewFromInt converts an int into a Percent value.
// An error is returned if n represents a value larger than 100 or negative.
func NewFromInt(n int) Percent {
	if n < 0 || n > 100 {
		return Percent{err: fmt.Errorf("invalid percentage: %v", n)}
	}
	return Percent{val: int(n * 10)}
}

// Error returns the error associated with this percentage, if any.
func (p Percent) Error() error {
	return p.err
}

// AsFloat converts p into a string. It panics if p has an error.
func (p Percent) String() string {
	if p.err != nil {
		panic(fmt.Sprintf("can't call String on invalid Percent: %v", p.err))
	}
	return fmt.Sprintf("%.3g%%", float64(p.val)/10)
}

// AsFloat converts p into a float64. It panics if p has an error.
func (p Percent) AsFloat() float64 {
	if p.err != nil {
		panic(fmt.Sprintf("can't call AsFloat on invalid Percent: %v", p.err))
	}
	return float64(p.val) / 10
}

// IsZero returns true if and only if p represents 0%. It panics if p has an error.
func (p Percent) IsZero() bool {
	if p.err != nil {
		panic(fmt.Sprintf("can't call IsZero on invalid Percent: %v", p.err))
	}
	return p.val == 0
}

// Is100 returns true if and only if p represents 100%. It panics if p has an error.
func (p Percent) Is100() bool {
	if p.err != nil {
		panic(fmt.Sprintf("can't call Is100 on invalid Percent: %v", p.err))
	}
	return p.val == 1000
}

// Cmp compares p and q and returns X such as:
// X < 0 if p < q, X == 0 if p == q, and X > 0 if p > q.
// It panics if either p or q have an error.
func (p Percent) Cmp(q Percent) int {
	if p.err != nil {
		panic(fmt.Sprintf("can't call Cmp on invalid Percent: %v", p.err))
	}
	if q.err != nil {
		panic(fmt.Sprintf("can't call Cmp on invalid Percent: %v", q.err))
	}
	return p.val - q.val
}

func shortCircuitErrors(res, p, q Percent) bool {
	if p.err != nil {
		res.err = p.err
		return true
	}
	if q.err != nil {
		res.err = q.err
		return true
	}
	return false
}

// Add sets z to the sum x+y and returns z.
// The result has an error if either x or y have an error, or if x+y is larger than 100.
func (p Percent) Add(q Percent) Percent {
	res := Percent{}
	if shortCircuitErrors(res, p, q) {
		return res
	}
	sum := p.val + q.val
	if sum > 1000 {
		res.err = fmt.Errorf("cannot add %s to %s: result is above 100%%", p, q)
	} else {
		res.val = sum
	}
	return res
}

// AddCapped returns min(p+q, 100).
// The result has an error if either x or y have an error.
func (p Percent) AddCapped(q Percent) Percent {
	res := Percent{val: 1000}
	if shortCircuitErrors(res, p, q) {
		return res
	}
	sum := p.val + q.val
	if sum < 1000 {
		res.val = sum
	}
	return res
}

// SubCapped returns max(p-q, 0).
// The result has an error if either x or y have an error.
func (p Percent) SubCapped(q Percent) Percent {
	res := Percent{}
	if shortCircuitErrors(res, p, q) {
		return res
	}
	result := p.val - q.val
	if result > 0 {
		res.val = result
	}
	return res
}

// Mul sets z to the product x×y and returns z.
// The result has an error if either x or y have an error.
func (p Percent) Mul(q Percent) Percent {
	res := Percent{}
	if shortCircuitErrors(res, p, q) {
		return res
	}
	res.val = int(math.Round(float64(p.val*q.val) / 1000)) // <0 or >100 are impossible
	return res
}
