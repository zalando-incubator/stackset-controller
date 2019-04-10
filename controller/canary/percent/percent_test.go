package percent_test

import (
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/zalando-incubator/stackset-controller/controller/canary/percent"
)

func TestPercent_String(t *testing.T) {
	pc100 := percent.NewFromInt(100)
	assert.Equal(t, "100%", pc100.String())

	pc25_1 := percent.NewFromFloat(25.1)
	assert.Equal(t, "25.1%", fmt.Sprintf("%v", pc25_1))
}

func TestPercent_AsFloat(t *testing.T) {
	validFloats := []float64{0.0, -0.0, 1.0, 1.2, 0.1, 50.9, 99.9, 100.0}
	for _, f := range validFloats {
		f := f // Run runs a goroutine.
		t.Run(fmt.Sprintf("valid %v", f), func(t *testing.T) {
			pc := percent.NewFromFloat(f)
			assert.Nil(t, pc.Error())
			assert.Equal(t, f, pc.AsFloat())
		})
	}
	invalidFloats := []float64{-1.0, -100.0, math.NaN(), math.Inf(1), math.Inf(-1)}
	for _, f := range invalidFloats {
		f := f
		t.Run(fmt.Sprintf("invalid %v", f), func(t *testing.T) {
			pc := percent.NewFromFloat(f)
			assert.Error(t, pc.Error())
			assert.Panics(t, func() { pc.AsFloat() })
		})
	}
}

func TestPercent_Add(t *testing.T) {
	t.Run("basic use case", func(t *testing.T) {
		a := percent.NewFromInt(32)
		b := percent.NewFromFloat(22.1)
		assert.Equal(t, percent.NewFromFloat(54.1), a.Add(b), "result should be valid")
		assert.Equal(t, percent.NewFromFloat(32), a, "receiver should not change")
		assert.Equal(t, percent.NewFromFloat(22.1), b, "argument should not change")
	})
	t.Run("twice the same operand", func(t *testing.T) {
		pc25 := percent.NewFromInt(25)
		assert.Equal(t, percent.NewFromInt(50), pc25.Add(pc25), "result should be valid")
		assert.Equal(t, percent.NewFromInt(25), pc25, "receiver should not change")
	})
	t.Run("percentage overflow", func(t *testing.T) {
		pc50_1 := percent.NewFromFloat(50.1)
		result := pc50_1.Add(pc50_1)
		assert.Error(t, result.Error())
		assert.Equal(t, percent.NewFromFloat(50.1), pc50_1, "receiver should not change")
	})
}

func TestPercent_Mul(t *testing.T) {
	t.Run("basic use case", func(t *testing.T) {
		a := percent.NewFromInt(32)
		b := percent.NewFromFloat(22.1)
		assert.Equal(t, percent.NewFromFloat(7.1), a.Mul(b), "result should be valid")
		assert.Equal(t, percent.NewFromFloat(32), a, "receiver should not change")
		assert.Equal(t, percent.NewFromFloat(22.1), b, "argument should not change")
	})
	t.Run("twice the same operand", func(t *testing.T) {
		pc25 := percent.NewFromInt(25)
		assert.Equal(t, percent.NewFromFloat(6.3), pc25.Mul(pc25), "result should be valid")
		assert.Equal(t, percent.NewFromFloat(25), pc25, "receiver should not change")
	})
}
