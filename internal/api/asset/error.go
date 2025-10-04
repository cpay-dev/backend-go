package asset

import "errors"

var ErrAssetNotFound = errors.New("asset not found")
var ErrPriceUnknown = errors.New("price unknown")
