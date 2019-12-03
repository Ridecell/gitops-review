package main

import (
	"testing"

	"github.com/google/go-github/v28/github"
	"github.com/stretchr/testify/assert"
)

func TestFetch(t *testing.T) {
	client := github.NewClient(nil)
	data, err := fetchGithubFile(client, "microsoft", "vscode", "LICENSE.txt", "12ab70d329a13dd5b18d892cd40edd7138259bc3")
	assert.Nil(t, err)
	assert.Contains(t, string(data), "Copyright (c) 2015 - present Microsoft Corporation")
}
