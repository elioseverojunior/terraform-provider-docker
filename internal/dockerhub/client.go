package dockerhub

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const (
	DefaultBaseURL = "https://hub.docker.com/v2"
	DefaultTimeout = 30 * time.Second
)

// Client represents a Docker Hub API client
type Client struct {
	baseURL    string
	httpClient *http.Client
	token      string
	username   string
}

// Config holds Docker Hub client configuration
type Config struct {
	Username string
	Password string
	Token    string // Personal Access Token (PAT)
}

// NewClient creates a new Docker Hub API client
func NewClient(ctx context.Context, config Config) (*Client, error) {
	client := &Client{
		baseURL: DefaultBaseURL,
		httpClient: &http.Client{
			Timeout: DefaultTimeout,
		},
	}

	// If token is provided, use it directly (PAT)
	if config.Token != "" {
		client.token = config.Token
		client.username = config.Username
		return client, nil
	}

	// Otherwise, authenticate with username/password
	if config.Username != "" && config.Password != "" {
		if err := client.authenticate(ctx, config.Username, config.Password); err != nil {
			return nil, fmt.Errorf("authentication failed: %w", err)
		}
	}

	return client, nil
}

// authenticate performs username/password authentication
func (c *Client) authenticate(ctx context.Context, username, password string) error {
	payload := map[string]string{
		"username": username,
		"password": password,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/users/login", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("authentication failed: %s - %s", resp.Status, string(body))
	}

	var result struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	c.token = result.Token
	c.username = username
	return nil
}

// doRequest performs an authenticated HTTP request
func (c *Client) doRequest(ctx context.Context, method, path string, body interface{}, result interface{}) error {
	var bodyReader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return err
		}
		bodyReader = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return err
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error: %s - %s", resp.Status, string(respBody))
	}

	if result != nil && resp.StatusCode != http.StatusNoContent {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return err
		}
	}

	return nil
}

// Repository represents a Docker Hub repository
type Repository struct {
	ID              int64     `json:"id,omitempty"`
	Namespace       string    `json:"namespace"`
	Name            string    `json:"name"`
	Description     string    `json:"description,omitempty"`
	FullDescription string    `json:"full_description,omitempty"`
	IsPrivate       bool      `json:"is_private"`
	PullCount       int64     `json:"pull_count,omitempty"`
	StarCount       int64     `json:"star_count,omitempty"`
	LastUpdated     time.Time `json:"last_updated,omitempty"`
	DateRegistered  time.Time `json:"date_registered,omitempty"`
	User            string    `json:"user,omitempty"`
	Status          int       `json:"status,omitempty"`
}

// CreateRepository creates a new repository
func (c *Client) CreateRepository(ctx context.Context, repo *Repository) (*Repository, error) {
	path := fmt.Sprintf("/repositories/%s", repo.Namespace)
	payload := map[string]interface{}{
		"name":             repo.Name,
		"namespace":        repo.Namespace,
		"description":      repo.Description,
		"full_description": repo.FullDescription,
		"is_private":       repo.IsPrivate,
	}

	var result Repository
	if err := c.doRequest(ctx, "POST", path, payload, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetRepository retrieves a repository
func (c *Client) GetRepository(ctx context.Context, namespace, name string) (*Repository, error) {
	path := fmt.Sprintf("/repositories/%s/%s", namespace, name)
	var result Repository
	if err := c.doRequest(ctx, "GET", path, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// UpdateRepository updates a repository
func (c *Client) UpdateRepository(ctx context.Context, repo *Repository) (*Repository, error) {
	path := fmt.Sprintf("/repositories/%s/%s", repo.Namespace, repo.Name)
	payload := map[string]interface{}{
		"description":      repo.Description,
		"full_description": repo.FullDescription,
	}

	var result Repository
	if err := c.doRequest(ctx, "PATCH", path, payload, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// DeleteRepository deletes a repository
func (c *Client) DeleteRepository(ctx context.Context, namespace, name string) error {
	path := fmt.Sprintf("/repositories/%s/%s", namespace, name)
	return c.doRequest(ctx, "DELETE", path, nil, nil)
}

// ListRepositories lists repositories for a namespace
func (c *Client) ListRepositories(ctx context.Context, namespace string, page, pageSize int) ([]Repository, int, error) {
	path := fmt.Sprintf("/repositories/%s?page=%d&page_size=%d", namespace, page, pageSize)
	var result struct {
		Count   int          `json:"count"`
		Results []Repository `json:"results"`
	}
	if err := c.doRequest(ctx, "GET", path, nil, &result); err != nil {
		return nil, 0, err
	}
	return result.Results, result.Count, nil
}

// Team represents a Docker Hub team
type Team struct {
	ID          int64  `json:"id,omitempty"`
	UUID        string `json:"uuid,omitempty"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MemberCount int    `json:"member_count,omitempty"`
}

// CreateTeam creates a new team in an organization
func (c *Client) CreateTeam(ctx context.Context, orgName string, team *Team) (*Team, error) {
	path := fmt.Sprintf("/orgs/%s/groups", orgName)
	payload := map[string]interface{}{
		"name":        team.Name,
		"description": team.Description,
	}

	var result Team
	if err := c.doRequest(ctx, "POST", path, payload, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetTeam retrieves a team
func (c *Client) GetTeam(ctx context.Context, orgName, teamName string) (*Team, error) {
	path := fmt.Sprintf("/orgs/%s/groups/%s", orgName, url.PathEscape(teamName))
	var result Team
	if err := c.doRequest(ctx, "GET", path, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// UpdateTeam updates a team
func (c *Client) UpdateTeam(ctx context.Context, orgName string, team *Team) (*Team, error) {
	path := fmt.Sprintf("/orgs/%s/groups/%s", orgName, url.PathEscape(team.Name))
	payload := map[string]interface{}{
		"description": team.Description,
	}

	var result Team
	if err := c.doRequest(ctx, "PATCH", path, payload, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// DeleteTeam deletes a team
func (c *Client) DeleteTeam(ctx context.Context, orgName, teamName string) error {
	path := fmt.Sprintf("/orgs/%s/groups/%s", orgName, url.PathEscape(teamName))
	return c.doRequest(ctx, "DELETE", path, nil, nil)
}

// Member represents an organization member
type Member struct {
	ID       string `json:"id,omitempty"`
	Username string `json:"username"`
	FullName string `json:"full_name,omitempty"`
	Email    string `json:"email,omitempty"`
	Role     string `json:"role,omitempty"` // "member" or "owner"
}

// AddOrgMember adds a member to an organization
func (c *Client) AddOrgMember(ctx context.Context, orgName, username, role string) error {
	path := fmt.Sprintf("/orgs/%s/members", orgName)
	payload := map[string]interface{}{
		"member": username,
		"role":   role,
	}
	return c.doRequest(ctx, "POST", path, payload, nil)
}

// GetOrgMember gets an organization member
func (c *Client) GetOrgMember(ctx context.Context, orgName, username string) (*Member, error) {
	path := fmt.Sprintf("/orgs/%s/members/%s", orgName, username)
	var result Member
	if err := c.doRequest(ctx, "GET", path, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// RemoveOrgMember removes a member from an organization
func (c *Client) RemoveOrgMember(ctx context.Context, orgName, username string) error {
	path := fmt.Sprintf("/orgs/%s/members/%s", orgName, username)
	return c.doRequest(ctx, "DELETE", path, nil, nil)
}

// UpdateOrgMember updates a member's role in an organization
func (c *Client) UpdateOrgMember(ctx context.Context, orgName, username, role string) error {
	path := fmt.Sprintf("/orgs/%s/members/%s", orgName, username)
	payload := map[string]interface{}{
		"role": role,
	}
	return c.doRequest(ctx, "PATCH", path, payload, nil)
}

// ListOrgMembers lists organization members
func (c *Client) ListOrgMembers(ctx context.Context, orgName string, page, pageSize int) ([]Member, int, error) {
	path := fmt.Sprintf("/orgs/%s/members?page=%d&page_size=%d", orgName, page, pageSize)
	var result struct {
		Count   int      `json:"count"`
		Results []Member `json:"results"`
	}
	if err := c.doRequest(ctx, "GET", path, nil, &result); err != nil {
		return nil, 0, err
	}
	return result.Results, result.Count, nil
}

// AddTeamMember adds a member to a team
func (c *Client) AddTeamMember(ctx context.Context, orgName, teamName, username string) error {
	path := fmt.Sprintf("/orgs/%s/groups/%s/members", orgName, url.PathEscape(teamName))
	payload := map[string]interface{}{
		"member": username,
	}
	return c.doRequest(ctx, "POST", path, payload, nil)
}

// RemoveTeamMember removes a member from a team
func (c *Client) RemoveTeamMember(ctx context.Context, orgName, teamName, username string) error {
	path := fmt.Sprintf("/orgs/%s/groups/%s/members/%s", orgName, url.PathEscape(teamName), username)
	return c.doRequest(ctx, "DELETE", path, nil, nil)
}

// IsTeamMember checks if a user is a member of a team
func (c *Client) IsTeamMember(ctx context.Context, orgName, teamName, username string) (bool, error) {
	members, _, err := c.ListTeamMembers(ctx, orgName, teamName, 1, 100)
	if err != nil {
		return false, err
	}
	for _, member := range members {
		if member.Username == username {
			return true, nil
		}
	}
	return false, nil
}

// ListTeamMembers lists team members
func (c *Client) ListTeamMembers(ctx context.Context, orgName, teamName string, page, pageSize int) ([]Member, int, error) {
	path := fmt.Sprintf("/orgs/%s/groups/%s/members?page=%d&page_size=%d", orgName, url.PathEscape(teamName), page, pageSize)
	var result struct {
		Count   int      `json:"count"`
		Results []Member `json:"results"`
	}
	if err := c.doRequest(ctx, "GET", path, nil, &result); err != nil {
		return nil, 0, err
	}
	return result.Results, result.Count, nil
}

// RepositoryPermission represents team permissions on a repository
type RepositoryPermission struct {
	TeamName   string `json:"group_name"`
	Permission string `json:"permission"` // "read", "write", or "admin"
}

// SetRepositoryTeamPermission sets team permission on a repository
func (c *Client) SetRepositoryTeamPermission(ctx context.Context, namespace, repoName, teamName, permission string) error {
	path := fmt.Sprintf("/repositories/%s/%s/groups", namespace, repoName)
	payload := map[string]interface{}{
		"group_name": teamName,
		"permission": permission,
	}
	return c.doRequest(ctx, "POST", path, payload, nil)
}

// GetRepositoryTeamPermission gets team permission on a repository
func (c *Client) GetRepositoryTeamPermission(ctx context.Context, namespace, repoName, teamName string) (*RepositoryPermission, error) {
	path := fmt.Sprintf("/repositories/%s/%s/groups/%s", namespace, repoName, url.PathEscape(teamName))
	var result RepositoryPermission
	if err := c.doRequest(ctx, "GET", path, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// RemoveRepositoryTeamPermission removes team permission from a repository
func (c *Client) RemoveRepositoryTeamPermission(ctx context.Context, namespace, repoName, teamName string) error {
	path := fmt.Sprintf("/repositories/%s/%s/groups/%s", namespace, repoName, url.PathEscape(teamName))
	return c.doRequest(ctx, "DELETE", path, nil, nil)
}

// AccessToken represents a Docker Hub access token
type AccessToken struct {
	UUID        string    `json:"uuid,omitempty"`
	Token       string    `json:"token,omitempty"` // Only returned on creation
	Label       string    `json:"token_label"`
	Scopes      []string  `json:"scopes,omitempty"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at,omitempty"`
	LastUsedAt  time.Time `json:"last_used_at,omitempty"`
	GeneratedBy string    `json:"generated_by,omitempty"`
}

// CreateAccessToken creates a new access token
func (c *Client) CreateAccessToken(ctx context.Context, token *AccessToken) (*AccessToken, error) {
	path := "/access-tokens"
	payload := map[string]interface{}{
		"token_label": token.Label,
		"scopes":      token.Scopes,
	}

	var result AccessToken
	if err := c.doRequest(ctx, "POST", path, payload, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetAccessToken retrieves an access token
func (c *Client) GetAccessToken(ctx context.Context, uuid string) (*AccessToken, error) {
	path := fmt.Sprintf("/access-tokens/%s", uuid)
	var result AccessToken
	if err := c.doRequest(ctx, "GET", path, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// UpdateAccessToken updates an access token
func (c *Client) UpdateAccessToken(ctx context.Context, uuid string, isActive bool) (*AccessToken, error) {
	path := fmt.Sprintf("/access-tokens/%s", uuid)
	payload := map[string]interface{}{
		"is_active": isActive,
	}

	var result AccessToken
	if err := c.doRequest(ctx, "PATCH", path, payload, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// DeleteAccessToken deletes an access token
func (c *Client) DeleteAccessToken(ctx context.Context, uuid string) error {
	path := fmt.Sprintf("/access-tokens/%s", uuid)
	return c.doRequest(ctx, "DELETE", path, nil, nil)
}

// ListAccessTokens lists access tokens
func (c *Client) ListAccessTokens(ctx context.Context, page, pageSize int) ([]AccessToken, int, error) {
	path := fmt.Sprintf("/access-tokens?page=%d&page_size=%d", page, pageSize)
	var result struct {
		Count   int           `json:"count"`
		Results []AccessToken `json:"results"`
	}
	if err := c.doRequest(ctx, "GET", path, nil, &result); err != nil {
		return nil, 0, err
	}
	return result.Results, result.Count, nil
}

// Organization represents a Docker Hub organization
type Organization struct {
	ID          string    `json:"id,omitempty"`
	OrgName     string    `json:"orgname"`
	FullName    string    `json:"full_name,omitempty"`
	Location    string    `json:"location,omitempty"`
	Company     string    `json:"company,omitempty"`
	DateJoined  time.Time `json:"date_joined,omitempty"`
	GravatarURL string    `json:"gravatar_url,omitempty"`
	ProfileURL  string    `json:"profile_url,omitempty"`
	Type        string    `json:"type,omitempty"`
}

// GetOrganization retrieves an organization
func (c *Client) GetOrganization(ctx context.Context, orgName string) (*Organization, error) {
	path := fmt.Sprintf("/orgs/%s", orgName)
	var result Organization
	if err := c.doRequest(ctx, "GET", path, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// RepositoryTag represents a Docker Hub repository tag
type RepositoryTag struct {
	Name        string    `json:"name"`
	FullSize    int64     `json:"full_size"`
	LastUpdated time.Time `json:"last_updated"`
	Digest      string    `json:"digest,omitempty"`
}

// ListRepositoryTags lists tags for a repository
func (c *Client) ListRepositoryTags(ctx context.Context, namespace, repoName string, page, pageSize int) ([]RepositoryTag, int, error) {
	path := fmt.Sprintf("/repositories/%s/%s/tags?page=%d&page_size=%d", namespace, repoName, page, pageSize)
	var result struct {
		Count   int             `json:"count"`
		Results []RepositoryTag `json:"results"`
	}
	if err := c.doRequest(ctx, "GET", path, nil, &result); err != nil {
		return nil, 0, err
	}
	return result.Results, result.Count, nil
}
