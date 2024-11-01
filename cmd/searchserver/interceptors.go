package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
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
				return jwtInterceptor(ctx, secretKey, req, next)
			}
			return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("no auth method"))
		})
	}
	return connect.UnaryInterceptorFunc(interceptor)
}

func jwtInterceptor(ctx context.Context, secretKey []byte, req connect.AnyRequest, next connect.UnaryFunc) (
	connect.AnyResponse, error) {

	ctx, err := authenticateJWT(ctx, req.Header(), secretKey)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	return next(ctx, req)
}

func authenticateJWT(ctx context.Context, reqHeader http.Header, secretKey []byte) (context.Context, error) {
	authHeader := reqHeader.Get("Authorization")
	if authHeader == "" {
		return nil, errors.New("no auth method")
	}

	userToken := strings.TrimPrefix(authHeader, "Bearer ")
	token, err := jwt.Parse(userToken, func(token *jwt.Token) (interface{}, error) {
		// Ensure the signing method is HMAC
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return secretKey, nil
	})
	if err != nil {
		log.Err(err).Msg("err-parsing-token")
		return nil, errors.New("could not parse token")
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		// Extract the subject (uid)
		uidStr, ok := claims["sub"].(string)
		if !ok {
			return nil, errors.New("could not parse uid claim")
		}
		uid, err := strconv.Atoi(uidStr)
		if err != nil {
			return nil, errors.New("could not parse uid as an integer")
		}

		// Extract the issuer
		iss, ok := claims["iss"].(string)
		if !ok || (iss != "aerolith.org" && iss != "aerolith.localhost") {
			return nil, errors.New("unexpected iss claim")
		}

		// Extract the username
		usn, ok := claims["usn"].(string)
		if !ok || usn == "" {
			return nil, errors.New("unexpected usn claim")
		}

		// Extract the mbr claim
		mbr, ok := claims["mbr"].(bool)
		if !ok {
			return nil, errors.New("unexpected mbr claim")
		}

		// Store user information in context
		ctx := auth.StoreUserInContext(ctx, uid, usn, mbr)
		return ctx, nil
	} else {
		return nil, errors.New("could not parse token claims")
	}
}
