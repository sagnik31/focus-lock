package updater

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
)

// SignatureAsset is the detached Ed25519 signature of ChecksumsAsset that
// every release since the silent updater (ADR 0009) carries: the base64 of
// the 64-byte signature over the exact bytes of SHA256SUMS.txt.
const SignatureAsset = ChecksumsAsset + ".sig"

// releasePublicKeyB64 is the release signing key's public half. The private
// seed is the RELEASE_SIGNING_KEY secret of the GitHub repository; to rotate,
// generate a pair with `go run ./cmd/signsums -genkey -out <file>`, put the
// new public key here and the seed in the secret, in the same release.
const releasePublicKeyB64 = "5YXB3VaSCuN1K1NAeDMYymm41bdVbxbDXMHefdziZQ0="

// ErrBadSignature means SHA256SUMS.txt is not signed by the release key.
var ErrBadSignature = errors.New("the release's checksums are not signed by the Focus Lock release key, so the update was not used")

// ReleasePublicKey returns the public key built into this app.
func ReleasePublicKey() ed25519.PublicKey {
	b, err := base64.StdEncoding.DecodeString(releasePublicKeyB64)
	if err != nil || len(b) != ed25519.PublicKeySize {
		return nil
	}
	return ed25519.PublicKey(b)
}

// VerifySignature checks that sigB64 (the contents of SHA256SUMS.txt.sig) is
// a valid signature of data under pub. A missing or malformed key, or a
// malformed signature, is a failure.
func VerifySignature(data, sigB64 []byte, pub ed25519.PublicKey) error {
	if len(pub) != ed25519.PublicKeySize {
		return ErrBadSignature
	}
	sig, err := base64.StdEncoding.DecodeString(string(bytes.TrimSpace(sigB64)))
	if err != nil || len(sig) != ed25519.SignatureSize {
		return ErrBadSignature
	}
	if !ed25519.Verify(pub, data, sig) {
		return ErrBadSignature
	}
	return nil
}
