package cmd

import (
	"fmt"
	"os"

	"github.com/0xdps/daemon-hound/internal/git"
	"github.com/0xdps/daemon-hound/internal/models"
	"github.com/0xdps/daemon-hound/internal/storage"
)

// vaultCommitPush is the standard write pattern for all vault mutations:
//  1. Pull the latest remote state (so we start from the freshest version)
//  2. Migrate legacy inline secrets to per-secret files if needed (once, then commit+push)
//  3. Load the decrypted state
//  4. Apply the caller's mutation via fn
//  5. Save (re-encrypt) the state
//  6. Commit
//  7. Push — if rejected because remote moved ahead, pull and retry once
//
// This prevents the "local changes would be overwritten" abort and the
// "non-fast-forward rejected" push error that occur when the daemon or
// another machine pushed between our load and our push.
func vaultCommitPush(gc *git.Client, vault *storage.Vault, commitMsg string, fn func(*models.VaultState) error) error {
	// Step 1: pull first so we mutate the latest state.
	if err := gc.Pull(); err != nil {
		// Non-fatal — we may be offline; proceed with local state.
		fmt.Fprintf(os.Stderr, "Warning: could not pull before write (offline?): %v\n", err)
	}

	// Step 2: migrate legacy inline secrets if this vault hasn't been migrated yet.
	// This writes per-secret files + updates state.toml.age, then commits and pushes
	// the migration as a separate commit before proceeding with the actual mutation.
	migrated, err := vault.MigrateIfNeeded()
	if err != nil {
		return fmt.Errorf("secret migration failed: %w", err)
	}
	if migrated {
		fmt.Fprintln(os.Stderr, "→ Migrated inline secrets to per-secret files")
		if err := gc.CommitAll("daemon-hound: migrate secrets to per-secret files"); err != nil {
			return fmt.Errorf("commit migration: %w", err)
		}
		if err := gc.Push(); err != nil {
			// Non-fatal — migration is on disk; it will be pushed with the next commit.
			fmt.Fprintf(os.Stderr, "Warning: could not push migration (will retry on next operation): %v\n", err)
		}
	}

	// Step 3+4: load and mutate.
	state, err := vault.LoadState()
	if err != nil {
		return fmt.Errorf("failed to load vault state: %w", err)
	}
	if err := fn(state); err != nil {
		return err
	}

	// Step 5: save.
	if err := vault.SaveState(state); err != nil {
		return fmt.Errorf("failed to save vault state: %w", err)
	}

	// Step 6: commit.
	if err := gc.CommitAll(commitMsg); err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}

	// Step 7: push with one pull-and-retry on non-fast-forward rejection.
	if err := gc.Push(); err != nil {
		// Pull to integrate remote changes and retry.
		if pullErr := gc.Pull(); pullErr != nil {
			return fmt.Errorf("failed to push: %w (and pull retry failed: %v)", err, pullErr)
		}
		if err := gc.Push(); err != nil {
			return fmt.Errorf("failed to push: %w", err)
		}
	}
	return nil
}
