package ssh

import "testing"

func TestRedactSecrets(t *testing.T) {
	input := "PASSWORD_HASH=$2y$secret\nPrivateKey=abc\nstatus=healthy\n"
	output := Redact(input)
	for _, secret := range []string{"$2y$secret", "abc"} {
		if contains(output, secret) {
			t.Fatalf("secret leaked: %q in %q", secret, output)
		}
	}
	if !contains(output, "healthy") {
		t.Fatal("safe status was removed")
	}
}

func contains(s, part string) bool {
	for i := 0; i+len(part) <= len(s); i++ {
		if s[i:i+len(part)] == part {
			return true
		}
	}
	return false
}
