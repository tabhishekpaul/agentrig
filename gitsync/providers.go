package gitsync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// RepoName is the repo arig looks for (and offers to create) on each host.
const RepoName = "arig-sync"

const repoDescription = "agentrig: AI coding skills and plugin settings, synced between my machines"

// Provider is a git host reachable through its official CLI, already signed in.
type Provider struct {
	ID        string      `json:"id"`
	Name      string      `json:"name"`
	CLI       string      `json:"cli"`
	Install   string      `json:"install"` // where to get the CLI
	Installed bool        `json:"installed"`
	LoggedIn  bool        `json:"loggedIn"`
	User      string      `json:"user,omitempty"`
	Repo      *RemoteRepo `json:"repo,omitempty"` // arig-sync, if it exists
	Error     string      `json:"error,omitempty"`
}

type RemoteRepo struct {
	Name string `json:"name"` // owner/arig-sync
	URL  string `json:"url"`  // clone URL in the user's preferred protocol
	Web  string `json:"web"`
}

type host struct {
	id, name, cli, install string
	user                   func() (string, error)
	find                   func(user string) (*RemoteRepo, error) // nil, nil = not found
	create                 func() error
}

var hosts = []host{
	{
		id: "github", name: "GitHub", cli: "gh", install: "https://cli.github.com",
		user: func() (string, error) { return cli("gh", "api", "user", "--jq", ".login") },
		find: func(user string) (*RemoteRepo, error) {
			out, err := cli("gh", "repo", "view", user+"/"+RepoName, "--json", "url,sshUrl")
			if err != nil {
				if strings.Contains(err.Error(), "Could not resolve") {
					return nil, nil
				}
				return nil, err
			}
			var r struct{ URL, SSHURL string }
			if err := json.Unmarshal([]byte(out), &r); err != nil {
				return nil, err
			}
			clone := r.URL + ".git"
			if p, _ := cli("gh", "config", "get", "git_protocol", "-h", "github.com"); p == "ssh" {
				clone = r.SSHURL
			}
			return &RemoteRepo{Name: user + "/" + RepoName, URL: clone, Web: r.URL}, nil
		},
		create: func() error {
			if _, err := cli("gh", "repo", "create", RepoName, "--private", "--description", repoDescription); err != nil {
				return err
			}
			// HTTPS pushes from the background need git to use gh's login.
			if p, _ := cli("gh", "config", "get", "git_protocol", "-h", "github.com"); p != "ssh" {
				if h, _ := cli("git", "config", "--global", "--get-all", "credential.https://github.com.helper"); h == "" {
					_, err := cli("gh", "auth", "setup-git")
					return err
				}
			}
			return nil
		},
	},
	{
		// ponytail: written against glab's documented API, not yet tried on a real account.
		id: "gitlab", name: "GitLab", cli: "glab", install: "https://gitlab.com/gitlab-org/cli",
		user: func() (string, error) {
			out, err := cli("glab", "api", "user")
			if err != nil {
				return "", err
			}
			var u struct{ Username string }
			return u.Username, json.Unmarshal([]byte(out), &u)
		},
		find: func(user string) (*RemoteRepo, error) {
			out, err := cli("glab", "api", "projects/"+url.PathEscape(user+"/"+RepoName))
			if err != nil {
				if strings.Contains(err.Error(), "404") {
					return nil, nil
				}
				return nil, err
			}
			var r struct {
				WebURL string `json:"web_url"`
				SSH    string `json:"ssh_url_to_repo"`
				HTTP   string `json:"http_url_to_repo"`
			}
			if err := json.Unmarshal([]byte(out), &r); err != nil {
				return nil, err
			}
			clone := r.SSH
			if p, _ := cli("glab", "config", "get", "git_protocol"); p == "https" || p == "http" {
				clone = r.HTTP
			}
			return &RemoteRepo{Name: user + "/" + RepoName, URL: clone, Web: r.WebURL}, nil
		},
		create: func() error {
			_, err := cli("glab", "repo", "create", RepoName, "--private", "--description", repoDescription)
			return err
		},
	},
}

func (h host) probe() Provider {
	p := Provider{ID: h.id, Name: h.name, CLI: h.cli, Install: h.install}
	if _, err := exec.LookPath(h.cli); err != nil {
		return p
	}
	p.Installed = true
	user, err := h.user()
	if err != nil || user == "" {
		return p // installed, not signed in
	}
	p.LoggedIn, p.User = true, user
	if p.Repo, err = h.find(user); err != nil {
		p.Error = err.Error()
	}
	return p
}

// Providers checks every supported git host CLI in parallel.
func Providers() []Provider {
	out := make([]Provider, len(hosts))
	var wg sync.WaitGroup
	for i, h := range hosts {
		wg.Add(1)
		go func() { defer wg.Done(); out[i] = h.probe() }()
	}
	wg.Wait()
	return out
}

// Setup connects to the user's arig-sync repo on a provider, creating it
// first (private) if create is set and it doesn't exist yet.
func (s *Syncer) Setup(providerID string, create bool) (Config, error) {
	for _, h := range hosts {
		if h.id != providerID {
			continue
		}
		p := h.probe()
		if !p.LoggedIn {
			return Config{}, fmt.Errorf("%s isn't signed in: run `%s auth login` in a terminal", p.Name, p.CLI)
		}
		if p.Repo == nil {
			if !create {
				return Config{}, fmt.Errorf("no %s repo on %s yet", RepoName, p.Name)
			}
			s.step(fmt.Sprintf("Creating the private repo on %s…", p.Name))
			if err := h.create(); err != nil {
				s.step("")
				return Config{}, fmt.Errorf("couldn't create %s: %w", RepoName, err)
			}
			repo, err := h.find(p.User) // one lookup for the URL, not a full re-probe
			if err != nil || repo == nil {
				s.step("")
				return Config{}, fmt.Errorf("created %s but can't find it: %v", RepoName, err)
			}
			p.Repo = repo
		}
		return s.Connect(p.Repo.URL)
	}
	return Config{}, errors.New("unknown provider")
}

func cli(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(cmd.Environ(), "GIT_TERMINAL_PROMPT=0", "GH_PROMPT_DISABLED=1", "NO_COLOR=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s %s: %s", name, args[0], strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}
