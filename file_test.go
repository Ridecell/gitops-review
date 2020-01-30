package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpand(t *testing.T) {
	content := `
a: b
`
	keys, err := expandYamlFile([]byte(content))
	require.NoError(t, err)
	expected := map[string]string{
		"a": "b",
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
	expected := map[string]string{
		"a.b": "c",
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
	expected := map[string]string{
		"a.0": "foo",
		"a.1": "bar",
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
	expected := map[string]string{
		"a.b.0":   "c",
		"a.d":     "e",
		"f":       "g",
		"h.i.j.k": "l",
		"h.i.j.m": "false",
		"h.i.o":   "3.14",
	}
	assert.Equal(t, expected, keys)
}
