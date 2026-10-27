package jira

import "context"

func Connect(ctx context.Context, base, token string, validate bool) (*Client, error) {
	client, err := New(base, token)
	if err != nil {
		return nil, err
	}
	if validate {
		_, err = client.Search(ctx, "assignee = currentUser() AND resolution = Unresolved")
		if err != nil {
			return nil, err
		}
	}
	return client, nil
}
