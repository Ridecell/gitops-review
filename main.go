package main

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"regexp"
	"strconv"

	"github.com/bradleyfalzon/ghinstallation"
	"github.com/google/go-github/v28/github"
	"github.com/sourcegraph/go-diff/diff"
)

// NB: This whole thing is very single-task. If we add more to it, probably time to
// make it more than file and beef up the structure.

var appID int
var webhookSecret []byte

// Check if the diff matches the rules for auto-approve (QA or Dev change, only changing `version`).
func diffOkayForAutoApprove(diffData string) (bool, error) {
	diffs, err := diff.ParseMultiFileDiff([]byte(diffData))
	if err != nil {
		return false, err
	}
	// Must be only 1 file.
	if len(diffs) != 1 {
		log.Printf("[autoapprove] Rejecting because not exactly 1 diff: %v", len(diffs))
		return false, nil
	}
	diff := diffs[0]
	// Must be a dev or qa folder.
	pathRE := regexp.MustCompile(`^a/\w+-(dev|qa)/\w+\.yml$`)
	if !pathRE.MatchString(diff.OrigName) {
		log.Printf("[autoapprove] Rejecting because OrigName does not match: %#v", diff.OrigName)
		return false, nil
	}
	// Must have exactly 1 hunk.
	if len(diff.Hunks) != 1 {
		log.Printf("[autoapprove] Rejecting because not exactly 1 hunk: %v", len(diff.Hunks))
		return false, nil
	}
	hunk := diff.Hunks[0]
	// Must change only version field.
	// First check + lines, should be exactly one, starting with `+  version:`.
	plusRE := regexp.MustCompile(`(?m:^\+.*$)`)
	plusLines := plusRE.FindAll(hunk.Body, -1)
	if len(plusLines) != 1 {
		log.Printf("[autoapprove] Rejecting because not exactly 1 plus lines: %v", len(plusLines))
		return false, nil
	}
	lineRE := regexp.MustCompile(`^[+-]\s+version:`)
	if !lineRE.Match(plusLines[0]) {
		log.Printf("[autoapprove] Rejecting because plus line does not match: %s", plusLines[0])
		return false, nil
	}
	// Then check minus lines.
	minusRE := regexp.MustCompile(`(?m:^-.*$)`)
	minusLines := minusRE.FindAll(hunk.Body, -1)
	if len(minusLines) != 1 {
		log.Printf("[autoapprove] Rejecting because not exactly 1 minus lines: %v", len(minusLines))
		return false, nil
	}
	if !lineRE.Match(minusLines[0]) {
		log.Printf("[autoapprove] Rejecting because minus line does not match: %s", minusLines[0])
		return false, nil
	}
	// Should be good.
	return true, nil
}

func approvePullRequest(client *github.Client, event *github.PullRequestEvent) error {
	reviews, _, err := client.PullRequests.ListReviews(context.Background(), *event.Organization.Login, *event.Repo.Name, *event.PullRequest.Number, nil)
	if err != nil {
		return err
	}
	if len(reviews) != 0 {
		// TODO More here to check if the PR is already approved.
		return nil
	}
	approvalMessage := "Automatically approving deploy"
	approvalEvent := "APPROVE"
	approval := &github.PullRequestReviewRequest{
		CommitID: event.PullRequest.Head.SHA,
		Body:     &approvalMessage,
		Event:    &approvalEvent,
	}
	_, _, err = client.PullRequests.CreateReview(context.Background(), *event.Organization.Login, *event.Repo.Name, *event.PullRequest.Number, approval)
	if err != nil {
		return err
	}
	return nil
}

func getClient(installation *github.Installation) (*github.Client, error) {
	clientTransport, err := ghinstallation.NewKeyFromFile(http.DefaultTransport, appID, int(*installation.ID), "conf/private-key.pem")
	if err != nil {
		return nil, err
	}
	client := github.NewClient(&http.Client{Transport: clientTransport})
	return client, nil
}

func handlePullRequestEvent(event *github.PullRequestEvent) error {
	log.Printf("Checking %s", *event.PullRequest.URL)

	if *event.Action != "opened" && *event.Action != "synchronized" {
		log.Printf("Ignoring PR event type %s", *event.Action)
		return nil
	}

	client, err := getClient(event.Installation)
	if err != nil {
		return err
	}

	//  GetRaw(ctx context.Context, owner string, repo string, number int, opt RawOptions
	diffData, _, err := client.PullRequests.GetRaw(context.Background(), *event.Organization.Login, *event.Repo.Name, *event.PullRequest.Number, github.RawOptions{Type: github.Diff})
	if err != nil {
		return err
	}
	okayToApprove, err := diffOkayForAutoApprove(diffData)
	if err != nil {
		return err
	}
	if okayToApprove {
		log.Printf("Approving %s", *event.PullRequest.URL)
		err = approvePullRequest(client, event)
		if err != nil {
			return err
		}
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
		return handlePullRequestEvent(event)
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
	// keyData, err := ioutil.ReadFile("conf/private-key.pem")
	// if err != nil {
	// 	panic(err)
	// }
	// block, _ := pem.Decode([]byte(keyData))
	// key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	// if err != nil {
	// 	panic(err)
	// }

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

	// // Create a new token object, specifying signing method and the claims
	// // you would like it to contain.
	// now := time.Now().Unix()
	// token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
	// 	"iss": appID,
	// 	"iat": now,
	// 	"exp": now + 60,
	// })

	// // Sign and get the complete encoded token as a string using the secret
	// tokenString, err := token.SignedString(key)
	// if err != nil {
	// 	panic(err)
	// }

	// fmt.Println(tokenString)
	// atr, err := ghinstallation.NewAppsTransportKeyFromFile(http.DefaultTransport, appID, "conf/private-key.pem")
	// if err != nil {
	// 	log.Fatal(err)
	// }

	// itr, err := ghinstallation.NewKeyFromFile(http.DefaultTransport, appID, 0, "conf/private-key.pem")
	// if err != nil {
	// 	log.Fatal(err)
	// }

	// Use installation transport with github.com/google/go-github
	// client := github.NewClient(&http.Client{Transport: atr})
	// l, _, err := client.Apps.ListInstallations(context.Background(), nil)
	// fmt.Printf("%#v %s\n", l[0], err)

	http.HandleFunc("/webhook", handleWebhook)
	log.Fatal(http.ListenAndServe(":8000", nil))
}
