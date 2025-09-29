package tokenmath_test

import (
	"testing"

	"github.com/cpay-dev/backend-go/pkg/tokenmath"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

func u(t *testing.T, s string) *uint256.Int {
	t.Helper()
	z, err := uint256.FromDecimal(s)
	require.NoError(t, err, "from decimal %s", s)
	return z
}

func TestParseDecimal(t *testing.T) {
	cases := []struct {
		in         string
		mant       string
		fracDigits uint
		ok         bool
	}{
		{"0", "0", 0, true},
		{"000", "0", 0, true},
		{"1", "1", 0, true},
		{"1.", "1", 0, true},
		{".5", "5", 1, true},
		{"25.45", "2545", 2, true},
		{"123", "123", 0, true},
		{"000123.4500", "1234500", 4, true},
		{"000123.45", "12345", 2, true},

		// invalids
		{"", "", 0, false},
		{"-1", "", 0, false},
		{"12.3.4", "", 0, false},
		{"1a", "", 0, false},
	}

	for _, tc := range cases {
		m, f, err := tokenmath.ParseDecimal(tc.in)
		if tc.ok {
			if err != nil {
				t.Fatalf("parse %q got err %v", tc.in, err)
			}
			if f != tc.fracDigits {
				t.Fatalf("parse %q frac=%d want %d", tc.in, f, tc.fracDigits)
			}
			want := u(t, tc.mant)
			if m.Cmp(want) != 0 {
				t.Fatalf("parse %q mant=%s want %s", tc.in, m.Dec(), want.Dec())
			}
		} else {
			if err == nil {
				t.Fatalf("parse %q expected error", tc.in)
			}
		}
	}
}

// Div

func TestCeilDiv(t *testing.T) {
	type C struct{ n, d, want string }
	for _, tc := range []C{
		{"0", "7", "0"},
		{"1", "7", "1"},
		{"6", "3", "2"}, // exact
		{"7", "3", "3"}, // round up
		{"999", "1000", "1"},
	} {
		got, err := tokenmath.CeilDiv(u(t, tc.n), u(t, tc.d))
		require.NoError(t, err, "ceilDiv(%s,%s)", tc.n, tc.d)
		require.Equal(t, got.Dec(), tc.want, "ceilDiv(%s,%s)", tc.n, tc.d)
	}

	// division by zero
	if _, err := tokenmath.CeilDiv(u(t, "1"), u(t, "0")); err == nil {
		t.Fatalf("ceilDiv division by zero should error")
	}
}

func TestTokensFromUSD_Ceil_Basic(t *testing.T) {
	got, err := tokenmath.TokensFromUSD_Ceil("25.45", "45.85", 6)
	require.NoError(t, err)
	require.Equal(t, got.Dec(), "555071", "wrong math")
}

func TestTokensFromUSD_Ceil_ExactDivision(t *testing.T) {
	got, err := tokenmath.TokensFromUSD_Ceil("91.70", "45.85", 6)
	require.NoError(t, err)
	require.Equal(t, got.Dec(), "2000000", "wrong math")
}

func TestTokensFromUSD_Ceil_VerySmallAmount(t *testing.T) {
	got, err := tokenmath.TokensFromUSD_Ceil("0.0000001", "1000000", 0)
	require.NoError(t, err)
	require.Equal(t, got.Dec(), "1", "wrong math")
}

func TestTokensFromUSD_Ceil_ZeroAmount(t *testing.T) {
	got, err := tokenmath.TokensFromUSD_Ceil("0", "12.34", 18)
	require.NoError(t, err)
	require.Equal(t, got.Dec(), "0", "wrong math")
}

func TestTokensFromUSD_Ceil_ZeroPrice(t *testing.T) {
	_, err := tokenmath.TokensFromUSD_Ceil("1.23", "0", 6)
	require.Error(t, err)
	require.Equal(t, err.Error(), "price is zero")
}

func TestTokensFromUSD_Ceil_CancellationHelps(t *testing.T) {
	// amount has 18 fractional digits; price has 1; decimals=18.
	// After cancellation we only compute 10^(1) in numerator, avoiding huge exponents.
	got, err := tokenmath.TokensFromUSD_Ceil("1.234567890123456789", "2.5", 18)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	// Check against rational math: ceil((1.234567890123456789 / 2.5) * 1e18)
	// = ceil(0.4938271560493827156 * 1e18) = ceil(493,827,156,049,382,715.6)
	// = 493827156049382716
	if got.Dec() != "493827156049382716" {
		t.Fatalf("got %s want 493827156049382716", got.Dec())
	}
}

func TestTokensFromUSD_Ceil_InputShapes(t *testing.T) {
	cases := []struct {
		amount, price string
		dec           uint
		want          string
	}{
		{"1.", "2", 0, "1"}, // ceil(0.5) => 1
		{".5", "2", 0, "1"}, // ceil(0.25) => 1
		{"0003.000", "2", 0, "2"},
	}
	for _, tc := range cases {
		got, err := tokenmath.TokensFromUSD_Ceil(tc.amount, tc.price, tc.dec)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if got.Dec() != tc.want {
			t.Fatalf("(%q,%q,%d) got %s want %s", tc.amount, tc.price, tc.dec, got.Dec(), tc.want)
		}
	}
}

func TestTokensFromUSD_Ceil_TooLargePow10(t *testing.T) {
	// Force a too-large exponent after cancellation.
	// price has dp=1; make tokenDecimals huge so (dp+decimals) > 77.
	_, err := tokenmath.TokensFromUSD_Ceil("1", "1.0", 80)
	require.Error(t, err)
	require.Equal(t, err.Error(), "pow10 exponent too large: 81")
}

// Mul

func TestFormatFixed_ZeroAndPadding(t *testing.T) {
	if got := tokenmath.FormatFixed(u(t, "0"), 0); got != "0" {
		t.Fatalf("got %q want %q", got, "0")
	}
	if got := tokenmath.FormatFixed(u(t, "0"), 4); got != "0.0000" {
		t.Fatalf("got %q want %q", got, "0.0000")
	}
	if got := tokenmath.FormatFixed(u(t, "2"), 6); got != "0.000002" {
		t.Fatalf("got %q want %q", got, "0.000002")
	}
	if got := tokenmath.FormatFixed(u(t, "12345"), 2); got != "123.45" {
		t.Fatalf("got %q want %q", got, "123.45")
	}
	if got := tokenmath.FormatFixed(u(t, "123"), 5); got != "0.00123" {
		t.Fatalf("got %q want %q", got, "0.00123")
	}
}

func TestValueUSD_ScaledFloor_TinyResult(t *testing.T) {
	// 1e-7 * 2e-7 = 2e-14; n=10 → floor(2e-14 * 1e10) = 0
	scaled, err := tokenmath.ValueUSD_ScaledFloor("0.0000001", "0.0000002", 10)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !scaled.IsZero() {
		t.Fatalf("scaled=%s want 0", scaled.Dec())
	}
	// Formatting should yield exactly 10 zeros after dot
	if got := tokenmath.FormatFixed(scaled, 10); got != "0.0000000000" {
		t.Fatalf("got %q want %q", got, "0.0000000000")
	}
}

func TestValueUSD_ScaledFloor_PaddingZeros(t *testing.T) {
	// 0.001 * 0.002 = 2e-6; n=6 → scaled=2
	scaled, err := tokenmath.ValueUSD_ScaledFloor("0.001", "0.002", 6)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if scaled.Dec() != "2" {
		t.Fatalf("scaled=%s want 2", scaled.Dec())
	}
	if got := tokenmath.FormatFixed(scaled, 6); got != "0.000002" {
		t.Fatalf("got %q want %q", got, "0.000002")
	}
}

func TestValueUSD_ScaledFloor_ZeroInputs(t *testing.T) {
	scaled, err := tokenmath.ValueUSD_ScaledFloor("0", "123.45", 5)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !scaled.IsZero() {
		t.Fatalf("scaled=%s want 0", scaled.Dec())
	}
	if got := tokenmath.FormatFixed(scaled, 5); got != "0.00000" {
		t.Fatalf("got %q want %q", got, "0.00000")
	}
}

func TestValueUSD_Floor_BasicExamples(t *testing.T) {
	// 2.5317 * 1.34856 = 3.414149352
	got, err := tokenmath.ValueUSD_Floor("2.5317", "1.34856", 5)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "3.41414" {
		t.Fatalf("n=5 got %s want 3.41414", got)
	}
	got, err = tokenmath.ValueUSD_Floor("2.5317", "1.34856", 2)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "3.41" {
		t.Fatalf("n=2 got %s want 3.41", got)
	}
	got, err = tokenmath.ValueUSD_Floor("2.5317", "1.34856", 0)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "3" {
		t.Fatalf("n=0 got %s want 3", got)
	}
}

func TestValueUSD_Floor_ExactProduct(t *testing.T) {
	// 2 * 1.25 = 2.50 exactly
	got, err := tokenmath.ValueUSD_Floor("2", "1.25", 2)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "2.50" {
		t.Fatalf("got %s want 2.50", got)
	}
	got, err = tokenmath.ValueUSD_Floor("2", "1.25", 1) // -> 2.5
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "2.5" {
		t.Fatalf("got %s want 2.5", got)
	}
}

func TestValueUSD_Floor_LargeDecimal(t *testing.T) {
	got, err := tokenmath.ValueUSD_Floor("1.000000000000000001", "2.500000", 20)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "2.50000000000000000250" {
		t.Fatalf("got %s want 2.50000000000000000250", got)
	}
}

func TestValueUSD_Floor_LargeDecimalsWithCancellation(t *testing.T) {
	got, err := tokenmath.ValueUSD_Floor("1.000000000000000001", "2.500000", 8)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	// true value: 2.5000000000000000025; floor to 8dp -> 2.50000000
	if got != "2.50000000" {
		t.Fatalf("got %s want 2.50000000", got)
	}
}

func TestValueUSD_Floor_InvalidInputs(t *testing.T) {
	if _, err := tokenmath.ValueUSD_Floor("-1", "1.0", 2); err == nil {
		t.Fatalf("expected error on negative amount")
	}
	if _, err := tokenmath.ValueUSD_Floor("1", "1.0.2", 2); err == nil {
		t.Fatalf("expected error on malformed price")
	}
}

func TestValueUSD_Floor_Pow10Limit(t *testing.T) {
	_, err := tokenmath.ValueUSD_Floor("1", "1", tokenmath.MaxPow10Exp+1)
	if err == nil {
		t.Fatalf("expected pow10 exponent error")
	}
}
