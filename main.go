package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"
)

var version = "*unset*"

func main() {
	port := flag.Uint("port", 8080, "listen on port")
	addr := flag.String("addr", "0.0.0.0", "lisen on address")
	dumpRequest := flag.Bool("dump", false, "dump client request")
	dumpBody := flag.Bool("dump-body", false, "dump client request body")
	statusCode := flag.Uint("status", 200, "return status code")
	responseHeader := flag.String("h", "", "add HTTP response header")
	responseBody := flag.String("b", "", "add HTTP response body")
	exitAfter := flag.Uint("c", 0, "exit after number of requests (0 keep running)")
	flag.Parse()

	srv := http.Server{Addr: fmt.Sprintf("%s:%d", *addr, *port)}
	description := fmt.Sprintf("surl/%s",version)

	sigChan := make(chan os.Signal, 1)

	var responseCount uint = 0

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		responseCount += 1
		w.Header().Add("Server",description)
		if *dumpRequest || *dumpBody {
			dump, err := httputil.DumpRequest(r, *dumpBody)
			if err != nil {
				log.Printf("error: unable to dump client request: %s", err)
				return
			}
			log.Printf("\n--\n%q\n--\n", dump)
		}
		w.WriteHeader(int(*statusCode))

		if *responseHeader != "" {
			addRawHeader(w.Header(), *responseHeader)
		}
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

	log.Printf("starting %s on %s:%d %s", description,*addr, *port, desc(*exitAfter))
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

	log.Printf("shutting down after %d responses",responseCount)
	err := srv.Shutdown(shutdownCtx)
	if err != nil {
		log.Fatalf("shutdown error: %v", err)
	}
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
