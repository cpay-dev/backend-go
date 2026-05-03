package ids

import (
	"crypto/rand"
	"errors"
	"io"
	"strings"
	"time"
)

const (
	encodedLen = 26
	timeLen    = 10
	randLen    = 16
)

var crockford = []byte("0123456789ABCDEFGHJKMNPQRSTVWXYZ")

// New returns a canonical ULID string.
func New() string {
	var entropy [10]byte
	if _, err := io.ReadFull(rand.Reader, entropy[:]); err != nil {
		panic(err)
	}

	var out [encodedLen]byte
	encodeTime(out[:timeLen], uint64(time.Now().UTC().UnixMilli()))
	encodeRandom(out[timeLen:], entropy)
	return string(out[:])
}

func Parse(raw string) (string, error) {
	id := strings.ToUpper(strings.TrimSpace(raw))
	if !IsValid(id) {
		return "", errors.New("invalid ulid")
	}
	return id, nil
}

func IsValid(raw string) bool {
	if len(raw) != encodedLen {
		return false
	}
	for i := 0; i < encodedLen; i++ {
		c := raw[i]
		if c >= 'a' && c <= 'z' {
			c -= 'a' - 'A'
		}
		if !isCrockford(c) {
			return false
		}
	}
	return true
}

func encodeTime(dst []byte, ms uint64) {
	for i := timeLen - 1; i >= 0; i-- {
		dst[i] = crockford[ms&31]
		ms >>= 5
	}
}

func encodeRandom(dst []byte, entropy [10]byte) {
	value := new(bigInt80)
	value.setBytes(entropy)
	for i := randLen - 1; i >= 0; i-- {
		dst[i] = crockford[value.mod32()]
		value.div32()
	}
}

func isCrockford(c byte) bool {
	return (c >= '0' && c <= '9') ||
		(c >= 'A' && c <= 'H') ||
		(c >= 'J' && c <= 'K') ||
		(c >= 'M' && c <= 'N') ||
		(c >= 'P' && c <= 'T') ||
		(c >= 'V' && c <= 'Z')
}

type bigInt80 struct {
	hi uint16
	lo uint64
}

func (v *bigInt80) setBytes(b [10]byte) {
	v.hi = uint16(b[0])<<8 | uint16(b[1])
	for i := 2; i < len(b); i++ {
		v.lo = (v.lo << 8) | uint64(b[i])
	}
}

func (v *bigInt80) mod32() byte {
	return byte(v.lo & 31)
}

func (v *bigInt80) div32() {
	v.lo = (uint64(v.hi&31) << 59) | (v.lo >> 5)
	v.hi >>= 5
}
