package tls

import "testing"

func TestCipherCodeRejectsMappedButUnimplementedNames(t *testing.T) {
	t.Parallel()

	if got := cipherCode("DHE-RSA-AES128-GCM-SHA256"); got != 0 {
		t.Fatalf("cipherCode() = %#x, want unsupported cipher code 0", got)
	}
}

func TestIsUnnameableInsecureCipherRejectsUnimplementedMappedNames(t *testing.T) {
	t.Parallel()

	if got := isUnnameableInsecureCipher("ECDHE-RSA-AES256-SHA384"); got {
		t.Fatal("isUnnameableInsecureCipher() classified an unimplemented cipher as an insecure runtime cipher")
	}
}
