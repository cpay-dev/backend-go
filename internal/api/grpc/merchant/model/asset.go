package model

import (
	"encoding/json"
	"fmt"

	pgblockchain "github.com/cpay-dev/backend-go/internal/api/repo/pg/blockchain"
	pbapiblockchain "github.com/cpay-dev/proto-go/api/v1/blockchain"
	"google.golang.org/protobuf/encoding/protojson"
)

func AssetToProto(asset pgblockchain.Asset) (*pbapiblockchain.Asset, error) {
	chain, err := ChainToProto(asset.ChainID)
	if err != nil {
		return nil, err
	}
	metadata, err := UnmarshalMetadata(asset.Metadata)
	if err != nil {
		return nil, err
	}
	return &pbapiblockchain.Asset{
		Id:       asset.ID,
		Chain:    chain,
		Name:     asset.Name,
		Symbol:   asset.Symbol,
		Metadata: metadata,
	}, nil
}

func MarshalMetadata(metadata *pbapiblockchain.AssetMetadata) (json.RawMessage, error) {
	json, err := protojson.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("marshal metadata: %w", err)
	}
	return json, nil
}

func UnmarshalMetadata(metadata json.RawMessage) (*pbapiblockchain.AssetMetadata, error) {
	var md pbapiblockchain.AssetMetadata
	err := protojson.Unmarshal(metadata, &md)
	if err != nil {
		return nil, fmt.Errorf("unmarshal metadata: %w", err)
	}
	return &md, nil
}
