package drnd

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/client"
	dhttp "github.com/drand/drand/client/http"
	bls "github.com/drand/kyber-bls12381"
	"github.com/drand/kyber/encrypt/ibe"
	"github.com/drand/kyber/pairing"
)

/*
	encrypt
	1) generate random key named "Data encryption key", DEK
	2) encrypt the data using random key, get ciphertext
	3) encrypt the DEK using IBE and append it to our ciphertext.

	decryption is done by:
	1) decrypt the DEK using IBE and drand round
	2) use the decrypted DEK to decrypt the rest of the ciphertext

	// Random Key generation
	https://github.com/FiloSottile/age/blob/c50f1ae2e1778edd5d1f780a3dcf3892c7d845db/age.go#L125

	// Encryption Chacha20Poly1305
	https://github.com/FiloSottile/age/blob/main/primitives.go
*/

// EncryptWithRound will encrypt the message to be decrypted in the future based
// on the specified round.
func EncryptWithRound(ctx context.Context, dst io.Writer, dataToEncrypt io.Reader, network string, chainHash string, round uint64) error {
	ni, err := retrieveNetworkInfo(ctx, network, chainHash)
	if err != nil {
		return fmt.Errorf("network info: %w", err)
	}

	roundData, err := ni.client.Get(ctx, round)
	if err != nil {
		return fmt.Errorf("client get round: %w", err)
	}

	return encrypt(dst, dataToEncrypt, ni, chainHash, roundData.Round(), roundData.Signature())
}

// EncryptWithDuration will encrypt the message to be decrypted in the future based
// on the specified duration.
func EncryptWithDuration(ctx context.Context, dst io.Writer, dataToEncrypt io.Reader, network string, chainHash string, duration time.Duration) error {
	ni, err := retrieveNetworkInfo(ctx, network, chainHash)
	if err != nil {
		return fmt.Errorf("network info: %w", err)
	}

	roundIDHash, roundID, err := calculateRound(duration, ni)
	if err != nil {
		return fmt.Errorf("calculate future round: %w", err)
	}

	return encrypt(dst, dataToEncrypt, ni, chainHash, roundID, roundIDHash)
}

// Decrypt reads the encrypted output from the Encrypt function and decrypts
// the message if the time allows it.
func Decrypt(ctx context.Context, network string, dataToDecrypt io.Reader) ([]byte, error) {
	di, err := decode(dataToDecrypt)
	if err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}

	ni, err := retrieveNetworkInfo(ctx, network, di.chainHash)
	if err != nil {
		return nil, fmt.Errorf("network info: %w", err)
	}

	suite, err := retrievePairingSuite()
	if err != nil {
		return nil, fmt.Errorf("pairing suite: %w", err)
	}

	// Get returns the randomness at `round` or an error. If it does not exist
	// yet, it will return an EOF error (HTTP 404).
	clientResult, err := ni.client.Get(ctx, di.roundID)
	if err != nil {
		return nil, fmt.Errorf("client get round: %w", err)
	}

	// If we can get the data from the future round above, we need to create
	// another kyber point but this time using Group2.
	var g2 bls.KyberG2
	if err := g2.UnmarshalBinary(clientResult.Signature()); err != nil {
		return nil, fmt.Errorf("unmarshal kyber G2: %w", err)
	}

	var g1 bls.KyberG1
	if err := g1.UnmarshalBinary(di.kyberPoint); err != nil {
		return nil, fmt.Errorf("unmarshal kyber G1: %w", err)
	}

	newCipherText := ibe.Ciphertext{
		U: &g1,
		V: di.cipherV,
		W: di.cipherW,
	}

	decryptedData, err := ibe.Decrypt(suite, ni.chain.PublicKey, &g2, &newCipherText)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}

	return decryptedData, nil
}

// =============================================================================

// networkInfo provides network and chain information.
type networkInfo struct {
	client client.Client
	chain  *chain.Info
}

// retrieveNetworkInfo accesses the specified network for the specified chain
// hash to extract information.
func retrieveNetworkInfo(ctx context.Context, network string, chainHash string) (networkInfo, error) {
	hash, err := hex.DecodeString(chainHash)
	if err != nil {
		return networkInfo{}, fmt.Errorf("decoding chain hash: %w", err)
	}

	client, err := dhttp.New(network, hash, transport())
	if err != nil {
		return networkInfo{}, fmt.Errorf("creating client: %w", err)
	}

	chain, err := client.Info(ctx)
	if err != nil {
		return networkInfo{}, fmt.Errorf("getting client information: %w", err)
	}

	ni := networkInfo{
		client: client,
		chain:  chain,
	}

	return ni, nil
}

// retrievePairingSuite returns the pairing suite to use.
func retrievePairingSuite() (pairing.Suite, error) {
	return bls.NewBLS12381Suite(), nil
}

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

// calculateRound will generate the round information based on the specified duration.
func calculateRound(duration time.Duration, ni networkInfo) (roundIDHash []byte, roundID uint64, err error) {

	// We need to get the future round number based on the duration. The following
	// call will do the required calculations based on the network `period` property
	// and return a uint64 representing the round number in the future. This round
	// number is used to encrypt the data and will also be used by the decrypt function.
	roundID = ni.client.RoundAt(time.Now().Add(duration))

	h := sha256.New()
	if _, err := h.Write(chain.RoundToBytes(roundID)); err != nil {
		return nil, 0, fmt.Errorf("sha256 write: %w", err)
	}

	return h.Sum(nil), roundID, nil
}

// encode the meta data and encrypted data to the destination.
func encode(dst io.Writer, cipher *ibe.Ciphertext, roundID uint64, chainHash string) error {
	kyberPoint, err := cipher.U.MarshalBinary()
	if err != nil {
		return fmt.Errorf("marshal binary: %w", err)
	}

	rn := strconv.Itoa(int(roundID))
	ch := chainHash

	ww := bufio.NewWriter(dst)
	defer ww.Flush()

	ww.WriteString(rn + "\n")
	ww.WriteString(ch + "\n")
	ww.WriteString("--- HASH\n")

	ww.WriteString(fmt.Sprintf("%010d", len(kyberPoint)))
	ww.Write(kyberPoint)

	ww.WriteString(fmt.Sprintf("%010d", len(cipher.V)))
	ww.Write(cipher.V)

	ww.WriteString(fmt.Sprintf("%010d", len(cipher.W)))
	ww.Write(cipher.W)

	return nil
}

// decodeInfo represents the different parts of any encrypted data.
type decodeInfo struct {
	roundID    uint64
	chainHash  string
	kyberPoint []byte
	cipherV    []byte
	cipherW    []byte
}

// decode the encrypted data into its different parts.
func decode(src io.Reader) (decodeInfo, error) {
	rr := bufio.NewReader(src)

	roundIDStr, err := rr.ReadString('\n')
	if err != nil {
		return decodeInfo{}, fmt.Errorf("failed to read roundID: %w", err)
	}
	roundIDStr = roundIDStr[:len(roundIDStr)-1]

	roundID, err := strconv.Atoi(roundIDStr)
	if err != nil {
		return decodeInfo{}, fmt.Errorf("failed to convert round: %w", err)
	}

	chainHash, err := rr.ReadString('\n')
	if err != nil {
		return decodeInfo{}, fmt.Errorf("failed to read chain hash: %w", err)
	}
	chainHash = chainHash[:len(chainHash)-1]

	hdrHash, err := rr.ReadString('\n')
	if err != nil {
		return decodeInfo{}, fmt.Errorf("failed to read header hash: %w", err)
	}
	hdrHash = hdrHash[:len(hdrHash)-1]

	kpLenStr := make([]byte, 10)
	if _, err := rr.Read(kpLenStr); err != nil {
		return decodeInfo{}, fmt.Errorf("failed to read kp length: %w", err)
	}

	kpLen, err := strconv.Atoi(string(kpLenStr))
	if err != nil {
		return decodeInfo{}, fmt.Errorf("failed to decode kp length: %w", err)
	}

	kyberPoint := make([]byte, kpLen)
	if _, err := rr.Read(kyberPoint); err != nil {
		return decodeInfo{}, fmt.Errorf("failed to read kyberPoint: %w", err)
	}

	vLenStr := make([]byte, 10)
	if _, err := rr.Read(vLenStr); err != nil {
		return decodeInfo{}, fmt.Errorf("failed to read v length: %w", err)
	}

	vLen, err := strconv.Atoi(string(vLenStr))
	if err != nil {
		return decodeInfo{}, fmt.Errorf("failed to decode v length: %w", err)
	}

	cipherV := make([]byte, vLen)
	if _, err := rr.Read(cipherV); err != nil {
		return decodeInfo{}, fmt.Errorf("failed to read cipherV: %w", err)
	}

	wLenStr := make([]byte, 10)
	if _, err := rr.Read(wLenStr); err != nil {
		return decodeInfo{}, fmt.Errorf("failed to read w length: %w", err)
	}

	wLen, err := strconv.Atoi(string(wLenStr))
	if err != nil {
		return decodeInfo{}, fmt.Errorf("failed to decode w length: %w", err)
	}

	cipherW := make([]byte, wLen)
	if _, err := rr.Read(cipherW); err != nil {
		return decodeInfo{}, fmt.Errorf("failed to read cipherW: %w", err)
	}

	fmt.Println("round:       ", roundIDStr)
	fmt.Println("chain hash:  ", chainHash)
	fmt.Println("Header hash: ", hdrHash)
	fmt.Println("kp len:      ", kpLen)
	fmt.Println("kp:          ", kyberPoint)
	fmt.Println("v len:       ", vLen)
	fmt.Println("v:           ", cipherV)
	fmt.Println("w len:       ", wLen)
	fmt.Println("w:           ", cipherW)

	di := decodeInfo{
		roundID:    uint64(roundID),
		chainHash:  chainHash,
		kyberPoint: kyberPoint,
		cipherV:    cipherV,
		cipherW:    cipherW,
	}

	return di, nil
}

// encrypt provides base functionality for all encryption operations.
func encrypt(dst io.Writer, dataToEncrypt io.Reader, ni networkInfo, chainHash string, round uint64, roundSignature []byte) error {
	suite, err := retrievePairingSuite()
	if err != nil {
		return fmt.Errorf("pairing suite: %w", err)
	}

	inputData, err := io.ReadAll(dataToEncrypt)
	if err != nil {
		return fmt.Errorf("reading input data: %w", err)
	}

	cipher, err := ibe.Encrypt(suite, ni.chain.PublicKey, roundSignature, inputData)
	if err != nil {
		return fmt.Errorf("encrypt: %w", err)
	}

	if err := encode(dst, cipher, round, chainHash); err != nil {
		return fmt.Errorf("encode: %w", err)
	}

	return nil
}
