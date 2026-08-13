package presentation

import (
	"testing"

	"github.com/jesseduffield/lazygit/pkg/commands/models"
	"github.com/jesseduffield/lazygit/pkg/i18n"
	"github.com/stretchr/testify/assert"
)

func TestFormatPullRequestDetails(t *testing.T) {
	tr := i18n.EnglishTranslationSet()

	details := &models.GithubPullRequestDetails{
		Number:    42,
		Title:     "Add a browser tab",
		State:     "OPEN",
		Author:    "alice",
		CreatedAt: "2026-01-05T12:00:00Z",
		Body:      "This adds a browser tab.",
		Checks: []models.GithubCheck{
			{Name: "build", State: "SUCCESS"},
			{Name: "lint", State: "FAILURE"},
		},
		Comments: []models.GithubComment{
			{Author: "bob", CreatedAt: "2026-01-06T09:00:00Z", Body: "Looks good to me."},
		},
		Reviews: []models.GithubReview{
			{
				Author: "carol",
				State:  "CHANGES_REQUESTED",
				Body:   "One nit.",
				Comments: []models.GithubReviewComment{
					{Author: "carol", Path: "pkg/main.go", Body: "rename this"},
				},
			},
		},
	}

	output := FormatPullRequestDetails(details, tr)

	assert.Contains(t, output, "Add a browser tab")
	assert.Contains(t, output, "#42")
	assert.Contains(t, output, "alice")
	assert.Contains(t, output, "This adds a browser tab.")
	// checks are listed by name
	assert.Contains(t, output, "build")
	assert.Contains(t, output, "lint")
	// conversation comment
	assert.Contains(t, output, "bob")
	assert.Contains(t, output, "Looks good to me.")
	// review + its inline comment
	assert.Contains(t, output, "carol")
	assert.Contains(t, output, tr.PullRequestReviewChangesRequested)
	assert.Contains(t, output, "pkg/main.go")
	assert.Contains(t, output, "rename this")
}

func TestFormatPullRequestDetailsEmptyBody(t *testing.T) {
	tr := i18n.EnglishTranslationSet()

	details := &models.GithubPullRequestDetails{
		Number: 1,
		Title:  "Title",
		State:  "OPEN",
		Author: "alice",
		Body:   "",
	}

	assert.Contains(t, FormatPullRequestDetails(details, tr), tr.PullRequestNoDescription)
}
