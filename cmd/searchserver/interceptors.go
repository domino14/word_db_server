package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"connectrpc.com/connect"
	"github.com/golang-jwt/jwt/v5"
	"github.com/rs/zerolog/log"

	"github.com/domino14/word_db_server/internal/auth"
)

// NewAuthInterceptor is a connectrpc interceptor that uses a JWT.
func NewAuthInterceptor(secretKey []byte) connect.UnaryInterceptorFunc {
	interceptor := func(next connect.UnaryFunc) connect.UnaryFunc {
		return connect.UnaryFunc(func(
			ctx context.Context,
			req connect.AnyRequest,
		) (connect.AnyResponse, error) {

			header := req.Header()

			if header.Get("Authorization") != "" {
				return userJWTAuth(ctx, secretKey, req, next)
			}
			return nil, errors.New("no auth method")
		})
	}
	return connect.UnaryInterceptorFunc(interceptor)
}

func userJWTAuth(ctx context.Context, secretKey []byte, req connect.AnyRequest, next connect.UnaryFunc) (
	connect.AnyResponse, error) {

	usertoken := strings.TrimPrefix(req.Header().Get("Authorization"), "Bearer ")
	token, err := jwt.Parse(usertoken, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("Unexpected signing method: %v", token.Header["alg"])
		}
		return secretKey, nil
	})
	if err != nil {
		log.Err(err).Msg("err-parsing-token")
		return nil, connect.NewError(
			connect.CodeUnauthenticated,
			errors.New("could not parse token"),
		)
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok {
		uidstr, err := claims.GetSubject()
		if err != nil {
			return nil, connect.NewError(
				connect.CodeUnauthenticated,
				errors.New("could not parse uid claim"),
			)
		}
		uid, err := strconv.Atoi(uidstr)
		if err != nil {
			return nil, connect.NewError(
				connect.CodeUnauthenticated,
				errors.New("could not parse uid as an integer"),
			)
		}
		iss, ok := claims["iss"].(string)
		if !ok || iss != "aerolith.org" {
			return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unexpected iss claim"))
		}
		usn, ok := claims["usn"].(string)
		if usn == "" {
			return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unexpected empty usn claim"))
		}
		ctx = auth.StoreUserInContext(ctx, uid, usn)
	} else {
		return nil, connect.NewError(
			connect.CodeUnauthenticated,
			errors.New("could not parse token claims"),
		)
	}

	return next(ctx, req)
}
