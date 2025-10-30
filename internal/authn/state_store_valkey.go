package authn

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/goccy/go-json"

	valkey "github.com/valkey-io/valkey-go"
)

type ValkeyKV interface {
	SetEX(ctx context.Context, key string, value []byte, ttl time.Duration) error
	GetDel(ctx context.Context, key string) ([]byte, error)
}

type ValkeyInitStateStore struct {
	kv ValkeyKV
}

func NewValkeyInitStateStore(kv ValkeyKV) *ValkeyInitStateStore {
	return &ValkeyInitStateStore{kv: kv}
}

func (s *ValkeyInitStateStore) Save(ctx context.Context, key string, state OAuthInitState, ttl time.Duration) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return s.kv.SetEX(ctx, key, data, ttl)
}

func (s *ValkeyInitStateStore) Pop(ctx context.Context, key string) (OAuthInitState, error) {
	data, err := s.kv.GetDel(ctx, key)
	if errors.Is(err, valkey.Nil) {
		return OAuthInitState{}, ErrStateNotFound
	} else if err != nil {
		return OAuthInitState{}, err
	}
	var st OAuthInitState
	if err = json.Unmarshal(data, &st); err != nil {
		return OAuthInitState{}, fmt.Errorf("unmarshal init state: %w", err)
	}
	return st, nil
}

// NewValkeyKVAdapter returns a ValkeyKV backed by a valkey-go client.
func NewValkeyKVAdapter(c valkey.Client) ValkeyKV { return valkeyKVAdapter{c: c} }

type valkeyKVAdapter struct {
	c valkey.Client
}

func (a valkeyKVAdapter) SetEX(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return a.c.Do(ctx, a.c.B().Set().Key(key).Value(string(value)).Ex(ttl).Build()).Error()
}

func (a valkeyKVAdapter) GetDel(ctx context.Context, key string) ([]byte, error) {
	return a.c.Do(ctx, a.c.B().Getdel().Key(key).Build()).AsBytes()
}
