package main

import "fmt"

// CleanupWorkersCount deletes at most count FlareTunnel Workers from one
// account. The worker list returned by Cloudflare is the source of truth, so
// the method never attempts to delete more workers than currently exist.
//
// CleanupWorkers remains available for the historical unbounded cleanup mode.
// This method is used by the automated bounded cleanup mode.
func (ft *FlareTunnel) CleanupWorkersCount(accountName string, count int) error {
	if count < 0 {
		return fmt.Errorf("cleanup count must be >= 0")
	}
	if count == 0 {
		return nil
	}

	client, ok := ft.Clients[accountName]
	if !ok {
		return fmt.Errorf("account '%s' not found", accountName)
	}

	workers, err := client.ListWorkers()
	if err != nil {
		return fmt.Errorf("list workers for account '%s': %w", accountName, err)
	}

	if count > len(workers) {
		count = len(workers)
	}

	var failures int
	for _, worker := range workers[:count] {
		if err := client.DeleteWorker(worker.Name); err != nil {
			failures++
			fmt.Printf("   ✗ Failed to delete: %s\n", worker.Name)
			continue
		}
		fmt.Printf("   ✓ Deleted: %s\n", worker.Name)
	}

	if failures > 0 {
		return fmt.Errorf("failed to delete %d selected worker(s)", failures)
	}
	return nil
}
