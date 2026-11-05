package core

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testKey(t *testing.T) string {
	t.Helper()
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(b)
}

func readyStore(t *testing.T) (FileStore, string) {
	t.Helper()
	// t.TempDir comes back 0755 on this platform, and the store refuses a loose
	// directory -- which is the requested behaviour, so the test adjusts to it.
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvCredentialDir, dir)
	t.Setenv(EnvCredentialKey, testKey(t))
	return FileStore{Name: "gabriel-session"}, dir
}

// TestItStoresAndReturns: the happy path, and the only one the consumer will
// ever see.
func TestItStoresAndReturns(t *testing.T) {
	s, dir := readyStore(t)

	const value = "session=eyJhbGciOiJkaXIi..QUJDRA=="
	if err := s.Save(value); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got != value {
		t.Errorf("Load = %q, want %q", got, value)
	}

	// And the value is not in the clear in the file.
	raw, err := os.ReadFile(filepath.Join(dir, "gabriel-session.cred"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "eyJhbGciOiJkaXIi") {
		t.Error("the credential is in the clear in the file")
	}
	if !strings.HasPrefix(string(raw), formatEncrypted+"\n") {
		t.Errorf("the file does not start with the version: %q", raw[:min(20, len(raw))])
	}
}

// TestAbsentIsNotAnError: there being no stored value is a normal state -- the
// very first run. The caller falls back to the seed.
func TestAbsentIsNotAnError(t *testing.T) {
	s, _ := readyStore(t)
	got, err := s.Load()
	if err != nil {
		t.Fatalf("Load of a missing file returned an error: %v", err)
	}
	if got != "" {
		t.Errorf("Load = %q, want empty", got)
	}
}

// TestTheNonceDoesNotRepeat: reusing a nonce with the same key in GCM breaks the
// cipher, and it is the most common mistake of whoever implements this for the
// first time.
func TestTheNonceDoesNotRepeat(t *testing.T) {
	s, dir := readyStore(t)

	vistos := map[string]bool{}
	for i := 0; i < 50; i++ {
		if err := s.Save("mesmo-valor-sempre"); err != nil {
			t.Fatal(err)
		}
		raw, err := os.ReadFile(filepath.Join(dir, "gabriel-session.cred"))
		if err != nil {
			t.Fatal(err)
		}
		_, body, _ := firstLine(raw)
		nonce := string(body[:12])
		if vistos[nonce] {
			t.Fatalf("nonce repeated in write %d", i)
		}
		vistos[nonce] = true
	}
}

// TestWithoutAKeyItWritesInTheClear: the cipher is optional. The real control is
// the storage's -- the directory's permissions, the bucket's IAM -- and a key
// living in the same secret as whoever reads the store protects against
// nobody.
func TestWithoutAKeyItWritesInTheClear(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvCredentialDir, dir)
	t.Setenv(EnvCredentialKey, "")

	s := FileStore{Name: "x"}
	if err := s.CheckStore(); err != nil {
		t.Fatalf("no key became an error: %v", err)
	}

	const value = "session=abc=="
	if err := s.Save(value); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := s.Load()
	if err != nil || got != value {
		t.Fatalf("Load = (%q, %v)", got, err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "x.cred"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(raw), formatPlaintext+"\n") {
		t.Errorf("the plain text also needs a version on the first line: %q", raw)
	}
}

// TestPlaintextWarnsOnlyOnce: a warning repeated on every pipeline becomes
// noise, and noise is how a warning stops being read.
func TestPlaintextWarnsOnlyOnce(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvCredentialDir, dir)
	t.Setenv(EnvCredentialKey, "")
	warned.Delete(filepath.Join(dir, "x.cred"))

	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	defer slog.SetDefault(previous)

	s := FileStore{Name: "x"}
	for i := 0; i < 5; i++ {
		if err := s.CheckStore(); err != nil {
			t.Fatal(err)
		}
	}
	if n := strings.Count(buf.String(), "writing in the clear"); n != 1 {
		t.Errorf("warned %d times, want 1", n)
	}
}

// TestWithAKeyItDoesNotWriteInTheClear: the other side of it -- the option to
// encrypt has to actually encrypt.
func TestWithAKeyItDoesNotWriteInTheClear(t *testing.T) {
	s, dir := readyStore(t)
	if err := s.Save("session=abc=="); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "gabriel-session.cred"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "session=abc==") {
		t.Error("with a key, it wrote in the clear")
	}
	if !strings.HasPrefix(string(raw), formatEncrypted+"\n") {
		t.Errorf("wrong header: %q", raw[:min(20, len(raw))])
	}
}

// TestEncryptedWithoutAKeyFallsBackToTheSeed: during a rollout, or after
// somebody removes the key, the store holds a value this process cannot read.
// Falling back to the seed is right; returning garbage would be worse.
func TestEncryptedWithoutAKeyFallsBackToTheSeed(t *testing.T) {
	s, dir := readyStore(t)
	if err := s.Save("segredo"); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvCredentialKey, "")

	got, err := FileStore{Name: "gabriel-session"}.Load()
	if err != nil {
		t.Fatalf("became an error: %v", err)
	}
	if got != "" {
		t.Errorf("Load = %q, want empty", got)
	}
	_ = dir
}

// TestAKeyOfTheWrongLength: AES-256 wants 32 bytes, and a short key would fail
// later with a message about block size. A key that is present and bad is an
// error; an ABSENT key is a choice.
func TestAKeyOfTheWrongLength(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvCredentialDir, dir)
	for _, bad := range []string{
		base64.StdEncoding.EncodeToString([]byte("curta")),
		"isto nao e base64!!",
	} {
		t.Setenv(EnvCredentialKey, bad)
		if err := (FileStore{Name: "x"}).CheckStore(); err == nil {
			t.Errorf("accepted the key %q", bad)
		}
	}
}

// TestWithNoDirectoryItTurnsOffInsteadOfFailing: with no Dir and no env var, the
// behaviour is exactly what it was before the feature -- and that is how it stays
// a shortcut, not a
// requisito.
func TestWithNoDirectoryItTurnsOffInsteadOfFailing(t *testing.T) {
	t.Setenv(EnvCredentialDir, "")
	t.Setenv(EnvCredentialKey, "")

	s := FileStore{Name: "x"}
	if err := s.CheckStore(); err != nil {
		t.Fatalf("a disabled store became an error: %v", err)
	}
	if v, err := s.Load(); err != nil || v != "" {
		t.Errorf("Load disabled = (%q, %v)", v, err)
	}
	if err := s.Save("qualquer"); err != nil {
		t.Errorf("Save disabled = %v", err)
	}
}

// TestALooseDirectoryIsRefused: a shared volume at 0777 is a public directory,
// and keeping a credential in it is no better than not keeping it.
func TestALooseDirectoryIsRefused(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o777); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvCredentialDir, dir)
	t.Setenv(EnvCredentialKey, testKey(t))

	err := FileStore{Name: "x"}.CheckStore()
	if err == nil {
		t.Fatal("a 0777 directory was accepted")
	}
	if !strings.Contains(err.Error(), "0700") {
		t.Errorf("the error does not say what to do: %v", err)
	}
}

// TestPermissions: the file at 0600, the directory at 0700.
func TestPermissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "criado-por-mim")
	t.Setenv(EnvCredentialDir, dir)
	t.Setenv(EnvCredentialKey, testKey(t))

	s := FileStore{Name: "x"}
	if err := s.Save("v"); err != nil {
		t.Fatal(err)
	}

	di, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := di.Mode().Perm(); got != 0o700 {
		t.Errorf("directory = %04o, want 0700", got)
	}
	fi, err := os.Stat(filepath.Join(dir, "x.cred"))
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0o600 {
		t.Errorf("file = %04o, want 0600", got)
	}
}

// TestAnUnreadableFileFallsBackToTheSeed: a future version, truncated, or a
// swapped key. None of them is an error: failing would trade a stale credential
// for none at all, and a future version on a shared volume is a normal scenario
// during a rollout.
func TestAnUnreadableFileFallsBackToTheSeed(t *testing.T) {
	cases := map[string][]byte{
		"versao futura": []byte("brevis-cred/9\nqualquer coisa aqui dentro"),
		"sem versao":    []byte("nao tem newline nenhum"),
		"truncado":      []byte(formatEncrypted + "\ncurto"),
		"nao decifra":   append([]byte(formatEncrypted+"\n"), make([]byte, 60)...),
		"arquivo vazio": {},
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			s, dir := readyStore(t)
			if err := os.WriteFile(filepath.Join(dir, "gabriel-session.cred"), content, 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := s.Load()
			if err != nil {
				t.Fatalf("Load became an error instead of falling back to the seed: %v", err)
			}
			if got != "" {
				t.Errorf("Load = %q, want empty", got)
			}
		})
	}
}

// TestASwappedKeyDoesNotDecrypt: it is the test proving the cipher is the key,
// not
// so um encode.
func TestASwappedKeyDoesNotDecrypt(t *testing.T) {
	// t.TempDir comes back 0755 on this platform, and the store refuses a loose
	// directory -- which is the requested behaviour, so the test adjusts to it.
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvCredentialDir, dir)

	t.Setenv(EnvCredentialKey, testKey(t))
	if err := (FileStore{Name: "x"}).Save("segredo"); err != nil {
		t.Fatal(err)
	}

	t.Setenv(EnvCredentialKey, testKey(t))
	got, err := FileStore{Name: "x"}.Load()
	if err != nil {
		t.Fatalf("a swapped key became an error: %v", err)
	}
	if got != "" {
		t.Errorf("decrypted with another key: %q", got)
	}
}

// TestAdulteracaoEDetectada: GCM autentica. Um byte trocado no texto cifrado
// has to invalidate, and not produce garbage that becomes a credential.
func TestTamperingIsDetected(t *testing.T) {
	s, dir := readyStore(t)
	if err := s.Save("segredo"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "gabriel-session.cred")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	raw[len(raw)-1] ^= 0xff
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	if got, err := s.Load(); err != nil || got != "" {
		t.Errorf("tampering got through: (%q, %v)", got, err)
	}
}

// TestANameThatIsAPathIsRefused: the name comes from the caller and never from
// the URL, and it must not escape the directory either.
func TestANameThatIsAPathIsRefused(t *testing.T) {
	t.Setenv(EnvCredentialDir, t.TempDir())
	t.Setenv(EnvCredentialKey, testKey(t))

	for _, bad := range []string{"", "../fora", "sub/dir", ".", ".."} {
		if err := (FileStore{Name: bad}).CheckStore(); err == nil {
			t.Errorf("accepted Name = %q", bad)
		}
	}
}

// TestConcurrentWritesDoNotCorrupt: the last writer wins, and that is the
// documented choice -- but the file has to stay readable, never half-written.
func TestConcurrentWritesDoNotCorrupt(t *testing.T) {
	s, _ := readyStore(t)

	ready := make(chan struct{})
	end := make(chan struct{})
	for i := 0; i < 8; i++ {
		go func(i int) {
			defer func() { end <- struct{}{} }()
			<-ready
			for j := 0; j < 30; j++ {
				if err := s.Save(strings.Repeat("v", i+1)); err != nil {
					t.Errorf("Save: %v", err)
					return
				}
				got, err := s.Load()
				if err != nil {
					t.Errorf("Load: %v", err)
					return
				}
				// Any of the eight values will do; what must not happen is
				// garbage.
				if got != "" && strings.Trim(got, "v") != "" {
					t.Errorf("Load returned corrupted content: %q", got)
					return
				}
			}
		}(i)
	}
	close(ready)
	for i := 0; i < 8; i++ {
		<-end
	}
}

// TestTheStoreComesBeforeTheSeed: it is the order that makes the feature worth
// anything. A stored value is the result of the last rotation; the seed is what
// somebody pasted in once, and it may have expired already.
func TestTheStoreComesBeforeTheSeed(t *testing.T) {
	s, _ := readyStore(t)
	if err := s.Save("do-store"); err != nil {
		t.Fatal(err)
	}

	var seedUsed bool
	c := &Credential{
		Value:   func(context.Context) (string, error) { seedUsed = true; return "da-semente", nil },
		Apply:   AsCookie,
		Refresh: &Refresh{URL: "http://x", Store: s},
	}

	got, err := c.Get(context.Background())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "do-store" {
		t.Errorf("Get = %q, want the store's value", got)
	}
	if seedUsed {
		t.Error("the seed was called even though there was a stored value")
	}
}

// TestWithNoStoredValueItUsesTheSeed: the very first run.
func TestWithNoStoredValueItUsesTheSeed(t *testing.T) {
	s, _ := readyStore(t)

	c := &Credential{
		Value:   func(context.Context) (string, error) { return "da-semente", nil },
		Apply:   AsCookie,
		Refresh: &Refresh{URL: "http://x", Store: s},
	}
	got, err := c.Get(context.Background())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "da-semente" {
		t.Errorf("Get = %q, want the seed", got)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
