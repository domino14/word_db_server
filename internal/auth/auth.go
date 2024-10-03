package auth

import (
	"context"

	"github.com/rs/zerolog/log"
)

type ctxkey string

const (
	userkey ctxkey = "autheduser"
)

type AuthedUser struct {
	DBID     int
	Username string
	Member   bool
}

func StoreUserInContext(ctx context.Context, dbid int, username string, member bool) context.Context {
	ctx = context.WithValue(ctx, userkey, &AuthedUser{
		DBID:     dbid,
		Username: username,
		Member:   member,
	})
	logger := log.With().Str("username", username).Logger()
	ctx = logger.WithContext(ctx)
	return ctx
}

func UserFromContext(ctx context.Context) *AuthedUser {
	au, ok := ctx.Value(userkey).(*AuthedUser)
	if ok {
		return au
	}
	return nil
}
