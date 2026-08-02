package auth

import "testing"

func TestValidNodeToken(t *testing.T) {
	token, err := GenerateNodeToken()
	if err != nil {
		t.Fatalf("GenerateNodeToken: %v", err)
	}
	hash := HashNodeToken(token)

	if !ValidNodeToken(token, hash) {
		t.Fatal("expected valid token")
	}
	if ValidNodeToken("wrong", hash) {
		t.Fatal("expected invalid token")
	}
	if ValidNodeToken(token, "") {
		t.Fatal("empty hash should not validate")
	}
}
