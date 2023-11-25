package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/spf13/pflag"
)

var version = "*unset*"

func main() {
	var responseCount uint = 0
	var responseHeaders []string

	versionFlag := pflag.Bool("version", false, "show version")
	dumpRequest := pflag.Bool("dump", false, "dump client request")
	dumpBody := pflag.Bool("dump-body", false, "dump client request body")
	statusCode := pflag.UintP("status", "s", 200, "return status code")
	pflag.StringArrayVarP(&responseHeaders, "header", "H", []string{}, "HTTP response header")
	responseBody := pflag.StringP("data", "d", "", "add HTTP response body")
	exitAfter := pflag.UintP("count", "c", 0, "exit after number of requests (0 keep running)")
	certFile := pflag.String("cert", "", "TLS certificate file")
	keyFile := pflag.String("key", "", "TLS certificate key-file")

	pflag.Usage = func() {
		_, _ = fmt.Fprintf(os.Stderr, "Usage: %s [options...] <addr>\n%s", filepath.Base(os.Args[0]),
			pflag.CommandLine.FlagUsages(),
		)
	}

	pflag.Parse()

	if *versionFlag {
		fmt.Printf("surl %s\n", version)
		os.Exit(0)
	}

	addr, err := parseAddr()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "%s\n", err)
		pflag.Usage()
		os.Exit(1)
	}

	srv := http.Server{Addr: addr}
	description := fmt.Sprintf("surl/%s", version)

	sigChan := make(chan os.Signal, 1)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		responseCount += 1

		log.Printf("request: %s %s %s", r.RemoteAddr, r.Method, r.URL)

		if *dumpRequest || *dumpBody {
			dump, err := httputil.DumpRequest(r, *dumpBody)
			if err != nil {
				log.Printf("error: unable to dump client request: %s", err)
				return
			}
			log.Printf("\n--\n%q\n--\n", dump)
		}

		if len(responseHeaders) != 0 {
			for _, hdr := range responseHeaders {
				if err := addRawHeader(w.Header(), hdr); err != nil {
					log.Printf("error: unable to add response header: %s", err)
				}
			}
		}

		if w.Header().Get("Server") == "" {
			w.Header().Add("Server", description)
		}

		w.WriteHeader(int(*statusCode))

		if *responseBody != "" {
			if strings.HasPrefix(*responseBody, "@") {
				// response is filename
				filename := trimFirst(*responseBody)
				file, err := os.Open(filename)
				if err != nil {
					log.Printf("error: unable to open file: '%s'", filename)
					return
				}
				defer quietClose(file)
				if _, err = io.Copy(w, file); err != nil {
					log.Printf("error: unable to write response body: %s", err)
				}
			} else {
				if _, err := w.Write([]byte(*responseBody)); err != nil {
					log.Printf("error: unable to write response body: %s", err)
				}
			}
		}
		if *exitAfter != 0 && *exitAfter == responseCount {
			log.Printf("response count of %d reached, shutting down", responseCount)
			sigChan <- os.Interrupt
		}

	})

	go func() {
		if *certFile != "" && *keyFile != "" {
			log.Printf("starting %s on %s %s", description, addr, desc(*exitAfter))
			if err := srv.ListenAndServeTLS(*certFile, *keyFile); !errors.Is(err, http.ErrServerClosed) {
				log.Fatalf("startup error: %v", err)
			}
			return
		}
		log.Printf("starting %s on %s %s", description, addr, desc(*exitAfter))
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("startup error: %v", err)
		}
	}()

	// wait for request count
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	shutdownCtx, shutdownRelease := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownRelease()

	if *exitAfter != responseCount {
		log.Printf("shutting down after %d responses", responseCount)
	}
	err = srv.Shutdown(shutdownCtx)
	if err != nil {
		log.Fatalf("shutdown error: %v", err)
	}
}

func validAddr(s string) error {
	p := strings.SplitN(s, ":", 2)
	if p == nil || len(p) != 2 {
		return fmt.Errorf("invalid format ([host]:<port>)")
	}
	if _, err := strconv.Atoi(p[1]); err != nil {
		return err
	}
	return nil
}

func parseAddr() (string, error) {
	if pflag.NArg() != 1 {
		return "", fmt.Errorf("requred: 'addr'")
	}
	addr := pflag.Arg(0)
	if err := validAddr(addr); err != nil {
		return "", fmt.Errorf("invalid addr: %s (%s)\n", addr, err)
	}
	return addr, nil
}

func desc(c uint) string {
	if c == 0 {
		return "(run for ever)"
	}
	return fmt.Sprintf("(run for %d requests)", c)
}

func trimFirst(s string) string {
	_, i := utf8.DecodeRuneInString(s)
	return s[i:]
}

func splitToKeyValue(s string, sep string) (string, string, error) {
	kv := strings.SplitN(s, sep, 2)
	if len(kv) != 2 {
		return "", "", fmt.Errorf("invalid key%svalue format: '%s'", sep, s)
	}
	return kv[0], kv[1], nil
}

func addRawHeader(headers http.Header, rawHeader string) error {
	name, value, err := splitToKeyValue(rawHeader, ":")
	if err != nil {
		return fmt.Errorf("invalid http header: '%w'", err)
	}
	headers.Add(name, value)
	return nil
}

func quietClose(c io.Closer) {
	if err := c.Close(); err != nil {
		log.Printf("error: unable to close: %s", err)
	}
}
