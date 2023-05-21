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

	dumpRequest := pflag.Bool("dump", false, "dump client request")
	dumpBody := pflag.Bool("dump-body", false, "dump client request body")
	statusCode := pflag.UintP("status", "s", 200, "return status code")
	pflag.StringArrayVarP(&responseHeaders, "header", "H", []string{}, "HTTP response header")
	responseBody := pflag.StringP("data", "d", "", "add HTTP response body")
	exitAfter := pflag.UintP("count", "c", 0, "exit after number of requests (0 keep running)")

	pflag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options...] <addr>\n%s",
			filepath.Base(os.Args[0]),
			pflag.CommandLine.FlagUsages(),
		)
	}

	pflag.Parse()

	addr,err := parseAddr()
	if err != nil  {
		fmt.Fprintf(os.Stderr,"%s\n",err)
		pflag.Usage()
		os.Exit(1)
	}
	
	srv := http.Server{Addr: addr}
	description := fmt.Sprintf("surl/%s", version)

	sigChan := make(chan os.Signal, 1)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		responseCount += 1

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
				addRawHeader(w.Header(), hdr)
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
				defer file.Close()
				if err != nil {
					log.Printf("error: unable to open file: '%s'", filename)
				}
				io.Copy(w, file)
			} else {
				w.Write([]byte(*responseBody))
			}
		}
		if *exitAfter != 0 && *exitAfter == responseCount {
			sigChan <- os.Interrupt
		}

	})

	log.Printf("starting %s on %s %s", description, addr, desc(*exitAfter))
	// http.ListenAndServe(fmt.Sprintf("%s:%d", *addr, *port), nil)

	go func() {
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("startup error: %v", err)
		}
	}()

	// wait for request count
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	shutdownCtx, shutdownRelease := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownRelease()

	log.Printf("shutting down after %d responses", responseCount)
	err = srv.Shutdown(shutdownCtx)
	if err != nil {
		log.Fatalf("shutdown error: %v", err)
	}
}

func validAddr(s string) error {
	p := strings.SplitN(s,":",2)
	if p == nil || len(p) != 2 {
		return fmt.Errorf("invalid format ([host]:<port>)")
	}
	if _,err := strconv.Atoi(p[1]); err != nil {
		return err
	}
	return nil
}

func parseAddr() (string,error) {
	if pflag.NArg() != 1 {
		return "", fmt.Errorf("requred: 'addr'")
	}
	addr := pflag.Arg(0)
	if err := validAddr(addr); err != nil {
		return "",fmt.Errorf("invalid addr: %s (%s)\n",addr,err)
	}
	return addr,nil
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

func addRawHeader(headers http.Header, rawHeader string) error {
	kv := strings.SplitN(rawHeader, ":", 2)
	if len(kv) != 2 {
		return fmt.Errorf("invalid http header: '%s'", rawHeader)
	}
	headers.Add(kv[0], kv[1])
	return nil
}
