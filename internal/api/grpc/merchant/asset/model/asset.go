package model

import (
	"encoding/json"
	"fmt"

	chainmodel "github.com/cpay-dev/backend-go/internal/api/grpc/merchant/chain/model"
	pgblockchain "github.com/cpay-dev/backend-go/internal/api/repo/pg/blockchain"
	pbasset "github.com/cpay-dev/proto-go/api/v1/merchant/asset"
	"google.golang.org/protobuf/encoding/protojson"
)

func AssetToProto(asset pgblockchain.Asset) (*pbasset.Asset, error) {
	chain, err := chainmodel.ChainToProto(asset.ChainID)
	if err != nil {
		return nil, err
	}
	metadata, err := UnmarshalMetadata(asset.Metadata)
	if err != nil {
		return nil, err
	}
	return &pbasset.Asset{
		Id:       asset.ID,
		Chain:    chain,
		Name:     asset.Name,
		Symbol:   asset.Symbol,
		Metadata: metadata,
	}, nil
}

func MarshalMetadata(metadata *pbasset.AssetMetadata) (json.RawMessage, error) {
	json, err := protojson.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("marshal metadata: %w", err)
	}
	return json, nil
}

func UnmarshalMetadata(metadata json.RawMessage) (*pbasset.AssetMetadata, error) {
	var md pbasset.AssetMetadata
	err := protojson.Unmarshal(metadata, &md)
	if err != nil {
		return nil, fmt.Errorf("unmarshal metadata: %w", err)
	}
	return &md, nil
}
