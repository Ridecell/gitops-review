package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type diffTestCase struct {
	diff      string
	filenames []string
	focus     bool
}

type parseContentCase struct {
	testContent  reviewableContent
	expectedKeys []map[string]string
	focus        bool
}

func TestParseDiff(t *testing.T) {
	cases := []diffTestCase{
		{
			diff:      string(""),
			filenames: []string{},
		},
		{
			diff: string(`diff --git a/us-dev/noah.yml b/us-dev/noah.yml
index 8tf4312..8tf4312 100644
--- a/us-dev/noah.yml
+++ b/us-dev/noah.yml
@@ -4,11 +4,11 @@ metadata:
   name: noah-dev
   namespace: summon-dev
 spec:
-  version: test1
+  version: test2
   replicas:
     web: 0
     daphne: 0
-    channelWorker: 0
+    channelWorker: 0
     static: 0
     celeryBeat: 0
     celeryd: 0
diff --git a/us-dev/rex.yml b/us-dev/rex.yml
index 8tf4312..8tf4312 100644
--- a/us-dev/rex.yml
+++ b/us-dev/rex.yml
@@ -4,7 +4,7 @@ metadata:
   name: rex-dev
   namespace: summon-dev
 spec:
-  version: test1
+  version: test2
   aliases:
   - rex-dev.ridecell.com
   waits:`),
			filenames: []string{"/us-dev/noah.yml", "/us-dev/rex.yml"},
		},
	}

	useFocus := false
	for _, c := range cases {
		if c.focus {
			useFocus = true
		}
	}

	for _, c := range cases {
		if useFocus && !c.focus {
			continue
		}
		//_, err := ParseDiff(diffData, *event.Repo.Owner.Name, *event.Repo.Name, *event.PullRequest.Head.SHA, *event.PullRequest.Base.SHA)
		parsedFiles, _ := ParseDiff([]byte(c.diff), "ownerName", "repoName", "headSHA", "baseSHA")

		assert.Equal(t, len(c.filenames), len(parsedFiles))

		for _, file := range parsedFiles {
			var foundFilename bool
			var currentFilename string
			if file.head != nil {
				currentFilename = file.head.path
				assert.Equal(t, file.head.sha, "headSHA")
			} else {
				currentFilename = file.base.path
				assert.Equal(t, file.base.sha, "baseSHA")
			}

			assert.Equal(t, file.owner, "ownerName")
			assert.Equal(t, file.repo, "repoName")

			for _, filename := range c.filenames {
				if currentFilename == filename {
					foundFilename = true
				}
			}
			assert.True(t, foundFilename)
		}
	}
}

func TestParseContent(t *testing.T) {
	cases := []parseContentCase{
		parseContentCase{
			testContent: reviewableContent{
				path: "test.yml",
				content: []byte(`apiVersion: summon.ridecell.io/v1beta1
kind: SummonPlatform
metadata:
  name: rex-dev
  namespace: summon-dev
spec:
  version: test
  config: {}
---
apiVersion: secrets.ridecell.io/v1beta1
kind: EncryptedSecret
metadata:
  name: rex-dev
  namespace: summon-dev
data: {}`),
			},
			expectedKeys: []map[string]string{
				map[string]string{
					"apiVersion":         "summon.ridecell.io/v1beta1",
					"kind":               "SummonPlatform",
					"metadata.name":      "rex-dev",
					"metadata.namespace": "summon-dev",
					"spec.version":       "test",
				},
				map[string]string{
					"apiVersion":         "secrets.ridecell.io/v1beta1",
					"kind":               "EncryptedSecret",
					"metadata.name":      "rex-dev",
					"metadata.namespace": "summon-dev",
					"spec.version":       "test",
				},
			},
		},
	}

	useFocus := false
	for _, c := range cases {
		if c.focus {
			useFocus = true
		}
	}

	for _, c := range cases {
		if useFocus && !c.focus {
			continue
		}

		assert.Nil(t, c.testContent.ParseContent())
		assert.Equal(t, c.testContent.keys, c.expectedKeys)
	}
}

func TestExpand(t *testing.T) {
	content := `
a: b
`
	keys, err := expandYamlFile([]byte(content))
	require.NoError(t, err)
	expected := []map[string]string{
		map[string]string{
			"a": "b",
		},
	}
	assert.Equal(t, expected, keys)
}

func TestExpandNested(t *testing.T) {
	content := `
a:
  b: c
`
	keys, err := expandYamlFile([]byte(content))
	require.NoError(t, err)
	expected := []map[string]string{
		map[string]string{
			"a.b": "c",
		},
	}
	assert.Equal(t, expected, keys)
}

func TestExpandArray(t *testing.T) {
	content := `
a:
- foo
- bar
`
	keys, err := expandYamlFile([]byte(content))
	require.NoError(t, err)
	expected := []map[string]string{
		map[string]string{
			"a.0": "foo",
			"a.1": "bar",
		},
	}
	assert.Equal(t, expected, keys)
}

func TestExpandComplex(t *testing.T) {
	content := `
a:
  b:
  - c
  d: e
f: g
h:
  i:
    j:
      k: l
      m: n # In YAML, this is a False value
    o: 3.14
`
	keys, err := expandYamlFile([]byte(content))
	require.NoError(t, err)
	expected := []map[string]string{
		map[string]string{
			"a.b.0":   "c",
			"a.d":     "e",
			"f":       "g",
			"h.i.j.k": "l",
			"h.i.j.m": "false",
			"h.i.o":   "3.14",
		},
	}
	assert.Equal(t, expected, keys)
}
