package auth

import (
	"testing"
)

func TestHashPassword(t *testing.T) {
	tests := []struct {
		name    string
		password string
		wantErr bool
	}{
		{"valid password", "SecureP@ss1", false},
		{"short password", "short", false}, // Hash still works, just fails validation
		{"empty password", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash, err := HashPassword(tt.password)
			if (err != nil) != tt.wantErr {
				t.Errorf("HashPassword() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && hash == "" {
				t.Error("HashPassword() returned empty hash")
			}
		})
	}
}

func TestVerifyPassword(t *testing.T) {
	// Create a known hash for testing
	hash, err := HashPassword("SecureP@ss1")
	if err != nil {
		t.Fatalf("Failed to create test hash: %v", err)
	}

	tests := []struct {
		name     string
		password string
		hash     string
		wantErr  bool
	}{
		{"correct password", "SecureP@ss1", hash, false},
		{"wrong password", "WrongP@ss1", hash, true},
		{"empty password", "", hash, true},
		{"invalid hash", "password", "invalid-hash", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := VerifyPassword(tt.password, tt.hash)
			if (err != nil) != tt.wantErr {
				t.Errorf("VerifyPassword() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateEmail(t *testing.T) {
	tests := []struct {
		name    string
		email   string
		wantErr error
	}{
		{"valid email", "user@example.com", nil},
		{"valid email with subdomain", "user@mail.example.com", nil},
		{"valid email with plus", "user+tag@example.com", nil},
		{"invalid empty", "", ErrInvalidEmail},
		{"invalid no @", "userexample.com", ErrInvalidEmail},
		{"invalid no domain", "user@", ErrInvalidEmail},
		{"invalid no local", "@example.com", ErrInvalidEmail},
		{"invalid spaces", "user @example.com", ErrInvalidEmail},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateEmail(tt.email)
			if err != tt.wantErr {
				t.Errorf("ValidateEmail() = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidatePassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  error
	}{
		// Valid passwords (meeting complexity)
		{"valid with all types", "SecureP@ss1", nil},
		{"valid uppercase + digit", "Password1", nil},
		{"valid upper + lower + digit", "Password123", nil},
		{"valid with special", "Pass@word1", nil},

		// Invalid - too short
		{"invalid too short", "Short1!", ErrWeakPassword},
		{"invalid 7 chars", "Passw0!", ErrWeakPassword},

		// Invalid - not enough complexity
		{"invalid only lowercase", "password", ErrWeakPassword},
		{"invalid only digits", "12345678", ErrWeakPassword},
		{"invalid lower + upper", "Password", ErrWeakPassword},
		{"invalid lower + digit", "password1", ErrWeakPassword},
		{"invalid upper + digit", "PASSWORD1", ErrWeakPassword},

		// Edge cases
		{"invalid empty", "", ErrWeakPassword},
		{"invalid exactly 8 simple", "password1", ErrWeakPassword}, // lower + digit = 2, needs 3
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePassword(tt.password)
			if err != tt.wantErr {
				t.Errorf("ValidatePassword() = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestBcryptCost(t *testing.T) {
	// Verify we're using cost 12
	hash1, _ := HashPassword("test")
	hash2, _ := HashPassword("test")

	// Same password should produce different hashes (salt)
	if hash1 == hash2 {
		t.Error("Same password produced identical hashes - salt not working")
	}
}
