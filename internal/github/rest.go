package github

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const apiBase = "https://api.github.com"

// ErrNotFound is returned when the API responds 404.
var ErrNotFound = errors.New("not found")

// rest performs a REST call against the GitHub API. If out is non-nil the JSON
// response body is decoded into it.
func (c *Client) rest(ctx context.Context, method, path string, body, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, apiBase+path, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("github rest %s %s: %s: %s", method, path, resp.Status, strings.TrimSpace(string(b)))
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

// File is the content and blob SHA of a repository file.
type File struct {
	Content string // decoded UTF-8 content
	SHA     string // blob SHA, required to update the file
}

// GetFile fetches a file's content at ref (branch, tag or commit).
func (c *Client) GetFile(ctx context.Context, owner, repo, path, ref string) (File, error) {
	var out struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
		SHA      string `json:"sha"`
	}
	p := fmt.Sprintf("/repos/%s/%s/contents/%s?ref=%s", owner, repo, path, url.QueryEscape(ref))
	if err := c.rest(ctx, http.MethodGet, p, nil, &out); err != nil {
		return File{}, err
	}
	if out.Encoding != "base64" {
		return File{}, fmt.Errorf("unexpected content encoding %q", out.Encoding)
	}
	dec, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(out.Content, "\n", ""))
	if err != nil {
		return File{}, err
	}
	return File{Content: string(dec), SHA: out.SHA}, nil
}

// GetFileMaybe is like GetFile but reports found=false instead of an error when
// the file does not exist.
func (c *Client) GetFileMaybe(ctx context.Context, owner, repo, path, ref string) (f File, found bool, err error) {
	f, err = c.GetFile(ctx, owner, repo, path, ref)
	if errors.Is(err, ErrNotFound) {
		return File{}, false, nil
	}
	if err != nil {
		return File{}, false, err
	}
	return f, true, nil
}

// PutFile creates or updates a file on branch. Pass the existing blob sha when
// updating; leave it empty to create.
func (c *Client) PutFile(ctx context.Context, owner, repo, path, branch, message, content, sha string) error {
	body := map[string]any{
		"message": message,
		"content": base64.StdEncoding.EncodeToString([]byte(content)),
		"branch":  branch,
	}
	if sha != "" {
		body["sha"] = sha
	}
	p := fmt.Sprintf("/repos/%s/%s/contents/%s", owner, repo, path)
	return c.rest(ctx, http.MethodPut, p, body, nil)
}

// BranchSHA returns the commit SHA that branch points to.
func (c *Client) BranchSHA(ctx context.Context, owner, repo, branch string) (string, error) {
	var out struct {
		Object struct {
			SHA string `json:"sha"`
		} `json:"object"`
	}
	p := fmt.Sprintf("/repos/%s/%s/git/ref/heads/%s", owner, repo, branch)
	if err := c.rest(ctx, http.MethodGet, p, nil, &out); err != nil {
		return "", err
	}
	return out.Object.SHA, nil
}

// EnsureBranch creates branch pointing at fromSHA if it does not already exist.
func (c *Client) EnsureBranch(ctx context.Context, owner, repo, branch, fromSHA string) error {
	if _, err := c.BranchSHA(ctx, owner, repo, branch); err == nil {
		return nil // already exists
	}
	body := map[string]any{"ref": "refs/heads/" + branch, "sha": fromSHA}
	p := fmt.Sprintf("/repos/%s/%s/git/refs", owner, repo)
	return c.rest(ctx, http.MethodPost, p, body, nil)
}

// FindOpenPR returns the URL of an open PR from head into base, or "" if none.
func (c *Client) FindOpenPR(ctx context.Context, owner, repo, head, base string) (string, error) {
	var out []struct {
		HTMLURL string `json:"html_url"`
	}
	p := fmt.Sprintf("/repos/%s/%s/pulls?state=open&head=%s:%s&base=%s", owner, repo, owner, head, base)
	if err := c.rest(ctx, http.MethodGet, p, nil, &out); err != nil {
		return "", err
	}
	if len(out) > 0 {
		return out[0].HTMLURL, nil
	}
	return "", nil
}

// CreatePR opens a pull request and returns its HTML URL.
func (c *Client) CreatePR(ctx context.Context, owner, repo, head, base, title, body string) (string, error) {
	req := map[string]any{"title": title, "head": head, "base": base, "body": body}
	var out struct {
		HTMLURL string `json:"html_url"`
	}
	p := fmt.Sprintf("/repos/%s/%s/pulls", owner, repo)
	if err := c.rest(ctx, http.MethodPost, p, req, &out); err != nil {
		return "", err
	}
	return out.HTMLURL, nil
}
