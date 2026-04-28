package config

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const (
	openAIClientID    = "app_EMoamEEZ73f0CkXaXp7hrann"
	openAIAuthURL     = "https://auth.openai.com/oauth/authorize"
	openAITokenURL    = "https://auth.openai.com/oauth/token"
	openAIRedirectURI = "http://localhost:1455/auth/callback"
	openAIScope       = "openid profile email offline_access"
)

// LoginOpenAIOAuth starts the OpenAI OAuth flow, opens a browser, waits for the
// localhost callback, stores credentials in auth.json, and returns the account ID.
func LoginOpenAIOAuth(ctx context.Context) (string, error) {
	verifier, challenge, err := generatePKCE()
	if err != nil {
		return "", err
	}
	state, err := randomHex(16)
	if err != nil {
		return "", err
	}

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)
	server := &http.Server{Addr: "127.0.0.1:1455"}
	mux := http.NewServeMux()
	server.Handler = mux
	mux.HandleFunc("/auth/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			http.Error(w, "State mismatch", http.StatusBadRequest)
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "Missing authorization code", http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte("OpenAI authentication completed. You can close this window."))
		codeCh <- code
	})

	ln, err := net.Listen("tcp", server.Addr)
	if err != nil {
		return "", fmt.Errorf("start oauth callback server: %w", err)
	}
	go func() {
		if err := server.Serve(ln); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()
	defer server.Shutdown(context.Background())

	authURL := buildOpenAIAuthURL(verifier, challenge, state)
	_ = openBrowser(authURL)

	var code string
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case err := <-errCh:
		return "", err
	case code = <-codeCh:
	}

	entry, err := exchangeOpenAICode(code, verifier)
	if err != nil {
		return "", err
	}
	auth, err := LoadAuth()
	if err != nil {
		return "", err
	}
	auth["openai-oauth"] = entry
	if err := SaveAuth(auth); err != nil {
		return "", err
	}
	return entry.AccountID, nil
}

func buildOpenAIAuthURL(verifier, challenge, state string) string {
	_ = verifier
	u, _ := url.Parse(openAIAuthURL)
	q := u.Query()
	q.Set("response_type", "code")
	q.Set("client_id", openAIClientID)
	q.Set("redirect_uri", openAIRedirectURI)
	q.Set("scope", openAIScope)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	q.Set("state", state)
	q.Set("id_token_add_organizations", "true")
	q.Set("codex_cli_simplified_flow", "true")
	q.Set("originator", "crobot")
	u.RawQuery = q.Encode()
	return u.String()
}

func exchangeOpenAICode(code, verifier string) (AuthProvider, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", openAIClientID)
	form.Set("code", code)
	form.Set("code_verifier", verifier)
	form.Set("redirect_uri", openAIRedirectURI)
	resp, err := http.PostForm(openAITokenURL, form)
	if err != nil {
		return AuthProvider{}, fmt.Errorf("exchange openai oauth code: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return AuthProvider{}, fmt.Errorf("exchange openai oauth code: %s", resp.Status)
	}
	var body struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return AuthProvider{}, err
	}
	if body.AccessToken == "" || body.RefreshToken == "" || body.ExpiresIn == 0 {
		return AuthProvider{}, fmt.Errorf("exchange openai oauth code: incomplete token response")
	}
	return AuthProvider{
		Type:      "oauth",
		Access:    body.AccessToken,
		Refresh:   body.RefreshToken,
		Expires:   time.Now().Add(time.Duration(body.ExpiresIn) * time.Second).UnixMilli(),
		AccountID: openAIAccountID(body.AccessToken),
	}, nil
}

func generatePKCE() (verifier, challenge string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	verifier = base64.RawURLEncoding.EncodeToString(b)
	h := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(h[:])
	return verifier, challenge, nil
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func openAIAccountID(accessToken string) string {
	parts := strings.Split(accessToken, ".")
	if len(parts) != 3 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return ""
	}
	auth, _ := claims["https://api.openai.com/auth"].(map[string]any)
	accountID, _ := auth["chatgpt_account_id"].(string)
	return accountID
}

func openBrowser(rawURL string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", rawURL)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL)
	default:
		cmd = exec.Command("xdg-open", rawURL)
	}
	return cmd.Start()
}
