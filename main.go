package main

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/bradleyfalzon/ghinstallation"
	"github.com/google/go-github/v28/github"
)

// NB: This whole thing is very single-task. If we add more to it, probably time to
// make it more than file and beef up the structure.

var appID int
var webhookSecret []byte

type PullRequestEvent struct {
	PullRequest  *github.PullRequest
	Action       *string
	Installation *github.Installation
	Organization *github.Organization
	Repo         *github.Repository
}

func getClient(installation *github.Installation) (*github.Client, error) {
	clientTransport, err := ghinstallation.NewKeyFromFile(http.DefaultTransport, appID, int(*installation.ID), "conf/private-key.pem")
	if err != nil {
		return nil, err
	}
	client := github.NewClient(&http.Client{Transport: clientTransport})
	return client, nil
}

func handlePullRequestEvent(event *PullRequestEvent) error {
	log.Printf("Checking %s", *event.PullRequest.URL)

	if *event.Action != "opened" && *event.Action != "synchronize" && *event.Action != "submitted" && *event.Action != "dismissed" {
		log.Printf("Ignoring PR event type %s", *event.Action)
		return nil
	}

	client, err := getClient(event.Installation)
	if err != nil {
		return err
	}

	rulesFile, err := fetchRulesFile(client, *event.Organization.Login, *event.Repo.Name)
	if err != nil {
		return err
	}

	rules, err := ParseRules(rulesFile)
	if err != nil {
		return err
	}

	//  GetRaw(ctx context.Context, owner string, repo string, number int, opt RawOptions
	diffData, _, err := client.PullRequests.GetRaw(context.Background(), *event.Organization.Login, *event.Repo.Name, *event.PullRequest.Number, github.RawOptions{Type: github.Diff})
	if err != nil {
		return err
	}

	reviewableFiles, err := ParseDiff([]byte(diffData), *event.Organization.Login, *event.Repo.Name, *event.PullRequest.Head.SHA, *event.PullRequest.Base.SHA)
	if err != nil {
		return err
	}

	// Restrcture
	requiredRules := Rules{
		DefaultReviewer: rules.DefaultReviewer,
	}
	for _, reviewableFile := range reviewableFiles {
		err = reviewableFile.FetchContent(client)
		if err != nil {
			return err
		}

		err = reviewableFile.ParseContent()
		if err != nil {
			return err
		}

		// Match the rules to the file.
		requiredRules.Items = append(requiredRules.Items, rules.MatchRules(*reviewableFile)...)
	}

	// Check if the checks should be pass/failed here
	pullRequestReviews, _, err := client.PullRequests.ListReviews(context.Background(), *event.Organization.Login, *event.Repo.Name, *event.PullRequest.Number, &github.ListOptions{})
	if err != nil {
		return err
	}

	// Determine whether our rule conditions have been satisfied or not.
	unsatisfiedRules, autoMerge, skipReview, err := requiredRules.GetRemainingActions(client, pullRequestReviews)
	if err != nil {
		return err
	}

	fmt.Printf("Automerge: %#v\nSkipReview: %#v\n", autoMerge, skipReview)

	checkConclusion := "success"
	if len(unsatisfiedRules) > 0 {
		checkConclusion = "action_required"
	}

	// Snag our check if it exists
	checkName := "gitops-review"
	checkRuns, _, err := client.Checks.ListCheckRunsForRef(context.Background(), *event.Organization.Login, *event.Repo.Name, *event.PullRequest.Head.SHA, &github.ListCheckRunsOptions{CheckName: &checkName})
	if err != nil {
		return err
	}

	var requiredReviewers []string
	for _, unsatisfiedRule := range unsatisfiedRules {
		requiredReviewers = append(requiredReviewers, unsatisfiedRule.Reviewer)
	}

	checkStatus := "completed"

	checkRunOutputTitle := "gitops-review"
	checkSummary := "Required reviewers"
	checkRunOutputText := fmt.Sprintf("%s\n", strings.Join(requiredReviewers, ", "))

	// If there are no check runs attached to this commit create one.
	if checkRuns.Total == nil || *checkRuns.Total == 0 {
		checkRunOpts := github.CreateCheckRunOptions{
			Name:        checkName,
			Status:      &checkStatus,
			Conclusion:  &checkConclusion,
			CompletedAt: &github.Timestamp{Time: time.Now()},
			HeadSHA:     *event.PullRequest.Head.SHA,
			Output: &github.CheckRunOutput{
				Title:   &checkRunOutputTitle,
				Summary: &checkSummary,
				Text:    &checkRunOutputText,
			},
		}
		_, _, err := client.Checks.CreateCheckRun(context.Background(), *event.Organization.Login, *event.Repo.Name, checkRunOpts)
		if err != nil {
			return err
		}
		return nil
	}

	checkRunForRef := checkRuns.CheckRuns[0]
	checkRunOpts := github.UpdateCheckRunOptions{
		Name:        *checkRunForRef.Name,
		HeadSHA:     event.PullRequest.Head.SHA,
		Status:      &checkStatus,
		Conclusion:  &checkConclusion,
		CompletedAt: &github.Timestamp{Time: time.Now()},
		Output: &github.CheckRunOutput{
			Title:   &checkRunOutputTitle,
			Summary: &checkSummary,
			Text:    &checkRunOutputText,
		},
	}

	_, _, err = client.Checks.UpdateCheckRun(context.Background(), *event.Organization.Login, *event.Repo.Name, *checkRunForRef.ID, checkRunOpts)
	if err != nil {
		return err
	}
	return nil
}

func handleWebhookInternal(w http.ResponseWriter, r *http.Request) error {
	payload, err := github.ValidatePayload(r, webhookSecret)
	if err != nil {
		return err
	}
	event, err := github.ParseWebHook(github.WebHookType(r), payload)
	if err != nil {
		return err
	}

	switch event := event.(type) {
	case *github.PullRequestEvent:
		return handlePullRequestEvent(&PullRequestEvent{PullRequest: event.PullRequest, Action: event.Action, Installation: event.Installation, Organization: event.Organization, Repo: event.Repo})
	case *github.PullRequestReviewEvent:
		return handlePullRequestEvent(&PullRequestEvent{PullRequest: event.PullRequest, Action: event.Action, Installation: event.Installation, Organization: event.Organization, Repo: event.Repo})
	default:
		return fmt.Errorf("Unknown webhook type: %s", github.WebHookType(r))
	}
}

func handleWebhook(w http.ResponseWriter, r *http.Request) {
	err := handleWebhookInternal(w, r)
	if err != nil {
		log.Printf("[error] %s", err.Error())
		http.Error(w, err.Error(), 500)
		return
	}
	_, _ = w.Write([]byte("ok"))
}

func main() {

	appIDBytes, err := ioutil.ReadFile("conf/app-id")
	if err != nil {
		log.Fatal(err)
	}
	appID, err = strconv.Atoi(string(bytes.TrimSpace(appIDBytes)))
	if err != nil {
		log.Fatal(err)
	}
	webhookSecretBytes, err := ioutil.ReadFile("conf/webhook-secret")
	if err != nil {
		log.Fatal(err)
	}
	webhookSecret = bytes.TrimSpace(webhookSecretBytes)

	http.HandleFunc("/webhook", handleWebhook)
	log.Fatal(http.ListenAndServe(":8000", nil))
}
