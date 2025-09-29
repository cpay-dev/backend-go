package tokenmath

import (
	"errors"
	"fmt"
	"strings"

	"github.com/holiman/uint256"
)

// Largest 10^exp that fits in 256 bits is 10^77.
const MaxPow10Exp = 77

// ParseDecimal parses a non-negative decimal string like "25.45" into:
// mantissa=2545, fracDigits=2.
func ParseDecimal(s string) (*uint256.Int, uint, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, 0, errors.New("empty number")
	}
	if strings.HasPrefix(s, "-") {
		return nil, 0, errors.New("negative values not supported")
	}
	mant := uint256.NewInt(0)
	ten := uint256.NewInt(10)

	seenDot := false
	var frac uint

	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '.':
			if seenDot {
				return nil, 0, errors.New("multiple dots")
			}
			seenDot = true
		case c >= '0' && c <= '9':
			// mant = mant*10 + digit
			mant.Mul(mant, ten)
			if c != '0' {
				mant.Add(mant, uint256.NewInt(uint64(c-'0')))
			}
			if seenDot {
				frac++
			}
		default:
			return nil, 0, fmt.Errorf("invalid character: %q", c)
		}
	}
	return mant, frac, nil
}

// Pow10Uint256 returns 10^exp (exp >= 0). Panics are NOT used; caller must bound exp.
func Pow10Uint256(exp uint) (*uint256.Int, error) {
	// 10^78 > 2^256-1, but 10^77 fits. Keep a conservative cap.
	if exp > MaxPow10Exp {
		return nil, fmt.Errorf("pow10 exponent too large: %d", exp)
	}
	res := uint256.NewInt(1)
	ten := uint256.NewInt(10)
	for range exp {
		res.Mul(res, ten)
	}
	return res, nil
}

// FormatWithDecimals renders z as a base-10 string with exactly dec places.
// For dec==0, returns integer with no dot.
func FormatWithDecimals(z *uint256.Int, dec int) string {
	if dec == 0 {
		return z.Dec()
	}
	digits := z.Dec()

	// Ensure at least dec+1 characters by left-padding with zeros if needed.
	if len(digits) <= dec {
		// "123" with dec=5 -> "0.00123"
		zeros := strings.Repeat("0", dec-len(digits)+1)
		return "0." + zeros + digits
	}
	intPart := digits[:len(digits)-dec]
	fracPart := digits[len(digits)-dec:]
	return intPart + "." + fracPart
}

// CeilDiv returns ceil(n / d) for n>=0, d>0 using integer ops only.
func CeilDiv(n, d *uint256.Int) (*uint256.Int, error) {
	if d.IsZero() {
		return nil, errors.New("division by zero")
	}
	if n.IsZero() {
		return uint256.NewInt(0), nil
	}
	q := uint256.NewInt(0).Div(n, d)
	r := uint256.NewInt(0).Mod(n, d)
	if !r.IsZero() {
		q.Add(q, uint256.NewInt(1)) // if this overflows, the true result doesn't fit in 256 bits anyway
	}
	return q, nil
}

// TokensFromUSD_Ceil computes ceil( (amountUSD / priceUSD) * 10^tokenDecimals )
// using only uint256.Int.
// amountUSD, priceUSD are decimal strings like "25.45" (no exponents).
func TokensFromUSD_Ceil(amountUSD, priceUSD string, tokenDecimals uint) (*uint256.Int, error) {
	amountMant, da, err := ParseDecimal(amountUSD)
	if err != nil {
		return nil, fmt.Errorf("amount: %w", err)
	}
	priceMant, dp, err := ParseDecimal(priceUSD)
	if err != nil {
		return nil, fmt.Errorf("price: %w", err)
	}
	if priceMant.IsZero() {
		return nil, errors.New("price is zero")
	}

	// tokens = ceil( amount_num * 10^(dp + tokenDecimals) / (price_num * 10^da) )
	expNum := dp + tokenDecimals
	expDen := da

	// Cancel common powers of 10 to reduce growth: divide both exponents by k.
	if expNum > 0 && expDen > 0 {
		if expNum < expDen {
			expDen -= expNum
			expNum = 0
		} else {
			expNum -= expDen
			expDen = 0
		}
	}

	var scaleNum *uint256.Int
	if expNum == 0 {
		scaleNum = uint256.NewInt(1)
	} else {
		scaleNum, err = Pow10Uint256(expNum)
		if err != nil {
			return nil, err
		}
	}
	var scaleDen *uint256.Int
	if expDen == 0 {
		scaleDen = uint256.NewInt(1)
	} else {
		scaleDen, err = Pow10Uint256(expDen)
		if err != nil {
			return nil, err
		}
	}

	numerator := uint256.NewInt(0).Mul(amountMant, scaleNum)
	denominator := uint256.NewInt(0).Mul(priceMant, scaleDen)

	return CeilDiv(numerator, denominator)
}

// ValueUSD_ScaledFloor returns floor( amountTokens * priceUSD * 10^outDecimals )
// as a uint256 integer, using only integer arithmetic.
func ValueUSD_ScaledFloor(amountTokens, priceUSD string, outDecimals uint) (*uint256.Int, error) {
	amtMant, da, err := ParseDecimal(amountTokens)
	if err != nil {
		return nil, fmt.Errorf("amount: %w", err)
	}
	priceMant, dp, err := ParseDecimal(priceUSD)
	if err != nil {
		return nil, fmt.Errorf("price: %w", err)
	}

	// scaled = floor( A * P * 10^n / 10^(da+dp) )
	numScale := outDecimals
	denScale := da + dp
	// Cancel common powers of 10 to keep exponents small.
	if numScale > 0 && denScale > 0 {
		if numScale < denScale {
			denScale -= numScale
			numScale = 0
		} else {
			numScale -= denScale
			denScale = 0
		}
	}

	scaleNum := uint256.NewInt(1)
	if numScale > 0 {
		scaleNum, err = Pow10Uint256(numScale)
		if err != nil {
			return nil, err
		}
	}
	scaleDen := uint256.NewInt(1)
	if denScale > 0 {
		scaleDen, err = Pow10Uint256(denScale)
		if err != nil {
			return nil, err
		}
	}

	// numerator = amtMant * priceMant * scaleNum
	num := uint256.NewInt(0).Mul(amtMant, priceMant)
	num.Mul(num, scaleNum)

	// floor division
	scaled := uint256.NewInt(0).Div(num, scaleDen)
	return scaled, nil
}

// FormatFixed renders z as a base-10 string with exactly dec fractional digits.
func FormatFixed(z *uint256.Int, dec int) string {
	if dec == 0 {
		return z.Dec()
	}
	digits := z.Dec() // integer digits, no sign
	if len(digits) <= dec {
		// Need exactly dec fractional digits: left-pad digits to length dec.
		frac := strings.Repeat("0", dec-len(digits)) + digits
		return "0." + frac
	}
	intPart := digits[:len(digits)-dec]
	fracPart := digits[len(digits)-dec:]
	return intPart + "." + fracPart
}

func ValueUSD_Floor(amountTokens, priceUSD string, outDecimals uint) (string, error) {
	scaled, err := ValueUSD_ScaledFloor(amountTokens, priceUSD, outDecimals)
	if err != nil {
		return "", err
	}
	return FormatFixed(scaled, int(outDecimals)), nil
}
