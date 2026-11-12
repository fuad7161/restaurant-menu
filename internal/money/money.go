// Package money handles taka amounts. Every amount is stored as an integer number of poisha.
package money

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Parse turns "120" or "120.50" taka into poisha. Invalid input gives 0.
func Parse(s string) int64 {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || f < 0 || f > 1e7 {
		return 0
	}
	return int64(math.Round(f * 100))
}

// Taka prints poisha as taka without the currency sign: "120" or "120.50".
func Taka(v int64) string {
	if v%100 == 0 {
		return strconv.FormatInt(v/100, 10)
	}
	return fmt.Sprintf("%d.%02d", v/100, v%100)
}

// Format prints poisha as "৳120".
func Format(v int64) string { return "৳" + Taka(v) }

// Percent returns p percent of amount, rounded to the nearest poisha.
func Percent(amount int64, p float64) int64 {
	return int64(math.Round(float64(amount) * p / 100))
}
