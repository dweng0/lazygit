package presentation

import (
	"fmt"
	"strings"

	"github.com/jesseduffield/lazygit/pkg/commands/models"
	"github.com/jesseduffield/lazygit/pkg/gui/style"
	"github.com/jesseduffield/lazygit/pkg/i18n"
)

// FormatPullRequestDetails renders a pull request's header, CI checks, body, and
// full discussion (conversation comments and reviews with their inline
// comments) as a scrollable block for the main view.
func FormatPullRequestDetails(details *models.GithubPullRequestDetails, tr *i18n.TranslationSet) string {
	var b strings.Builder

	b.WriteString(coloredPullRequestStateText(details.State))
	b.WriteString("  ")
	b.WriteString(style.AttrBold.Sprint(details.Title))
	b.WriteString("  ")
	b.WriteString(style.FgCyan.Sprintf("#%d\n", details.Number))
	b.WriteString(style.FgYellow.Sprint(prAuthorName(details.Author)))
	if date := formatPrDate(details.CreatedAt); date != "" {
		b.WriteString(style.FgCyan.Sprintf(" · %s", date))
	}
	b.WriteString("\n")

	if len(details.Checks) > 0 {
		b.WriteString("\n" + prSectionHeader(tr.PullRequestChecksHeader))
		for _, check := range details.Checks {
			icon, _, textStyle := checksStatePresentation(check.State, tr)
			b.WriteString(fmt.Sprintf("  %s %s\n", textStyle.Sprint(icon), check.Name))
		}
	}

	b.WriteString("\n")
	if body := strings.TrimSpace(details.Body); body != "" {
		b.WriteString(body + "\n")
	} else {
		b.WriteString(style.FgCyan.Sprint(tr.PullRequestNoDescription) + "\n")
	}

	if len(details.Comments) > 0 {
		b.WriteString("\n" + prSectionHeader(tr.PullRequestCommentsHeader))
		for _, comment := range details.Comments {
			b.WriteString(prCommentBlock(comment.Author, comment.CreatedAt, comment.Body))
		}
	}

	if len(details.Reviews) > 0 {
		b.WriteString("\n" + prSectionHeader(tr.PullRequestReviewsHeader))
		for _, review := range details.Reviews {
			b.WriteString(prReviewBlock(review, tr))
		}
	}

	return b.String()
}

func prSectionHeader(title string) string {
	return style.AttrBold.Sprint(title) + "\n" + strings.Repeat("─", 50) + "\n"
}

func prCommentBlock(author string, createdAt string, body string) string {
	header := style.FgYellow.Sprint(prAuthorName(author))
	if date := formatPrDate(createdAt); date != "" {
		header += style.FgCyan.Sprintf(" · %s", date)
	}

	return "\n" + header + "\n" + strings.TrimSpace(body) + "\n"
}

func prReviewBlock(review models.GithubReview, tr *i18n.TranslationSet) string {
	var b strings.Builder

	header := style.FgYellow.Sprint(prAuthorName(review.Author)) + " " + coloredReviewState(review.State, tr)
	if date := formatPrDate(review.CreatedAt); date != "" {
		header += style.FgCyan.Sprintf(" · %s", date)
	}
	b.WriteString("\n" + header + "\n")

	if body := strings.TrimSpace(review.Body); body != "" {
		b.WriteString(body + "\n")
	}

	for _, comment := range review.Comments {
		b.WriteString(style.FgCyan.Sprintf("  %s\n", comment.Path))
		b.WriteString(indentLines(strings.TrimSpace(comment.Body), "    ") + "\n")
	}

	return b.String()
}

func coloredReviewState(state string, tr *i18n.TranslationSet) string {
	switch state {
	case "APPROVED":
		return style.FgGreen.Sprint(tr.PullRequestReviewApproved)
	case "CHANGES_REQUESTED":
		return style.FgRed.Sprint(tr.PullRequestReviewChangesRequested)
	case "DISMISSED":
		return style.FgCyan.Sprint(tr.PullRequestReviewDismissed)
	default:
		return style.FgCyan.Sprint(tr.PullRequestReviewCommented)
	}
}

func prAuthorName(login string) string {
	if login == "" {
		return "ghost"
	}
	return login
}

// formatPrDate reduces an ISO-8601 timestamp to its date part, and returns the
// empty string unchanged.
func formatPrDate(timestamp string) string {
	if len(timestamp) >= 10 {
		return timestamp[:10]
	}
	return timestamp
}

func indentLines(text string, prefix string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}
