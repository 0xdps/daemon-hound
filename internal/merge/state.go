package merge

import (
	"bytes"
	"fmt"

	"github.com/0xdps/daemon-hound/internal/models"
	"github.com/BurntSushi/toml"
)

// StateDriver merges decrypted state.toml content at the structural level.
// Machine A adding a file + Machine B adding a different file → auto-merges.
// Both machines changing the SAME file → true conflict.
//
// When Encrypt/Decrypt are provided (injected via WithSecretDriver), the driver
// handles encrypted state.toml.age bytes from the git driver path: it decrypts
// all three versions before merging and re-encrypts the result.
//
// When Encrypt/Decrypt are nil (used in the in-process sync fallback path),
// the driver expects plaintext TOML bytes (already decrypted by the caller).
type StateDriver struct {
	Encrypt func(plaintext []byte) ([]byte, error)
	Decrypt func(ciphertext []byte) ([]byte, error)
}

func (d *StateDriver) CanHandle(filename string) bool {
	return filename == "state.toml.age" || filename == "state.toml"
}

func (d *StateDriver) Merge(base, local, remote []byte) ([]byte, Result, error) {
	// If decrypt/encrypt are available, unwrap the encryption layer first.
	isEncrypted := d.Decrypt != nil && d.Encrypt != nil
	if isEncrypted {
		var err error
		if len(base) > 0 {
			base, err = d.Decrypt(base)
			if err != nil {
				return nil, HasConflict, fmt.Errorf("decrypt base state: %w", err)
			}
		}
		local, err = d.Decrypt(local)
		if err != nil {
			return nil, HasConflict, fmt.Errorf("decrypt local state: %w", err)
		}
		remote, err = d.Decrypt(remote)
		if err != nil {
			return nil, HasConflict, fmt.Errorf("decrypt remote state: %w", err)
		}
	}

	var baseS, localS, remoteS models.VaultState
	if err := decodeVaultState(base, &baseS); err != nil {
		return nil, HasConflict, fmt.Errorf("decode base state: %w", err)
	}
	if err := decodeVaultState(local, &localS); err != nil {
		return nil, HasConflict, fmt.Errorf("decode local state: %w", err)
	}
	if err := decodeVaultState(remote, &remoteS); err != nil {
		return nil, HasConflict, fmt.Errorf("decode remote state: %w", err)
	}

	merged, ok := mergeVaultState(&baseS, &localS, &remoteS)
	if !ok {
		return nil, HasConflict, nil
	}

	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(merged); err != nil {
		return nil, HasConflict, fmt.Errorf("encode merged state: %w", err)
	}
	plainResult := buf.Bytes()

	if isEncrypted {
		enc, err := d.Encrypt(plainResult)
		if err != nil {
			return nil, HasConflict, fmt.Errorf("re-encrypt merged state: %w", err)
		}
		return enc, Merged, nil
	}
	return plainResult, Merged, nil
}

func decodeVaultState(data []byte, s *models.VaultState) error {
	s.Files = make(map[string]models.TrackedFile)
	s.Secrets = make(map[string]models.SecretIndex)
	_, err := toml.Decode(string(data), s)
	return err
}

func mergeVaultState(base, local, remote *models.VaultState) (*models.VaultState, bool) {
	out := &models.VaultState{
		Version: local.Version,
		Files:   make(map[string]models.TrackedFile),
		Secrets: make(map[string]models.SecretIndex),
	}

	// Merge Files map keyed by "namespace:relPath"
	for key := range unionStringSet(keysOfFiles(base.Files), keysOfFiles(local.Files), keysOfFiles(remote.Files)) {
		bf, hasBase := base.Files[key]
		lf, hasLocal := local.Files[key]
		rf, hasRemote := remote.Files[key]

		localChanged := hasLocal && (!hasBase || bf.Checksum != lf.Checksum)
		remoteChanged := hasRemote && (!hasBase || bf.Checksum != rf.Checksum)

		switch {
		case !hasLocal && !hasRemote:
			// Deleted by both — omit
		case hasBase && !hasLocal && remoteChanged:
			return nil, false // local deleted, remote modified — conflict
		case hasBase && !hasRemote && localChanged:
			return nil, false // remote deleted, local modified — conflict
		case !hasLocal:
			// local-only delete (remote unchanged) — apply delete
		case !hasRemote:
			// remote-only delete (local unchanged) — apply delete
		case !hasBase:
			if lf.Checksum == rf.Checksum {
				out.Files[key] = lf // both added same content
			} else {
				return nil, false // conflict: both added different content
			}
		case localChanged && remoteChanged:
			if lf.Checksum == rf.Checksum {
				out.Files[key] = lf // same change on both sides
			} else {
				return nil, false // diverged — conflict
			}
		case localChanged:
			out.Files[key] = lf
		case remoteChanged:
			out.Files[key] = rf
		default:
			out.Files[key] = lf // unchanged on both
		}
	}

	// Merge Secrets index map keyed by secret name.
	// Use UpdatedAt as proxy for "changed".
	for key := range unionStringSet(keysOfSecrets(base.Secrets), keysOfSecrets(local.Secrets), keysOfSecrets(remote.Secrets)) {
		bs, hasBase := base.Secrets[key]
		ls, hasLocal := local.Secrets[key]
		rs, hasRemote := remote.Secrets[key]

		localChanged := hasLocal && (!hasBase || !ls.UpdatedAt.Equal(bs.UpdatedAt))
		remoteChanged := hasRemote && (!hasBase || !rs.UpdatedAt.Equal(bs.UpdatedAt))

		switch {
		case !hasLocal && !hasRemote:
			// Deleted by both
		case hasBase && !hasLocal && remoteChanged:
			return nil, false // local deleted, remote modified — conflict
		case hasBase && !hasRemote && localChanged:
			return nil, false // remote deleted, local modified — conflict
		case !hasLocal:
			// local-only delete
		case !hasRemote:
			// remote-only delete
		case !hasBase:
			if ls.UpdatedAt.Equal(rs.UpdatedAt) {
				out.Secrets[key] = ls
			} else {
				return nil, false // both added different values — conflict
			}
		case localChanged && remoteChanged:
			if ls.UpdatedAt.Equal(rs.UpdatedAt) {
				out.Secrets[key] = ls // concurrent identical update
			} else {
				return nil, false // both changed secret differently — conflict
			}
		case localChanged:
			out.Secrets[key] = ls
		case remoteChanged:
			out.Secrets[key] = rs
		default:
			out.Secrets[key] = ls
		}
	}

	return out, true
}

func keysOfFiles(m map[string]models.TrackedFile) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func keysOfSecrets(m map[string]models.SecretIndex) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func unionStringSet(sets ...[]string) map[string]struct{} {
	out := make(map[string]struct{})
	for _, s := range sets {
		for _, v := range s {
			out[v] = struct{}{}
		}
	}
	return out
}
