package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
)

type ctxKey int

const claimsCtxKey ctxKey = 1

func withCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-Id")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next(w, r)
	}
}

func withJWT(k keyfunc.Keyfunc, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			writeProblem(w, http.StatusUnauthorized, "Unauthorized", "missing bearer token")
			return
		}
		tokenStr := strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
		tok, err := jwt.Parse(tokenStr, k.Keyfunc,
			jwt.WithValidMethods([]string{"RS256"}),
			jwt.WithExpirationRequired(),
		)
		if err != nil || !tok.Valid {
			writeProblem(w, http.StatusUnauthorized, "Unauthorized", "invalid token")
			return
		}
		claims, ok := tok.Claims.(jwt.MapClaims)
		if !ok {
			writeProblem(w, http.StatusUnauthorized, "Unauthorized", "invalid claims")
			return
		}
		tc := TokenClaims{
			OrganizationID: claimString(claims, "organization_id"),
			TenantID:       claimString(claims, "tenant_id"),
			UserID:         claimString(claims, "user_id"),
			ClientID:       claimString(claims, "client_id"),
			Subject:        claimString(claims, "sub"),
		}
		if tc.ClientID == "" {
			tc.ClientID = claimString(claims, "app_id")
		}
		if tc.OrganizationID == "" {
			writeProblem(w, http.StatusForbidden, "Forbidden", "token missing organization_id")
			return
		}
		ctx := context.WithValue(r.Context(), claimsCtxKey, tc)
		next(w, r.WithContext(ctx))
	}
}

func claimsFrom(ctx context.Context) (TokenClaims, bool) {
	v, ok := ctx.Value(claimsCtxKey).(TokenClaims)
	return v, ok
}

func claimString(claims jwt.MapClaims, key string) string {
	v, ok := claims[key]
	if !ok || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		s = strings.TrimSpace(s)
		if s == "" || s == "null" {
			return ""
		}
		return s
	}
	return strings.TrimSpace(fmt.Sprint(v))
}
