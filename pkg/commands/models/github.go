package models

type GithubPullRequest struct {
	HeadRefName         string                `json:"headRefName"`
	Number              int                   `json:"number"`
	Title               string                `json:"title"`
	State               string                `json:"state"` // "MERGED", "OPEN", "CLOSED", "DRAFT"
	ChecksState         string                `json:"checksState"`
	Url                 string                `json:"url"`
	HeadRepositoryOwner GithubRepositoryOwner `json:"headRepositoryOwner"`
}

func (pr *GithubPullRequest) UserName() string {
	// e.g. 'jesseduffield'
	return pr.HeadRepositoryOwner.Login
}

func (pr *GithubPullRequest) BranchName() string {
	// e.g. 'feature/my-feature'
	return pr.HeadRefName
}

type GithubRepositoryOwner struct {
	Login string `json:"login"`
}

// GithubPullRequestDetails is the full content of a pull request: its body plus
// the whole discussion (conversation comments and reviews with their inline
// comments) and the state of its CI checks. It's fetched on demand rather than
// as part of the regular refresh.
type GithubPullRequestDetails struct {
	Number    int
	Title     string
	State     string // "OPEN", "CLOSED", "MERGED", "DRAFT"
	Author    string
	CreatedAt string
	Url       string
	Body      string
	Checks    []GithubCheck
	Comments  []GithubComment
	Reviews   []GithubReview
}

// GithubCheck is one CI check on the pull request's head commit, with its state
// normalised to one of "SUCCESS", "FAILURE", "PENDING".
type GithubCheck struct {
	Name  string
	State string
}

type GithubComment struct {
	Author    string
	CreatedAt string
	Body      string
}

type GithubReview struct {
	Author    string
	State     string // "APPROVED", "CHANGES_REQUESTED", "COMMENTED", ...
	CreatedAt string
	Body      string
	Comments  []GithubReviewComment
}

type GithubReviewComment struct {
	Author string
	Path   string
	Body   string
}
