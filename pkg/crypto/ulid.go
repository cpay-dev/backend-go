package crypto

import (
	"crypto/rand"

	"github.com/IndexStorm/ulid"
)

func RandomULID() ulid.ULID {
	var ulid ulid.ULID
	_, _ = rand.Read(ulid[:]) // can't fail
	return ulid
}
