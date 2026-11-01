package worktree

func ListAll(json bool) error {
	registry, err := newRegistry()
	if err != nil {
		return err
	}

	entries, err := registry.all()
	if err != nil {
		return registryReadError(err)
	}

	return printWorktrees(entries, json)
}
