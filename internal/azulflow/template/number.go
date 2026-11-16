package template

import (
	"math"
	"strconv"
	"strings"
)

// FormatNumber converts a number to a string the way GitHub does, with .NET's
// "G15" format: 15 significant digits, scientific notation such as
// "1.34567891234568E+18" for large and small exponents, and "Infinity",
// "-Infinity", and "NaN".
func FormatNumber(f float64) string {
	switch {
	case math.IsNaN(f):
		return "NaN"
	case math.IsInf(f, 1):
		return "Infinity"
	case math.IsInf(f, -1):
		return "-Infinity"
	}
	return strings.ToUpper(strconv.FormatFloat(f, 'g', 15, 64))
}
