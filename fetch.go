package main

import (
	"bytes"
	"context"
	"fmt"

	"github.com/google/go-github/v28/github"
	lru "github.com/hashicorp/golang-lru"
	"github.com/pkg/errors"
)

// Global LRU cache of recently accessed files to make Github less mad at us.
var githubCache *lru.TwoQueueCache

func init() {
	var err error
	githubCache, err = lru.New2Q(128)
	if err != nil {
		panic(err)
	}
}

// Download a file from Github at a given revision.
func fetchGithubFile(client *github.Client, owner, repo, path, sha string) ([]byte, error) {
	cacheKey := fmt.Sprintf("%s/%s/%s@%s", owner, repo, path, sha)
	cached, ok := githubCache.Get(cacheKey)
	if ok {
		return cached.([]byte), nil
	}

	reader, err := client.Repositories.DownloadContents(context.Background(), owner, repo, path, &github.RepositoryContentGetOptions{Ref: sha})
	if err != nil {
		return nil, errors.Wrapf(err, "error downloading content from %s", cacheKey)
	}
	defer reader.Close()

	var content bytes.Buffer
	_, err = content.ReadFrom(reader)
	if err != nil {
		return nil, errors.Wrapf(err, "error reading content from %s", cacheKey)
	}

	return content.Bytes(), nil
}
