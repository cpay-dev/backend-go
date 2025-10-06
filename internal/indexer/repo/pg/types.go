package pg

type (
	Chain        string
	TransferKind string
)

const (
	ChainUnichain Chain = "UNICHAIN"

	TransferKindNative   TransferKind = "NATIVE"
	TransferKindContract TransferKind = "CONTRACT"
)
