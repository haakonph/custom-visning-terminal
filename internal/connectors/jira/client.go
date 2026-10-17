// Package jira implements read-only Jira Data Center REST API v2 searches.
package jira

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var issueKey = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*-[1-9][0-9]*$`)

type Issue struct {
	Key, URL, Title, Status, Priority, Assignee, Project, Type, Description, Due string
	Labels                                                                       []string
	Updated                                                                      time.Time
}
type Client struct {
	base  *url.URL
	token string
	http  *http.Client
}

func New(base, token string) (*Client, error) {
	u, err := url.Parse(strings.TrimRight(base, "/"))
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("JIRA_BASE_URL must be an HTTPS URL, optionally with a context path")
	}
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("set JIRA_TOKEN to a Jira personal access token")
	}
	return &Client{u, token, &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

// Search retains Jira's ORDER BY and follows the server's actual page size.
// A failed later page returns no replacement dataset; callers keep their cache.
func (c *Client) Search(ctx context.Context, jql string) ([]Issue, error) {
	var result []Issue
	seen := map[string]bool{}
	for start := 0; ; {
		body, err := json.Marshal(map[string]any{"jql": jql, "startAt": start, "maxResults": 100,
			"fields": []string{"summary", "status", "priority", "assignee", "project", "issuetype", "description", "duedate", "labels", "updated"}})
		if err != nil {
			return nil, err
		}
		endpoint := *c.base
		endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/rest/api/2/search"
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
		if err != nil {
			return nil, errors.New("could not create Jira request")
		}
		req.Header.Set("Authorization", "Bearer "+c.token)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		resp, err := c.http.Do(req)
		if err != nil {
			return nil, errors.New("Jira request failed; check network, TLS and server address")
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			switch resp.StatusCode {
			case 400:
				return nil, errors.New("invalid JQL or unsupported field; check the query")
			case 401, 403:
				return nil, errors.New("Jira access denied; check PAT and project permissions")
			case 429:
				return nil, errors.New("Jira rate limit reached; retry later")
			default:
				return nil, fmt.Errorf("Jira returned HTTP %d", resp.StatusCode)
			}
		}
		var page struct {
			StartAt, Total *int
			Issues         []struct {
				Key    string
				Fields struct {
					Summary, Description, Duedate, Updated string
					Labels                                 []string
					Status, Priority, Project, Issuetype   struct {
						Name string
						Key  string
					}
					Assignee *struct{ DisplayName string }
				}
			}
		}
		err = json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(&page)
		resp.Body.Close()
		if err != nil {
			return nil, errors.New("invalid Jira search response")
		}
		if page.StartAt == nil || page.Total == nil || *page.Total < 0 || page.Issues == nil {
			return nil, errors.New("incomplete Jira search response")
		}
		if *page.StartAt != start {
			return nil, errors.New("Jira returned an unexpected page")
		}
		for _, row := range page.Issues {
			if !issueKey.MatchString(row.Key) {
				return nil, errors.New("invalid Jira issue key")
			}
			if seen[row.Key] {
				continue
			}
			seen[row.Key] = true
			f := row.Fields
			updated, _ := time.Parse("2006-01-02T15:04:05.000-0700", f.Updated)
			if updated.IsZero() {
				updated, _ = time.Parse(time.RFC3339Nano, f.Updated)
			}
			link := *c.base
			link.Path = strings.TrimRight(link.Path, "/") + "/browse/" + row.Key
			assignee := "Unassigned"
			if f.Assignee != nil {
				assignee = f.Assignee.DisplayName
			}
			result = append(result, Issue{Key: row.Key, URL: link.String(), Title: f.Summary, Status: f.Status.Name,
				Priority: f.Priority.Name, Assignee: assignee, Project: f.Project.Key, Type: f.Issuetype.Name,
				Description: f.Description, Due: f.Duedate, Labels: f.Labels, Updated: updated})
		}
		start += len(page.Issues)
		if start >= *page.Total {
			return result, nil
		}
		if len(page.Issues) == 0 {
			return nil, errors.New("incomplete Jira results; previous data retained")
		}
	}
}

type Filters struct{ Project, Status, Assignee, Priority, Type, Sprint, Epic, Label string }

func quote(s string) string {
	return "\"" + strings.NewReplacer("\\", "\\\\", "\"", "\\\"").Replace(s) + "\""
}
func (f Filters) JQL(base string) string {
	// Standard queries have a known ORDER BY; custom JQL is left intact by the UI.
	parts := strings.SplitN(base, " ORDER BY ", 2)
	clauses := []string{"(" + parts[0] + ")"}
	for _, pair := range [][2]string{{"project", f.Project}, {"status", f.Status}, {"assignee", f.Assignee}, {"priority", f.Priority}, {"issuetype", f.Type}, {"sprint", f.Sprint}, {"\"Epic Link\"", f.Epic}, {"labels", f.Label}} {
		if pair[1] != "" {
			clauses = append(clauses, pair[0]+" = "+quote(pair[1]))
		}
	}
	query := strings.Join(clauses, " AND ")
	if len(parts) == 2 {
		query += " ORDER BY " + parts[1]
	}
	return query
}

// ValidIssueURL limits browser opening to the configured Jira origin and context.
func (c *Client) ValidIssueURL(link string) bool {
	u, err := url.Parse(link)
	if err != nil || u.User != nil || u.Scheme != c.base.Scheme || u.Host != c.base.Host || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	prefix := strings.TrimRight(c.base.Path, "/") + "/browse/"
	key := strings.TrimPrefix(u.Path, prefix)
	return strings.HasPrefix(u.Path, prefix) && issueKey.MatchString(key)
}
