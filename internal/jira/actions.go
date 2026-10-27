package jira

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Transition IDs belong to the issue's workflow and must never be hard-coded.
type Transition struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	To   struct {
		Name string `json:"name"`
	} `json:"to"`
}

func (c *Client) request(ctx context.Context, method, path string, body any, result any) error {
	var data bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&data).Encode(body); err != nil {
			return err
		}
	}
	endpoint := *c.base
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/rest/api/2/" + path
	req, err := http.NewRequestWithContext(ctx, method, endpoint.String(), &data)
	if err != nil {
		return errors.New("could not create Jira request")
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return errors.New("Jira request failed; check network, TLS and server address")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Jira action failed (HTTP %d); check permissions and workflow requirements", resp.StatusCode)
	}
	if result != nil {
		if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(result); err != nil {
			return errors.New("invalid Jira action response")
		}
	}
	return nil
}
func (c *Client) Transitions(ctx context.Context, key string) ([]Transition, error) {
	if !issueKey.MatchString(key) {
		return nil, errors.New("invalid Jira issue key")
	}
	var response struct {
		Transitions []Transition `json:"transitions"`
	}
	err := c.request(ctx, http.MethodGet, "issue/"+key+"/transitions", nil, &response)
	return response.Transitions, err
}
func (c *Client) Transition(ctx context.Context, key, id string) error {
	if !issueKey.MatchString(key) || id == "" {
		return errors.New("invalid Jira transition")
	}
	return c.request(ctx, http.MethodPost, "issue/"+key+"/transitions", map[string]any{"transition": map[string]string{"id": id}}, nil)
}
func (c *Client) Assign(ctx context.Context, key string, me bool) error {
	if !issueKey.MatchString(key) {
		return errors.New("invalid Jira issue key")
	}
	var name any
	{
		var user struct {
			Name string `json:"name"`
		}
		if err := c.request(ctx, http.MethodGet, "myself", nil, &user); err != nil {
			return err
		}
		if user.Name == "" {
			return errors.New("Jira did not return the current username")
		}
		if me {
			name = user.Name
		} else {
			var issue struct {
				Fields struct {
					Assignee *struct {
						Name string `json:"name"`
					} `json:"assignee"`
				} `json:"fields"`
			}
			if err := c.request(ctx, http.MethodGet, "issue/"+key, nil, &issue); err != nil {
				return err
			}
			if issue.Fields.Assignee == nil {
				return nil
			}
			if issue.Fields.Assignee.Name != user.Name {
				return errors.New("Jira issue is assigned to someone else")
			}
		}
	}
	return c.request(ctx, http.MethodPut, "issue/"+key+"/assignee", map[string]any{"name": name}, nil)
}
