package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type ParseRulesTestCase struct {
	ruleYaml      []byte
	expectedRules []Rule
	focus         bool
}

func TestParseRules(t *testing.T) {
	cases := []ParseRulesTestCase{
		{
			ruleYaml: []byte(`defaultReviewer: me
rules:
- filter:
    kind: SummonPlatform
    metadata.namespace: summon-dev
  skipReview: true
  autoMerge: true
- filter:
    kind: SummonPlatform
    metadata.namespace: summon-qa
  change:
    spec.version: .*
`),
			expectedRules: []Rule{
				Rule{
					Filter: map[string]string{
						"kind":               "SummonPlatform",
						"metadata.namespace": "summon-dev",
					},
					SkipReview: true,
					AutoMerge:  true,
				},
				Rule{
					Filter: map[string]string{
						"kind":               "SummonPlatform",
						"metadata.namespace": "summon-qa",
					},
					SkipReview: false,
					AutoMerge:  false,
					Change: map[string]string{
						"spec.version": ".*",
					},
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
		rules, err := ParseRules(c.ruleYaml)
		assert.Nil(t, err)
		assert.Equal(t, rules.Items, c.expectedRules)
	}
}

type MatchRulesTestCase struct {
	testLabel       string
	rules           Rules
	file            reviewableFile
	expectedMatches []Rule
	focus           bool
}

func TestRuleMatching(t *testing.T) {
	cases := []MatchRulesTestCase{
		{
			testLabel: "Doesn't match any rules",
			rules: Rules{
				Items: []Rule{
					Rule{
						Filter: map[string]string{
							"kind":               "SummonPlatform",
							"metadata.namespace": "summon-qa",
						},
					},
				},
				DefaultReviewer: "DefaultReviewer",
			},
			file: reviewableFile{
				base: &reviewableContent{
					keys: []map[string]string{
						map[string]string{
							"kind":               "SummonPlatform",
							"metadata.namespace": "summon-dev",
							"spec.version":       "base",
						},
					},
				},
				head: &reviewableContent{
					keys: []map[string]string{
						map[string]string{
							"kind":               "SummonPlatform",
							"metadata.namespace": "summon-qa",
							"spec.version":       "base",
						},
					},
				},
			},
			expectedMatches: []Rule{
				Rule{
					Reviewer: "DefaultReviewer",
				},
			},
		},
		{
			testLabel: "amount of yaml blobs mismatched, match no rules",
			rules: Rules{
				Items: []Rule{
					Rule{
						Filter: map[string]string{
							"kind":               "EncryptedSecret",
							"metadata.namespace": "summon-dev",
						},
					},
				},
				DefaultReviewer: "DefaultReviewer",
			},
			file: reviewableFile{
				base: &reviewableContent{
					keys: []map[string]string{
						map[string]string{
							"kind":               "SummonPlatform",
							"metadata.namespace": "summon-qa",
							"spec.version":       "base",
						},
						map[string]string{
							"kind":               "EncryptedSecret",
							"metadata.namespace": "summon-dev",
							"data":               "base",
						},
					},
				},
				head: &reviewableContent{
					keys: []map[string]string{
						map[string]string{
							"kind":               "SummonPlatform",
							"metadata.namespace": "summon-qa",
							"spec.version":       "head",
						},
					},
				},
			},
			expectedMatches: []Rule{
				Rule{
					Reviewer: "DefaultReviewer",
				},
			},
		},
		{
			testLabel: "skip multiple rules that don't match",
			rules: Rules{
				Items: []Rule{
					Rule{
						Filter: map[string]string{
							"kind":               "SummonPlatform",
							"metadata.namespace": "summon-qa",
						},
						Change: map[string]string{
							"spec.version": ".*",
						},
					},
					Rule{
						Filter: map[string]string{
							"kind":               "SummonPlatform",
							"metadata.namespace": "summon-qa",
						},
					},
					Rule{
						Filter: map[string]string{
							"kind":               ".*",
							"metadata.namespace": "summon-(dev|qa)",
						},
						Change: map[string]string{
							"spec.version": ".*",
						},
					},
					Rule{
						Filter: map[string]string{
							"kind":               "SummonPlatform",
							"metadata.namespace": "summon-dev",
						},
					},
				},
				DefaultReviewer: "DefaultReviewer",
			},
			file: reviewableFile{
				base: &reviewableContent{
					keys: []map[string]string{
						map[string]string{
							"kind":               "SummonPlatform",
							"metadata.namespace": "summon-dev",
							"spec.version":       "base",
							"spec.test":          "base",
						},
					},
				},
				head: &reviewableContent{
					keys: []map[string]string{
						map[string]string{
							"kind":               "SummonPlatform",
							"metadata.namespace": "summon-dev",
							"spec.version":       "base",
							"spec.test":          "head",
						},
					},
				},
			},
			expectedMatches: []Rule{
				Rule{
					Filter: map[string]string{
						"kind":               "SummonPlatform",
						"metadata.namespace": "summon-dev",
					},
				},
			},
		},
		{
			testLabel: "match rules with regex",
			rules: Rules{
				Items: []Rule{
					Rule{
						Filter: map[string]string{
							"kind":               "SummonPlatform",
							"metadata.namespace": "summon-qa",
						},
						SkipReview: false,
						AutoMerge:  false,
						Change: map[string]string{
							"spec.version": ".*",
						},
					},
					Rule{
						Filter: map[string]string{
							"kind":               "Encrypted.*",
							"metadata.namespace": "summon-(dev|qa)",
						},
						SkipReview: true,
						AutoMerge:  true,
					},
				},
				DefaultReviewer: "DefaultReviewer",
			},
			file: reviewableFile{
				base: &reviewableContent{
					keys: []map[string]string{
						map[string]string{
							"kind":               "SummonPlatform",
							"metadata.namespace": "summon-qa",
							"spec.version":       "base",
						},
						map[string]string{
							"kind":               "EncryptedSecret",
							"metadata.namespace": "summon-dev",
							"data":               "base",
						},
					},
				},
				head: &reviewableContent{
					keys: []map[string]string{
						map[string]string{
							"kind":               "SummonPlatform",
							"metadata.namespace": "summon-qa",
							"spec.version":       "head",
						},
						map[string]string{
							"kind":               "EncryptedSecret",
							"metadata.namespace": "summon-dev",
							"data":               "head",
						},
					},
				},
			},
			expectedMatches: []Rule{
				Rule{
					Filter: map[string]string{
						"kind":               "SummonPlatform",
						"metadata.namespace": "summon-qa",
					},
					SkipReview: false,
					AutoMerge:  false,
					Change: map[string]string{
						"spec.version": ".*",
					},
				},
				Rule{
					Filter: map[string]string{
						"kind":               "Encrypted.*",
						"metadata.namespace": "summon-(dev|qa)",
					},
					SkipReview: true,
					AutoMerge:  true,
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
		rules := c.rules.MatchRules(c.file)
		assert.Equal(t, c.expectedMatches, rules)
	}
}
