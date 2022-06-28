// Package http implements the network interface for acessing a drand service
// using HTTP.
package http

import (
	"context"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/drand/drand/client"
	dhttp "github.com/drand/drand/client/http"
	"github.com/drand/kyber"
	bls "github.com/drand/kyber-bls12381"
	"github.com/drand/kyber/pairing"
	"github.com/drand/tlock/foundation/drnd"
)

// HTTP provides network and chain information.
type HTTP struct {
	host      string
	chainHash string
	client    client.Client
	publicKey kyber.Point
}

// New constructs an HTTP network for use.
func New(host string, chainHash string) *HTTP {
	return &HTTP{
		host:      host,
		chainHash: chainHash,
	}
}

// Host returns the host network information.
func (n *HTTP) Host() string {
	return n.host
}

// ChainHash returns the chain hash for this network.
func (n *HTTP) ChainHash() string {
	return n.chainHash
}

// PairingSuite returns the pairing suite to use.
func (*HTTP) PairingSuite() pairing.Suite {
	return bls.NewBLS12381Suite()
}

// Client returns an HTTP client used to talk to the network.
func (n *HTTP) Client(ctx context.Context) (client.Client, error) {
	if n.client != nil {
		return n.client, nil
	}

	hash, err := hex.DecodeString(n.chainHash)
	if err != nil {
		return nil, fmt.Errorf("decoding chain hash: %w", err)
	}

	client, err := dhttp.New(n.host, hash, transport())
	if err != nil {
		return nil, fmt.Errorf("creating client: %w", err)
	}

	n.client = client
	return client, nil
}

// PublicKey returns the kyber point needed for encryption and decryption.
func (n *HTTP) PublicKey(ctx context.Context) (kyber.Point, error) {
	if n.publicKey != nil {
		return n.publicKey, nil
	}

	if n.client == nil {
		n.Client(ctx)
	}

	chain, err := n.client.Info(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting client information: %w", err)
	}

	n.publicKey = chain.PublicKey
	return chain.PublicKey, nil

}

// RoundByNumber returns the round id and signature for the specified round number.
// If the it does not exist, we generate the signature.
func (n *HTTP) RoundByNumber(ctx context.Context, roundNumber uint64) (uint64, []byte, error) {
	client, err := n.Client(ctx)
	if err != nil {
		return 0, nil, fmt.Errorf("client: %w", err)
	}

	result, err := client.Get(ctx, roundNumber)
	if err != nil {

		// If the number does not exist, we still need have to generate the signature.
		if strings.Contains(err.Error(), "EOF") {
			signature, err := drnd.CalculateRoundByNumber(roundNumber)
			if err != nil {
				return 0, nil, fmt.Errorf("round by number: %w", err)
			}
			return roundNumber, signature, nil
		}

		return 0, nil, fmt.Errorf("client get round: %w", err)
	}

	return result.Round(), result.Signature(), nil
}

// RoundByDuration returns the round id and signature for the specified duration.
func (n *HTTP) RoundByDuration(ctx context.Context, duration time.Duration) (uint64, []byte, error) {
	roundID, roundSignature, err := drnd.CalculateRoundByDuration(ctx, duration, n)
	if err != nil {
		return 0, nil, fmt.Errorf("calculate future round: %w", err)
	}

	return roundID, roundSignature, nil
}

// =============================================================================

// transport sets reasonable defaults for the connection.
func transport() *http.Transport {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 5 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          2,
		IdleConnTimeout:       5 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
}
