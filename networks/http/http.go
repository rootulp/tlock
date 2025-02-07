// Package http implements the Network interface for the tlock package.
package http

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/drand/drand/client"
	dhttp "github.com/drand/drand/client/http"
	"github.com/drand/drand/common/scheme"
	"github.com/drand/kyber"
)

// timeout represents the maximum amount of time to wait for network operations.
const timeout = 5 * time.Second

// ErrNotUnchained represents an error when the informed chain belongs to a
// chained network.
var ErrNotUnchained = errors.New("hash does not belong to an unchained network")

// =============================================================================

// Network represents the network support using the drand http client.
type Network struct {
	chainHash string
	client    client.Client
	publicKey kyber.Point
}

// NewNetwork constructs a network for use that will use the http client.
func NewNetwork(host string, chainHash string) (*Network, error) {
	hash, err := hex.DecodeString(chainHash)
	if err != nil {
		return nil, fmt.Errorf("decoding chain hash: %w", err)
	}

	client, err := dhttp.New(host, hash, transport())
	if err != nil {
		return nil, fmt.Errorf("creating client: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	info, err := client.Info(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting client information: %w", err)
	}

	if info.Scheme.ID != scheme.UnchainedSchemeID {
		return nil, ErrNotUnchained
	}

	network := Network{
		chainHash: chainHash,
		client:    client,
		publicKey: info.PublicKey,
	}

	return &network, nil
}

// ChainHash returns the chain hash for this network.
func (n *Network) ChainHash() string {
	return n.chainHash
}

// PublicKey returns the kyber point needed for encryption and decryption.
func (n *Network) PublicKey() kyber.Point {
	return n.publicKey
}

// Signature makes a call to the network to retrieve the signature for the
// specified round number.
func (n *Network) Signature(roundNumber uint64) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	result, err := n.client.Get(ctx, roundNumber)
	if err != nil {
		return nil, err
	}

	return result.Signature(), nil
}

// RoundNumber will return the latest round of randomness that is available
// for the specified time. To handle a duration construct time like this:
// time.Now().Add(6*time.Second)
func (n *Network) RoundNumber(t time.Time) uint64 {
	return n.client.RoundAt(t)
}

// =============================================================================

// transport sets reasonable defaults for the connection.
func transport() *http.Transport {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   timeout,
			KeepAlive: 5 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          2,
		IdleConnTimeout:       5 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
}
