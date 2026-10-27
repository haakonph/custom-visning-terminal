package github

import (
	"context"
	"github.com/haakonph/custom-visning-terminal/internal/cache"
	"github.com/haakonph/custom-visning-terminal/internal/pullrequests"
)

type Connection struct {
	Client       *Client
	Cache        pullrequests.Cache
	CacheWarning error
}

// Connect constructs a client and its credential-scoped cache together. Startup
// can use verified cache data offline; interactive setup requests API validation.
func Connect(ctx context.Context, credential string, validate bool) (Connection, error) {
	var result Connection
	var err error
	if credential == "" {
		credential, err = Token(ctx)
		if err != nil {
			return result, err
		}
	}
	result.Client, err = New(credential)
	if err != nil {
		return result, err
	}
	if validate {
		if _, err = result.Client.User(ctx); err != nil {
			return Connection{}, err
		}
	}
	store, cacheErr := cache.Default(credential)
	if cacheErr != nil {
		result.CacheWarning = cacheErr
	} else {
		result.Cache = store
	}
	return result, nil
}
