package storage

// NewLocalStore returns a reduced-mode Store that keeps its config in the file
// at path and signs it with secret, with no registry backup and no secure
// copy. It exists for tests in other packages (for example bridge), which
// must never call NewStore because it touches the real registry.
func NewLocalStore(path string, secret []byte) *Store {
	return &Store{
		filePath:     path,
		activeSecret: secret,
		canSign:      true,
	}
}
