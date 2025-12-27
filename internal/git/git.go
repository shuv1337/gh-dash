package git

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	gitm "github.com/aymanbagabas/git-module"

	"github.com/dlvhdr/gh-dash/v4/internal/utils"
)

// Extends git.Repository
type Repo struct {
	gitm.Repository
	Origin         string
	Remotes        []string
	Branches       []Branch
	HeadBranchName string
	Status         gitm.NameStatus
}

type Branch struct {
	Name          string
	LastUpdatedAt *time.Time
	CreatedAt     *time.Time
	LastCommitMsg *string
	CommitsAhead  int
	CommitsBehind int
	IsCheckedOut  bool
	Remotes       []string
}

func GetOriginUrl(dir string) (string, error) {
	repo, err := gitm.Open(dir)
	if err != nil {
		return "", err
	}
	remotes, err := repo.Remotes()
	if err != nil {
		return "", err
	}

	for _, remote := range remotes {
		if remote != "origin" {
			continue
		}

		urls, err := gitm.RemoteGetURL(dir, remote)
		if err != nil || len(urls) == 0 {
			return "", err
		}
		return urls[0], nil
	}

	return "", errors.New("no origin remote found")
}

func GetRepo(dir string) (*Repo, error) {
	repo, err := gitm.Open(dir)
	if err != nil {
		return nil, err
	}

	bNames, err := repo.Branches()
	if err != nil {
		return nil, err
	}

	headRef, err := repo.RevParse("HEAD", gitm.RevParseOptions{
		CommandOptions: gitm.CommandOptions{Args: []string{"--abbrev-ref"}},
	})
	if err != nil {
		return nil, err
	}
	status, err := getUnstagedStatus(repo)
	if err != nil {
		return nil, err
	}

	branches := make([]Branch, len(bNames))
	for i, b := range bNames {
		var updatedAt *time.Time
		var lastCommitMsg *string
		isHead := b == headRef
		commits, err := gitm.Log(dir, b, gitm.LogOptions{MaxCount: 1})
		if err == nil && len(commits) > 0 {
			updatedAt = &commits[0].Committer.When
			lastCommitMsg = utils.StringPtr(commits[0].Summary())
		}
		commitsAhead, err := repo.RevListCount([]string{fmt.Sprintf("origin/%s..%s", b, b)})
		if err != nil {
			commitsAhead = 0
		}
		commitsBehind, err := repo.RevListCount([]string{fmt.Sprintf("%s..origin/%s", b, b)})
		if err != nil {
			commitsBehind = 0
		}
		remotes, _ := repo.RemoteGetURL(b)
		branches[i] = Branch{
			Name:          b,
			LastUpdatedAt: updatedAt,
			CreatedAt:     updatedAt,
			IsCheckedOut:  isHead,
			Remotes:       remotes,
			LastCommitMsg: lastCommitMsg,
			CommitsAhead:  int(commitsAhead),
			CommitsBehind: int(commitsBehind),
		}
	}
	sort.Slice(branches, func(i, j int) bool {
		if branches[j].LastUpdatedAt == nil || branches[i].LastUpdatedAt == nil {
			return false
		}
		return branches[i].LastUpdatedAt.After(*branches[j].LastUpdatedAt)
	})

	headBranch, err := repo.SymbolicRef()
	if err != nil {
		return nil, err
	}
	headBranch, _ = strings.CutPrefix(headBranch, gitm.RefsHeads)

	remotes, err := repo.Remotes(gitm.RemotesOptions{CommandOptions: gitm.CommandOptions{Args: []string{"show"}}})
	if err != nil {
		return nil, err
	}
	origin, err := gitm.RemoteGetURL(dir, "origin", gitm.RemoteGetURLOptions{All: true})
	if err != nil {
		return nil, err
	}

	return &Repo{
		Repository: *repo, Origin: origin[0], Remotes: remotes,
		HeadBranchName: headBranch, Branches: branches, Status: status,
	}, nil
}

func GetStatus(dir string) (gitm.NameStatus, error) {
	repo, err := gitm.Open(dir)
	if err != nil {
		return gitm.NameStatus{}, err
	}
	return getUnstagedStatus(repo)
}

// test
func getUnstagedStatus(repo *gitm.Repository) (gitm.NameStatus, error) {
	cmd := gitm.NewCommand("diff", "HEAD", "--name-status")
	stdout, err := cmd.RunInDir(repo.Path())
	if err != nil {
		return gitm.NameStatus{}, err
	}
	status := gitm.NameStatus{}
	scanner := bufio.NewScanner(bytes.NewReader(stdout))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}

		switch fields[0][0] {
		case 'A':
			status.Added = append(status.Added, fields[1])
		case 'D':
			status.Removed = append(status.Removed, fields[1])
		case 'M':
			status.Modified = append(status.Modified, fields[1])
		}
	}
	return status, err
}

func FetchRepo(dir string) (*Repo, error) {
	repo, err := gitm.Open(dir)
	if err != nil {
		return nil, err
	}
	err = repo.Fetch(gitm.FetchOptions{CommandOptions: gitm.CommandOptions{Args: []string{"--all"}}})
	if err != nil {
		return nil, err
	}
	return GetRepo(dir)
}

func GetRepoInPwd() (*gitm.Repository, error) {
	return gitm.Open(".")
}

func GetRepoShortName(url string) string {
	r, _ := strings.CutPrefix(url, "https://github.com/")
	r, _ = strings.CutSuffix(r, ".git")
	return r
}

// GetUpstreamUrl returns the URL of the "upstream" remote if it exists.
// This is typically the parent repository when working in a fork.
func GetUpstreamUrl(dir string) (string, error) {
	repo, err := gitm.Open(dir)
	if err != nil {
		return "", err
	}
	remotes, err := repo.Remotes()
	if err != nil {
		return "", err
	}

	for _, remote := range remotes {
		if remote != "upstream" {
			continue
		}

		urls, err := gitm.RemoteGetURL(dir, remote)
		if err != nil || len(urls) == 0 {
			return "", err
		}
		return urls[0], nil
	}

	return "", errors.New("no upstream remote found")
}

// ParseGitHubRepoFromUrl extracts the owner and repo name from a GitHub URL
// Supports formats:
//   - HTTPS: https://github.com/owner/repo.git, https://github.enterprise.com/owner/repo
//   - SSH: git@github.com:owner/repo.git, git@github.enterprise.com:owner/repo.git
func ParseGitHubRepoFromUrl(remoteUrl string) (owner, name string, err error) {
	remoteUrl = strings.TrimSpace(remoteUrl)
	remoteUrl = strings.TrimSuffix(remoteUrl, "/")

	// Handle SSH format: git@host:owner/repo.git
	if strings.HasPrefix(remoteUrl, "git@") {
		// Find the colon that separates host from path
		colonIdx := strings.Index(remoteUrl, ":")
		if colonIdx == -1 {
			return "", "", errors.New("invalid SSH URL format: missing colon separator")
		}
		path := remoteUrl[colonIdx+1:]
		path = strings.TrimSuffix(path, ".git")
		path = strings.TrimPrefix(path, "/") // Handle git@host:/owner/repo format
		parts := strings.Split(path, "/")
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return "", "", errors.New("invalid SSH URL format: expected owner/repo")
		}
		return parts[0], parts[1], nil
	}

	// Handle HTTPS/HTTP format using net/url parsing
	if strings.HasPrefix(remoteUrl, "https://") || strings.HasPrefix(remoteUrl, "http://") {
		// Simple path extraction - find host end and parse remaining path
		var path string
		if strings.HasPrefix(remoteUrl, "https://") {
			path = strings.TrimPrefix(remoteUrl, "https://")
		} else {
			path = strings.TrimPrefix(remoteUrl, "http://")
		}

		// Find the first slash after host
		slashIdx := strings.Index(path, "/")
		if slashIdx == -1 {
			return "", "", errors.New("invalid HTTPS URL format: missing path")
		}
		path = path[slashIdx+1:]
		path = strings.TrimSuffix(path, ".git")
		path = strings.TrimSuffix(path, "/")

		parts := strings.Split(path, "/")
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return "", "", errors.New("invalid HTTPS URL format: expected owner/repo")
		}
		return parts[0], parts[1], nil
	}

	return "", "", errors.New("unsupported URL format: expected git@ or https:// prefix")
}
