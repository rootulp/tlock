package tlock_test

import (
	"bytes"
	_ "embed" // Calls init function.
	"errors"
	"os"
	"testing"
	"time"

	"github.com/drand/drand/chain"
	"github.com/drand/tlock"
	"github.com/drand/tlock/networks/http"
)

var (
	//go:embed test_artifacts/data.txt
	dataFile []byte
)

const (
	testnetHost      = "http://pl-us.testnet.drand.sh/"
	testnetChainHash = "7672797f548f3f4748ac4bf3352fc6c6b6468c9ad40ad456a397545c6e2df5bf"
)

func Test_EarlyDecryptionWithDuration(t *testing.T) {
	network, err := http.NewNetwork(testnetHost, testnetChainHash)
	if err != nil {
		t.Fatalf("network error %s", err)
	}

	// =========================================================================
	// Encrypt

	// Read the plaintext data to be encrypted.
	in, err := os.Open("test_artifacts/data.txt")
	if err != nil {
		t.Fatalf("reader error %s", err)
	}
	defer in.Close()

	// Write the encoded information to this buffer.
	var cipherData bytes.Buffer

	// Enough duration to check for an non-existing beacon.
	duration := 10 * time.Second

	roundNumber := network.RoundNumber(time.Now().Add(duration))
	if err := tlock.New(network).Encrypt(&cipherData, in, roundNumber); err != nil {
		t.Fatalf("encrypt with duration error %s", err)
	}

	// =========================================================================
	// Decrypt

	// Write the decoded information to this buffer.
	var plainData bytes.Buffer

	// We DO NOT wait for the future beacon to exist.
	err = tlock.New(network).Decrypt(&plainData, &cipherData)
	if err == nil {
		t.Fatal("expecting decrypt error")
	}

	if !errors.Is(err, tlock.ErrTooEarly) {
		t.Fatalf("expecting decrypt error to contain '%s'; got %s", tlock.ErrTooEarly, err)
	}
}

func Test_EarlyDecryptionWithRound(t *testing.T) {
	network, err := http.NewNetwork(testnetHost, testnetChainHash)
	if err != nil {
		t.Fatalf("network error %s", err)
	}
	// =========================================================================
	// Encrypt

	// Read the plaintext data to be encrypted.
	in, err := os.Open("test_artifacts/data.txt")
	if err != nil {
		t.Fatalf("reader error %s", err)
	}
	defer in.Close()

	var cipherData bytes.Buffer
	futureRound := network.RoundNumber(time.Now().Add(1 * time.Minute))

	if err := tlock.New(network).Encrypt(&cipherData, in, futureRound); err != nil {
		t.Fatalf("encrypt with round error %s", err)
	}

	// =========================================================================
	// Decrypt

	// Write the decoded information to this buffer.
	var plainData bytes.Buffer

	// We DO NOT wait for the future beacon to exist.
	err = tlock.New(network).Decrypt(&plainData, &cipherData)
	if err == nil {
		t.Fatal("expecting decrypt error")
	}

	if !errors.Is(err, tlock.ErrTooEarly) {
		t.Fatalf("expecting decrypt error to contain '%s'; got %s", tlock.ErrTooEarly, err)
	}
}

func Test_EncryptionWithDuration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping testing in short mode")
	}

	network, err := http.NewNetwork(testnetHost, testnetChainHash)
	if err != nil {
		t.Fatalf("network error %s", err)
	}

	// =========================================================================
	// Encrypt

	// Read the plaintext data to be encrypted.
	in, err := os.Open("test_artifacts/data.txt")
	if err != nil {
		t.Fatalf("reader error %s", err)
	}
	defer in.Close()

	// Write the encoded information to this buffer.
	var cipherData bytes.Buffer

	// Enough duration to check for an non-existing beacon.
	duration := 4 * time.Second

	roundNumber := network.RoundNumber(time.Now().Add(duration))
	if err := tlock.New(network).Encrypt(&cipherData, in, roundNumber); err != nil {
		t.Fatalf("encrypt with duration error %s", err)
	}

	// =========================================================================
	// Decrypt

	time.Sleep(5 * time.Second)

	// Write the decoded information to this buffer.
	var plainData bytes.Buffer

	if err := tlock.New(network).Decrypt(&plainData, &cipherData); err != nil {
		t.Fatalf("unexpected error %s", err)
	}

	if !bytes.Equal(plainData.Bytes(), dataFile) {
		t.Fatalf("decrypted file is invalid; expected %d; got %d", len(dataFile), len(plainData.Bytes()))
	}
}

func Test_EncryptionWithRound(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping testing in short mode")
	}

	network, err := http.NewNetwork(testnetHost, testnetChainHash)
	if err != nil {
		t.Fatalf("network error %s", err)
	}

	// =========================================================================
	// Encrypt

	// Read the plaintext data to be encrypted.
	in, err := os.Open("test_artifacts/data.txt")
	if err != nil {
		t.Fatalf("reader error %s", err)
	}
	defer in.Close()

	// Write the encoded information to this buffer.
	var cipherData bytes.Buffer

	futureRound := network.RoundNumber(time.Now().Add(6 * time.Second))
	if err := tlock.New(network).Encrypt(&cipherData, in, futureRound); err != nil {
		t.Fatalf("encrypt with duration error %s", err)
	}

	// =========================================================================
	// Decrypt

	var plainData bytes.Buffer

	// Wait for the future beacon to exist.
	time.Sleep(10 * time.Second)

	if err := tlock.New(network).Decrypt(&plainData, &cipherData); err != nil {
		t.Fatalf("unexpected error %s", err)
	}

	if !bytes.Equal(plainData.Bytes(), dataFile) {
		t.Fatalf("decrypted file is invalid; expected %d; got %d", len(dataFile), len(plainData.Bytes()))
	}
}

func Test_TimeLockUnlock(t *testing.T) {
	network, err := http.NewNetwork(testnetHost, testnetChainHash)
	if err != nil {
		t.Fatalf("network error %s", err)
	}

	futureRound := network.RoundNumber(time.Now())

	id, err := network.Signature(futureRound)
	if err != nil {
		t.Fatalf("ready to decrypt error %s", err)
	}

	data := []byte(`anything`)

	cipherText, err := tlock.TimeLock(network.PublicKey(), futureRound, data)
	if err != nil {
		t.Fatalf("timelock error %s", err)
	}

	beacon := chain.Beacon{
		Round:     futureRound,
		Signature: id,
	}

	b, err := tlock.TimeUnlock(network.PublicKey(), beacon, cipherText)
	if err != nil {
		t.Fatalf("timeunlock error %s", err)
	}

	if !bytes.Equal(data, b) {
		t.Fatalf("unexpected bytes; expected len %d; got %d", len(data), len(b))
	}
}
