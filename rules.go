package main

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/google/go-github/v28/github"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

type Rule struct {
	Filter     map[string]string `yaml:"filter,omitempty"`
	Change     map[string]string `yaml:"change,omitempty"`
	Reviewer   string            `yaml:"reviewer,omitempty"`
	SkipReview bool              `yaml:"skipReview,omitempty"`
	AutoMerge  bool              `yaml:"autoMerge,omitempty"`
}

type Rules struct {
	Items           []Rule `yaml:"rules"`
	DefaultReviewer string `yaml:"defaultReviewer"`
}

func ParseRules(rulesContent []byte) (Rules, error) {
	allRules := Rules{}
	err := yaml.Unmarshal(rulesContent, &allRules)
	if err != nil {
		return Rules{}, errors.Wrap(err, "error parsing YAML content")
	}
	return allRules, nil
}

func (rules Rules) GetRemainingActions(client *github.Client, approvingReviews []*github.PullRequestReview) ([]Rule, bool, bool, error) {

	var unsatisfiedRules []Rule
	autoMerge := true
	skipReview := true

	for _, requiredRule := range rules.Items {
		if requiredRule.AutoMerge == false {
			autoMerge = false
		}

		if requiredRule.SkipReview == false {
			skipReview = false
		}

		// Default the reviwer if not present
		if requiredRule.Reviewer == "" {
			requiredRule.Reviewer = rules.DefaultReviewer
		}

		satisfied, err := requiredRule.Satisfied(client, approvingReviews)
		if err != nil {
			return nil, false, false, err
		}
		if !satisfied {
			unsatisfiedRules = append(unsatisfiedRules, requiredRule)
		}
	}
	return unsatisfiedRules, autoMerge, skipReview, nil
}

func (rule Rule) Satisfied(client *github.Client, approvingReviews []*github.PullRequestReview) (bool, error) {
	// Making massive assumption here
	// If "/" present in reviewer name it is a team
	var requiredReviewerIsTeam bool
	if strings.Contains(rule.Reviewer, "/") {
		requiredReviewerIsTeam = true
	}

	for _, approvingReview := range approvingReviews {
		// If the review is dismissed it don't count
		if *approvingReview.State == "DISMISSED" {
			continue
		}
		// If the rule doesn't require a team review skip team membership bits
		if !requiredReviewerIsTeam {
			if rule.Reviewer == *approvingReview.User.Name {
				return true, nil
			}
			continue
		}

		splitReviewer := strings.Split(rule.Reviewer, "/")
		orgName := splitReviewer[0]
		teamSlug := splitReviewer[1]

		team, _, err := client.Teams.GetTeamBySlug(context.Background(), orgName, teamSlug)
		if err != nil {
			return false, err
		}

		fmt.Printf("%#v\n", *team.ID)
		membership, _, err := client.Teams.GetTeamMembership(context.Background(), *team.ID, *approvingReview.User.Login)
		if err != nil || *membership.State != "active" {
			return false, err
		}
		// If we got this far the user is part of the team
		return true, nil
	}
	return false, nil
}

func (rule Rule) MatchRule(baseKeys map[string]string, headKeys map[string]string) bool {
	if reflect.DeepEqual(baseKeys, headKeys) {
		return false
	}
	for filterKey, filterContent := range rule.Filter {
		fileContent, ok := baseKeys[filterKey]
		if !ok {
			return false
		}

		match, err := regexp.MatchString(filterContent, fileContent)
		if err != nil || !match {
			return false
		}
	}

	for changeKey, changeRegex := range rule.Change {
		baseContent, foundInBase := baseKeys[changeKey]
		headContent, foundInHead := headKeys[changeKey]

		// If we can't find the key or the two values match
		if !foundInBase || !foundInHead || baseContent == headContent {
			return false
		}

		match, err := regexp.MatchString(changeRegex, headContent)
		if err != nil || !match {
			return false
		}
	}
	return true
}

func (rules Rules) MatchRules(targetFile reviewableFile) []Rule {
	matchedRules := []Rule{}

	defaultReviewerRule := []Rule{
		Rule{
			Reviewer: rules.DefaultReviewer,
		},
	}

	// comparison is hard here, default the reviewer?
	if len(targetFile.base.keys) != len(targetFile.head.keys) {
		return defaultReviewerRule
	}

	for keyIndex := 0; keyIndex < len(targetFile.base.keys); keyIndex++ {
		for _, rule := range rules.Items {
			if rule.MatchRule(targetFile.base.keys[keyIndex], targetFile.head.keys[keyIndex]) {
				matchedRules = append(matchedRules, rule)
				break
			}
		}
	}
	// No rules matched, force a review from default reviewer
	if len(matchedRules) == 0 {
		return defaultReviewerRule
	}
	return matchedRules
}
