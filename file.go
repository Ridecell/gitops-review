package main

import (
	"fmt"
	"regexp"

	"github.com/google/go-github/v28/github"
	"github.com/pkg/errors"
	"github.com/sourcegraph/go-diff/diff"
	"gopkg.in/yaml.v2"
)

type reviewableContent struct {
	path    string
	sha     string
	content []byte
	keys    map[string]string
}

type reviewableFile struct {
	owner string
	repo  string
	head  *reviewableContent
	base  *reviewableContent
}

// Paths which are probably YAML.
var yamlPathRegexp *regexp.Regexp

func init() {
	yamlPathRegexp = regexp.MustCompile(`\.yam?l$`)
}

// Visitor function for expandYamlFile.
func expandYamlFileVisit(keys map[string]string, prefix string, obj interface{}) {
	mapObj, ok := obj.(map[interface{}]interface{})
	if ok {
		for k, v := range mapObj {
			var newPrefix string
			if prefix == "" {
				newPrefix = fmt.Sprintf("%v", k)
			} else {
				newPrefix = fmt.Sprintf("%s.%v", prefix, k)
			}
			expandYamlFileVisit(keys, newPrefix, v)
		}
		return
	}
	arrObj, ok := obj.([]interface{})
	if ok {
		for i, v := range arrObj {
			var newPrefix string
			if prefix == "" {
				newPrefix = fmt.Sprintf("%d", i)
			} else {
				newPrefix = fmt.Sprintf("%s.%d", prefix, i)
			}
			expandYamlFileVisit(keys, newPrefix, v)
		}
		return
	}
	keys[prefix] = fmt.Sprintf("%v", obj)
}

// Convert
func expandYamlFile(content []byte) (map[string]string, error) {
	data := map[interface{}]interface{}{}
	err := yaml.Unmarshal(content, &data)
	if err != nil {
		return nil, errors.Wrap(err, "error parsing YAML content")
	}
	keys := map[string]string{}
	expandYamlFileVisit(keys, "", data)
	return keys, nil
}

func ParseDiff(diffData []byte, owner, repo, headSHA, baseSHA string) ([]*reviewableFile, error) {
	diffs, err := diff.ParseMultiFileDiff(diffData)
	if err != nil {
		return nil, errors.Wrap(err, "error parsing diff")
	}
	files := []*reviewableFile{}
	for _, diff := range diffs {
		file := &reviewableFile{
			owner: owner,
			repo:  repo,
		}
		if diff.NewName != "/dev/null" {
			file.head = &reviewableContent{
				path: diff.NewName,
				sha:  headSHA,
			}
		}
		if diff.OrigName != "/dev/null" {
			file.base = &reviewableContent{
				path: diff.OrigName,
				sha:  baseSHA,
			}
		}
		files = append(files, file)
	}
	return files, nil
}

func (f *reviewableFile) FetchContent(client *github.Client) error {
	if f.head != nil {
		err := f.head.FetchContent(client, f.owner, f.repo)
		if err != nil {
			return errors.Wrap(err, "error fetching head content")
		}
	}
	if f.base != nil {
		err := f.base.FetchContent(client, f.owner, f.repo)
		if err != nil {
			return errors.Wrap(err, "error fetching base content")
		}
	}
	return nil
}

func (c *reviewableContent) FetchContent(client *github.Client, owner, repo string) error {
	content, err := fetchGithubFile(client, owner, repo, c.path, c.sha)
	if err != nil {
		return err
	}
	c.content = content
	return nil
}

func (f *reviewableFile) ParseContent() error {
	if f.head != nil {
		err := f.head.ParseContent()
		if err != nil {
			return errors.Wrap(err, "error parsing head content")
		}
	}
	if f.base != nil {
		err := f.base.ParseContent()
		if err != nil {
			return errors.Wrap(err, "error parsing base content")
		}
	}
	return nil
}

func (c *reviewableContent) ParseContent() error {
	if !yamlPathRegexp.MatchString(c.path) {
		// Not a YAML file, don't even try.
		return nil
	}
	keys, err := expandYamlFile(c.content)
	if err != nil {
		return errors.Wrapf(err, "error parsing file %s@%s", c.path, c.sha)
	}
	c.keys = keys
	return nil
}
