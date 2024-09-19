package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/google/go-github/v38/github"
	"golang.org/x/oauth2"
)

type Config struct {
	GithubToken string
	StartDate   time.Time
	EndDate     time.Time
	Concurrency int
	OutputFile  string
}

type Package struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type CommitData struct {
	SHA      string    `json:"sha"`
	Date     time.Time `json:"date"`
	Packages []Package `json:"packages"`
}

func main() {
	config := parseFlags()

	ctx := context.Background()
	client := getGithubClient(ctx, config.GithubToken)

	commits, err := fetchCommits(ctx, client, config.StartDate, config.EndDate)
	if err != nil {
		log.Fatalf("Failed to fetch commits: %v", err)
	}

	file, err := os.Create(config.OutputFile)
	if err != nil {
		log.Fatalf("Failed to create output file: %v", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")

	var mutex sync.Mutex
	processCommits(commits, config.Concurrency, encoder, &mutex)

	log.Printf("Data collection complete. Results written to %s", config.OutputFile)
}

func parseFlags() Config {
	token := flag.String("token", "", "GitHub API token")
	start := flag.String("start", "", "Start date (YYYY-MM-DD)")
	end := flag.String("end", "", "End date (YYYY-MM-DD)")
	concurrency := flag.Int("concurrency", 4, "Number of concurrent processes")
	output := flag.String("output", "nixpkgs_data.json", "Output file name")
	flag.Parse()

	if *token == "" || *start == "" || *end == "" {
		log.Fatal("GitHub token, start date, and end date are required")
	}

	startDate, err := time.Parse("2006-01-02", *start)
	if err != nil {
		log.Fatalf("Invalid start date: %v", err)
	}

	endDate, err := time.Parse("2006-01-02", *end)
	if err != nil {
		log.Fatalf("Invalid end date: %v", err)
	}

	return Config{
		GithubToken: *token,
		StartDate:   startDate,
		EndDate:     endDate,
		Concurrency: *concurrency,
		OutputFile:  *output,
	}
}

func getGithubClient(ctx context.Context, token string) *github.Client {
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(ctx, ts)
	return github.NewClient(tc)
}

func fetchCommits(ctx context.Context, client *github.Client, startDate, endDate time.Time) ([]*github.RepositoryCommit, error) {
	opts := &github.CommitsListOptions{
		Since: startDate,
		Until: endDate,
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	}

	var allCommits []*github.RepositoryCommit
	for {
		commits, resp, err := client.Repositories.ListCommits(ctx, "NixOS", "nixpkgs", opts)
		if err != nil {
			return nil, err
		}
		allCommits = append(allCommits, commits...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return allCommits, nil
}

func processCommits(commits []*github.RepositoryCommit, concurrency int, encoder *json.Encoder, mutex *sync.Mutex) {
	semaphore := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for _, commit := range commits {
		wg.Add(1)
		semaphore <- struct{}{}
		go func(c *github.RepositoryCommit) {
			defer wg.Done()
			defer func() { <-semaphore }()
			processCommit(c, encoder, mutex)
		}(commit)
	}

	wg.Wait()
}

func processCommit(commit *github.RepositoryCommit, encoder *json.Encoder, mutex *sync.Mutex) {
	sha := commit.GetSHA()
	log.Printf("Processing commit: %s", sha)

	packages, err := extractPackages(sha)
	if err != nil {
		log.Printf("Error extracting packages for commit %s: %v", sha, err)
		return
	}

	commitData := CommitData{
		SHA:      sha,
		Date:     commit.Commit.Author.GetDate(),
		Packages: packages,
	}

	mutex.Lock()
	err = encoder.Encode(commitData)
	mutex.Unlock()

	if err != nil {
		log.Printf("Error writing commit data for %s: %v", sha, err)
	}
}

func extractPackages(sha string) ([]Package, error) {
	cmd := exec.Command("nix-env", "-qa", "--json", "-f", fmt.Sprintf("https://github.com/NixOS/nixpkgs/archive/%s.tar.gz", sha))
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var packagesMap map[string]map[string]interface{}
	err = json.Unmarshal(output, &packagesMap)
	if err != nil {
		return nil, err
	}

	var packages []Package
	for name, attrs := range packagesMap {
		version, ok := attrs["version"].(string)
		if !ok {
			version = "unknown"
		}
		packages = append(packages, Package{
			Name:    strings.TrimPrefix(name, "nixpkgs."),
			Version: version,
		})
	}

	return packages, nil
}