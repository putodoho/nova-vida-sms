// Command cli runs administrative tasks:
//
//	cli migrate
//	cli seed
//	cli create-admin -login admin@escola.co.mz -name "Administrador" -password '...'
package main

import (
	_ "time/tzdata" // embed zone data: the container image has none

	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/putodoho/nova-vida-sms/api/internal/db"
	"github.com/putodoho/nova-vida-sms/api/internal/seed"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: cli <migrate|seed|create-admin> [flags]")
	}
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	ctx := context.Background()
	pool, err := db.Connect(ctx, url)
	if err != nil {
		return err
	}
	defer pool.Close()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	switch args[0] {
	case "migrate":
		return db.Migrate(ctx, pool, log)
	case "seed":
		if err := db.Migrate(ctx, pool, log); err != nil {
			return err
		}
		if err := seed.Run(ctx, pool); err != nil {
			return err
		}
		fmt.Println("seed applied for", seed.Year)
		return nil
	case "create-admin":
		fs := flag.NewFlagSet("create-admin", flag.ContinueOnError)
		login := fs.String("login", "", "email or phone used to sign in")
		name := fs.String("name", "Administrador", "display name")
		password := fs.String("password", "", "initial password (min. 8 characters)")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *login == "" || *password == "" {
			return fmt.Errorf("-login and -password are required")
		}
		id, err := seed.CreateAdmin(ctx, pool, *login, *name, *password)
		if err != nil {
			return err
		}
		fmt.Println("admin ready:", *login, id)
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}
