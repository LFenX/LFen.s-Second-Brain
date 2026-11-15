package storage

import (
	"context"
	"errors"
)

type privateReadKey struct{}

// WithPrivateRead keeps storage locators and backend error details out of the
// private reader's logs. It grants no authority and has no wire representation.
func WithPrivateRead(ctx context.Context) context.Context {
	return context.WithValue(ctx, privateReadKey{}, true)
}
func privateRead(ctx context.Context) bool {
	value, _ := ctx.Value(privateReadKey{}).(bool)
	return value
}
func privateReadError(ctx context.Context, err error) error {
	if !privateRead(ctx) {
		return err
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return errors.New("private object read unavailable")
}
