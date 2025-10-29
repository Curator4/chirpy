package auth

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestHashPassword(t *testing.T) {
	password := "mechaman"
	hash, err := HashPassword(password)

	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}
	if hash == "" {
		t.Error("Expected hash to be non-empty")
	}
	if hash == password {
		t.Error("Hash should not equal plain password")
	}
}

func TestCheckPasswordHash(t *testing.T) {
	password := "mechaman"

	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	// correct
	match, err := CheckPasswordHash(password, hash)
	if err != nil {
		t.Fatalf("CheckPasswordHash failed: %v", err)
	}
	if !match {
		t.Error("Expected password to match hash")
	}

	// incorrect
	match, err = CheckPasswordHash("mechaboy", hash)
	if err != nil {
		t.Fatalf("CheckPasswordHash failed: %v", err)
	}
	if match {
		t.Error("Expected wrong password to not match")
	}
}

func TestMakeAndValidateJWT(t *testing.T) {
	userID := uuid.New()
	secret := "blonde-blazer"
	expiresIn := time.Hour

	// create
	tokenString, err := MakeJWT(userID, secret, expiresIn)
	if err != nil {
		t.Fatalf("MakeJWT failed %v", err)
	}

	// validate
	validatedID, err := ValidateJWT(tokenString, secret)
	if err != nil {
		t.Errorf("ValidateJWT failed %v", err)
	}

	// ensure they match
	if validatedID != userID {
		t.Errorf("expected user ID %v, got %v", userID, validatedID)
	}
}

func TestValidateJWT_Expired(t *testing.T) {
	userID := uuid.New()
	secret := "mechaman"
	expiresIn := -time.Hour

	tokenString, err := MakeJWT(userID, secret, expiresIn)
	if err != nil {
		t.Fatalf("MakeJWT failed %v", err)
	}

	_, err = ValidateJWT(tokenString, secret)
	if err == nil {
		t.Error("Expected expired token to be rejected")
	}
}

func TestValidateJWT_WrongSecret(t *testing.T) {
	userID := uuid.New()
	secret := "blonde-blazer"
	expiresIn := time.Hour

	tokenString, err := MakeJWT(userID, secret, expiresIn)
	if err != nil {
		t.Fatalf("makeJWT failed %v", err)
	}

	_, err = ValidateJWT(tokenString, "bronze-blazer")
	if err == nil {
		t.Error("Expected token with wrong secret to be rejected")
	}
}

func TestGetBearerToken(t *testing.T) {
	tests := []struct {
		name      string
		headers   http.Header
		wantToken string
		wantErr   bool
	}{
		{
			name: "valid token",
			headers: http.Header{
				"Authorization": []string{"Bearer corpo"},
			},
			wantToken: "corpo",
			wantErr:   false,
		},
		{
			name: "no prefix",
			headers: http.Header{
				"Authorization": []string{"corpo"},
			},
			wantToken: "",
			wantErr:   true,
		},
		{
			name:      "missing header",
			headers:   http.Header{},
			wantToken: "",
			wantErr:   true,
		},
		{
			name: "empty token after bearer",
			headers: http.Header{
				"Authorization": []string{"Bearer "},
			},
			wantToken: "",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := GetBearerToken(tt.headers)

			if (err != nil) != tt.wantErr {
				t.Errorf("ExtractToken() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if token != tt.wantToken {
				t.Errorf("ExtractToken() = %v, want %v", token, tt.wantToken)
			}
		})
	}
}
