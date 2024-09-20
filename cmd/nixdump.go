package cmd

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/docker/go-connections/nat"
	"github.com/google/go-github/v38/github"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"golang.org/x/oauth2"
	"github.com/spf13/cobra"
	_"github.com/lib/pq"
)

type Package struct {
	Name    string
	Version string
}

// NewNixDumpCmd creates the nixdump command
func NewNixDumpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "nixdump [channel]",
		Short: "Fetches Nixpkgs channel packages and stores them in a Postgres DB",
		Args:  cobra.ExactArgs(1),
		Run:   runNixDump,
	}
}

func runNixDump(cmd *cobra.Command, args []string) {
	channel := args[0] 
	githubToken := os.Getenv("GITHUB_TOKEN")
	if githubToken == "" {
		log.Fatal("GITHUB_TOKEN environment variable is required")
	}

	ctx := context.Background()
	container, db, err := setupTestcontainer(ctx)
	if err != nil {
		log.Fatalf("Failed to setup testcontainer: %v", err)
	}
	defer container.Terminate(ctx)

	client := getGithubClient(ctx, githubToken)

	branch, err := fetchReleaseByChannel(ctx, client, channel)
	if err != nil {
		log.Fatalf("Failed to fetch branch: %v", err)
	}

	processCommit(branch.GetCommit().GetSHA(), db)

	err = createSQLDump(db)
	if err != nil {
		log.Fatalf("Failed to create SQL dump: %v", err)
	}

	log.Println("Process completed successfully.")
}



func setupTestcontainer(ctx context.Context) (testcontainers.Container, *sql.DB, error) {
	container, err := postgres.RunContainer(ctx,
		testcontainers.WithImage("postgres:14.5"),
		postgres.WithDatabase("nixpkgs"),
		postgres.WithUsername("user"),
		postgres.WithPassword("password"),
		testcontainers.WithWaitStrategy(wait.ForListeningPort(nat.Port("5432/tcp"))),
	)
	if err != nil {
		return nil, nil, err
	}

	host, err := container.Host(ctx)
	if err != nil {
		return nil, nil, err
	}

	port, err := container.MappedPort(ctx, nat.Port("5432/tcp"))
	if err != nil {
		return nil, nil, err
	}

	dsn := fmt.Sprintf("host=%s port=%s user=user password=password dbname=nixpkgs sslmode=disable", host, port.Port())
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, nil, err
	}

	_, err = db.Exec(`CREATE TABLE packages (name TEXT, version TEXT);`)
	if err != nil {
		return nil, nil, err
	}

	return container, db, nil
}

func getGithubClient(ctx context.Context, token string) *github.Client {
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(ctx, ts)
	return github.NewClient(tc)
}

func fetchReleaseByChannel(ctx context.Context, client *github.Client, channel string) (*github.Branch, error) {
	branchName := fmt.Sprintf("release-%s", channel)

	branch, _, err := client.Repositories.GetBranch(ctx, "NixOS", "nixpkgs", branchName, false)
	if err != nil {
		return nil, err
	}

	return branch, nil
}



func processCommit(sha string, db *sql.DB) {
	log.Printf("Processing commit: %s", sha)

	packages, err := extractPackages(sha)
	if err != nil {
		log.Printf("Error extracting packages for commit %s: %v", sha, err)
		return
	}

	err = insertPackages(db, sha, packages)
	if err != nil {
		log.Printf("Error inserting packages for commit %s: %v", sha, err)
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

func insertPackages(db *sql.DB, sha string, packages []Package) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT INTO packages (name, version) VALUES ($1, $2)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, pkg := range packages {
		_, err = stmt.Exec(pkg.Name, pkg.Version)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func createSQLDump(db *sql.DB) error {
	rows, err := db.Query("SELECT name, version FROM packages")
	if err != nil {
		return err
	}
	defer rows.Close()

	file, err := os.Create("dump.sql")
	if err != nil {
		return err
	}
	defer file.Close()

	for rows.Next() {
		var name, version string
		err := rows.Scan(&name, &version)
		if err != nil {
			return err
		}
		_, err = file.WriteString(fmt.Sprintf("INSERT INTO packages (name, version) VALUES ('%s', '%s');\n", name, version))
		if err != nil {
			return err
		}
	}

	return nil
}
