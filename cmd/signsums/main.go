// Command signsums signs a release's SHA256SUMS.txt with the Focus Lock
// release key (Ed25519), so the app can tell a genuine release from one
// published by someone else (docs/adr/0009-silent-updates-via-updater-task.md).
//
// Sign (release workflow; the key comes from the RELEASE_SIGNING_KEY secret):
//
//	RELEASE_SIGNING_KEY=<base64 seed> go run ./cmd/signsums -sign installer/Output/SHA256SUMS.txt
//
// writes SHA256SUMS.txt.sig (base64 of the 64-byte signature) next to it.
//
// Generate a new key pair (once, or to rotate the key):
//
//	go run ./cmd/signsums -genkey -out <file outside the repo>
//
// writes the private seed (base64) to -out and prints only the public key,
// which goes into backend/updater/signature.go (releasePublicKeyB64).
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"focus-lock/backend/updater"
)

func main() {
	genkey := flag.Bool("genkey", false, "generate a new key pair")
	out := flag.String("out", "", "with -genkey: file to write the private seed to")
	sign := flag.String("sign", "", "file to sign (writes <file>.sig)")
	flag.Parse()

	var err error
	switch {
	case *genkey:
		err = genKey(*out)
	case *sign != "":
		err = signFile(*sign)
	default:
		flag.Usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "signsums:", err)
		os.Exit(1)
	}
}

func genKey(out string) error {
	if out == "" {
		return errors.New("-genkey needs -out <file>")
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	seed := base64.StdEncoding.EncodeToString(priv.Seed())
	if err := os.WriteFile(out, []byte(seed+"\n"), 0o600); err != nil {
		return err
	}
	fmt.Println("Private seed written to", out)
	fmt.Println("Public key:", base64.StdEncoding.EncodeToString(pub))
	return nil
}

func signFile(path string) error {
	raw := strings.TrimSpace(os.Getenv("RELEASE_SIGNING_KEY"))
	if raw == "" {
		return errors.New("RELEASE_SIGNING_KEY is not set; refusing to publish an unsigned release")
	}
	seed, err := base64.StdEncoding.DecodeString(raw)
	if err != nil || len(seed) != ed25519.SeedSize {
		return errors.New("RELEASE_SIGNING_KEY must be the base64 of a 32-byte Ed25519 seed")
	}
	priv := ed25519.NewKeyFromSeed(seed)
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	sig := ed25519.Sign(priv, data)

	// Refuse to publish a signature the app would reject (wrong key in the
	// secret, or the embedded public key not updated after a rotation).
	if err := updater.VerifySignature(data, []byte(base64.StdEncoding.EncodeToString(sig)), updater.ReleasePublicKey()); err != nil {
		return fmt.Errorf("the signing key does not match the public key built into the app: %w", err)
	}
	if err := os.WriteFile(path+".sig", []byte(base64.StdEncoding.EncodeToString(sig)+"\n"), 0o644); err != nil {
		return err
	}
	fmt.Println("Signed", path)
	return nil
}
