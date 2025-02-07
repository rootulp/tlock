## tlock: Timelock Encryption/Decryption Made Practical

tlock gives you time based encryption and decryption capabilities by relying on a [drand](https://drand.love/) threshold network.  
It's also a Go library, which is used to implement the `tle` command line tool enabling anybody to leverage timelock encryption.

Our timelock encryption system relies on an unchained drand network. Currently, the only publicly available one is the League of Entropy Testnet.
However, it should soon also be available on the League's Mainnet.

Working endpoints to access it are, for now:
- https://pl-us.testnet.drand.sh/
- https://testnet0-api.drand.cloudflare.com/

Notice this is relying on the League of Entropy **Testnet**, which should not be considered secure. 
A compatible League of Entropy Mainnet network is going to be launched in mid September, which can be considered secure.
In the meantime, we recommend only using the Testnet network for development and testing purposes.

You can also spin up a new drand network and run your own, but notice that the security guarantees boil down to the trust you can have in your network.

---

### See How It Works

<p align="center">
	<img src="https://user-images.githubusercontent.com/181501/177999855-cc1cfef7-ee1c-4193-bea7-4ee2e689f2d1.svg"/>
</p>

---

### Table Of Contents
 - [Install the CLI](#install-the-cli)
 - [Build it](#or-build-it)
 - [CLI usage](#cli-usage)
	- [Encryption](#cli-encryption)
	- [Decryption](#cli-decryption)
 - [Library usage](#library-usage)
 - [Applying another layer of encryption](#applying-another-layer-of-encryption)
 - [Security considerations](#security-considerations)

---

### Install or Build the CLI

This tool is pure Go, it works without CGO (`CGO_ENABLED=0`)

```bash
go install github.com/drand/tlock/cmd/tle@latest
```

```bash
git clone https://github.com/drand/tlock
go build cmd/tle/tle.go
```

---

### CLI Usage

```
Usage:
	tle [--encrypt] (-r round)... [--armor] [-o OUTPUT] [INPUT]
	tle --decrypt [-o OUTPUT] [INPUT]

Options:
	-e, --encrypt  Encrypt the input to the output. Default if omitted.
	-d, --decrypt  Decrypt the input to the output.
	-n, --network  The drand API endpoint to use.
	-c, --chain    The chain to use. Can use either beacon ID name or beacon hash. Use beacon hash in order to ensure public key integrity.
	-r, --round    The specific round to use to encrypt the message. Cannot be used with --duration.
	-D, --duration How long to wait before the message can be decrypted. Defaults to 120d (120 days).
	-o, --output   Write the result to the file at path OUTPUT.
	-a, --armor    Encrypt or Decrypt to a PEM encoded format.

If the OUTPUT exists, it will be overwritten.

NETWORK defaults to the Drand test network http://pl-us.testnet.drand.sh/.

CHAIN defaults to the "unchained" hash in the default test network:
7672797f548f3f4748ac4bf3352fc6c6b6468c9ad40ad456a397545c6e2df5bf

DURATION has a default value of 120d. When it is specified, it expects a number
followed by one of these units: "ns", "us" (or "µs"), "ms", "s", "m", "h", "d", "M", "y").

Example:
    $ tle -D 10d -o encrypted_file data_to_encrypt

After the specified duration:
    $ tle -d -o dencrypted_file.txt encrypted_file
```

#### Time Lock Encryption

Files can be encrypted using a duration (`--duration/-D`) in which the `encrypted_data` can be decrypted.

```bash
$ tle -n="http://pl-us.testnet.drand.sh/" -c="7672797f548f3f4748ac4bf3352fc6c6b6468c9ad40ad456a397545c6e2df5bf" -D=5s -o=encrypted_data data.txt
```

If a round (`--round/-R`) number is known, it can be used instead of the duration. The data can be decrypted only when that round becomes available in the network.

```bash
$ tle -n="http://pl-us.testnet.drand.sh/" -c="7672797f548f3f4748ac4bf3352fc6c6b6468c9ad40ad456a397545c6e2df5bf" -r=123456 -o=encrypted_data data.txt
```

It is also possible to encrypt the data to a PEM encoded format using the armor (`--armor/-a`) flag.
```bash
$ tle -a -n="http://pl-us.testnet.drand.sh/" -c="7672797f548f3f4748ac4bf3352fc6c6b6468c9ad40ad456a397545c6e2df5bf" -r=123456 -o=encrypted_data.PEM data.txt
```

#### Time Lock Decryption

For decryption, it's only necessary to specify the network.

```bash
$ tle -d -n="http://pl-us.testnet.drand.sh/" -o=decrypted_data encrypted_data
```

If decoding a PEM source.

```bash
$ tle -a -d -n="http://pl-us.testnet.drand.sh/" -o=decrypted_data encrypted_data
```

---

### Library Usage

These example show how to use the API to time lock encrypt and decrypt data.

#### Time Lock Encryption

```go
// Open an io.Reader to the data to be encrypted.
in, err := os.Open("data.txt")
if err != nil {
	log.Fatalf("open: %s", err)
	return
}
defer in.Close()

// Construct a network that can talk to a drand network.
// host:      "http://pl-us.testnet.drand.sh/"
// chainHash: "7672797f548f3f4748ac4bf3352fc6c6b6468c9ad40ad456a397545c6e2df5bf"
network := http.NewNetwork(host, chainHash)

// Specify how long we need to wait before the file can be decrypted.
duration := 10 * time.Second

// Use the network to identify the round number that represents the duration.
roundNumber, err := network.RoundNumber(time.Now().Add(duration))
if err != nil {
	log.Fatalf("round by duration: %s", err)
	return
}

// Write the encrypted file data to this buffer.
var cipherData bytes.Buffer

// Encrypt the data for the given round.
if err := tlock.New(network).Encrypt(&cipherData, in, roundNumber); err != nil {
	log.Fatalf("encrypt: %v", err)
	return
}
```

#### Time Lock Decryption

```go
// Open an io.Reader to the data to be decrypted.
in, err := os.Open("data.tle")
if err != nil {
	log.Fatalf("open: %v", err)
	return
}
defer in.Close()

// Construct a network that can talk to a drand network.
// host:      "http://pl-us.testnet.drand.sh/"
// chainHash: "7672797f548f3f4748ac4bf3352fc6c6b6468c9ad40ad456a397545c6e2df5bf"
network := http.NewNetwork(host, chainHash)

// Write the decrypted file data to this buffer.
var plainData bytes.Buffer

// Decrypt the data. If you try to decrypt the data *before* the specified
// duration, it will fail with the message: "too early to decrypt".
if err := tlock.New(network).Decrypt(&plainData, in); err != nil {
	log.Fatalf("decrypt: %v", err)
	return
}
```

---

### Applying another layer of encryption

The recommended way of doing "hybrid" encryption where you both encrypt your data using timelock encryption, but also with another encryption scheme, such as a public-key or a symmetric-key scheme is to simple re-encrypt your encrypted data using tlock.

For example, you can use the [age](https://github.com/FiloSottile/age) cli to encrypt your data with a passphrase as follows.

#### Encrypting Data With Passphrase
```bash
$ cat data.txt | age -p | tle -D 30s -o encrypted_data
```

#### Decrypting Data With Passphrase
```bash
$ cat encrypted_data | tle -d | age -d -o data.txt
```

Note that you could do the same with PGP or any other encryption tool.

--- 

### Security considerations

Currently, this is relying on the League of Entropy **Testnet**, which should not be considered secure. 
A compatible League of Entropy Mainnet network is going to be launched in mid September, which can be considered secure.

The security of our timelock encryption mechanism relies on four main things:
- The security of the underlying [Identity Encryption Scheme](https://crypto.stanford.edu/~dabo/pubs/papers/bfibe.pdf) (proposed in 2001) and [its implementation](https://github.com/drand/kyber/blob/a780ab21355ebe7f60b441a586d5e73a40c564eb/encrypt/ibe/ibe.go#L39-L47) that we're using.
- The security of the [threshold BLS scheme](https://link.springer.com/content/pdf/10.1007/s00145-004-0314-9.pdf) (proposed in 2003), and [its impementation](https://github.com/drand/kyber/blob/master/sign/tbls/tbls.go) by the network you're relying on.
- The security of [age](https://age-encryption.org/)'s underlying primitives, and that of the [age implementation](https://age-encryption.org/) we're using to encrypt the data, since we rely on the [hybrid encryption](https://en.wikipedia.org/wiki/Hybrid_cryptosystem) principle, where we only timelock encrypt ("wrap") a random symmetric key that is used by age to actually symmetrically encrypt the data using [Chacha20Poly1305](https://datatracker.ietf.org/doc/html/rfc8439)).  
- The security of the threshold network providing you with its BLS signatures **at a given frequency**, for instance the default for `tle` is to rely on drand and its existing League of Entropy network. 
 
In practice this means that if you trust there are never more than the threshold `t` malicious nodes on the network you're relying on, you are guaranteed that you timelocked data cannot be decrypted earlier than what you intended. 

Please note that neither BLS nor the IBE scheme we are relying on are "quantum resistant", therefore shall a Quantum Computer be built that's able to threaten their security, our current design wouldn't resist. There are also no quantum resistant scheme that we're aware of that could be used to replace our current design since post-quantum signatures schemes do not "thresholdize" too well in a post-quantum IBE-compatible way. 

However, such a quantum computer seems unlikely to be built within the next 5-10 years and therefore we currently consider that you can expect a "**long term security**" horizon of at least 5 years by relying on our design.

---

### License

This project is licensed using the [Permissive License Stack](https://protocol.ai/blog/announcing-the-permissive-license-stack/) which means that all contributions are available under the most permissive commonly-used licenses, and dependent projects can pick the license that best suits them.

Therefore, the project is dual-licensed under Apache 2.0 and MIT terms:

- Apache License, Version 2.0, ([LICENSE-APACHE](https://github.com/drand/drand/blob/master/LICENSE-APACHE) or http://www.apache.org/licenses/LICENSE-2.0)
- MIT license ([LICENSE-MIT](https://github.com/drand/drand/blob/master/LICENSE-MIT) or http://opensource.org/licenses/MIT)
89 
