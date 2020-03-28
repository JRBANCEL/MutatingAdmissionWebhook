package certificate

import (
	"testing"
	"time"
)

func TestCreateSecretData(t *testing.T) {
	data, err := GenerateSecretData(time.Now(), time.Now().Add(1*time.Hour+1*time.Minute))
	if err != nil {
		t.Fatalf("Failed to create the Secret: %v", err)
	}
	_, err = ParseSecretData(data)
	if err != nil {
		t.Fatalf("Failed to parse the Secret: %v", err)
	}
	expiration, err := GetDurationBeforeExpiration(data)
	if err != nil {
		t.Fatalf("Failed to parse the Secret: %v", err)
	}
	if expiration < 1*time.Hour {
		t.Fatalf("The Secret expires too soon: %v", expiration)
	}
}
