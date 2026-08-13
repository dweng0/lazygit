package git_commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/cli/go-gh/v2/pkg/auth"
	"github.com/jesseduffield/lazygit/pkg/commands/hosting_service"
	"github.com/jesseduffield/lazygit/pkg/commands/models"
	"github.com/samber/lo"
	"golang.org/x/sync/errgroup"
)

type GitHubCommands struct {
	*GitCommon
}

func NewGitHubCommands(gitCommon *GitCommon) *GitHubCommands {
	return &GitHubCommands{
		GitCommon: gitCommon,
	}
}

// https://github.com/cli/cli/issues/2300
func (self *GitHubCommands) ConfiguredBaseRemoteName() string {
	// TODO: we only support the (common) case where the value of the config is "base", meaning that
	// the remote's URL determines the GitHub repo. Since `gh repo set-default` on the command line
	// sets the config this way, it's probably good enough in practice, but for completeness it
	// would be nice to also support the case where the config value is a full remote name (e.g.
	// "jesseduffield/lazygit").

	cmdArgs := NewGitCmd("config").
		Arg("--local", "--get-regexp", `remote\..*\.gh-resolved`).
		ToArgv()

	output, _, err := self.cmd.New(cmdArgs).DontLog().RunWithOutputs()
	if err != nil {
		return ""
	}

	regex := regexp.MustCompile(`remote\.(.+)\.gh-resolved`)
	matches := regex.FindStringSubmatch(output)
	if len(matches) < 2 {
		return ""
	}

	return matches[1]
}

func (self *GitHubCommands) SetConfiguredBaseRemoteName(remoteName string) error {
	cmdArgs := NewGitCmd("config").
		Arg("--local", "--add", fmt.Sprintf("remote.%s.gh-resolved", remoteName), "base").
		ToArgv()

	return self.cmd.New(cmdArgs).DontLog().Run()
}

type Response struct {
	Data RepositoryQuery `json:"data"`
}

type RepositoryQuery struct {
	Repository map[string]PullRequest `json:"repository"`
}

type PullRequest struct {
	Edges []PullRequestEdge `json:"edges"`
}

type PullRequestEdge struct {
	Node PullRequestNode `json:"node"`
}

type PullRequestNode struct {
	Title               string                `json:"title"`
	HeadRefName         string                `json:"headRefName"`
	Number              int                   `json:"number"`
	Url                 string                `json:"url"`
	HeadRepositoryOwner GithubRepositoryOwner `json:"headRepositoryOwner"`
	State               string                `json:"state"`
	IsDraft             bool                  `json:"isDraft"`
	HeadRef             GithubRef             `json:"headRef"`
}

type GithubRepositoryOwner struct {
	Login string `json:"login"`
}

type GithubRef struct {
	Target GithubGitObject `json:"target"`
}

type GithubGitObject struct {
	StatusCheckRollup GithubStatusCheckRollup `json:"statusCheckRollup"`
}

type GithubStatusCheckRollup struct {
	State string `json:"state"`
}

type graphQLRequest struct {
	Query     string            `json:"query"`
	Variables map[string]string `json:"variables"`
}

func fetchPullRequestsQuery(branches []string, owner string, repo string) (string, map[string]string) {
	variables := make(map[string]string, len(branches)+2)
	variables["owner"] = owner
	variables["repo"] = repo
	varDecls := make([]string, 0, len(branches)+2)
	varDecls = append(varDecls, "$owner: String!", "$repo: String!")
	queries := make([]string, 0, len(branches))
	for i, branch := range branches {
		// We're making a sub-query per branch, and arbitrarily labelling each subquery
		// as a1, a2, etc.
		fieldName := fmt.Sprintf("a%d", i+1)
		varName := fmt.Sprintf("branch%d", i+1)
		variables[varName] = branch
		varDecls = append(varDecls, fmt.Sprintf("$%s: String!", varName))
		// We fetch a few PRs per branch name because multiple forks may have PRs
		// with the same head ref name. The mapping logic filters by owner later.
		queries = append(queries, fmt.Sprintf(`%s: pullRequests(first: 5, headRefName: $%s, orderBy: {field: CREATED_AT, direction: DESC}) {
      edges {
        node {
          title
          headRefName
          state
          number
          url
          isDraft
          headRef {
            target {
              ... on Commit {
                statusCheckRollup {
                  state
                }
              }
            }
          }
          headRepositoryOwner {
            login
          }
        }
      }
    }`, fieldName, varName))
	}

	queryString := fmt.Sprintf(`query(%s) {
  repository(owner: $owner, name: $repo) {
    %s
  }
}`, strings.Join(varDecls, ", "), strings.Join(queries, "\n"))

	return queryString, variables
}

// GetAuthToken returns the token to authenticate against the given host with,
// or an empty string if there is none.
//
// The token has to come from gh itself rather than from an in-process lookup
// with go-gh: that reads gh's config file once per process and answers from
// that snapshot ever after, whereas gh rewrites the file whenever the active
// account changes, and keeps the active account's token either there or in the
// system keyring. Under a long-running lazygit the snapshot therefore drifts
// out of date, leaving us with a token for an account that is no longer active,
// or with no token at all.
func (self *GitHubCommands) GetAuthToken(host string) string {
	ghExe := ghExecutable()
	if ghExe == "" {
		// Without gh installed, the environment variables and config file that
		// gh would have consulted are still worth a look.
		token, _ := auth.TokenFromEnvOrConfig(host)
		return token
	}

	cmdArgs := []string{ghExe, "auth", "token", "--hostname", host}
	output, _, err := self.cmd.New(cmdArgs).DontLog().RunWithOutputs()
	if err != nil {
		// Not being logged in to this host is a normal state rather than
		// something to report; the runner logs gh's stderr for the rest.
		return ""
	}

	return strings.TrimSpace(output)
}

// ghExecutable returns the path of the gh binary, or an empty string if it
// isn't installed.
func ghExecutable() string {
	if ghExe := os.Getenv("GH_PATH"); ghExe != "" {
		return ghExe
	}

	// A gh found in the current directory rather than on PATH comes back as
	// exec.ErrDot, which we treat as not having found one at all.
	ghExe, err := exec.LookPath("gh")
	if err != nil {
		return ""
	}

	return ghExe
}

// FetchRecentPRs fetches recent pull requests using GraphQL. serviceInfo
// identifies the GitHub instance (github.com or a GitHub Enterprise Server)
// and the owner/repo to query against.
func (self *GitHubCommands) FetchRecentPRs(branches []string, serviceInfo *hosting_service.ServiceInfo, token string) ([]*models.GithubPullRequest, error) {
	endpoint := graphQLEndpoint(serviceInfo.WebDomain)
	t := time.Now()

	var g errgroup.Group

	// We want at most 5 concurrent requests, but no less than 10 branches per request
	concurrency := 5
	minBranchesPerRequest := 10
	branchesPerRequest := max(len(branches)/concurrency, minBranchesPerRequest)
	numChunks := (len(branches) + branchesPerRequest - 1) / branchesPerRequest
	results := make(chan []*models.GithubPullRequest, numChunks)

	for i := 0; i < len(branches); i += branchesPerRequest {
		end := i + branchesPerRequest
		if end > len(branches) {
			end = len(branches)
		}
		branchChunk := branches[i:end]

		// Launch a goroutine for each chunk of branches
		g.Go(func() error {
			prs, err := self.fetchRecentPRsAux(endpoint, serviceInfo.Owner, serviceInfo.Repository, branchChunk, token)
			if err != nil {
				return err
			}
			results <- prs
			return nil
		})
	}

	// Wait for all goroutines, then close the channel so the range loop exits
	err := g.Wait()
	close(results)
	if err != nil {
		return nil, err
	}

	// Collect results from all goroutines
	var allPRs []*models.GithubPullRequest
	for prs := range results {
		allPRs = append(allPRs, prs...)
	}

	self.Log.Infof("Fetched %d PRs in %s", len(allPRs), time.Since(t))

	return allPRs, nil
}

func (self *GitHubCommands) fetchRecentPRsAux(endpoint string, repoOwner string, repoName string, branches []string, token string) ([]*models.GithubPullRequest, error) {
	queryString, variables := fetchPullRequestsQuery(branches, repoOwner, repoName)

	respBytes, err := runGraphQLQuery(endpoint, token, queryString, variables)
	if err != nil {
		return nil, err
	}

	return parsePullRequestsResponse(respBytes)
}

// runGraphQLQuery POSTs a GraphQL query to endpoint, authenticated with token,
// and returns the raw response body.
func runGraphQLQuery(endpoint string, token string, query string, variables map[string]string) ([]byte, error) {
	bodyBytes, err := json.Marshal(graphQLRequest{Query: query, Variables: variables})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Content-Type", "application/json")

	// Bound the request so that a dead or extremely slow network can't leave
	// the caller in flight for minutes. The data is auxiliary, so giving up
	// beats waiting.
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyStr := new(bytes.Buffer)
		_, _ = bodyStr.ReadFrom(resp.Body)
		return nil, fmt.Errorf("GraphQL query failed with status: %s. Body: %s", resp.Status, bodyStr.String())
	}

	return io.ReadAll(resp.Body)
}

func parsePullRequestsResponse(respBytes []byte) ([]*models.GithubPullRequest, error) {
	var result Response
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, err
	}

	prs := []*models.GithubPullRequest{}
	for _, repoQuery := range result.Data.Repository {
		for _, edge := range repoQuery.Edges {
			node := edge.Node
			pr := &models.GithubPullRequest{
				HeadRefName: node.HeadRefName,
				Number:      node.Number,
				Title:       node.Title,
				State:       lo.Ternary(node.IsDraft && node.State != "CLOSED", "DRAFT", node.State),
				ChecksState: node.HeadRef.Target.StatusCheckRollup.State,
				Url:         node.Url,
				HeadRepositoryOwner: models.GithubRepositoryOwner{
					Login: node.HeadRepositoryOwner.Login,
				},
			}
			prs = append(prs, pr)
		}
	}

	return prs, nil
}

// FetchPullRequestDetails fetches the body, full discussion, and CI check state
// of a single pull request. The repo owner, name and host are taken from the
// PR's URL so no extra remote resolution is needed.
func (self *GitHubCommands) FetchPullRequestDetails(pr *models.GithubPullRequest) (*models.GithubPullRequestDetails, error) {
	host, owner, repo, err := parsePullRequestURL(pr.Url)
	if err != nil {
		return nil, err
	}

	token := self.GetAuthToken(host)
	if token == "" {
		return nil, fmt.Errorf("no GitHub auth token available for %s", host)
	}

	query, variables := pullRequestDetailsQuery(owner, repo, pr.Number)

	respBytes, err := runGraphQLQuery(graphQLEndpoint(host), token, query, variables)
	if err != nil {
		return nil, err
	}

	return parsePullRequestDetailsResponse(respBytes)
}

// parsePullRequestURL pulls the host, owner and repo out of a pull request URL
// of the form https://<host>/<owner>/<repo>/pull/<number>.
func parsePullRequestURL(prURL string) (host string, owner string, repo string, err error) {
	u, err := url.Parse(prURL)
	if err != nil {
		return "", "", "", err
	}

	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 4 || parts[2] != "pull" {
		return "", "", "", fmt.Errorf("unexpected pull request URL: %s", prURL)
	}

	return u.Host, parts[0], parts[1], nil
}

func pullRequestDetailsQuery(owner string, repo string, number int) (string, map[string]string) {
	// The number is an int rather than a string variable, so it's interpolated
	// into the query directly; owner and repo stay as variables.
	query := fmt.Sprintf(`query($owner: String!, $repo: String!) {
  repository(owner: $owner, name: $repo) {
    pullRequest(number: %d) {
      number
      title
      state
      isDraft
      url
      createdAt
      author { login }
      body
      comments(first: 100) {
        nodes { author { login } createdAt body }
      }
      reviews(first: 50) {
        nodes {
          author { login }
          state
          createdAt
          body
          comments(first: 50) {
            nodes { author { login } path body }
          }
        }
      }
      commits(last: 1) {
        nodes {
          commit {
            statusCheckRollup {
              contexts(first: 100) {
                nodes {
                  __typename
                  ... on CheckRun { name status conclusion }
                  ... on StatusContext { context state }
                }
              }
            }
          }
        }
      }
    }
  }
}`, number)

	return query, map[string]string{"owner": owner, "repo": repo}
}

type pullRequestDetailsResponse struct {
	Data struct {
		Repository struct {
			PullRequest struct {
				Number    int    `json:"number"`
				Title     string `json:"title"`
				State     string `json:"state"`
				IsDraft   bool   `json:"isDraft"`
				Url       string `json:"url"`
				CreatedAt string `json:"createdAt"`
				Author    struct {
					Login string `json:"login"`
				} `json:"author"`
				Body     string `json:"body"`
				Comments struct {
					Nodes []struct {
						Author    struct{ Login string } `json:"author"`
						CreatedAt string                 `json:"createdAt"`
						Body      string                 `json:"body"`
					} `json:"nodes"`
				} `json:"comments"`
				Reviews struct {
					Nodes []struct {
						Author    struct{ Login string } `json:"author"`
						State     string                 `json:"state"`
						CreatedAt string                 `json:"createdAt"`
						Body      string                 `json:"body"`
						Comments  struct {
							Nodes []struct {
								Author struct{ Login string } `json:"author"`
								Path   string                 `json:"path"`
								Body   string                 `json:"body"`
							} `json:"nodes"`
						} `json:"comments"`
					} `json:"nodes"`
				} `json:"reviews"`
				Commits struct {
					Nodes []struct {
						Commit struct {
							StatusCheckRollup struct {
								Contexts struct {
									Nodes []prCheckContextNode `json:"nodes"`
								} `json:"contexts"`
							} `json:"statusCheckRollup"`
						} `json:"commit"`
					} `json:"nodes"`
				} `json:"commits"`
			} `json:"pullRequest"`
		} `json:"repository"`
	} `json:"data"`
}

type prCheckContextNode struct {
	Typename   string `json:"__typename"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	Context    string `json:"context"`
	State      string `json:"state"`
}

func parsePullRequestDetailsResponse(respBytes []byte) (*models.GithubPullRequestDetails, error) {
	var result pullRequestDetailsResponse
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, err
	}

	pr := result.Data.Repository.PullRequest

	details := &models.GithubPullRequestDetails{
		Number:    pr.Number,
		Title:     pr.Title,
		State:     lo.Ternary(pr.IsDraft && pr.State != "CLOSED", "DRAFT", pr.State),
		Author:    pr.Author.Login,
		CreatedAt: pr.CreatedAt,
		Url:       pr.Url,
		Body:      pr.Body,
	}

	for _, node := range pr.Comments.Nodes {
		details.Comments = append(details.Comments, models.GithubComment{
			Author:    node.Author.Login,
			CreatedAt: node.CreatedAt,
			Body:      node.Body,
		})
	}

	for _, node := range pr.Reviews.Nodes {
		review := models.GithubReview{
			Author:    node.Author.Login,
			State:     node.State,
			CreatedAt: node.CreatedAt,
			Body:      node.Body,
		}
		for _, comment := range node.Comments.Nodes {
			review.Comments = append(review.Comments, models.GithubReviewComment{
				Author: comment.Author.Login,
				Path:   comment.Path,
				Body:   comment.Body,
			})
		}
		details.Reviews = append(details.Reviews, review)
	}

	if len(pr.Commits.Nodes) > 0 {
		for _, node := range pr.Commits.Nodes[0].Commit.StatusCheckRollup.Contexts.Nodes {
			details.Checks = append(details.Checks, checkFromContextNode(node))
		}
	}

	return details, nil
}

// checkFromContextNode normalises a statusCheckRollup context (either a
// GitHub Actions CheckRun or a third-party StatusContext) into a GithubCheck
// with a name and a coarse state.
func checkFromContextNode(node prCheckContextNode) models.GithubCheck {
	if node.Typename == "CheckRun" {
		state := "PENDING"
		if node.Status == "COMPLETED" {
			state = normalizeCheckConclusion(node.Conclusion)
		}
		return models.GithubCheck{Name: node.Name, State: state}
	}

	return models.GithubCheck{Name: node.Context, State: normalizeCheckConclusion(node.State)}
}

// normalizeCheckConclusion collapses the various GitHub conclusion/state values
// into SUCCESS, FAILURE, or PENDING.
func normalizeCheckConclusion(value string) string {
	switch value {
	case "SUCCESS", "NEUTRAL", "SKIPPED":
		return "SUCCESS"
	case "FAILURE", "ERROR", "TIMED_OUT", "CANCELLED", "ACTION_REQUIRED", "STARTUP_FAILURE":
		return "FAILURE"
	default:
		return "PENDING"
	}
}

// returns a map from branch name to pull request
func GenerateGithubPullRequestMap(
	prs []*models.GithubPullRequest,
	branches []*models.Branch,
	remotes []*models.Remote,
) map[string]*models.GithubPullRequest {
	res := map[string]*models.GithubPullRequest{}

	if len(prs) == 0 {
		return res
	}

	remotesToOwnersMap := getRemotesToOwnersMap(remotes)

	// A PR can be identified by two things: the owner e.g. 'jesseduffield' and the
	// branch name e.g. 'feature/my-feature'. The owner might be different
	// to the owner of the repo if the PR is from a fork of that repo.
	type prKey struct {
		owner      string
		branchName string
	}

	prByKey := map[prKey]models.GithubPullRequest{}

	for _, pr := range prs {
		key := prKey{owner: strings.ToLower(pr.UserName()), branchName: pr.BranchName()}
		// PRs are returned newest-first from the API, so the first one we
		// see for each key is the most recent and therefore the most relevant.
		if _, exists := prByKey[key]; !exists {
			prByKey[key] = *pr
		}
	}

	for _, branch := range branches {
		if !branch.IsTrackingRemote() {
			continue
		}

		owner, foundRemoteOwner := remotesToOwnersMap[branch.UpstreamRemote]
		if !foundRemoteOwner {
			// UpstreamRemote may be a full URL rather than a remote name;
			// try parsing the owner directly from it.
			repoInfo, err := hosting_service.GetRepoInfoFromURL(branch.UpstreamRemote)
			if err != nil {
				continue
			}
			owner = repoInfo.Owner
		}

		pr, hasPr := prByKey[prKey{owner: strings.ToLower(owner), branchName: branch.UpstreamBranch}]

		if !hasPr {
			continue
		}

		res[branch.Name] = &pr
	}

	return res
}

func getRemotesToOwnersMap(remotes []*models.Remote) map[string]string {
	res := map[string]string{}
	for _, remote := range remotes {
		if len(remote.Urls) == 0 {
			continue
		}

		repoInfo, err := hosting_service.GetRepoInfoFromURL(remote.Urls[0])
		if err != nil {
			continue
		}

		res[remote.Name] = repoInfo.Owner
	}
	return res
}

// graphQLEndpoint returns the GraphQL API URL for a GitHub host. github.com
// uses a dedicated api. subdomain; GitHub Enterprise Server hangs the API off
// the web host under /api/graphql.
func graphQLEndpoint(host string) string {
	if auth.NormalizeHostname(host) == "github.com" {
		return "https://api.github.com/graphql"
	}
	return "https://" + host + "/api/graphql"
}
