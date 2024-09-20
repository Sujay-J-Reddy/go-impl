// search_cli.go

package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

type SearchResult struct {
	Name    string
	Version string
	Rank    float64
}

func main() {
	dbName := flag.String("db", "nixpkgs.db", "SQLite database file name")
	query := flag.String("query", "", "Search query")
	limit := flag.Int("limit", 10, "Maximum number of search results to return")
	flag.Parse()

	if *query == "" {
		log.Fatal("Search query is required. Use --query flag to specify a search term.")
	}

	db, err := sql.Open("sqlite3", *dbName)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	results, err := FullTextSearch(db, *query, *limit)
	if err != nil {
		log.Fatalf("Failed to perform search: %v", err)
	}

	fmt.Println(FormatSearchResults(results))
}

func FullTextSearch(db *sql.DB, query string, limit int) ([]SearchResult, error) {
	searchQuery := `
		SELECT
			p.name,
			p.version,
			fts.rank
		FROM
			packages_fts fts
		JOIN
			packages p ON fts.rowid = p.id
		WHERE
			packages_fts MATCH ?
		ORDER BY
			fts.rank
		LIMIT ?
	`

	rows, err := db.Query(searchQuery, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to execute search query: %v", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var result SearchResult
		err := rows.Scan(&result.Name, &result.Version, &result.Rank)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %v", err)
		}
		results = append(results, result)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over rows: %v", err)
	}

	return results, nil
}

func FormatSearchResults(results []SearchResult) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d results:\n\n", len(results)))
	for i, result := range results {
		sb.WriteString(fmt.Sprintf("%d. %s (version %s)\n", i+1, result.Name, result.Version))
		sb.WriteString(fmt.Sprintf("   Rank: %.2f\n\n", result.Rank))
	}
	return sb.String()
}