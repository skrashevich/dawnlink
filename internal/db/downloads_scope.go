package db

import "strings"

// RepoRef identifies a repository for scoped analytics queries.
type RepoRef struct {
	Owner string
	Repo  string
}

func repoScopeClause(refs []RepoRef) (clause string, args []any) {
	if len(refs) == 0 {
		return " AND 0", nil
	}
	var b strings.Builder
	b.WriteString(" AND (")
	for i, ref := range refs {
		if i > 0 {
			b.WriteString(" OR ")
		}
		b.WriteString("(owner = ? COLLATE NOCASE AND repo = ? COLLATE NOCASE)")
		args = append(args, ref.Owner, ref.Repo)
	}
	b.WriteString(")")
	return b.String(), args
}

func appendScopeArgs(scopeArgs []any, extra ...any) []any {
	out := make([]any, 0, len(scopeArgs)+len(extra))
	out = append(out, extra...)
	out = append(out, scopeArgs...)
	return out
}
