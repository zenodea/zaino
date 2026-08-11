package permission

import "context"

type Gate struct {
	Policy   *Policy
	Approver Approver
}

// A nil Gate allows everything, so tools stay usable in tests and in code
// that has not opted into a policy.
func (g *Gate) Check(ctx context.Context, req Request) error {
	if g == nil || g.Policy == nil {
		return nil
	}

	switch verdict, reason := g.Policy.Decide(req); verdict {
	case Allow:
		return nil
	case Deny:
		return &DeniedError{Request: req, Reason: reason}
	}

	if g.Approver == nil {
		return &DeniedError{Request: req, Reason: "nothing here can ask for approval"}
	}

	grant, err := g.Approver.Approve(ctx, req)
	if err != nil {
		return err
	}
	switch grant {
	case Always:
		g.Policy.Remember(req)
		return nil
	case Once:
		return nil
	}
	return &DeniedError{Request: req, Reason: "you said no"}
}

func (g *Gate) Mode() Mode {
	if g == nil || g.Policy == nil {
		return Bypass
	}
	g.Policy.mu.Lock()
	defer g.Policy.mu.Unlock()
	return g.Policy.Mode
}
