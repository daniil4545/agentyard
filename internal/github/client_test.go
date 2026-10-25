package github

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newServer(t *testing.T, key *rsa.PrivateKey, routes map[string]string) (*httptest.Server, *int) {
	t.Helper()
	tokenCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/repos/o/r/installation"), strings.HasPrefix(r.URL.Path, "/app/"):
			checkJWT(t, &key.PublicKey, strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
			if strings.HasSuffix(r.URL.Path, "/access_tokens") {
				tokenCalls++
				w.Write([]byte(`{"token":"inst-tok","expires_at":"` + time.Now().Add(time.Hour).Format(time.RFC3339) + `"}`))
				return
			}
			w.Write([]byte(`{"id":7}`))
		default:
			if r.Header.Get("Authorization") != "token inst-tok" {
				t.Errorf("%s: auth = %q", r.URL.Path, r.Header.Get("Authorization"))
			}
			body, ok := routes[r.URL.Path]
			if !ok {
				http.NotFound(w, r)
				return
			}
			w.Write([]byte(body))
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &tokenCalls
}

func checkJWT(t *testing.T, pub *rsa.PublicKey, jwt string) {
	t.Helper()
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		t.Fatalf("jwt = %q", jwt)
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, sum[:], sig); err != nil {
		t.Fatalf("jwt signature: %v", err)
	}
}

func testKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func TestOpenIssuesSkipsPullRequestsAndCachesToken(t *testing.T) {
	key := testKey(t)
	srv, tokenCalls := newServer(t, key, map[string]string{
		"/repos/o/r/issues": `[{"number":1,"title":"a","labels":[{"name":"agent"}]},{"number":2,"title":"pr","pull_request":{}}]`,
	})
	c := NewClient(srv.URL, "42", key)
	for range 2 {
		issues, err := c.OpenIssues(context.Background(), "o/r", "agent")
		if err != nil {
			t.Fatal(err)
		}
		if len(issues) != 1 || issues[0].Number != 1 || !issues[0].HasLabel("agent") {
			t.Fatalf("issues = %+v", issues)
		}
	}
	if *tokenCalls != 1 {
		t.Fatalf("token calls = %d, want 1 (cached)", *tokenCalls)
	}
}

func TestNotesMergesKindsAfterSince(t *testing.T) {
	key := testKey(t)
	srv, _ := newServer(t, key, map[string]string{
		"/repos/o/r/issues/5/comments": `[{"user":{"login":"owner"},"body":"old","created_at":"2026-10-25T09:00:00Z"},
			{"user":{"login":"owner"},"body":"late comment","created_at":"2026-10-25T12:00:00Z"}]`,
		"/repos/o/r/pulls/5/comments": `[{"user":{"login":"owner"},"body":"line","path":"a.go","line":3,"created_at":"2026-10-25T11:00:00Z"}]`,
		"/repos/o/r/pulls/5/reviews": `[{"user":{"login":"owner"},"body":"","state":"COMMENTED","submitted_at":"2026-10-25T11:00:00Z"},
			{"user":{"login":"owner"},"body":"","state":"CHANGES_REQUESTED","submitted_at":"2026-10-25T11:30:00Z"}]`,
	})
	c := NewClient(srv.URL, "42", key)
	since := time.Date(2026, 10, 25, 10, 0, 0, 0, time.UTC)
	notes, err := c.Notes(context.Background(), "o/r", 5, true, since)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, n := range notes {
		got = append(got, n.Kind+":"+n.Body)
	}
	want := "line:line|review CHANGES_REQUESTED:|comment:late comment"
	if strings.Join(got, "|") != want {
		t.Fatalf("notes = %q, want %q", strings.Join(got, "|"), want)
	}
}

func TestParseKeyRejectsGarbage(t *testing.T) {
	for _, in := range []string{"", "not base64!", base64.StdEncoding.EncodeToString([]byte("not pem"))} {
		if _, err := ParseKey(in); err == nil {
			t.Errorf("ParseKey(%q) must fail", in)
		}
	}
}
