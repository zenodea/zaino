package tool

import "context"

type callIDKey struct{}

func WithCallID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, callIDKey{}, id)
}

func CallID(ctx context.Context) string {
	id, _ := ctx.Value(callIDKey{}).(string)
	return id
}

type progressKey struct{}

// WithProgress gives a call somewhere to send output as it arrives, for the
// commands that run long enough for the waiting to be worth watching.
func WithProgress(ctx context.Context, report func(chunk string)) context.Context {
	return context.WithValue(ctx, progressKey{}, report)
}

// Progress is nil when nobody is listening.
func Progress(ctx context.Context) func(chunk string) {
	report, _ := ctx.Value(progressKey{}).(func(string))
	return report
}
