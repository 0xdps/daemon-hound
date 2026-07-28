// Copyright (C) 2026 DaemonHound Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package merge

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/0xdps/daemon-hound/internal/models"
	"github.com/BurntSushi/toml"
)

// SecretDriver performs semantic 3-way merge of per-secret encrypted TOML files.
// When two machines rotate the same secret concurrently, Git sees a binary conflict
// because the encrypted bytes differ. This driver decrypts all three versions,
// merges the SecretFile structures structurally, then re-encrypts the result.
//
// Merge rules:
//   - Union all versions (v1, v2, v3...) — nothing is ever lost
//   - Take the Latest pointer from the side with the most versions
//   - If both sides have the same number of versions, take the lexicographically
//     larger Latest (e.g., v3 > v2)
//   - Union all Refs (mappings) — deduplicate by (namespace, file, key)
//   - If both sides changed the same existing version's value differently → conflict
//     (this should be impossible in practice since versions are immutable)
type SecretDriver struct {
	// Encrypt and Decrypt are injected by the caller (syncer/daemon) because
	// the merge package must not depend on storage.Vault to avoid a circular
	// import. The caller provides these functions when creating the driver.
	Encrypt func(plaintext []byte) ([]byte, error)
	Decrypt func(ciphertext []byte) ([]byte, error)
}

func (d *SecretDriver) CanHandle(filename string) bool {
	return strings.HasPrefix(filename, "secrets/") && strings.HasSuffix(filename, ".toml.age")
}

func (d *SecretDriver) Merge(base, local, remote []byte) ([]byte, Result, error) {
	if d.Encrypt == nil || d.Decrypt == nil {
		return nil, Unsupported, fmt.Errorf("SecretDriver: Encrypt/Decrypt not configured")
	}

	var baseSF, localSF, remoteSF models.SecretFile

	if err := d.decodeSecretFile(base, &baseSF); err != nil {
		return nil, HasConflict, fmt.Errorf("decode base secret: %w", err)
	}
	if err := d.decodeSecretFile(local, &localSF); err != nil {
		return nil, HasConflict, fmt.Errorf("decode local secret: %w", err)
	}
	if err := d.decodeSecretFile(remote, &remoteSF); err != nil {
		return nil, HasConflict, fmt.Errorf("decode remote secret: %w", err)
	}

	merged, ok := mergeSecretFile(&baseSF, &localSF, &remoteSF)
	if !ok {
		return nil, HasConflict, nil
	}

	// Encode merged SecretFile to TOML.
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(merged); err != nil {
		return nil, HasConflict, fmt.Errorf("encode merged secret: %w", err)
	}

	// Re-encrypt.
	enc, err := d.Encrypt(buf.Bytes())
	if err != nil {
		return nil, HasConflict, fmt.Errorf("re-encrypt merged secret: %w", err)
	}
	return enc, Merged, nil
}

func (d *SecretDriver) decodeSecretFile(ciphertext []byte, sf *models.SecretFile) error {
	if len(ciphertext) == 0 {
		// Empty file (e.g., a side that didn't exist in base).
		*sf = models.SecretFile{Versions: make(map[string]models.SecretVersion)}
		return nil
	}
	plain, err := d.Decrypt(ciphertext)
	if err != nil {
		return fmt.Errorf("decrypt: %w", err)
	}
	if _, err := toml.Decode(string(plain), sf); err != nil {
		return fmt.Errorf("decode toml: %w", err)
	}
	if sf.Versions == nil {
		sf.Versions = make(map[string]models.SecretVersion)
	}
	return nil
}

// mergeSecretFile merges three SecretFiles. Returns (merged, true) on success,
// (nil, false) on unresolvable conflict.
func mergeSecretFile(base, local, remote *models.SecretFile) (*models.SecretFile, bool) {
	out := &models.SecretFile{
		Name:      local.Name,
		CreatedBy: local.CreatedBy,
		CreatedAt: local.CreatedAt,
		Versions:  make(map[string]models.SecretVersion),
		Refs:      make([]models.SecretRef, 0),
	}

	// Union all versions from all three sides.
	allVersions := make(map[string]struct{})
	for v := range base.Versions {
		allVersions[v] = struct{}{}
	}
	for v := range local.Versions {
		allVersions[v] = struct{}{}
	}
	for v := range remote.Versions {
		allVersions[v] = struct{}{}
	}

	for v := range allVersions {
		bv, hasBase := base.Versions[v]
		lv, hasLocal := local.Versions[v]
		rv, hasRemote := remote.Versions[v]

		// If a version exists in both local and remote with different values,
		// and the base also had it with a different value, that's a true conflict.
		// (In practice this should never happen because versions are immutable.)
		if hasLocal && hasRemote {
			if !bytes.Equal(lv.Value, rv.Value) {
				if hasBase && !bytes.Equal(bv.Value, lv.Value) && !bytes.Equal(bv.Value, rv.Value) {
					// Both changed the same version differently — true conflict.
					return nil, false
				}
				// One side kept base, the other changed it — take the changed one.
				if hasBase && bytes.Equal(bv.Value, lv.Value) {
					out.Versions[v] = rv
				} else if hasBase && bytes.Equal(bv.Value, rv.Value) {
					out.Versions[v] = lv
				} else {
					// No base or both diverged from base — take local (arbitrary but deterministic).
					out.Versions[v] = lv
				}
				continue
			}
			// Same value in both — take either.
			out.Versions[v] = lv
			continue
		}

		// Version only in one side — keep it.
		if hasLocal {
			out.Versions[v] = lv
		} else if hasRemote {
			out.Versions[v] = rv
		} else if hasBase {
			out.Versions[v] = bv
		}
	}

	// Determine Latest pointer.
	// Take from the side with the most versions. If tied, take lexicographically larger.
	localCount := len(local.Versions)
	remoteCount := len(remote.Versions)

	if localCount > remoteCount {
		out.Latest = local.Latest
	} else if remoteCount > localCount {
		out.Latest = remote.Latest
	} else {
		// Same count — take lexicographically larger Latest.
		if local.Latest > remote.Latest {
			out.Latest = local.Latest
		} else {
			out.Latest = remote.Latest
		}
	}

	// Ensure Latest actually exists in the merged versions.
	if _, ok := out.Versions[out.Latest]; !ok {
		// Fallback: pick the lexicographically largest version present.
		var maxVer string
		for v := range out.Versions {
			if v > maxVer {
				maxVer = v
			}
		}
		out.Latest = maxVer
	}

	// Union Refs, deduplicating by (namespace, file, key).
	refKey := func(r models.SecretRef) string {
		return r.Namespace + ":" + r.File + ":" + r.Key
	}
	seenRefs := make(map[string]bool)
	for _, refs := range [][]models.SecretRef{base.Refs, local.Refs, remote.Refs} {
		for _, ref := range refs {
			k := refKey(ref)
			if !seenRefs[k] {
				seenRefs[k] = true
				out.Refs = append(out.Refs, ref)
			}
		}
	}

	return out, true
}
