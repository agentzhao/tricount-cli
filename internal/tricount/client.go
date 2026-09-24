package tricount

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	DefaultBaseURL = "https://api.tricount.bunq.com"
	userAgent      = "com.bunq.tricount.android:RELEASE:7.0.7:3174:ANDROID:13:C"
	requestID      = "049bfcdf-6ae4-4cee-af7b-45da31ea85d0"
	maxBody        = 8 << 20
)

// Client talks to the Tricount registry API.
// Authentication is a device registration: the public key is sent once and the
// returned token is sent as X-Bunq-Client-Authentication on later requests.
type Client struct {
	creds      Credentials
	httpClient *http.Client
	baseURL    string
	authToken  string
	userID     int
}

// New returns a client for the public Tricount API.
func New(creds Credentials) *Client {
	return &Client{
		creds:      creds,
		httpClient: &http.Client{Timeout: 45 * time.Second},
		baseURL:    DefaultBaseURL,
	}
}

// APIError is an HTTP error from the Tricount API, including a short response body.
type APIError struct {
	Method string
	Path   string
	Status int
	Body   string
}

func (e *APIError) Error() string {
	msg := fmt.Sprintf("%s %s returned HTTP %d", e.Method, e.Path, e.Status)
	if e.Body != "" {
		msg += ": " + e.Body
	}
	switch e.Status {
	case http.StatusUnauthorized, http.StatusForbidden:
		msg += ". Inspect this device with: tricount auth status"
	case http.StatusNotFound:
		msg += ". Check the sharing token with: tricount group get --help"
	}
	return msg
}

// Authenticate registers the device key and stores the session token on the client.
func (c *Client) Authenticate(ctx context.Context) (User, error) {
	v, err := c.do(ctx, http.MethodPost, "/v1/session-registry-installation", nil, "application/json", mustJSON(map[string]any{
		"app_installation_uuid": c.creds.AppID,
		"client_public_key":     c.creds.PublicKeyPEM,
		"device_description":    "Android",
	}), nil)
	if err != nil {
		return User{}, err
	}
	token, user, err := parseSession(v)
	if err != nil {
		return User{}, err
	}
	c.authToken = token
	c.userID = user.ID
	return user, nil
}

func (c *Client) ensure(ctx context.Context) error {
	if c.userID != 0 && c.authToken != "" {
		return nil
	}
	_, err := c.Authenticate(ctx)
	return err
}

// WhoAmI returns the profile of the authenticated device user.
func (c *Client) WhoAmI(ctx context.Context) (User, error) {
	if err := c.ensure(ctx); err != nil {
		return User{}, err
	}
	v, err := c.do(ctx, http.MethodGet, c.userPath(""), nil, "", nil, nil)
	if err != nil {
		return User{}, err
	}
	return parseUser(v)
}

// List returns groups synced to this device.
func (c *Client) List(ctx context.Context) ([]Tricount, error) {
	if err := c.ensure(ctx); err != nil {
		return nil, err
	}
	v, err := c.do(ctx, http.MethodGet, c.userPath("/registry"), nil, "", nil, nil)
	if err != nil {
		return nil, err
	}
	return parseRegistries(v), nil
}

// GetByToken reads a group by its sharing token. This does not sync the group
// onto the device. Membership link data is only present after a join.
func (c *Client) GetByToken(ctx context.Context, token string) (Tricount, error) {
	if err := c.ensure(ctx); err != nil {
		return Tricount{}, err
	}
	token = NormalizeToken(token)
	if token == "" {
		return Tricount{}, fmt.Errorf("sharing token is empty")
	}
	q := url.Values{"public_identifier_token": {token}}
	v, err := c.do(ctx, http.MethodGet, c.userPath("/registry"), q, "", nil, nil)
	if err != nil {
		return Tricount{}, err
	}
	groups := parseRegistries(v)
	for _, g := range groups {
		if g.Token == token {
			return g, nil
		}
	}
	if len(groups) > 0 {
		return groups[0], nil
	}
	return Tricount{}, fmt.Errorf("no group found for token %s", token)
}

// GetByID reads a group that is already synced to this device.
func (c *Client) GetByID(ctx context.Context, id int) (Tricount, error) {
	groups, err := c.List(ctx)
	if err != nil {
		return Tricount{}, err
	}
	for _, g := range groups {
		if g.ID == id {
			return g, nil
		}
	}
	return Tricount{}, fmt.Errorf("no synced group with id %d", id)
}

// Join syncs a sharing token onto this device so later writes succeed.
// full fetches transactions after the sync.
func (c *Client) Join(ctx context.Context, token string, full bool) (Tricount, error) {
	if err := c.ensure(ctx); err != nil {
		return Tricount{}, err
	}
	token = NormalizeToken(token)
	if token == "" {
		return Tricount{}, fmt.Errorf("sharing token is empty")
	}
	v, err := c.doJSON(ctx, http.MethodPost, c.userPath("/registry-synchronization"), map[string]any{
		"all_registry_active":   []any{map[string]string{"public_identifier_token": token}},
		"all_registry_archived": []any{},
		"all_registry_deleted":  []any{},
	})
	if err != nil {
		return Tricount{}, err
	}
	synced := findSynced(v, token)
	if !full && (synced.ID != 0 || synced.Token != "") {
		return synced, nil
	}
	if synced.ID != 0 {
		if tc, err := c.GetByID(ctx, synced.ID); err == nil {
			return tc, nil
		}
	}
	return c.GetByToken(ctx, token)
}

// Leave removes a token from this device's synced list. The group itself remains.
func (c *Client) Leave(ctx context.Context, token string) error {
	if err := c.ensure(ctx); err != nil {
		return err
	}
	token = NormalizeToken(token)
	_, err := c.doJSON(ctx, http.MethodPost, c.userPath("/registry-synchronization"), map[string]any{
		"all_registry_active":   []any{},
		"all_registry_archived": []any{},
		"all_registry_deleted":  []any{map[string]string{"public_identifier_token": token}},
	})
	return err
}

// Sync fetches several tokens in one request. Empty lists return this device's groups.
func (c *Client) Sync(ctx context.Context, active, archived []string) (SyncResult, error) {
	if err := c.ensure(ctx); err != nil {
		return SyncResult{}, err
	}
	v, err := c.doJSON(ctx, http.MethodPost, c.userPath("/registry-synchronization"), map[string]any{
		"all_registry_active":   tokenObjects(active),
		"all_registry_archived": tokenObjects(archived),
		"all_registry_deleted":  []any{},
	})
	if err != nil {
		return SyncResult{}, err
	}
	return parseSync(v), nil
}

// CreateGroup creates a group and returns its internal id.
func (c *Client) CreateGroup(ctx context.Context, title, currency, description string) (int, error) {
	if err := c.ensure(ctx); err != nil {
		return 0, err
	}
	v, err := c.doJSON(ctx, http.MethodPost, c.userPath("/registry"), map[string]any{
		"title":       title,
		"currency":    currency,
		"description": description,
	})
	if err != nil {
		return 0, err
	}
	return extractID(v)
}

// UpdateGroup changes title, emoji, or category. Description and currency stay as created.
func (c *Client) UpdateGroup(ctx context.Context, id int, title, emoji, category *string) error {
	payload := map[string]any{}
	if title != nil {
		payload["title"] = *title
	}
	if emoji != nil {
		payload["emoji"] = *emoji
	}
	if category != nil {
		payload["category"] = *category
	}
	if len(payload) == 0 {
		return fmt.Errorf("nothing to update")
	}
	return c.putRegistry(ctx, id, payload)
}

// SetStatus sets READ_ONLY (archived) or READ_WRITE (active).
func (c *Client) SetStatus(ctx context.Context, id int, status string) error {
	return c.putRegistry(ctx, id, map[string]any{"status": status})
}

// DeleteGroup permanently deletes a group this device created.
func (c *Client) DeleteGroup(ctx context.Context, id int) error {
	if err := c.ensure(ctx); err != nil {
		return err
	}
	_, err := c.do(ctx, http.MethodDelete, c.userPath(fmt.Sprintf("/registry/%d", id)), nil, "", nil, nil)
	return err
}

// AddMembers appends members and keeps the current membership list.
func (c *Client) AddMembers(ctx context.Context, tc Tricount, names []string) error {
	if len(names) == 0 {
		return fmt.Errorf("at least one member name is required")
	}
	members := make([]any, 0, len(tc.Members)+len(names))
	for _, m := range tc.Members {
		members = append(members, memberBody(m, m.Name))
	}
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			return fmt.Errorf("member name is empty")
		}
		id, err := NewUUID()
		if err != nil {
			return err
		}
		members = append(members, memberBody(Member{UUID: id, Name: name, Status: "ACTIVE"}, name))
	}
	return c.putRegistry(ctx, tc.ID, map[string]any{"memberships": members})
}

// RenameMember changes one display name and sends the full membership list.
func (c *Client) RenameMember(ctx context.Context, tc Tricount, memberUUID, newName string) error {
	newName = strings.TrimSpace(newName)
	if newName == "" {
		return fmt.Errorf("new name is empty")
	}
	members := make([]any, 0, len(tc.Members))
	found := false
	for _, m := range tc.Members {
		name := m.Name
		if m.UUID == memberUUID {
			name = newName
			found = true
		}
		members = append(members, memberBody(m, name))
	}
	if !found {
		return fmt.Errorf("member %s is not in this group", memberUUID)
	}
	return c.putRegistry(ctx, tc.ID, map[string]any{"memberships": members})
}

// DeleteMember removes a member. Members who already have transactions stay
// in the data with status DELETED.
func (c *Client) DeleteMember(ctx context.Context, tc Tricount, member Member) error {
	if member.ID == 0 {
		return fmt.Errorf("member id is missing, so the API cannot delete them")
	}
	members := make([]any, 0, len(tc.Members))
	for _, m := range tc.Members {
		if m.UUID == member.UUID {
			continue
		}
		members = append(members, memberBody(m, m.Name))
	}
	return c.putRegistry(ctx, tc.ID, map[string]any{
		"memberships":            members,
		"deleted_membership_ids": []int{member.ID},
	})
}

// LinkMember sets which member this device represents in the Tricount app.
// Expenses can still be recorded as any member.
func (c *Client) LinkMember(ctx context.Context, registryID int, memberUUID string) error {
	return c.putRegistry(ctx, registryID, map[string]any{"membership_uuid_active": memberUUID})
}

// CreateEntry posts a transaction and returns its id.
func (c *Client) CreateEntry(ctx context.Context, registryID int, payload map[string]any) (int, error) {
	if err := c.ensure(ctx); err != nil {
		return 0, err
	}
	v, err := c.doJSON(ctx, http.MethodPost, c.userPath(fmt.Sprintf("/registry/%d/registry-entry", registryID)), payload)
	if err != nil {
		return 0, err
	}
	return extractID(v)
}

// UpdateEntry replaces an existing transaction. The body must include the full entry.
func (c *Client) UpdateEntry(ctx context.Context, registryID, entryID int, payload map[string]any) error {
	if err := c.ensure(ctx); err != nil {
		return err
	}
	_, err := c.doJSON(ctx, http.MethodPut, c.userPath(fmt.Sprintf("/registry/%d/registry-entry/%d", registryID, entryID)), payload)
	return err
}

// DeleteEntry deletes a transaction.
func (c *Client) DeleteEntry(ctx context.Context, registryID, entryID int) error {
	if err := c.ensure(ctx); err != nil {
		return err
	}
	_, err := c.do(ctx, http.MethodDelete, c.userPath(fmt.Sprintf("/registry/%d/registry-entry/%d", registryID, entryID)), nil, "", nil, nil)
	return err
}

// Rates lists conversion rates from one currency into the others.
func (c *Client) Rates(ctx context.Context, from string) ([]ExchangeRate, error) {
	if err := c.ensure(ctx); err != nil {
		return nil, err
	}
	q := url.Values{"currency": {strings.ToUpper(from)}}
	v, err := c.do(ctx, http.MethodGet, c.userPath("/exchange-rate"), q, "", nil, nil)
	if err != nil {
		return nil, err
	}
	rates := parseRates(v)
	sort.Slice(rates, func(i, j int) bool { return rates[i].Target < rates[j].Target })
	return rates, nil
}

// Rate returns the conversion from one major unit of from into to.
func (c *Client) Rate(ctx context.Context, from, to string) (ExchangeRate, error) {
	from = strings.ToUpper(from)
	to = strings.ToUpper(to)
	rates, err := c.Rates(ctx, from)
	if err != nil {
		return ExchangeRate{}, err
	}
	for _, r := range rates {
		if strings.EqualFold(r.Target, to) {
			return r, nil
		}
	}
	return ExchangeRate{}, fmt.Errorf("no exchange rate from %s to %s", from, to)
}

// ListGallery lists images stored on the group.
func (c *Client) ListGallery(ctx context.Context, registryID int) ([]GalleryAttachment, error) {
	if err := c.ensure(ctx); err != nil {
		return nil, err
	}
	v, err := c.do(ctx, http.MethodGet, c.userPath(fmt.Sprintf("/registry/%d/gallery-attachment", registryID)), nil, "", nil, nil)
	if err != nil {
		return nil, err
	}
	return parseGallery(v), nil
}

// UploadGallery stores an image in the group gallery and returns its uuid.
func (c *Client) UploadGallery(ctx context.Context, registryID int, contentType string, data []byte) (string, error) {
	if err := c.ensure(ctx); err != nil {
		return "", err
	}
	id, err := NewUUID()
	if err != nil {
		return "", err
	}
	v, err := c.do(ctx, http.MethodPost, c.userPath(fmt.Sprintf("/registry/%d/gallery-attachment/%s", registryID, id)), nil, contentType, data, attachmentHeaders())
	if err != nil {
		return "", err
	}
	if u := extractUUID(v); u != "" {
		return u, nil
	}
	return id, nil
}

// DeleteGallery removes a gallery image by uuid.
func (c *Client) DeleteGallery(ctx context.Context, registryID int, attachmentUUID string) error {
	if err := c.ensure(ctx); err != nil {
		return err
	}
	_, err := c.do(ctx, http.MethodDelete, c.userPath(fmt.Sprintf("/registry/%d/gallery-attachment/%s", registryID, attachmentUUID)), nil, "", nil, nil)
	return err
}

// UploadAttachment stores a receipt for use on a transaction and returns its id.
func (c *Client) UploadAttachment(ctx context.Context, registryID int, contentType string, data []byte) (int, error) {
	if err := c.ensure(ctx); err != nil {
		return 0, err
	}
	v, err := c.do(ctx, http.MethodPost, c.userPath(fmt.Sprintf("/registry/%d/attachment", registryID)), nil, contentType, data, attachmentHeaders())
	if err != nil {
		return 0, err
	}
	return extractID(v)
}

func (c *Client) putRegistry(ctx context.Context, id int, payload map[string]any) error {
	if err := c.ensure(ctx); err != nil {
		return err
	}
	_, err := c.doJSON(ctx, http.MethodPut, c.userPath(fmt.Sprintf("/registry/%d", id)), payload)
	return err
}

func (c *Client) doJSON(ctx context.Context, method, path string, payload any) (any, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return c.do(ctx, method, path, nil, "application/json", body, nil)
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, contentType string, body []byte, extra http.Header) (any, error) {
	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("app-id", c.creds.AppID)
	req.Header.Set("X-Bunq-Client-Request-Id", requestID)
	req.Header.Set("Accept", "application/json")
	if c.authToken != "" {
		req.Header.Set("X-Bunq-Client-Authentication", c.authToken)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for k, values := range extra {
		for _, v := range values {
			req.Header.Set(k, v)
		}
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return nil, fmt.Errorf("%s %s: read response: %w", method, path, err)
	}
	if len(raw) > maxBody {
		return nil, fmt.Errorf("%s %s: response exceeded %d bytes", method, path, maxBody)
	}
	if resp.StatusCode >= 300 {
		return nil, &APIError{Method: method, Path: path, Status: resp.StatusCode, Body: squash(string(raw))}
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return map[string]any{}, nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("%s %s: decode response: %w", method, path, err)
	}
	return v, nil
}

func (c *Client) userPath(suffix string) string {
	return fmt.Sprintf("/v1/user/%d%s", c.userID, suffix)
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func squash(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 400 {
		return s[:400] + "..."
	}
	return s
}

func tokenObjects(tokens []string) []any {
	out := make([]any, 0, len(tokens))
	for _, t := range tokens {
		t = NormalizeToken(t)
		if t == "" {
			continue
		}
		out = append(out, map[string]string{"public_identifier_token": t})
	}
	return out
}

func findSynced(v any, token string) Tricount {
	res := parseSync(v)
	for _, g := range res.Active {
		if g.Token == token {
			return g
		}
	}
	if len(res.Active) == 1 {
		return res.Active[0]
	}
	return Tricount{}
}

func memberBody(m Member, name string) map[string]any {
	if name == "" {
		name = m.Name
	}
	status := m.Status
	if status == "" {
		status = "ACTIVE"
	}
	return map[string]any{
		"uuid":                      m.UUID,
		"status":                    status,
		"auto_add_card_transaction": "",
		"setting":                   nil,
		"alias": map[string]any{
			"type":  "UUID",
			"value": m.UUID,
			"name":  name,
		},
	}
}

func attachmentHeaders() http.Header {
	h := make(http.Header)
	h.Set("X-Bunq-Attachment-Description", "")
	return h
}
