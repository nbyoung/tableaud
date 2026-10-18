package source

import "context"

// ResolveIn exposes resolve to the tests of package source_test: it resolves
// ref in the repository of project at its head.
func (t *Tablo) ResolveIn(ctx context.Context, project, ref string) (string, error) {
	ld, err := t.load(ctx, project, "")
	if err != nil {
		return "", err
	}
	return t.resolve(ctx, ld, ref)
}
