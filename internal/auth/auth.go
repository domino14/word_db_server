package auth

import (
	"context"
)

type ctxkey string

const (
	userkey ctxkey = "autheduser"
)

type AuthedUser struct {
	DBID     int
	Username string
}

func StoreUserInContext(ctx context.Context, dbid int, username string) context.Context {
	ctx = context.WithValue(ctx, userkey, &AuthedUser{
		DBID:     dbid,
		Username: username,
	})
	return ctx
}

func UserFromContext(ctx context.Context) *AuthedUser {
	au, ok := ctx.Value(userkey).(*AuthedUser)
	if ok {
		return au
	}
	return nil
}
