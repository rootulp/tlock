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
	host      string
	chainHash string
	c         client.Client
	publicKey kyber.Point
}

// NewNetwork constructs a network for use that will use the http client.
func NewNetwork(host string, chainHash string) (*Network, error) {
	network := Network{
		host:      host,
		chainHash: chainHash,
	}

	// Validate if the chainHash belongs to an unchained network.
	client, err := network.client()
	if err != nil {
		return nil, err
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

	network.c = client

	return &network, nil
}

// Host returns the host network information.
func (n *Network) Host() string {
	return n.host
}

// ChainHash returns the chain hash for this network.
func (n *Network) ChainHash() string {
	return n.chainHash
}

// PublicKey returns the kyber point needed for encryption and decryption.
func (n *Network) PublicKey() (kyber.Point, error) {
	if n.publicKey != nil {
		return n.publicKey, nil
	}

	client, err := n.client()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	chain, err := client.Info(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting client information: %w", err)
	}

	n.publicKey = chain.PublicKey
	return chain.PublicKey, nil

}

// IsReadyToDecrypt makes a call to the network to validate it's time to decrypt
// and if so, the required id is returned.
func (n *Network) IsReadyToDecrypt(roundNumber uint64) ([]byte, bool) {
	client, err := n.client()
	if err != nil {
		return nil, false
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	result, err := client.Get(ctx, roundNumber)
	if err != nil {
		return nil, false
	}

	return result.Signature(), true
}

// RoundNumber will return the latest round of randomness that is available
// for the specified time. To handle a duration construct time like this:
// time.Now().Add(6*time.Second)
func (n *Network) RoundNumber(t time.Time) (uint64, error) {
	client, err := n.client()
	if err != nil {
		return 0, fmt.Errorf("client: %w", err)
	}

	roundNumber := client.RoundAt(t)
	return roundNumber, nil
}

// =============================================================================

// client returns an HTTP client used to talk to the network.
func (n *Network) client() (client.Client, error) {
	if n.c != nil {
		return n.c, nil
	}

	hash, err := hex.DecodeString(n.chainHash)
	if err != nil {
		return nil, fmt.Errorf("decoding chain hash: %w", err)
	}

	client, err := dhttp.New(n.host, hash, transport())
	if err != nil {
		return nil, fmt.Errorf("creating client: %w", err)
	}

	n.c = client
	return client, nil
}

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
