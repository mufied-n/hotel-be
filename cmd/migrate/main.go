// Command migrate menjalankan migrasi database menggunakan goose secara standalone
// tanpa embed SQL ke dalam binary server.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func main() {
	var (
		dir = flag.String("dir", "migrations", "path ke direktori file SQL migrasi")
		dsn = flag.String("dsn", "", "database DSN (default: env DATABASE_URL)")
	)
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: migrate [flags] <command> [arguments...]\n\n")
		fmt.Fprintf(os.Stderr, "Commands:\n")
		fmt.Fprintf(os.Stderr, "  up                   Menjalankan seluruh migrasi yang belum teraplikasi\n")
		fmt.Fprintf(os.Stderr, "  up-by-one            Menjalankan 1 migrasi ke atas\n")
		fmt.Fprintf(os.Stderr, "  up-to <version>      Menjalankan migrasi hingga versi tertentu\n")
		fmt.Fprintf(os.Stderr, "  down                 Rollback 1 migrasi terakhir\n")
		fmt.Fprintf(os.Stderr, "  down-to <version>    Rollback hingga versi tertentu\n")
		fmt.Fprintf(os.Stderr, "  status               Menampilkan status migrasi\n")
		fmt.Fprintf(os.Stderr, "  version              Menampilkan versi schema database saat ini\n")
		fmt.Fprintf(os.Stderr, "  reset                Rollback seluruh migrasi\n")
		fmt.Fprintf(os.Stderr, "  create <name> sql    Membuat file migrasi SQL baru\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	args := flag.Args()
	if len(args) < 1 {
		flag.Usage()
		os.Exit(1)
	}

	command := args[0]
	cmdArgs := args[1:]

	dbDSN := *dsn
	if dbDSN == "" {
		dbDSN = os.Getenv("DATABASE_URL")
		if dbDSN == "" {
			dbDSN = "postgres://postgres:dev@localhost:5432/booking?sslmode=disable"
		}
	}

	if err := goose.SetDialect("postgres"); err != nil {
		log.Fatalf("goose set dialect: %v", err)
	}

	var db *sql.DB
	if command != "create" {
		var err error
		db, err = sql.Open("pgx", dbDSN)
		if err != nil {
			log.Fatalf("db open: %v", err)
		}
		defer func() { _ = db.Close() }()

		if err := db.Ping(); err != nil {
			log.Fatalf("db ping (%s): %v", dbDSN, err)
		}
	}

	if err := goose.RunContext(context.Background(), command, db, *dir, cmdArgs...); err != nil {
		log.Fatalf("migration %s failed: %v", command, err)
	}
}
