package main

import (
	"bytes"
	"fmt"
	"regexp"

	"github.com/google/go-github/v28/github"
	"github.com/pkg/errors"
	"github.com/sourcegraph/go-diff/diff"
	"gopkg.in/yaml.v2"
)

type reviewableContent struct {
	name    string
	keys    map[string]string
	diff    map[string]valueDiff
}

type reviewableFile struct {
	path string
	headPath string
	basePath string
	content []byte
	// blob "name" -> content
	blobs map[string]*reviewableContent

	// For fetching.
	owner     string
	repo      string
	sha string
}

type reviewableVersion struct {
	sha       string
	// path -> reviewableFile
	files map[string]*reviewableFile
}

type valueDiff struct {
	head *string
	base *string
}

type reviewablePatch struct {
	owner     string
	repo      string
	head reviewableVersion
	base reviewableVersion
}


// Paths which are probably YAML.
var yamlPathRegexp *regexp.Regexp

func init() {
	yamlPathRegexp = regexp.MustCompile(`\.ya?ml$`)
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
func expandYamlFile(content []byte) ([]map[string]string, error) {
	r := bytes.NewReader(content)
	dec := yaml.NewDecoder(r)

	allKeys := []map[string]string{}
	data := map[interface{}]interface{}{}

	for dec.Decode(&data) == nil {
		keys := map[string]string{}
		expandYamlFileVisit(keys, "", data)
		allKeys = append(allKeys, keys)
	}
	return allKeys, nil
}

func ParseDiff(diffData []byte, owner, repo, headSHA, baseSHA string) (*reviewablePatch, error) {
	diffs, err := diff.ParseMultiFileDiff(diffData)
	if err != nil {
		return nil, errors.Wrap(err, "error parsing diff")
	}
	review := &reviewablePatch{owner: owner, repo: repo}
	review.head.sha = headSHA
	review.base.sha = baseSHA

	for _, diff := range diffs {
		// [1:] to trim off the a or b
		headPath := diff.NewName[1:]
		basePath := diff.OrigName[1:]
		if diff.NewName != "/dev/null" {
			review.head.files[headPath] = &reviewableContent{
				path: headPath,
				headPath: headPath,
				basePath: basePath,
				sha: headSHA,
				owner: owner,
				repo, repo,
			}
		}
		if diff.OrigName != "/dev/null" {
			review.base.files[basePath] = &reviewableContent{
				path: basePath,
				headPath: headPath,
				basePath: basePath,
				sha: baseSHA,
				owner: owner,
				repo, repo,
			}
		}
	}
	return review, nil
}

func (f *reviewableFile) FetchContent(client *github.Client) error {
	content, err := fetchGithubFile(client, f.owner, f.repo, f.path, f.sha)
	if err != nil {
		return err
	}
	c.content = content
	return nil
}

func (v *reviewableVersion) FetchContent(client *github.Client) error {
	for _, file := range v.files {
		err := file.FetchContent(client)
		if err != nil {
			return errors.Wrapf("error fetching content for %s", file.path)
		}
	}
	return nil
}

func (p *reviewablePatch) FetchContent(client *github.Client) error {
	err := p.head.FetchContent(client)
	if err != nil {
		return errors.Wrap("error fetching head content")
	}
	err = p.base.FetchContent(client)
	if err != nil {
		return errors.Wrap("error fetching base content")
	}
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
