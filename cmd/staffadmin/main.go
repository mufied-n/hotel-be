// Command staffadmin mengelola kredensial staf (BE-R01).
//
//	STAFF_PASSWORD='min-12-karakter' go run ./cmd/staffadmin -dsn "$DATABASE_URL" set-password fo_receptionist
//
// Jika STAFF_PASSWORD kosong, password dibaca dari stdin (satu baris). Mengganti password
// mencabut seluruh sesi aktif akun tersebut dan mereset lockout.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/example/hotel-booking/internal/staffauth"
)

func main() {
	dsn := flag.String("dsn", os.Getenv("DATABASE_URL"), "PostgreSQL DSN (default: $DATABASE_URL)")
	flag.Parse()
	if err := run(*dsn, flag.Args(), os.Getenv("STAFF_PASSWORD"), os.Stdin); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(dsn string, args []string, envPassword string, stdin *os.File) error {
	if len(args) != 2 || args[0] != "set-password" {
		return errors.New("usage: staffadmin [-dsn DSN] set-password <username>")
	}
	if dsn == "" {
		return errors.New("DSN kosong: set -dsn atau DATABASE_URL")
	}
	password := envPassword
	if password == "" {
		line, err := bufio.NewReader(stdin).ReadString('\n')
		if err != nil && line == "" {
			return fmt.Errorf("gagal membaca password dari stdin: %w", err)
		}
		password = strings.TrimRight(line, "\r\n")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return fmt.Errorf("koneksi database: %w", err)
	}
	defer pool.Close()

	switch err := staffauth.NewService(staffauth.NewPostgresStore(pool)).SetPassword(ctx, args[1], password); {
	case errors.Is(err, staffauth.ErrWeakPassword):
		return fmt.Errorf("password minimal %d karakter", staffauth.MinPasswordLen)
	case errors.Is(err, staffauth.ErrUserNotFound):
		return fmt.Errorf("user %q tidak ditemukan", args[1])
	case err != nil:
		return err
	}
	fmt.Printf("password untuk %q diperbarui; sesi lama dicabut\n", args[1])
	return nil
}
