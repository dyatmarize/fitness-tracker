package coros

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
	"golang.org/x/oauth2"

	"github.com/dyatmarize/fitness-tracker/backend/internal/config"
	"github.com/dyatmarize/fitness-tracker/backend/internal/store"
)

// ErrNotLoggedIn is returned when no COROS authorization exists yet.
var ErrNotLoggedIn = errors.New("coros: not logged in; open /api/auth/start")

const clientName = "fitness-tracker"
const clientVersion = "0.1.0"

// Client is a persistent MCP client for the COROS server.
type Client struct {
	cfg  *config.Config
	pool *pgxpool.Pool

	mu      sync.Mutex
	session *mcp.ClientSession
	flow    *loginFlow
}

// loginFlow tracks one in-progress browser authorization flow.
type loginFlow struct {
	ctx    context.Context
	cancel context.CancelFunc

	mu     sync.Mutex
	url    string
	urlSet bool

	codeCh chan *auth.AuthorizationResult
}

// New creates a COROS MCP client.
func New(cfg *config.Config, pool *pgxpool.Pool) *Client {
	return &Client{cfg: cfg, pool: pool}
}

// RedirectURL is the local OAuth redirect target.
func (c *Client) RedirectURL() string {
	addr := c.cfg.AuthListenAddr
	if strings.HasPrefix(addr, ":") {
		addr = "localhost" + addr
	}
	return "http://" + addr + "/callback"
}

// ---------- login flow ----------

// BeginLogin starts the browser OAuth flow and returns the authorization URL
// the user should open. Completes asynchronously; poll /api/meta/status.
func (c *Client) BeginLogin(ctx context.Context) (string, error) {
	c.mu.Lock()
	if c.flow != nil {
		fl := c.flow
		c.mu.Unlock()
		// Reuse the pending flow: wait briefly for its URL.
		deadline := time.Now().Add(10 * time.Second)
		for {
			fl.mu.Lock()
			url, ok := fl.url, fl.urlSet
			fl.mu.Unlock()
			if ok {
				return url, nil
			}
			if time.Now().After(deadline) {
				return "", errors.New("coros: login already in progress, but no URL was produced yet; retry shortly")
			}
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(200 * time.Millisecond):
			}
		}
	}

	flowCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	fl := &loginFlow{ctx: flowCtx, cancel: cancel, codeCh: make(chan *auth.AuthorizationResult, 1)}
	c.flow = fl
	c.mu.Unlock()

	go func() {
		if err := c.connect(flowCtx); err != nil {
			c.logf("login flow failed: %v", err)
		}
		c.mu.Lock()
		if c.flow == fl {
			c.flow = nil
		}
		c.mu.Unlock()
		fl.cancel()
	}()

	deadline := time.Now().Add(30 * time.Second)
	for {
		fl.mu.Lock()
		url, ok := fl.url, fl.urlSet
		fl.mu.Unlock()
		if ok {
			return url, nil
		}
		if time.Now().After(deadline) {
			return "", errors.New("coros: authorization server did not produce a login URL in time")
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// fetchAuthCode is the AuthorizationCodeFetcher: it surfaces the auth URL to
// BeginLogin and waits for the browser callback.
func (c *Client) fetchAuthCode(ctx context.Context, args *auth.AuthorizationArgs) (*auth.AuthorizationResult, error) {
	c.mu.Lock()
	fl := c.flow
	c.mu.Unlock()
	if fl == nil {
		return nil, errors.New("coros: login required but no login flow is active; open /api/auth/start")
	}
	fl.mu.Lock()
	if fl.urlSet {
		fl.mu.Unlock()
		return nil, errors.New("coros: login URL already issued for this flow")
	}
	fl.url = args.URL
	fl.urlSet = true
	fl.mu.Unlock()
	c.logf("login URL issued")

	select {
	case res := <-fl.codeCh:
		return res, nil
	case <-fl.ctx.Done():
		return nil, fl.ctx.Err()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// handleCallback receives the OAuth redirect from the user's browser.
func (c *Client) handleCallback(code, state, iss string) error {
	c.mu.Lock()
	fl := c.flow
	c.mu.Unlock()
	if fl == nil {
		return errors.New("no login in progress")
	}
	select {
	case fl.codeCh <- &auth.AuthorizationResult{Code: code, State: state, Iss: iss}:
		return nil
	default:
		return errors.New("login flow is not waiting for a code")
	}
}

// LoggedIn reports whether tokens are stored.
func (c *Client) LoggedIn(ctx context.Context) (bool, error) {
	tok, err := store.GetTokens(ctx, c.pool)
	if err != nil {
		return false, err
	}
	return tok != nil, nil
}

func (c *Client) logf(format string, args ...any) {
	fmt.Printf("[coros] "+format+"\n", args...)
}

// ---------- session management ----------

// connect establishes an MCP session, authorizing via stored tokens.
func (c *Client) connect(ctx context.Context) error {
	handler, err := c.newAuthHandler(ctx)
	if err != nil {
		return err
	}
	transport := &mcp.StreamableClientTransport{
		Endpoint:             c.cfg.CorosMCPURL,
		OAuthHandler:         handler,
		DisableStandaloneSSE: true,
	}
	client := mcp.NewClient(&mcp.Implementation{Name: clientName, Version: clientVersion}, nil)
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return fmt.Errorf("connect to COROS MCP: %w", err)
	}
	c.mu.Lock()
	old := c.session
	c.session = session
	c.mu.Unlock()
	if old != nil {
		_ = old.Close()
	}
	c.logf("MCP session established")
	return nil
}

func (c *Client) ensureSession(ctx context.Context) error {
	c.mu.Lock()
	sess := c.session
	c.mu.Unlock()
	if sess != nil {
		return nil
	}
	tok, err := store.GetTokens(ctx, c.pool)
	if err != nil {
		return err
	}
	if tok == nil {
		return ErrNotLoggedIn
	}
	return c.connect(ctx)
}

func (c *Client) dropSession() {
	c.mu.Lock()
	sess := c.session
	c.session = nil
	c.mu.Unlock()
	if sess != nil {
		_ = sess.Close()
	}
}

// callTool calls a COROS MCP tool, reconnecting once on failure.
func (c *Client) callTool(ctx context.Context, name string, args map[string]any) (*mcp.CallToolResult, error) {
	if err := c.ensureSession(ctx); err != nil {
		return nil, err
	}
	call := func() (*mcp.CallToolResult, error) {
		c.mu.Lock()
		sess := c.session
		c.mu.Unlock()
		if sess == nil {
			return nil, ErrNotLoggedIn
		}
		return sess.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	}
	res, err := call()
	if err != nil {
		c.dropSession()
		if err := c.ensureSession(ctx); err != nil {
			return nil, err
		}
		if res, err = call(); err != nil {
			return nil, fmt.Errorf("call %s: %w", name, err)
		}
	}
	if res.IsError {
		return nil, fmt.Errorf("coros: tool %s failed: %s", name, resultText(res))
	}
	return res, nil
}

// CallText calls a tool and returns its text content.
func (c *Client) CallText(ctx context.Context, name string, args map[string]any) (string, error) {
	res, err := c.callTool(ctx, name, args)
	if err != nil {
		return "", err
	}
	return resultText(res), nil
}

// CallJSON calls a tool and returns its raw JSON payload, preferring
// structured content and falling back to JSON-encoded text.
func (c *Client) CallJSON(ctx context.Context, name string, args map[string]any) ([]byte, error) {
	res, err := c.callTool(ctx, name, args)
	if err != nil {
		return nil, err
	}
	if res.StructuredContent != nil {
		return marshalJSON(res.StructuredContent)
	}
	text := strings.TrimSpace(resultText(res))
	if strings.HasPrefix(text, "{") || strings.HasPrefix(text, "[") {
		return []byte(text), nil
	}
	return nil, fmt.Errorf("coros: tool %s returned no JSON payload", name)
}

func resultText(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, content := range res.Content {
		if tc, ok := content.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
			b.WriteString("\n")
		}
	}
	return b.String()
}

// ---------- OAuth handler construction ----------

func (c *Client) newAuthHandler(ctx context.Context) (auth.OAuthHandler, error) {
	handlerCfg := &auth.AuthorizationCodeHandlerConfig{
		DynamicClientRegistrationConfig: &auth.DynamicClientRegistrationConfig{
			Metadata: &oauthex.ClientRegistrationMetadata{
				ClientName:               clientName,
				RedirectURIs:             []string{c.RedirectURL()},
				GrantTypes:               []string{"authorization_code", "refresh_token"},
				ResponseTypes:            []string{"code"},
				TokenEndpointAuthMethod:  "none", // public client, PKCE
			},
		},
		RedirectURL:            c.RedirectURL(),
		AuthorizationCodeFetcher: c.fetchAuthCode,
		RequestRefreshToken:    true,
		NewTokenSource:         c.persistTokenSource,
	}

	if ts, err := c.storedTokenSource(ctx); err != nil {
		return nil, err
	} else if ts != nil {
		handlerCfg.InitialTokenSource = ts
	}
	return auth.NewAuthorizationCodeHandler(handlerCfg)
}

// storedTokenSource rebuilds an oauth2 token source from persisted tokens.
func (c *Client) storedTokenSource(ctx context.Context) (oauth2.TokenSource, error) {
	tok, err := store.GetTokens(ctx, c.pool)
	if err != nil {
		return nil, err
	}
	if tok == nil || tok.TokenURL == "" {
		return nil, nil
	}
	oc := &oauth2.Config{
		ClientID:     tok.ClientID,
		ClientSecret: tok.ClientSecret,
		Endpoint: oauth2.Endpoint{AuthURL: tok.AuthURL, TokenURL: tok.TokenURL},
	}
	t := &oauth2.Token{
		AccessToken:  tok.AccessToken,
		RefreshToken: tok.RefreshToken,
		Expiry:       tok.Expiry,
		TokenType:    tok.TokenType,
	}
	return &savingTokenSource{
		inner: oc.TokenSource(ctx, t),
		save:  func(newTok *oauth2.Token) { c.saveTokens(oc, newTok) },
	}, nil
}

// persistTokenSource is the NewTokenSource hook: it stores the freshly
// exchanged token and client details, then returns a saving wrapper.
func (c *Client) persistTokenSource(ctx context.Context, oc *oauth2.Config, tok *oauth2.Token) (oauth2.TokenSource, error) {
	c.saveTokens(oc, tok)
	return &savingTokenSource{
		inner: oc.TokenSource(ctx, tok),
		save:  func(newTok *oauth2.Token) { c.saveTokens(oc, newTok) },
	}, nil
}

func (c *Client) saveTokens(oc *oauth2.Config, tok *oauth2.Token) {
	if tok == nil {
		return
	}
	sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := store.SaveTokens(sctx, c.pool, &store.Tokens{
		AccessToken:  tok.AccessToken,
		RefreshToken: tok.RefreshToken,
		Expiry:       tok.Expiry,
		TokenType:    tok.TokenType,
		Scope:        "",
		ClientID:     oc.ClientID,
		ClientSecret: oc.ClientSecret,
		AuthURL:      oc.Endpoint.AuthURL,
		TokenURL:     oc.Endpoint.TokenURL,
	})
	if err != nil {
		c.logf("saving tokens failed: %v", err)
	}
}

// savingTokenSource wraps a token source and persists every new token.
type savingTokenSource struct {
	inner oauth2.TokenSource
	save  func(*oauth2.Token)
}

func (s *savingTokenSource) Token() (*oauth2.Token, error) {
	t, err := s.inner.Token()
	if err == nil && t != nil {
		s.save(t)
	}
	return t, err
}
