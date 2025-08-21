package main

import (
	"context"
	"encoding/hex"
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

var (
	version             = "*unset*"
	versionFlag         bool
	dumpRequestFlag     bool
	dumpBodyFlag        bool
	statusCodeFlag      uint
	responseHeadersFlag []string
	responseBodyFlag    string
	exitAfterFlag       uint
	certFileFlag        string
	keyFileFlag         string
	userFlag            string

	responseCount uint = 0
	absBaseDir    string
)

func main() {

	pflag.BoolVar(&versionFlag, "version", false, "show version")
	pflag.BoolVar(&dumpRequestFlag, "dump", false, "dump client request")
	pflag.BoolVar(&dumpBodyFlag, "dump-body", false, "dump client request body")
	pflag.UintVarP(&statusCodeFlag, "status", "s", 200, "return status code")
	pflag.StringArrayVarP(&responseHeadersFlag, "header", "H", []string{}, "HTTP response header")
	pflag.StringVarP(&responseBodyFlag, "data", "d", "", "add HTTP response body")
	pflag.UintVarP(&exitAfterFlag, "count", "c", 0, "exit after number of requests (0 keep running)")
	pflag.StringVar(&certFileFlag, "cert", "", "TLS certificate file")
	pflag.StringVarP(&userFlag, "user", "u", "", "user credentials '<user:password>' for Basic Auth")

	pflag.Usage = func() {
		_, _ = fmt.Fprintf(os.Stderr, "Usage: %s [options...] <addr>\n%s", filepath.Base(os.Args[0]),
			pflag.CommandLine.FlagUsages(),
		)
	}

	pflag.Parse()

	if versionFlag {
		fmt.Printf("surl %s\n", version)
		os.Exit(0)
	}

	addr, err := parseAddr()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "%s\n", err)
		pflag.Usage()
		os.Exit(1)
	}

	if responseBodyFlag != "" && strings.HasPrefix(responseBodyFlag, "@") {
		fn := trimFirst(responseBodyFlag)
		s, err := os.Stat(fn)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "%s\n", err)
			pflag.Usage()
			os.Exit(1)
		}
		if s.IsDir() {
			absBaseDir, err = filepath.Abs(fn)
			if err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "%s\n", err)
				pflag.Usage()
				os.Exit(1)
			}
		}
	}

	srv := http.Server{Addr: addr}
	description := fmt.Sprintf("surl/%s", version)

	sigChan := make(chan os.Signal, 1)
	http.Handle("/", requestLogger(globalHandler(description, sigChan)))

	go func() {
		log.Printf("starting %s on %s %s", description, addr, desc(exitAfterFlag))
		if absBaseDir != "" {
			log.Printf("serving files from: %s", absBaseDir)
		}
		if certFileFlag != "" && keyFileFlag != "" {
			if err := srv.ListenAndServeTLS(certFileFlag, keyFileFlag); !errors.Is(err, http.ErrServerClosed) {
				log.Fatalf("startup error: %v", err)
			}
			return
		}
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("startup error: %v", err)
		}
	}()

	// wait for request count
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	shutdownCtx, shutdownRelease := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownRelease()

	if exitAfterFlag != responseCount {
		log.Printf("shutting down after %d responses", responseCount)
	}
	err = srv.Shutdown(shutdownCtx)
	if err != nil {
		log.Fatalf("shutdown error: %v", err)
	}
}

type ctxLogDataKey struct{}
type collectingResponseWriter struct {
	http.ResponseWriter
	statusCode int
	size       int
}

func (crw *collectingResponseWriter) WriteHeader(code int) {
	crw.statusCode = code
	crw.ResponseWriter.WriteHeader(code)
}
func (crw *collectingResponseWriter) Write(b []byte) (int, error) {
	n, err := crw.ResponseWriter.Write(b)
	crw.size += n
	return n, err
}

func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()

		logData := make(map[string]any)
		ctx := context.WithValue(r.Context(), ctxLogDataKey{}, logData)
		r = r.WithContext(ctx)
		cw := &collectingResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(cw, r)

		log.Printf("%s - %s \"%s %s %s\" %d %d %s %s [%s]%s",
			logUser(r),
			r.RemoteAddr,
			r.Method,
			logPath(r, logData),
			r.Proto,
			cw.statusCode,
			cw.size,
			r.Referer(),
			r.UserAgent(),
			time.Since(started),
			logDump(logData),
		)
	})
}
func logDump(logData map[string]any) string {
	if d, ok := logData["dump"].([]byte); ok {
		return fmt.Sprintf("\n%s", indent(hex.Dump(d), 20))
	}
	return ""
}

func indent(s string, spaces int) string {
	indent := strings.Repeat(" ", spaces)
	return fmt.Sprintln(indent + strings.ReplaceAll(s, "\n", "\n"+indent))
}

func logUser(r *http.Request) string {
	if user, _, ok := r.BasicAuth(); ok {
		return user
	}
	return "???"
}

func logPath(r *http.Request, logdata map[string]any) string {
	p := r.URL.Path
	if logdata != nil {
		if s, ok := logdata["served-file"]; ok && s != "" {
			p += fmt.Sprintf(" (%s)", s)
		}
	}
	return p
}

func addLogData(r *http.Request, key string, value any) {
	if logData, ok := r.Context().Value(ctxLogDataKey{}).(map[string]any); ok {
		logData[key] = value
	}
}

func globalHandler(description string, sigChan chan os.Signal) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if userFlag != "" {
			if !validateBasicAuth(r, userFlag) {
				w.Header().Add("WWW-Authenticate", "Basic realm=\"Auth Required\"")
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
		}

		responseCount += 1

		if dumpRequestFlag || dumpBodyFlag {
			dump, err := httputil.DumpRequest(r, dumpBodyFlag)
			if err != nil {
				log.Printf("error: unable to dump client request: %s", err)
				return
			}
			addLogData(r, "dump", dump)
			// log.Printf("\n--\n%q\n--\n", dump)
		}

		if len(responseHeadersFlag) != 0 {
			for _, hdr := range responseHeadersFlag {
				if err := addRawHeader(w.Header(), hdr); err != nil {
					log.Printf("error: unable to add response header: %s", err)
				}
			}
		}

		if w.Header().Get("Server") == "" {
			w.Header().Add("Server", description)
		}

		if responseBodyFlag != "" {
			if strings.HasPrefix(responseBodyFlag, "@") {
				// response is filename
				filename := trimFirst(responseBodyFlag)
				s, err := os.Stat(filename)
				if err != nil {
					log.Printf("error: unable to stat file: '%s'", filename)
					return
				}
				if s.IsDir() {
					requestPath := filepath.Join(filename, filepath.Clean("/"+r.URL.Path))
					absRequestPath, err := filepath.Abs(requestPath)
					if err != nil {
						log.Printf("error: unable to establish absolute path from '%s': %s", requestPath, err)
						return
					}

					if !strings.HasPrefix(absRequestPath, absBaseDir) {
						log.Printf("error: path '%s' outside of base '%s'", requestPath, filename)
						return
					}
					filename = absRequestPath

					s, err = os.Stat(filename)
					if err != nil {
						log.Printf("error: unable to stat file: '%s'", filename)
						return
					}
				}
				addLogData(r, "served-file", filename)
				file, err := os.Open(filename)
				if err != nil {
					log.Printf("error: unable to open file: '%s'", filename)
					return
				}
				defer quietClose(file)
				if w.Header().Get("Content-Length") == "" {
					w.Header().Add("Content-Length", strconv.Itoa(int(s.Size())))
				}
				w.WriteHeader(int(statusCodeFlag)) // start sending body
				if _, err = io.Copy(w, file); err != nil {
					log.Printf("error: unable to write response body: %s", err)
				}
			} else {
				w.WriteHeader(int(statusCodeFlag)) // start sending body
				if _, err := w.Write([]byte(responseBodyFlag)); err != nil {
					log.Printf("error: unable to write response body: %s", err)
				}
			}
		} else {
			w.WriteHeader(int(statusCodeFlag))
		}

		if exitAfterFlag != 0 && exitAfterFlag == responseCount {
			log.Printf("response count of %d reached, shutting down", responseCount)
			sigChan <- os.Interrupt
		}
	})
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

func validateBasicAuth(r *http.Request, up string) bool {
	if user, pass, ok := r.BasicAuth(); ok {
		return up == user+":"+pass
	}
	return false
	// ah := r.Header.Get("Authorization")
	// if ah == "" || !strings.HasPrefix(ah, "Basic ") {
	// 	return false
	// }
	// ah = strings.TrimPrefix(ah, "Basic ")
	// if clear, err := base64.StdEncoding.DecodeString(ah); err != nil || string(clear) != up {
	// 	return false
	// }
	// return true
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
