package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/frodex/prd2wiki/internal/auth"
	"github.com/frodex/prd2wiki/internal/index"
)

func main() {
	dbPath := flag.String("db", "./data/index.db", "path to wiki SQLite db")
	principal := flag.String("principal", "svc:agent", "principal id (e.g. svc:cursor, user:alice)")
	scopesRaw := flag.String("scopes", "read,write", "comma-separated scopes")
	ttl := flag.Duration("ttl", 0, "key TTL (0 = no expiry)")
	useOnce := flag.Bool("use-once", false, "key is invalidated after first use")
	list := flag.Bool("list", false, "list existing keys instead of issuing a new one")
	revoke := flag.String("revoke", "", "revoke a key by ID")
	flag.Parse()

	db, err := index.OpenDatabase(*dbPath)
	if err != nil {
		log.Fatalf("open database %q: %v", *dbPath, err)
	}
	defer db.Close()

	store, err := auth.NewServiceKeyStore(db)
	if err != nil {
		log.Fatalf("init service key store: %v", err)
	}

	ctx := context.Background()

	if *revoke != "" {
		if err := store.Revoke(ctx, *revoke); err != nil {
			log.Fatalf("revoke key %q: %v", *revoke, err)
		}
		fmt.Printf("revoked key: %s\n", *revoke)
		return
	}

	if *list {
		keys, err := store.List(ctx, 100)
		if err != nil {
			log.Fatalf("list keys: %v", err)
		}
		if len(keys) == 0 {
			fmt.Println("no keys found")
			return
		}
		fmt.Printf("%-30s %-20s %-20s %-8s %-8s %s\n", "ID", "PRINCIPAL", "PREFIX", "REVOKED", "USE_COUNT", "SCOPES")
		for _, k := range keys {
			revoked := "no"
			if k.Revoked {
				revoked = "yes"
			}
			fmt.Printf("%-30s %-20s %-20s %-8s %-8d %s\n",
				k.ID, k.Principal, k.Prefix, revoked, k.UseCount, strings.Join(k.Scopes, ","))
		}
		return
	}

	// Issue a new key.
	scopes := []string{}
	for _, s := range strings.Split(*scopesRaw, ",") {
		s = strings.TrimSpace(s)
		if s != "" {
			scopes = append(scopes, s)
		}
	}

	key, rawKey, err := store.Issue(ctx, *principal, scopes, *ttl, *useOnce)
	if err != nil {
		log.Fatalf("issue key: %v", err)
	}

	fmt.Printf("key issued\n")
	fmt.Printf("  id:        %s\n", key.ID)
	fmt.Printf("  principal: %s\n", key.Principal)
	fmt.Printf("  scopes:    %s\n", strings.Join(key.Scopes, ", "))
	fmt.Printf("  use_once:  %v\n", key.UseOnce)
	if key.ExpiresAt != nil {
		fmt.Printf("  expires:   %s\n", key.ExpiresAt.Format(time.RFC3339))
	} else {
		fmt.Printf("  expires:   never\n")
	}
	fmt.Printf("\n")
	fmt.Printf("RAW KEY (store securely, shown once):\n")
	fmt.Printf("  %s\n", rawKey)
}
