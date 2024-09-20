// package main

// import (
// 	"context"
// 	"database/sql"
// 	"encoding/json"
// 	"flag"
// 	"fmt"
// 	"log"
// 	"os/exec"
// 	"strings"

// 	"github.com/google/go-github/v38/github"
// 	"golang.org/x/oauth2"
// )

// type Config struct {
// 	GithubToken string
// 	Channel     string
// 	Commit      string
// 	DBConfig    DBConfig
// }

// type Package struct {
// 	Name    string
// 	Version string
// }

// func main() {
// 	config := parseFlags()

// 	ctx := context.Background()
// 	client := getGithubClient(ctx, config.GithubToken)

// 	commit, err := fetchCommit(ctx, client, config.Channel, config.Commit)
// 	if err != nil {
// 		log.Fatalf("Failed to fetch commit: %v", err)
// 	}

// 	db, err := SetupDB(config.DBConfig)
// 	if err != nil {
// 		log.Fatalf("Failed to setup database: %v", err)
// 	}
// 	defer db.Close()

// 	processCommit(commit, config.Channel, db)

// 	log.Printf("Data collection complete. Results written to the database.")
// }

// func parseFlags() Config {
// 	token := flag.String("token", "", "GitHub API token")
// 	channel := flag.String("channel", "", "Nixpkgs channel (e.g., nixos-23.05)")
// 	commit := flag.String("commit", "", "Commit SHA")
// 	dbDriver := flag.String("db-driver", "sqlite3", "Database driver (sqlite3 or postgres)")
// 	dbHost := flag.String("db-host", "localhost", "Database host")
// 	dbPort := flag.Int("db-port", 5432, "Database port")
// 	dbUser := flag.String("db-user", "", "Database user")
// 	dbPassword := flag.String("db-password", "", "Database password")
// 	dbName := flag.String("db-name", "nixpkgs.db", "Database name")
// 	flag.Parse()

// 	if *token == "" || *channel == "" || *commit == "" {
// 		log.Fatal("GitHub token, channel, and commit are required")
// 	}

// 	return Config{
// 		GithubToken: *token,
// 		Channel:     *channel,
// 		Commit:      *commit,
// 		DBConfig: DBConfig{
// 			Driver:   *dbDriver,
// 			Host:     *dbHost,
// 			Port:     *dbPort,
// 			User:     *dbUser,
// 			Password: *dbPassword,
// 			DBName:   *dbName,
// 		},
// 	}
// }

// func getGithubClient(ctx context.Context, token string) *github.Client {
// 	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
// 	tc := oauth2.NewClient(ctx, ts)
// 	return github.NewClient(tc)
// }

// func fetchCommit(ctx context.Context, client *github.Client, channel, sha string) (*github.RepositoryCommit, error) {
// 	opt := &github.ListOptions{
// 		PerPage: 10,
// 	}
// 	commit, _, err := client.Repositories.GetCommit(ctx, "NixOS", "nixpkgs", sha, opt)
// 	if err != nil {
// 		return nil, err
// 	}
// 	return commit, nil
// }

// func processCommit(commit *github.RepositoryCommit, channel string, db *sql.DB) {
// 	sha := commit.GetSHA()
// 	log.Printf("Processing commit: %s for channel: %s", sha, channel)

// 	packages, err := extractPackages(sha)
// 	if err != nil {
// 		log.Printf("Error extracting packages for commit %s: %v", sha, err)
// 		return
// 	}

// 	err = insertPackages(db, sha, channel, packages)
// 	if err != nil {
// 		log.Printf("Error inserting packages for commit %s: %v", sha, err)
// 	}
// }

// func extractPackages(sha string) ([]Package, error) {
// 	cmd := exec.Command("nix-env", "-qa", "--json", "-f", fmt.Sprintf("https://github.com/NixOS/nixpkgs/archive/%s.tar.gz", sha))
// 	output, err := cmd.Output()
// 	if err != nil {
// 		return nil, err
// 	}

// 	var packagesMap map[string]map[string]interface{}
// 	err = json.Unmarshal(output, &packagesMap)
// 	if err != nil {
// 		return nil, err
// 	}

// 	var packages []Package
// 	for name, attrs := range packagesMap {
// 		version, ok := attrs["version"].(string)
// 		if !ok {
// 			version = "unknown"
// 		}
// 		packages = append(packages, Package{
// 			Name:    strings.TrimPrefix(name, "nixpkgs."),
// 			Version: version,
// 		})
// 	}

// 	return packages, nil
// }

// func insertPackages(db *sql.DB, sha, channel string, packages []Package) error {
// 	tx, err := db.Begin()
// 	if err != nil {
// 		return err
// 	}
// 	defer tx.Rollback()

// 	stmt, err := tx.Prepare(`
// 		INSERT INTO packages (name, version)
// 		VALUES (?, ?)
// 	`)
// 	if err != nil {
// 		return err
// 	}
// 	defer stmt.Close()

// 	for _, pkg := range packages {
// 		_, err = stmt.Exec( pkg.Name, pkg.Version)
// 		if err != nil {
// 			return err
// 		}
// 	}

// 	return tx.Commit()
// }

package main

import (
	"log"
	"os"

	"github.com/joho/godotenv"
	"valkyrie/nix-search/cmd" // Ensure the correct module path
	"github.com/spf13/cobra"
)

func main() {
	// Load .env file
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	rootCmd := &cobra.Command{Use: "myapp"}
	rootCmd.AddCommand(cmd.NewNixDumpCmd())

	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
		os.Exit(1)
	}
}

