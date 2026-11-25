package cache

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

func TestRedisStoreWithMockRESP(t *testing.T) {
	if _, err := NewRedisStore(context.Background(), "not-a-valid-url://:::"); err == nil {
		t.Fatalf("expected error for invalid redis URL")
	}

	// Start a minimal in-process RESP mock server
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	kv := map[string]string{}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				r := bufio.NewReader(c)
				for {
					line, err := r.ReadString('\n')
					if err != nil {
						return
					}
					if !strings.HasPrefix(line, "*") {
						continue
					}
					var argc int
					fmt.Sscanf(line, "*%d", &argc)
					args := make([]string, 0, argc)
					for i := 0; i < argc; i++ {
						_, _ = r.ReadString('\n') // $len
						val, _ := r.ReadString('\n')
						args = append(args, strings.TrimRight(val, "\r\n"))
					}
					if len(args) == 0 {
						continue
					}
					cmd := strings.ToUpper(args[0])
					switch cmd {
					case "PING":
						_, _ = c.Write([]byte("+PONG\r\n"))
					case "HELLO":
						_, _ = c.Write([]byte("-ERR unknown command\r\n"))
					case "CLIENT":
						_, _ = c.Write([]byte("+OK\r\n"))
					case "SET":
						if len(args) >= 3 {
							kv[args[1]] = args[2]
						}
						_, _ = c.Write([]byte("+OK\r\n"))
					case "GET":
						if len(args) >= 2 {
							if v, ok := kv[args[1]]; ok {
								_, _ = fmt.Fprintf(c, "$%d\r\n%s\r\n", len(v), v)
							} else {
								_, _ = c.Write([]byte("$-1\r\n"))
							}
						}
					case "DEL":
						if len(args) >= 2 {
							delete(kv, args[1])
						}
						_, _ = c.Write([]byte(":1\r\n"))
					case "INCR":
						_, _ = c.Write([]byte(":1\r\n"))
					case "EXPIRE", "PEXPIRE":
						_, _ = c.Write([]byte(":1\r\n"))
					default:
						_, _ = c.Write([]byte("+OK\r\n"))
					}
				}
			}(conn)
		}
	}()

	store, err := NewRedisStore(context.Background(), "redis://" + ln.Addr().String() + "/0")
	if err != nil {
		t.Fatalf("NewRedisStore failed: %v", err)
	}
	defer store.Close()

	if store.Backend() != "redis" {
		t.Fatalf("expected backend redis, got %s", store.Backend())
	}
	ctx := context.Background()
	if err := store.Set(ctx, "k1", "v1", time.Minute); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	got, err := store.Get(ctx, "k1")
	if err != nil || got != "v1" {
		t.Fatalf("expected v1, got %q err=%v", got, err)
	}
	if _, err := store.Get(ctx, "missing"); err != ErrCacheMiss {
		t.Fatalf("expected ErrCacheMiss, got %v", err)
	}
	n, err := store.Incr(ctx, "counter", time.Minute)
	if err != nil || n != 1 {
		t.Fatalf("expected Incr=1, got %d err=%v", n, err)
	}
	if err := store.Delete(ctx, "k1"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
}
